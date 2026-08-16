import { describe, expect, test } from "bun:test";

import type { StdioRPCClient } from "../core/rpc";
import { AgentyClient } from "./client";
import type { AgentDto, ChatMessageDto, ChatSessionDto, ModelDto, ModelProviderDto } from "./types";

describe("AgentyClient session list", () => {
    test("treats a null initialize status as not initialized", async () => {
        const rpc = {
            call: async () => null,
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.isInitialized()).resolves.toBe(false);
        await expect(client.completeInitialization({
            agentSlug: "default",
            providerSlug: "openai",
            modelSlug: "gpt-test",
        })).resolves.toEqual({ initialized: false });
    });

    test("normalizes an empty core result from null to an empty array", async () => {
        const rpc = {
            call: async () => null,
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.listSessionSummaries()).resolves.toEqual([]);
    });

    test("normalizes null agent and provider lists", async () => {
        const rpc = {
            call: async () => null,
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.listAgents()).resolves.toEqual([]);
        await expect(client.listProviders()).resolves.toEqual([]);
    });

    test("normalizes null provider models before model projection", async () => {
        const provider = {
            slug: "empty",
            name: "Empty",
            type: "openai",
            baseUrl: "https://example.invalid",
            apiKey: "",
            models: null,
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
        } as unknown as ModelProviderDto;
        const rpc = {
            call: async () => [provider],
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.listModels()).resolves.toEqual([]);
    });

    test("normalizes null event messages and nested tool results", () => {
        let notify: ((event: unknown) => void) | undefined;
        let received: ChatMessageDto | undefined;
        const rpc = {
            onNotification: (_method: string, listener: (event: unknown) => void) => {
                notify = listener;
                return () => {};
            },
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        client.onSessionEvent((event) => {
            received = event.message;
        });
        notify?.({
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
        });

        expect(received?.content).toEqual([
            { type: "tool_result", toolUseId: "tool", content: [], isError: false },
        ]);
    });

    test("normalizes null session collections for old core responses", async () => {
        const session = {
            id: "session",
            agentSlug: "default",
            contextWindow: 128000,
            rounds: null,
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
        } as unknown as ChatSessionDto;
        const rpc = {
            call: async () => session,
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.getSession("session")).resolves.toMatchObject({ rounds: [] });
    });

    test("updates a resumed session when an explicit model is requested", async () => {
        const agent = { slug: "default", name: "Default" } as AgentDto;
        const currentModel = { providerSlug: "openai", modelSlug: "gpt-old" };
        const requestedModel = {
            slug: "gpt-new",
            providerSlug: "openai",
            providerName: "OpenAI",
        } as ModelDto;
        const existing = {
            id: "session",
            agentSlug: "default",
            currentModel,
            rounds: [],
        } as unknown as ChatSessionDto;
        let updatedWith: ModelDto | undefined;
        const client = new AgentyClient({} as StdioRPCClient);
        client.resolveAgent = async () => agent;
        client.resolveModel = async () => requestedModel;
        client.getLastSessionByAgent = async () => existing;
        client.setSessionModel = async (_id, model) => {
            updatedWith = model;
            return { ...existing, currentModel: { providerSlug: model.providerSlug, modelSlug: model.slug } };
        };

        const prepared = await client.prepareSession({
            agentRef: "default",
            modelRef: "openai/gpt-new",
            newSession: false,
        });

        expect(updatedWith).toBe(requestedModel);
        expect(prepared.model).toBe(requestedModel);
        expect(prepared.session.currentModel).toEqual({ providerSlug: "openai", modelSlug: "gpt-new" });
    });
});
