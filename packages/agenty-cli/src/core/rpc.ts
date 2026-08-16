export interface RPCErrorData {
    code: number;
    message: string;
    data?: unknown;
}

interface RPCResponse {
    jsonrpc: "2.0";
    id: number;
    result?: unknown;
    error?: RPCErrorData;
}

interface RPCNotification {
    jsonrpc: "2.0";
    method: string;
    params?: unknown;
}

interface PendingRequest {
    resolve: (value: unknown) => void;
    reject: (reason: Error) => void;
}

export class RPCError extends Error {
    constructor(
        message: string,
        readonly code: number,
        readonly data?: unknown,
    ) {
        super(message);
    }
}

export class StdioRPCClient {
    private nextId = 1;
    private buffer = "";
    private closed = false;
    private readonly pending = new Map<number, PendingRequest>();
    private readonly listeners = new Map<string, Set<(params: unknown) => void>>();
    private readonly closeListeners = new Set<(reason: Error) => void>();
    private readonly decoder = new TextDecoder();
    private readonly encoder = new TextEncoder();

    constructor(
        private readonly stdin: Bun.FileSink,
        stdout: ReadableStream<Uint8Array>,
    ) {
        void this.read(stdout);
    }

    async call<T>(method: string, params: unknown = {}): Promise<T> {
        if (this.closed) {
            throw new Error("core RPC transport is closed");
        }
        const id = this.nextId++;
        const response = new Promise<T>((resolve, reject) => {
            this.pending.set(id, {
                resolve: (value) => resolve(value as T),
                reject,
            });
        });
        try {
            this.stdin.write(this.encoder.encode(`${JSON.stringify({
                jsonrpc: "2.0",
                id,
                method,
                params,
            })}\n`));
            await this.stdin.flush();
        } catch (error) {
            this.pending.delete(id);
            throw error;
        }
        return response;
    }

    onNotification<T>(method: string, listener: (params: T) => void): () => void {
        const listeners = this.listeners.get(method) ?? new Set();
        const wrapped = listener as (params: unknown) => void;
        listeners.add(wrapped);
        this.listeners.set(method, listeners);
        return () => {
            listeners.delete(wrapped);
            if (listeners.size === 0) {
                this.listeners.delete(method);
            }
        };
    }

    onClose(listener: (reason: Error) => void): () => void {
        if (this.closed) {
            listener(new Error("core RPC transport is closed"));
            return () => {};
        }
        this.closeListeners.add(listener);
        return () => this.closeListeners.delete(listener);
    }

    close(reason = new Error("core RPC transport closed")): void {
        if (this.closed) {
            return;
        }
        this.closed = true;
        for (const request of this.pending.values()) {
            request.reject(reason);
        }
        this.pending.clear();
        this.listeners.clear();
        for (const listener of this.closeListeners) {
            listener(reason);
        }
        this.closeListeners.clear();
    }

    private async read(stdout: ReadableStream<Uint8Array>): Promise<void> {
        try {
            for await (const chunk of stdout) {
                this.buffer += this.decoder.decode(chunk, { stream: true });
                this.drainLines();
            }
            this.buffer += this.decoder.decode();
            this.drainLines();
            this.close();
        } catch (error) {
            this.close(error instanceof Error ? error : new Error(String(error)));
        }
    }

    private drainLines(): void {
        for (;;) {
            const newline = this.buffer.indexOf("\n");
            if (newline < 0) {
                return;
            }
            const line = this.buffer.slice(0, newline).trim();
            this.buffer = this.buffer.slice(newline + 1);
            if (line !== "") {
                this.route(JSON.parse(line) as RPCResponse | RPCNotification);
            }
        }
    }

    private route(message: RPCResponse | RPCNotification): void {
        if ("method" in message) {
            for (const listener of this.listeners.get(message.method) ?? []) {
                listener(message.params);
            }
            return;
        }

        const request = this.pending.get(message.id);
        if (!request) {
            return;
        }
        this.pending.delete(message.id);
        if (message.error) {
            request.reject(new RPCError(message.error.message, message.error.code, message.error.data));
            return;
        }
        request.resolve(message.result);
    }
}
