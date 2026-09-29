import {
    type ClientHttp2Session,
    type ClientHttp2Stream,
    connect as connectHttp2
} from "node:http2";
import { createConnection } from "node:net";

export interface EventCursor {
    streamId: string;
    sequence: number;
}

export interface CoreTransport {
    request<T>(method: string, path: string, body?: unknown): Promise<T>;
    onFrame(listener: (frame: StreamFrame) => void | Promise<void>): () => void;
    onClose(listener: (reason: Error) => void): () => void;
    subscribe(topic: string, after?: EventCursor): void;
    unsubscribe(topic: string): void;
    getCursor(topic: string): EventCursor | undefined;
    setCursor(topic: string, cursor: EventCursor): void;
    subscribeAndWait(topic: string, after?: EventCursor): Promise<void>;
    disconnect(): void;
}

export interface StreamFrame<T = unknown> {
    type: "event" | "ready" | "snapshot" | "disconnect" | "error";
    topic: string;
    streamId?: string;
    sequence?: number;
    cursor?: EventCursor;
    reason?: string;
    event?: T;
    snapshot?: { state?: unknown; recentEvents?: StreamFrame[] };
    code?: string;
    message?: string;
}

interface SystemInfo {
    dataDir: string;
    ipcVersion: string;
    apiContract: string;
    processId: number;
    initialized: boolean;
    eventStreamId: string;
}

interface APIResponseBody {
    code: number;
    message: string;
    data: unknown;
    errorCode?: string;
}

const isAPIResponseBody = (value: unknown): value is APIResponseBody => {
    if (typeof value !== "object" || value === null) {
        return false;
    }
    return "code" in value && typeof value.code === "number" && Number.isInteger(value.code) &&
        "message" in value && typeof value.message === "string" &&
        Object.hasOwn(value, "data") &&
        (!("errorCode" in value) || typeof value.errorCode === "string");
};

interface EventStreamConnection {
    session: ClientHttp2Session;
    generation: number;
    stream: ClientHttp2Stream;
    decoder: NdjsonFrameDecoder;
}

export class NdjsonFrameDecoder {
    private decoder = new TextDecoder("utf-8");
    private buffer = "";

    push(chunk: Buffer | string): string[] {
        this.buffer += typeof chunk === "string" ? chunk : this.decoder.decode(chunk, { stream: true });
        const lines: string[] = [];
        for (;;) {
            const newline = this.buffer.indexOf("\n");
            if (newline < 0) {
                return lines;
            }
            const line = this.buffer.slice(0, newline).trim();
            this.buffer = this.buffer.slice(newline + 1);
            if (line) {
                lines.push(line);
            }
        }
    }

    reset(): void {
        this.decoder = new TextDecoder("utf-8");
        this.buffer = "";
    }
}

const RECONNECT_GRACE_MS = 8_000;
const MAX_RECONNECT_DELAY_MS = 1_000;

export class CoreHttp2Client implements CoreTransport {
    private session?: ClientHttp2Session;
    private sessionGeneration = 0;
    private readonly listeners = new Set<(reason: Error) => void>();
    private readonly topicCursors = new Map<string, EventCursor>();
    private readonly activeTopics = new Set<string>();
    private readonly topicWaiters = new Map<string, Array<{ resolve: () => void; reject: (error: Error) => void }>>();
    private stream?: EventStreamConnection;
    private readonly replayPending = new Set<string>();
    private connectionRetryTimer?: ReturnType<typeof setTimeout>;
    private streamRetryTimer?: ReturnType<typeof setTimeout>;
    private outageTimer?: ReturnType<typeof setTimeout>;
    private reconnectAttempt = 0;
    private closed = false;
    private closeReason = new Error("core HTTP/2 connection closed");
    private readonly frameListeners = new Set<(frame: StreamFrame) => void | Promise<void>>();
    private frameProcessing: Promise<void> = Promise.resolve();

