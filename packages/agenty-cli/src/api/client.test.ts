import { describe, expect, test } from "bun:test";

import type { CoreTransport, StreamFrame } from "../core/http2";
import { AgentyClient } from "./client";
import type {
    ChatMessageDto,
    ChatSessionDto,
    ModelDto,
    ModelProviderDto,
} from "./types";

class MockTransport implements CoreTransport {
    readonly calls: Array<{ method: string; path: string; body?: unknown }> = [];
    readonly frames = new Set<(frame: StreamFrame) => void | Promise<void>>();
    private readonly cursors = new Map<string, { streamId: string; sequence: number }>();
    private closeListener?: (reason: Error) => void;

    constructor(private readonly respond: (method: string, path: string, body?: unknown) => unknown = () => ({})) {}

    async request<T>(method: string, path: string, body?: unknown): Promise<T> {
        this.calls.push({ method, path, ...(body === undefined ? {} : { body }) });
        return this.respond(method, path, body) as T;
    }

    onFrame(listener: (frame: StreamFrame) => void | Promise<void>): () => void {
        this.frames.add(listener);
        return () => this.frames.delete(listener);
    }

    onClose(listener: (reason: Error) => void): () => void {
        this.closeListener = listener;
        return () => { this.closeListener = undefined; };
    }

    subscribe(): void {}
    subscribeAndWait(): Promise<void> {
        return Promise.resolve();
    }
    unsubscribe(): void {}
    getCursor(topic: string): { streamId: string; sequence: number } | undefined {
        return this.cursors.get(topic);
    }
    setCursor(topic: string, cursor: { streamId: string; sequence: number }): void {
        this.cursors.set(topic, cursor);
    }
    disconnect(): void {
        this.closeListener?.(new Error("disconnected"));
    }

    async emit(frame: StreamFrame): Promise<void> {
        for (const listener of this.frames) await listener(frame);
    }
}

const emptyTransport = () => new MockTransport();

