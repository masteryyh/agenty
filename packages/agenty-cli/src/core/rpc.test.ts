import { describe, expect, test } from "bun:test";

import { StdioRPCClient } from "./rpc";

class TestSink {
    written = "";

    write(chunk: Uint8Array): number {
        this.written += new TextDecoder().decode(chunk);
        return chunk.byteLength;
    }

    async flush(): Promise<number> {
        return 0;
    }
}

describe("StdioRPCClient", () => {
    test("routes a notification received before the matching response", async () => {
        const sink = new TestSink();
        let output!: ReadableStreamDefaultController<Uint8Array>;
        const stdout = new ReadableStream<Uint8Array>({
            start(controller) {
                output = controller;
            },
        });
        const client = new StdioRPCClient(sink as unknown as Bun.FileSink, stdout);
        const events: unknown[] = [];
        client.onNotification("session.event", (event) => events.push(event));

        const call = client.call<{ ok: boolean }>("session.start", { id: "session-1" });
        await Promise.resolve();
        expect(JSON.parse(sink.written.trim())).toEqual({
            jsonrpc: "2.0",
            id: 1,
            method: "session.start",
            params: { id: "session-1" },
        });

        const encoder = new TextEncoder();
        output.enqueue(encoder.encode(`${JSON.stringify({
            jsonrpc: "2.0",
            method: "session.event",
            params: { sessionId: "session-1", sequence: 1 },
        })}\n`));
        output.enqueue(encoder.encode(`${JSON.stringify({
            jsonrpc: "2.0",
            id: 1,
            result: { ok: true },
        })}\n`));

        await expect(call).resolves.toEqual({ ok: true });
        expect(events).toEqual([{ sessionId: "session-1", sequence: 1 }]);
        output.close();
    });

    test("notifies active consumers when the transport closes", async () => {
        const sink = new TestSink();
        let output!: ReadableStreamDefaultController<Uint8Array>;
        const stdout = new ReadableStream<Uint8Array>({
            start(controller) {
                output = controller;
            },
        });
        const client = new StdioRPCClient(sink as unknown as Bun.FileSink, stdout);
        const closed = new Promise<Error>((resolve) => client.onClose(resolve));

        output.error(new Error("core crashed"));

        await expect(closed).resolves.toEqual(new Error("core crashed"));
    });
});