    private constructor(private readonly address: string) {
        this.session = this.createSession();
    }

    private createSession(): ClientHttp2Session {
        if (this.connectionRetryTimer) {
            clearTimeout(this.connectionRetryTimer);
            this.connectionRetryTimer = undefined;
        }
        this.session = connectHttp2("http://agenty.local", {
            createConnection: () => createConnection(this.address),
        });
        const session = this.session;
        const generation = ++this.sessionGeneration;
        session.on("connect", () => this.onSessionConnected(session, generation));
        session.on("error", (error: Error) => this.onSessionFailure(session, generation, error));
        session.on("close", () => this.onSessionFailure(session, generation, new Error("core HTTP/2 connection closed")));
        session.on("goaway", () => this.onSessionFailure(session, generation, new Error("core HTTP/2 connection received GOAWAY")));
        return session;
    }

    static async connectFromEnvironment(): Promise<CoreHttp2Client> {
        const transport = process.env.AGENTY_TRANSPORT;
        const address = process.env.AGENTY_CORE_ADDR;
        const dataDir = process.env.AGENTY_DATA_DIR;
        const ipcVersion = process.env.AGENTY_IPC_VERSION;
        if (!transport || !address || !dataDir || !ipcVersion) {
            throw new Error("missing bootstrap IPC environment; start the CLI through agenty bootstrap");
        }
        if (ipcVersion !== "1") {
            throw new Error(`unsupported AGENTY_IPC_VERSION: ${ipcVersion}`);
        }
        const expectedTransport = process.platform === "win32" ? "named_pipe" : "uds";
        if (transport !== expectedTransport) {
            throw new Error(`AGENTY_TRANSPORT must be ${expectedTransport} on this platform`);
        }

        const client = new CoreHttp2Client(address);
        try {
            const info = await client.request<SystemInfo>("GET", "/v1/system");
            if (info.ipcVersion !== ipcVersion || info.apiContract !== "v1") {
                throw new Error(`core IPC protocol mismatch (ipc=${info.ipcVersion}, api=${info.apiContract})`);
            }
            const matchesDataDir = process.platform === "win32"
                ? info.dataDir.toLowerCase() === dataDir.toLowerCase()
                : info.dataDir === dataDir;
            if (!matchesDataDir) {
                throw new Error("connected core belongs to a different data directory");
            }
            return client;
        } catch (error) {
            client.close(error instanceof Error ? error : new Error(String(error)));
            throw error;
        }
    }