describe("AgentyClient HTTP endpoints", () => {
    test("posts a scoped tool approval decision to the approval resource", async () => {
        const transport = emptyTransport();
        const resolution = {
            sessionId: "session",
            roundId: "round",
            approvalId: "approval",
            decision: "deny" as const,
        };
        await new AgentyClient(transport).resolveToolApproval(resolution);
        expect(transport.calls).toEqual([{
            method: "POST",
            path: "/v1/tool-approvals/approval/resolution",
            body: resolution,
        }]);
    });

    test("normalizes a null initialization response", async () => {
        const transport = new MockTransport(() => null);
        const client = new AgentyClient(transport);
        await expect(client.isInitialized()).resolves.toBe(false);
        await expect(client.completeInitialization({
            providerCode: "openai",
            modelCode: "gpt-test",
        })).resolves.toEqual({ initialized: false });
        expect(transport.calls.map(({ method, path }) => [method, path])).toEqual([
            ["GET", "/v1/initialization"],
            ["POST", "/v1/initialization"],
        ]);
    });

    test("normalizes null session and provider collections", async () => {
        const transport = new MockTransport(() => null);
        const client = new AgentyClient(transport);
        await expect(client.listSessionSummaries()).resolves.toEqual([]);
        await expect(client.listProviders()).resolves.toEqual([]);
        expect(transport.calls.map(({ path }) => path)).toEqual(["/v1/sessions", "/v1/providers"]);
    });

    test("normalizes provider models before projecting a model list", async () => {
        const provider = {
            code: "empty",
            name: "Empty",
            type: "openai",
            baseUrl: "https://example.invalid",
            apiKey: "",
            models: null,
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
        } as unknown as ModelProviderDto;
        const transport = new MockTransport(() => [provider]);
        await expect(new AgentyClient(transport).listModels()).resolves.toEqual([]);
    });

    test("normalizes provider OAuth capability and authentication mode", async () => {
        const openrouter = {
            code: "openrouter",
            name: "OpenRouter",
            type: "openai",
            baseUrl: "https://openrouter.ai/api/v1",
            apiKey: "oauth-key",
            oauth: true,
            authMethod: "oauth",
            models: [],
            createdAt: "",
            updatedAt: "",
        } as ModelProviderDto;
        const deepseek = {
            ...openrouter,
            code: "deepseek",
            name: "DeepSeek",
            oauth: undefined,
            authMethod: undefined,
        };
        const client = new AgentyClient(new MockTransport(() => [openrouter, deepseek]));

        await expect(client.listProviders()).resolves.toMatchObject([
            { code: "openrouter", oauth: true, authMethod: "oauth" },
            { code: "deepseek", oauth: false, authMethod: "apiKey" },
        ]);
    });

    test("normalizes session messages and collections", async () => {
        const session = {
            id: "session",
            contextWindow: 128000,
            rounds: null,
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
        } as unknown as ChatSessionDto;
        const transport = new MockTransport(() => session);
        await expect(new AgentyClient(transport).getSession("session"))
            .resolves.toMatchObject({ rounds: [] });
        expect(transport.calls[0]).toMatchObject({
            method: "GET",
            path: "/v1/sessions/session",
        });
    });

    test("normalizes null event messages and nested tool results", async () => {
        const transport = emptyTransport();
        let received: ChatMessageDto | undefined;
        const client = new AgentyClient(transport);
        client.onSessionEvent((event) => { received = event.message; });
        await transport.emit({
            type: "event",
            topic: "session:session",
            streamId: "stream",
            sequence: 1,
            cursor: { streamId: "stream", sequence: 1 },
            event: {
                kind: "session",
                type: "message_appended",
                sessionId: "session",
                roundId: "round",
                sequence: 1,
                message: {
                    id: "message",
                    roundId: "round",
                    role: "assistant",
                    content: [{ type: "tool_result", toolUseId: "tool", content: null, isError: false }],
                    createdAt: "2026-01-01T00:00:00Z",
                },
            },
        });
        expect(received?.content).toEqual([
            { type: "tool_result", toolUseId: "tool", content: [], isError: false },
        ]);
    });

    test("queries a provider-scoped model endpoint", async () => {
        const provider = {
            code: "openrouter",
            name: "OpenRouter",
            type: "openai",
            baseUrl: "https://example.invalid",
            apiKey: "configured",
            models: [{
                code: "deepseek/deepseek-v4-pro",
                name: "DeepSeek: DeepSeek V4 Pro",
                contextWindow: 128000,
                maxOutputTokens: 8192,
                multiModal: false,
                light: false,
                isDefault: false,
            }],
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
        };
        const transport = new MockTransport(() => [provider]);
        const model = await new AgentyClient(transport).getModel({
            providerCode: "openrouter",
            modelCode: "deepseek/deepseek-v4-pro",
        });
        expect(model).toMatchObject({ providerCode: "openrouter", code: "deepseek/deepseek-v4-pro" });
        expect(transport.calls[0].path).toBe("/v1/providers?providerCode=openrouter");
    });

    test("uses the current HTTP API for model discovery and resolves defaults", async () => {
        const providers: ModelProviderDto[] = [{
            code: "anthropic",
            name: "Anthropic",
            type: "anthropic",
            baseUrl: "https://example.invalid",
            apiKey: "configured",
            models: [{
                code: "claude-configured",
                name: "Claude",
                contextWindow: 128000,
                maxOutputTokens: 8192,
                multiModal: false,
                light: false,
                reasoning: true,
                reasoningEfforts: [],
                isDefault: true,
            }],
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
        }];
        const transport = new MockTransport(() => providers);
        await expect(new AgentyClient(transport).getDefaultModel()).resolves.toMatchObject({
            providerCode: "anthropic",
            code: "claude-configured",
            reasoningEfforts: ["low", "medium", "high", "xhigh", "max"],
        });
        expect(transport.calls[0].path).toBe("/v1/initialization");
        expect(transport.calls[1].path).toBe("/v1/providers");
    });

    test("creates sessions with an explicit cwd", async () => {
        const session = { id: "session", rounds: [] } as unknown as ChatSessionDto;
        const transport = new MockTransport(() => session);
        const model = { providerCode: "openai", code: "gpt-test", contextWindow: 128000 } as ModelDto;
        await new AgentyClient(transport).createSession(model, "off", "ask", "D:\\work");
        expect(transport.calls[0]).toMatchObject({
            method: "POST",
            path: "/v1/sessions",
            body: { cwd: "D:\\work", providerCode: "openai", modelCode: "gpt-test" },
        });
    });

    test("creates Codex Mode sessions with the requested tool dialect", async () => {
        const session = { id: "session", rounds: [], toolDialect: "codex" } as unknown as ChatSessionDto;
        const transport = new MockTransport(() => session);
        const model = { providerCode: "openai", code: "gpt-test", contextWindow: 128000 } as ModelDto;

        await new AgentyClient(transport).createSession(model, "off", "ask", "/workspace", "codex");

        expect(transport.calls[0]).toMatchObject({
            method: "POST",
            path: "/v1/sessions",
            body: { toolDialect: "codex" },
        });
    });

    test("updates the session tool dialect", async () => {
        const session = { id: "session", rounds: [], toolDialect: "default" } as unknown as ChatSessionDto;
        const transport = new MockTransport(() => session);

        await new AgentyClient(transport).setToolDialect("session", "default");

        expect(transport.calls).toEqual([{
            method: "PUT",
            path: "/v1/sessions/session/tool-dialect",
            body: { toolDialect: "default" },
        }]);
    });
});
