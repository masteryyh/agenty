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
            agentCode: "default",
            providerCode: "openai",
            modelCode: "gpt-test",
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
            code: "empty",
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
            agentCode: "default",
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
        const agent = { code: "default", name: "Default" } as AgentDto;
        const currentModel = { providerCode: "openai", modelCode: "gpt-old" };
        const requestedModel = {
            code: "gpt-new",
            providerCode: "openai",
            providerName: "OpenAI",
        } as ModelDto;
        const existing = {
            id: "session",
            agentCode: "default",
            currentModel,
            rounds: [],
        } as unknown as ChatSessionDto;
        let updatedWith: ModelDto | undefined;
        const client = new AgentyClient({} as StdioRPCClient);
        client.resolveAgent = async () => agent;
        client.resolveModelInput = async () => requestedModel;
        client.getLastSessionByAgent = async () => existing;
        client.setSessionModel = async (_id, model) => {
            updatedWith = model;
            return { ...existing, currentModel: { providerCode: model.providerCode, modelCode: model.code } };
        };

        const prepared = await client.prepareSession({
            agentRef: "default",
            modelInput: "openai/gpt-new",
            newSession: false,
        });

        expect(updatedWith).toBe(requestedModel);
        expect(prepared.model).toBe(requestedModel);
        expect(prepared.session.currentModel).toEqual({ providerCode: "openai", modelCode: "gpt-new" });
    });

    test("resolves a persisted current model through its structured reference", async () => {
        const agent = { code: "default", name: "Default" } as AgentDto;
        const currentModel = { providerCode: "deepseek", modelCode: "deepseek-v4-pro" };
        const resolvedModel = {
            code: "deepseek-v4-pro",
            providerCode: "deepseek",
            providerName: "DeepSeek",
            name: "DeepSeek V4 Pro",
        } as ModelDto;
        const session = {
            id: "session",
            agentCode: "default",
            currentModel,
            rounds: [],
        } as unknown as ChatSessionDto;
        let requestedRef: unknown;
        const client = new AgentyClient({} as StdioRPCClient);
        client.resolveAgent = async () => agent;
        client.getLastSessionByAgent = async () => session;
        client.getModel = async (ref) => {
            requestedRef = ref;
            return resolvedModel;
        };

        const prepared = await client.prepareSession({ agentRef: "default", newSession: false });

        expect(requestedRef).toEqual(currentModel);
        expect(prepared.model).toBe(resolvedModel);
    });
});

describe("AgentyClient provider model discovery", () => {
    test("expands default efforts for a reasoning model with an empty list", async () => {
        const rpc = {
            call: async () => [{
                code: "provider",
                name: "Provider",
                type: "openai",
                baseUrl: "https://example.invalid",
                apiKey: "configured",
                models: [{
                    code: "reasoning-model",
                    name: "Reasoning model",
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
            }],
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.listModels()).resolves.toMatchObject([{
            reasoning: true,
            reasoningEfforts: ["low", "medium", "high", "xhigh", "max"],
        }]);
    });

    test("skips unconfigured providers when resolving the default model", async () => {
        const provider = (code: string, apiKey: string, modelCode: string): ModelProviderDto => ({
            code,
            name: code,
            type: "openai",
            baseUrl: "https://example.invalid",
            apiKey,
            models: [{
                code: modelCode,
                name: modelCode,
                contextWindow: 128000,
                maxOutputTokens: 8192,
                multiModal: false,
                light: false,
                isDefault: true,
            }],
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
        });
        const rpc = {
            call: async () => [
                provider("openai", "", "gpt-unconfigured"),
                provider("anthropic", "configured", "claude-configured"),
            ],
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.getDefaultModel()).resolves.toMatchObject({
            providerCode: "anthropic",
            code: "claude-configured",
        });
    });

    test("resolves structured references within the requested provider", async () => {
        let method = "";
        let params: unknown;
        const rpc = {
            call: async (name: string, input?: unknown) => {
                method = name;
                params = input;
                return [{
                    code: "openrouter",
                    name: "OpenRouter",
                    type: "openai",
                    baseUrl: "https://example.invalid",
                    apiKey: "configured",
                    models: [{
                        code: "deepseek/deepseek-v4-pro",
                        name: "DeepSeek: DeepSeek V4 Pro 0423",
                        contextWindow: 128000,
                        maxOutputTokens: 8192,
                        multiModal: false,
                        light: false,
                        isDefault: false,
                    }],
                    createdAt: "2026-01-01T00:00:00Z",
                    updatedAt: "2026-01-01T00:00:00Z",
                }];
            },
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.getModel({
            providerCode: "openrouter",
            modelCode: "deepseek/deepseek-v4-pro",
        })).resolves.toMatchObject({
            providerCode: "openrouter",
            code: "deepseek/deepseek-v4-pro",
        });
        expect(method).toBe("provider.list");
        expect(params).toEqual({ providerCode: "openrouter" });
    });

    test("passes an optional target provider to core provider.list", async () => {
        let method = "";
        let params: unknown;
        const rpc = {
            call: async (name: string, input?: unknown) => {
                method = name;
                params = input;
                return [];
            },
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.listProviders("openrouter")).resolves.toEqual([]);
        expect(method).toBe("provider.list");
        expect(params).toEqual({ providerCode: "openrouter" });
    });

    test("normalizes a null model list and reasoning capabilities", async () => {
        let params: unknown;
        const rpc = {
            call: async (_method: string, input: unknown) => {
                params = input;
                return [{
                    code: "gpt-test",
                    name: "GPT Test",
                    contextWindow: 256000,
                    maxOutputTokens: 65536,
                    multiModal: false,
                    reasoningEfforts: undefined,
                }, null];
            },
        } as unknown as StdioRPCClient;
        const client = new AgentyClient(rpc);

        await expect(client.listProviderModels("openai")).resolves.toEqual([{
            code: "gpt-test",
            name: "GPT Test",
            contextWindow: 256000,
            maxOutputTokens: 65536,
            multiModal: false,
            reasoningEfforts: [],
        }]);
        expect(params).toEqual({ providerCode: "openai" });
    });
});