    async request<T>(method: string, path: string, body?: unknown): Promise<T> {
        if (this.closed) {
            throw this.closeReason;
        }
        const session = this.session ?? this.createSession();
        await this.waitForSessionConnect(session);
        if (this.closed) {
            throw this.closeReason;
        }
        if (session !== this.session || session.destroyed || session.closed) {
            throw new Error("core HTTP/2 connection changed before the request was sent");
        }
        return new Promise<T>((resolve, reject) => {
            const headers: Record<string, string> = { ":method": method, ":path": path };
            if (body !== undefined) {
                headers["content-type"] = "application/json";
            }
            let request: ClientHttp2Stream;
            try {
                request = session.request(headers, { endStream: body === undefined });
            } catch (error) {
                reject(error);
                return;
            }
            const chunks: Buffer[] = [];
            let status = 0;
            request.on("response", (response) => {
                status = Number(response[":status"] ?? 0);
            });
            request.on("data", (chunk: Buffer | string) => chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)));
            request.on("error", reject);
            request.on("end", () => {
                const text = Buffer.concat(chunks).toString("utf8");
                let decoded: unknown;
                try {
                    decoded = text ? JSON.parse(text) : undefined;
                } catch (error) {
                    reject(new Error(`core returned invalid JSON: ${(error as Error).message}`));
                    return;
                }
                if (!isAPIResponseBody(decoded)) {
                    reject(new Error("core returned an invalid API response"));
                    return;
                }
                if (status < 200 || status >= 300 || decoded.code < 200 || decoded.code >= 300) {
                    const failure = new Error(decoded.message || `core request failed with HTTP ${status}`);
                    Object.assign(failure, { code: decoded.errorCode ?? "http_error", status, responseCode: decoded.code });
                    reject(failure);
                    return;
                }
                resolve(decoded.data as T);
            });
            if (body !== undefined) {
                request.end(JSON.stringify(body));
            }
        });
    }

    onFrame(listener: (frame: StreamFrame) => void | Promise<void>): () => void {
        this.frameListeners.add(listener);
        return () => this.frameListeners.delete(listener);
    }

    onClose(listener: (reason: Error) => void): () => void {
        if (this.closed) {
            listener(this.closeReason);
            return () => {};
        }
        this.listeners.add(listener);
        return () => this.listeners.delete(listener);
    }

    subscribe(topic: string, after?: EventCursor): void {
        this.activeTopics.add(topic);
        if (after && !this.topicCursors.has(topic)) {
            this.topicCursors.set(topic, after);
        }
        this.replayPending.delete(topic);
        const cursor = after ?? this.topicCursors.get(topic);
        const session = this.session ?? this.createSession();
        if (!session.connecting) {
            this.ensureStream(session, this.sessionGeneration);
            this.writeCommand({ type: "subscribe", topic, ...(cursor ? { after: cursor } : {}) });
        }
    }

    subscribeAndWait(topic: string, after?: EventCursor): Promise<void> {
        if (this.closed) {
            return Promise.reject(this.closeReason);
        }
        this.activeTopics.add(topic);
        if (after && !this.topicCursors.has(topic)) {
            this.topicCursors.set(topic, after);
        }
        this.replayPending.delete(topic);
        const cursor = after ?? this.topicCursors.get(topic);
        return new Promise<void>((resolve, reject) => {
            const waiters = this.topicWaiters.get(topic) ?? [];
            waiters.push({ resolve, reject });
            this.topicWaiters.set(topic, waiters);
            try {
                const session = this.session ?? this.createSession();
                if (!session.connecting) {
                    this.ensureStream(session, this.sessionGeneration);
                    this.writeCommand({ type: "subscribe", topic, ...(cursor ? { after: cursor } : {}) });
                }
            } catch (error) {
                waiters.pop();
                if (waiters.length === 0) {
                    this.topicWaiters.delete(topic);
                }
                reject(error instanceof Error ? error : new Error(String(error)));
            }
        });
    }

    unsubscribe(topic: string): void {
        this.activeTopics.delete(topic);
        this.replayPending.delete(topic);
        this.writeCommand({ type: "unsubscribe", topic });
    }

    getCursor(topic: string): EventCursor | undefined {
        return this.topicCursors.get(topic);
    }

    setCursor(topic: string, cursor: EventCursor): void {
        this.topicCursors.set(topic, cursor);
    }

    disconnect(): void {
        this.close(new Error("CLI disconnected from core"));
    }

    close(reason = new Error("core HTTP/2 connection closed")): void {
        if (this.closed) {
            return;
        }
        this.closed = true;
        this.closeReason = reason;
        if (this.connectionRetryTimer) {
            clearTimeout(this.connectionRetryTimer);
            this.connectionRetryTimer = undefined;
        }
        if (this.streamRetryTimer) {
            clearTimeout(this.streamRetryTimer);
            this.streamRetryTimer = undefined;
        }
        if (this.outageTimer) {
            clearTimeout(this.outageTimer);
            this.outageTimer = undefined;
        }
        const stream = this.stream;
        this.stream = undefined;
        stream?.decoder.reset();
        stream?.stream.close();
        this.session?.destroy(reason);
        this.session = undefined;
        for (const listener of this.listeners) {
            listener(reason);
        }
        for (const waiters of this.topicWaiters.values()) {
            for (const waiter of waiters) {
                waiter.reject(reason);
            }
        }
        this.topicWaiters.clear();
        this.listeners.clear();
        this.frameListeners.clear();
    }

    private async waitForSessionConnect(session: ClientHttp2Session): Promise<void> {
        if (session.destroyed || session.closed) {
            throw this.closeReason;
        }
        if (!session.connecting) {
            return;
        }
        await new Promise<void>((resolve, reject) => {
            const cleanup = () => {
                session.off("connect", onConnect);
                session.off("error", onError);
                session.off("close", onClose);
            };
            const onConnect = () => {
                cleanup();
                if (this.session !== session) {
                    reject(new Error("core HTTP/2 connection changed before it was ready"));
                } else {
                    resolve();
                }
            };
            const onError = (error: Error) => {
                cleanup();
                reject(error);
            };
            const onClose = () => {
                cleanup();
                reject(this.closeReason);
            };
            session.once("connect", onConnect);
            session.once("error", onError);
            session.once("close", onClose);
        });
    }

    private onSessionConnected(session: ClientHttp2Session, generation: number): void {
        if (!this.isCurrentSession(session, generation)) {
            return;
        }
        if (this.streamRetryTimer) {
            clearTimeout(this.streamRetryTimer);
            this.streamRetryTimer = undefined;
        }
        this.reconnectAttempt = 0;
        if (this.activeTopics.size === 0) {
            this.clearOutage();
            return;
        }
        this.restoreSubscriptions(session, generation);
    }

    private onSessionFailure(session: ClientHttp2Session, generation: number, reason: Error): void {
        if (!this.isCurrentSession(session, generation)) {
            return;
        }
        if (this.streamRetryTimer) {
            clearTimeout(this.streamRetryTimer);
            this.streamRetryTimer = undefined;
        }
        this.session = undefined;
        if (this.stream?.session === session) {
            this.invalidateStream(this.stream);
        }
        this.replayPending.clear();
        session.destroy();
        this.beginOutage(reason);
        this.scheduleConnectionReconnect();
    }

    private isCurrentSession(session: ClientHttp2Session, generation: number): boolean {
        return !this.closed && this.session === session && this.sessionGeneration === generation;
    }

    private isCurrentStream(connection: EventStreamConnection): boolean {
        return !this.closed && this.stream === connection &&
            this.isCurrentSession(connection.session, connection.generation);
    }

    private ensureStream(
        session: ClientHttp2Session | undefined = this.session,
        generation = this.sessionGeneration,
    ): EventStreamConnection | undefined {
        if (this.closed || !session || !this.isCurrentSession(session, generation) ||
            session.connecting || session.closed || session.destroyed) {
            return undefined;
        }
        if (this.stream && this.isCurrentStream(this.stream)) {
            return this.stream;
        }
        let stream: ClientHttp2Stream;
        try {
            stream = session.request({
                ":method": "POST",
                ":path": "/v1/stream",
                "content-type": "application/x-ndjson",
            }, { endStream: false });
        } catch (error) {
            this.onSessionFailure(session, generation, error instanceof Error ? error : new Error(String(error)));
            return undefined;
        }
        const connection: EventStreamConnection = {
            session,
            generation,
            stream,
            decoder: new NdjsonFrameDecoder(),
        };
        this.stream = connection;
        stream.on("response", (headers) => {
            if (!this.isCurrentStream(connection)) {
                return;
            }
            const status = Number(headers[":status"] ?? 0);
            if (status < 200 || status >= 300) {
                this.onStreamFailure(connection, new Error(`core event stream failed with HTTP ${status}`));
                return;
            }
            this.reconnectAttempt = 0;
            this.clearOutage();
        });
        stream.on("data", (chunk: Buffer | string) => {
            if (!this.isCurrentStream(connection)) {
                return;
            }
            for (const line of connection.decoder.push(chunk)) {
                try {
                    this.enqueueFrame(JSON.parse(line) as StreamFrame, connection);
                } catch (error) {
                    this.close(new Error(`invalid core event frame: ${(error as Error).message}`));
                    return;
                }
            }
        });
        stream.on("error", (error: Error) => this.onStreamFailure(connection, error));
        stream.on("close", () => this.onStreamFailure(connection, new Error("core event stream closed")));
        return connection;
    }

    private writeCommand(command: { type: string; topic: string; after?: EventCursor }): boolean {
        const connection = this.stream;
        if (!connection || !this.isCurrentStream(connection) ||
            connection.stream.destroyed || !connection.stream.writable) {
            return false;
        }
        return connection.stream.write(`${JSON.stringify(command)}\n`);
    }

    private restoreSubscriptions(session: ClientHttp2Session, generation: number): void {
        void this.frameProcessing.then(() => {
            if (!this.isCurrentSession(session, generation) || this.activeTopics.size === 0) {
                return;
            }
            const connection = this.ensureStream(session, generation);
            if (!connection) {
                this.scheduleConnectionReconnect();
                return;
            }
            this.replayPending.clear();
            for (const topic of this.activeTopics) {
                const cursor = this.topicCursors.get(topic);
                this.writeCommand({ type: "subscribe", topic, ...(cursor ? { after: cursor } : {}) });
            }
        }).catch((error: unknown) => {
            this.close(error instanceof Error ? error : new Error(String(error)));
        });
    }

    private onStreamFailure(connection: EventStreamConnection, reason: Error): void {
        if (!this.isCurrentStream(connection)) {
            return;
        }
        this.invalidateStream(connection);
        this.replayPending.clear();
        if (this.activeTopics.size === 0) {
            this.clearOutage();
            return;
        }
        if (this.session && !this.session.destroyed && !this.session.closed) {
            this.beginOutage(reason);
            this.scheduleStreamReconnect(this.session, this.sessionGeneration);
            return;
        }
        this.beginOutage(reason);
        this.scheduleConnectionReconnect();
    }

    private invalidateStream(connection: EventStreamConnection): void {
        if (this.stream === connection) {
            this.stream = undefined;
        }
        connection.decoder.reset();
    }

    private scheduleStreamReconnect(session: ClientHttp2Session, generation: number): void {
        if (this.closed || this.activeTopics.size === 0 || this.streamRetryTimer) {
            return;
        }
        const delay = Math.min(100 * (2 ** this.reconnectAttempt), MAX_RECONNECT_DELAY_MS);
        this.reconnectAttempt += 1;
        this.streamRetryTimer = setTimeout(() => {
            this.streamRetryTimer = undefined;
            this.restoreSubscriptions(session, generation);
        }, delay);
    }

    private scheduleConnectionReconnect(): void {
        if (this.closed || this.connectionRetryTimer) {
            return;
        }
        const delay = Math.min(100 * (2 ** this.reconnectAttempt), MAX_RECONNECT_DELAY_MS);
        this.reconnectAttempt += 1;
        this.connectionRetryTimer = setTimeout(() => {
            this.connectionRetryTimer = undefined;
            if (this.closed || this.session) {
                return;
            }
            try {
                this.createSession();
            } catch (error) {
                this.beginOutage(error instanceof Error ? error : new Error(String(error)));
                this.scheduleConnectionReconnect();
            }
        }, delay);
    }

    private beginOutage(reason: Error): void {
        if (this.outageTimer || this.closed) {
            return;
        }
        this.outageTimer = setTimeout(() => {
            this.outageTimer = undefined;
            this.close(new Error(`core connection could not be restored: ${reason.message}`));
        }, RECONNECT_GRACE_MS);
    }

    private clearOutage(): void {
        if (this.outageTimer) {
            clearTimeout(this.outageTimer);
            this.outageTimer = undefined;
        }
    }

    private requestReplay(topic: string): void {
        if (!this.activeTopics.has(topic) || this.replayPending.has(topic)) {
            return;
        }
        this.replayPending.add(topic);
        const cursor = this.topicCursors.get(topic);
        if (!this.writeCommand({ type: "subscribe", topic, ...(cursor ? { after: cursor } : {}) })) {
            this.replayPending.delete(topic);
        }
    }

    private enqueueFrame(frame: StreamFrame, connection: EventStreamConnection): void {
        this.frameProcessing = this.frameProcessing.then(() => this.processFrame(frame, connection)).catch((error: unknown) => {
            this.close(error instanceof Error ? error : new Error(String(error)));
        });
    }

    private async processFrame(frame: StreamFrame, connection: EventStreamConnection): Promise<void> {
        if (!this.isCurrentStream(connection)) {
            return;
        }
        const cursor = frame.cursor ?? (frame.streamId && frame.sequence !== undefined
            ? { streamId: frame.streamId, sequence: frame.sequence }
            : undefined);
        const current = this.topicCursors.get(frame.topic);

        if (frame.type === "event") {
            if (!cursor) {
                throw new Error(`event frame for ${frame.topic} has no cursor`);
            }
            if (current) {
                if (cursor.streamId !== current.streamId) {
                    this.requestReplay(frame.topic);
                    return;
                }
                if (cursor.sequence <= current.sequence) {
                    return;
                }
                if (cursor.sequence !== current.sequence + 1) {
                    this.requestReplay(frame.topic);
                    return;
                }
            } else if (cursor.sequence !== 1) {
                this.requestReplay(frame.topic);
                return;
            }
        } else if (frame.type === "ready") {
            if (!cursor) {
                throw new Error(`ready frame for ${frame.topic} has no cursor`);
            }
            if (!current || cursor.streamId !== current.streamId || cursor.sequence !== current.sequence) {
                this.replayPending.delete(frame.topic);
                this.requestReplay(frame.topic);
                return;
            }
        } else if (frame.type === "snapshot" && !cursor) {
            throw new Error(`snapshot frame for ${frame.topic} has no cursor`);
        }

        for (const listener of this.frameListeners) {
            await listener(frame);
        }

        if (frame.type === "event" && cursor) {
            const applied = this.topicCursors.get(frame.topic);
            if ((!current && !applied) || (current && applied?.streamId === current.streamId &&
                applied.sequence === current.sequence)) {
                this.topicCursors.set(frame.topic, cursor);
            }
            return;
        }
        if (frame.type === "snapshot" && cursor) {
            this.topicCursors.set(frame.topic, cursor);
            this.replayPending.delete(frame.topic);
            this.resolveTopicWaiters(frame.topic);
            return;
        }
        if (frame.type === "ready") {
            this.replayPending.delete(frame.topic);
            this.resolveTopicWaiters(frame.topic);
            return;
        }
        if (frame.type === "error" && frame.topic) {
            this.replayPending.delete(frame.topic);
            this.rejectTopicWaiters(frame.topic, new Error(frame.message ?? `core rejected subscription to ${frame.topic}`));
            return;
        }
        if (frame.type === "disconnect") {
            this.onStreamFailure(connection, new Error(frame.message ?? "core disconnected the event stream"));
        }
    }

    private resolveTopicWaiters(topic: string): void {
        const waiters = this.topicWaiters.get(topic);
        if (!waiters) {
            return;
        }
        this.topicWaiters.delete(topic);
        for (const waiter of waiters) {
            waiter.resolve();
        }
    }

    private rejectTopicWaiters(topic: string, error: Error): void {
        const waiters = this.topicWaiters.get(topic);
        if (!waiters) {
            return;
        }
        this.topicWaiters.delete(topic);
        for (const waiter of waiters) {
            waiter.reject(error);
        }
    }
}
