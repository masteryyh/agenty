import { afterEach, describe, expect, test } from "bun:test";
import { randomUUID } from "node:crypto";
import { createServer, type ServerHttp2Session, type ServerHttp2Stream, type ClientHttp2Stream } from "node:http2";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { CoreHttp2Client, NdjsonFrameDecoder, type StreamFrame } from "./http2";

const originalEnvironment = {
    transport: process.env.AGENTY_TRANSPORT,
    address: process.env.AGENTY_CORE_ADDR,
    dataDir: process.env.AGENTY_DATA_DIR,
    version: process.env.AGENTY_IPC_VERSION,
};

afterEach(() => {
    restoreEnvironment();
});

describe("HTTP/2 event stream recovery", () => {
    test("decodes split UTF-8 and discards an incomplete frame when a stream resets", () => {
        const decoder = new NdjsonFrameDecoder();
        const raw = Buffer.from('{"text":"中文"}\n', "utf8");
        const split = raw.indexOf(Buffer.from("中", "utf8")) + 1;

        expect(decoder.push(raw.subarray(0, split))).toEqual([]);
        const decoded = decoder.push(raw.subarray(split));
        expect(decoded).toEqual(['{"text":"中文"}']);
        expect(JSON.parse(decoded[0]).text).toBe("中文");

        expect(decoder.push(Buffer.from('{"type":"event","text":"半', "utf8"))).toEqual([]);
        decoder.reset();
        expect(decoder.push(Buffer.from('{"type":"ready"}\n', "utf8"))).toEqual(['{"type":"ready"}']);
    });

    test("reconnects the HTTP/2 session from the last applied cursor and ignores stale stream callbacks", async () => {
        const dataDir = await mkdtemp(join(tmpdir(), "agenty-http2-reconnect-"));
        const pipeAddress = process.platform === "win32"
            ? `\\\\.\\pipe\\agenty-http2-${process.pid}-${randomUUID()}`
            : join(dataDir, "ipc.sock");
        const transport = process.platform === "win32" ? "named_pipe" : "uds";
        process.env.AGENTY_TRANSPORT = transport;
        process.env.AGENTY_CORE_ADDR = pipeAddress;
        process.env.AGENTY_DATA_DIR = dataDir;
        process.env.AGENTY_IPC_VERSION = "1";

        const server = createServer();
        const sessions: ServerHttp2Session[] = [];
        const eventStreams = new Set<ServerHttp2Stream>();
        const subscriptions: Array<{ after?: { streamId: string; sequence: number }; streamIndex: number }> = [];
        const persistedEvents: StreamFrame[] = [];
        let streamId = "stream-one";
        let latest = 1;
        let acceptedNonIdempotentRequests = 0;
        let client: CoreHttp2Client | undefined;

        const sendFrame = (stream: ServerHttp2Stream, frame: StreamFrame) => {
            stream.write(`${JSON.stringify(frame)}\n`);
        };
        const eventFrame = (sequence: number, text: string): StreamFrame => ({
            type: "event",
            topic: "mcp",
            streamId,
            sequence,
            cursor: { streamId, sequence },
            event: { kind: "test", text },
        });

        server.on("session", (session) => sessions.push(session));
        server.on("stream", (stream, headers) => {
            const path = headers[":path"];
            if (path === "/v1/system") {
                stream.respond({ ":status": 200, "content-type": "application/json" });
                stream.end(JSON.stringify({
                    code: 200,
                    message: "ok",
                    data: {
                        dataDir,
                        ipcVersion: "1",
                        apiContract: "v1",
                        processId: process.pid,
                        initialized: true,
                        eventStreamId: streamId,
                    },
                }));
                return;
            }
            if (path === "/v1/stream") {
                stream.respond({ ":status": 200, "content-type": "application/x-ndjson" });
                eventStreams.add(stream);
                stream.on("close", () => eventStreams.delete(stream));
                let input = "";
                stream.on("data", (chunk: Buffer | string) => {
                    input += Buffer.isBuffer(chunk) ? chunk.toString("utf8") : chunk;
                    for (;;) {
                        const newline = input.indexOf("\n");
                        if (newline < 0) break;
                        const line = input.slice(0, newline).trim();
                        input = input.slice(newline + 1);
                        if (!line) continue;
                        const command = JSON.parse(line) as { type: string; topic: string; after?: { streamId: string; sequence: number } };
                        if (command.type !== "subscribe") continue;
                        const streamIndex = sessions.length;
                        subscriptions.push({ after: command.after, streamIndex });
                        if (!command.after || command.after.streamId !== streamId) {
                            sendFrame(stream, {
                                type: "snapshot",
                                topic: command.topic,
                                streamId,
                                sequence: latest,
                                cursor: { streamId, sequence: latest },
                                snapshot: { state: { ready: true }, recentEvents: [] },
                            });
                        } else {
                            for (const frame of persistedEvents) {
                                if (frame.sequence! > command.after.sequence) sendFrame(stream, frame);
                            }
                            sendFrame(stream, {
                                type: "ready",
                                topic: command.topic,
                                streamId,
                                sequence: latest,
                                cursor: { streamId, sequence: latest },
                            });
                        }
                    }
                });
                return;
            }
            if (path === "/v1/non-idempotent") {
                stream.on("end", () => {
                    acceptedNonIdempotentRequests += 1;
                    stream.respond({ ":status": 202, "content-type": "application/json" });
                    stream.end(JSON.stringify({ code: 202, message: "ok", data: {} }));
                });
                stream.resume();
                return;
            }
            if (path === "/v1/response-data") {
                stream.respond({ ":status": 200, "content-type": "application/json" });
                stream.end(JSON.stringify({ code: 200, message: "ok", data: null }));
                return;
            }
            if (path === "/v1/malformed-response") {
                stream.respond({ ":status": 200, "content-type": "application/json" });
                stream.end(JSON.stringify({ code: 200, message: "ok" }));
                return;
            }
            stream.respond({ ":status": 404, "content-type": "application/json" });
            stream.end(JSON.stringify({ code: 404, message: "API route not found", data: null, errorCode: "not_found" }));
        });

        await new Promise<void>((resolve, reject) => {
            server.once("error", reject);
            server.listen(pipeAddress, resolve);
        });

        try {
            client = await CoreHttp2Client.connectFromEnvironment();
            await expect(client.request("GET", "/v1/response-data")).resolves.toBeNull();
            await expect(client.request("GET", "/v1/missing")).rejects.toMatchObject({
                message: "API route not found", code: "not_found", status: 404, responseCode: 404,
            });
            await expect(client.request("GET", "/v1/malformed-response")).rejects.toThrow("invalid API response");
            const received: StreamFrame[] = [];
            client.onFrame((frame) => { received.push(frame); });
            await client.subscribeAndWait("mcp");
            expect(client.getCursor("mcp")).toEqual({ streamId, sequence: 1 });

            const firstEvent = eventFrame(2, "事件中文");
            latest = 2;
            persistedEvents.push(firstEvent);
            const initialStream = [...eventStreams][0];
            if (!initialStream) throw new Error("initial stream was not opened");
            const eventBytes = Buffer.from(`${JSON.stringify(firstEvent)}\n`, "utf8");
            const unicodeSplit = eventBytes.indexOf(Buffer.from("中", "utf8")) + 1;
            initialStream.write(eventBytes.subarray(0, unicodeSplit));
            await delay(25);
            initialStream.write(eventBytes.subarray(unicodeSplit));
            await waitFor(() => client?.getCursor("mcp")?.sequence === 2, "event sequence 2");

            const clientInternals = client as unknown as { stream?: { stream: ClientHttp2Stream } };
            const oldClientStream = clientInternals.stream?.stream;
            if (!oldClientStream) throw new Error("client stream was not created");

            const replayedEvent = eventFrame(3, "replayed 中文");
            latest = 3;
            persistedEvents.push(replayedEvent);
            const partial = Buffer.from(`${JSON.stringify(replayedEvent)}\n`, "utf8");
            const partialEnd = partial.indexOf(Buffer.from("中", "utf8")) + 1;
            initialStream.write(partial.subarray(0, partialEnd));
            await delay(25);

            const nonIdempotent = client.request("POST", "/v1/non-idempotent", { value: "once" });
            await waitFor(() => acceptedNonIdempotentRequests === 1, "single non-idempotent request");
            await expect(nonIdempotent).resolves.toEqual({});
            sessions[0]?.destroy();

            await waitFor(
                () => subscriptions.some((item) => item.streamIndex > 1 && item.after?.sequence === 2),
                "resume subscription after cursor 2",
            );
            await waitFor(() => client?.getCursor("mcp")?.sequence === 3, "replayed event sequence 3");

            const spuriousOldFrame = eventFrame(99, "must be ignored");
            oldClientStream.emit("data", Buffer.from(`${JSON.stringify(spuriousOldFrame)}\n`, "utf8"));
            oldClientStream.emit("close");
            await delay(30);
            expect(client.getCursor("mcp")).toEqual({ streamId, sequence: 3 });

            const currentStream = [...eventStreams][0];
            if (!currentStream) throw new Error("reconnected stream was not opened");
            latest = 4;
            const finalEvent = eventFrame(4, "live after reconnect");
            persistedEvents.push(finalEvent);
            sendFrame(currentStream, finalEvent);
            await waitFor(() => client?.getCursor("mcp")?.sequence === 4, "live event sequence 4");

            expect(received.filter((frame) => frame.type === "event").map((frame) => frame.sequence)).toEqual([2, 3, 4]);
            expect(received.find((frame) => frame.sequence === 2)?.event).toEqual({ kind: "test", text: "事件中文" });
            expect(acceptedNonIdempotentRequests).toBe(1);
            expect(subscriptions.find((item) => item.streamIndex > 1)?.after).toEqual({ streamId, sequence: 2 });

            const subscriptionsBeforeGap = subscriptions.length;
            const replayAfterGap = eventFrame(5, "recovered after gap");
            latest = 5;
            persistedEvents.push(replayAfterGap);
            sendFrame(currentStream, eventFrame(6, "must not be applied before replay"));
            await waitFor(() => subscriptions.length > subscriptionsBeforeGap, "gap replay subscription");
            await waitFor(() => client?.getCursor("mcp")?.sequence === 5, "replayed event after gap");
            expect(client.getCursor("mcp")).toEqual({ streamId, sequence: 5 });
            expect(received.filter((frame) => frame.type === "event").map((frame) => frame.sequence)).toEqual([2, 3, 4, 5]);

            const subscriptionsBeforeExpiredCursor = subscriptions.length;
            streamId = "stream-two";
            latest = 1;
            persistedEvents.length = 0;
            sendFrame(currentStream, eventFrame(7, "old stream id"));
            await waitFor(() => subscriptions.length > subscriptionsBeforeExpiredCursor, "expired cursor resubscription");
            await waitFor(() => client?.getCursor("mcp")?.streamId === "stream-two", "snapshot cursor after stream reset");
            expect(client.getCursor("mcp")).toEqual({ streamId: "stream-two", sequence: 1 });

            const afterSnapshot = eventFrame(2, "live after snapshot");
            latest = 2;
            persistedEvents.push(afterSnapshot);
            sendFrame(currentStream, afterSnapshot);
            await waitFor(() => client?.getCursor("mcp")?.sequence === 2, "event after snapshot");
            expect(client.getCursor("mcp")).toEqual({ streamId: "stream-two", sequence: 2 });
        } finally {
            client?.disconnect();
            for (const session of sessions) session.destroy();
            await new Promise<void>((resolve) => server.close(() => resolve()));
            await rm(dataDir, { recursive: true, force: true });
            restoreEnvironment();
        }
    });
});

function delay(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitFor(predicate: () => boolean, label: string): Promise<void> {
    const deadline = Date.now() + 5_000;
    while (Date.now() < deadline) {
        if (predicate()) return;
        await delay(10);
    }
    throw new Error(`timed out waiting for ${label}`);
}

function restoreEnvironment(): void {
    if (originalEnvironment.transport === undefined) delete process.env.AGENTY_TRANSPORT;
    else process.env.AGENTY_TRANSPORT = originalEnvironment.transport;
    if (originalEnvironment.address === undefined) delete process.env.AGENTY_CORE_ADDR;
    else process.env.AGENTY_CORE_ADDR = originalEnvironment.address;
    if (originalEnvironment.dataDir === undefined) delete process.env.AGENTY_DATA_DIR;
    else process.env.AGENTY_DATA_DIR = originalEnvironment.dataDir;
    if (originalEnvironment.version === undefined) delete process.env.AGENTY_IPC_VERSION;
    else process.env.AGENTY_IPC_VERSION = originalEnvironment.version;
}
