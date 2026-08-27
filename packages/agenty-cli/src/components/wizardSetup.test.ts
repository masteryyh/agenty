import { describe, expect, test } from "bun:test";

import type { AgentDto, ModelProviderDto } from "../api/types";
import {
    createBuiltinDraft,
    createCustomDraft,
    createModelDraft,
    type ModelDraft,
    modelDraftsForProvider,
    type ProviderDraft,
    validateModelDraft,
    validateProviderDraft,
} from "../consts/providerPresets";
import {
    persistWizardSetup,
    selectedModelId,
    validateWizardDrafts,
    type WizardSetupClient,
} from "./wizardSetup";

function createDraft(): ProviderDraft {
    return {
        ...createCustomDraft("custom:0", {
            ...createProviderResource(),
            code: "custom",
            name: "Custom",
            type: "openai_completions",
            builtin: false,
            official: false,
        }),
        apiKey: "test-key",
    };
}

function createModel(draft: ProviderDraft): ModelDraft {
    const provider = {
        ...createProviderResource(),
        code: draft.code,
        name: draft.name,
        type: draft.type,
        builtin: false,
        official: false,
    };
    return createModelDraft(draft, `${draft.id}:model`, provider.models[0]);
}

function createProviderResource(): ModelProviderDto {
    return {
        code: "openai",
        name: "OpenAI",
        type: "openai",
        baseUrl: "https://api.openai.com/v1",
        apiKey: "",
        builtin: true,
        official: true,
        models: [{
            code: "gpt-5-mini",
            name: "GPT-5 mini",
            contextWindow: 400_000,
            maxOutputTokens: 128_000,
            multiModal: true,
            light: true,
            reasoningEfforts: ["low", "medium", "high", "xhigh"],
            isDefault: true,
        }],
        createdAt: "",
        updatedAt: "",
    };
}

function createAgent(code: string, isDefault: boolean): AgentDto {
    return {
        code,
        name: code,
        soul: "",
        defaultContextWindow: 128_000,
        isDefault,
        createdAt: "",
        updatedAt: "",
    };
}

function createProvider(draft: ProviderDraft, model: ModelDraft = createModel(draft)): ModelProviderDto {
    return {
        code: draft.code,
        name: draft.name,
        type: draft.type,
        baseUrl: draft.baseUrl,
        apiKey: draft.apiKey,
        models: [{
            code: model.code,
            name: model.name,
            contextWindow: model.contextWindow,
            maxOutputTokens: 8_192,
            multiModal: false,
            light: false,
            isDefault: true,
        }],
        createdAt: "",
        updatedAt: "",
    };
}

function fakeClient(
    providers: ModelProviderDto[] = [],
    agents: AgentDto[] = [],
): WizardSetupClient & {
    calls: string[];
    createdModels: Array<{ modelCode: string; isDefault?: boolean }>;
    deletedModels: Array<{ providerCode: string; modelCode: string }>;
} {
    const calls: string[] = [];
    const createdModels: Array<{ modelCode: string; isDefault?: boolean }> = [];
    const deletedModels: Array<{ providerCode: string; modelCode: string }> = [];
    return {
        calls,
        createdModels,
        deletedModels,
        listProviders: async () => {
            calls.push("provider.list");
            return providers;
        },
        listProviderModels: async () => {
            calls.push("provider.listModels");
            return [];
        },
        listAgents: async () => {
            calls.push("agent.list");
            return agents;
        },
        createProvider: async () => {
            calls.push("provider.create");
            return providers[0] ?? createProvider(createDraft());
        },
        updateProvider: async () => {
            calls.push("provider.update");
            return providers[0] ?? createProvider(createDraft());
        },
        createModel: async (input) => {
            calls.push("provider.addModel");
            createdModels.push(input);
            return undefined;
        },
        deleteModel: async (providerCode, modelCode) => {
            calls.push("provider.removeModel");
            deletedModels.push({ providerCode, modelCode });
        },
        createAgent: async () => {
            calls.push("agent.create");
            return createAgent("default", true);
        },
        updateAgent: async () => {
            calls.push("agent.update");
            return createAgent("default", true);
        },
        completeInitialization: async () => {
            calls.push("initialize.complete");
            return { initialized: true };
        },
    };
}

describe("first-run provider setup", () => {
    test("restores built-in provider metadata from the core projection", () => {
        const provider = createProviderResource();
        const draft = createBuiltinDraft(provider);
        expect(draft.source).toBe("builtin");
        expect(draft.code).toBe("openai");
        expect(modelDraftsForProvider(draft, provider).map((model) => model.code)).toEqual(["gpt-5-mini"]);
    });

    test("restores an existing provider and its preferred model", () => {
        const draft = createDraft();
        const existingModel = createModel(draft);
        const existing: ModelProviderDto = {
            ...createProvider(draft, existingModel),
            name: "OpenAI gateway",
            baseUrl: "https://gateway.example/v1",
            models: [{
                ...createProvider(draft, existingModel).models[0],
                code: "gateway-model",
                name: "Gateway model",
                isDefault: true,
            }],
        };
        const restoredProvider = createBuiltinDraft(createProviderResource(), existing);
        const restoredModel = modelDraftsForProvider(restoredProvider, existing)[0];

        expect(restoredProvider.name).toBe("OpenAI");
        expect(restoredProvider.baseUrl).toBe("https://api.openai.com/v1");
        expect(restoredModel.code).toBe("gateway-model");
    });

    test("keeps provider validation separate from model validation", () => {
        const draft = createDraft();
        const model = createModel(draft);

        expect(validateProviderDraft({ ...draft, apiKey: "" })).toContain("API key");
        expect(validateProviderDraft(draft)).toBeNull();
        expect(validateModelDraft({ ...model, contextWindow: 0 })).toContain("Context window");
        expect(validateModelDraft({ ...model, code: "org/model_name[v2]" })).toBeNull();
    });

    test("creates resources in the core initialization order", async () => {
        const draft = createDraft();
        const model = createModel(draft);
        const client = fakeClient();

        await persistWizardSetup(client, [draft], [model], selectedModelId(model));

        expect(client.calls).toEqual([
            "provider.list",
            "agent.list",
            "provider.create",
            "provider.addModel",
            "agent.create",
            "initialize.complete",
        ]);
    });

    test("updates existing resources so a partial setup can resume", async () => {
        const draft = createDraft();
        const model = createModel(draft);
        const client = fakeClient([createProvider(draft, model)], [createAgent("default", true)]);

        await persistWizardSetup(client, [draft], [model], selectedModelId(model));

        expect(client.calls).toEqual([
            "provider.list",
            "agent.list",
            "provider.addModel",
            "agent.update",
            "initialize.complete",
        ]);
    });

    test("persists multiple custom models and removes models deleted in the wizard", async () => {
        const draft = createDraft();
        const first = { ...createModel(draft), id: `${draft.id}:first`, code: "first", name: "First" };
        const second = { ...createModel(draft), id: `${draft.id}:second`, code: "second", name: "Second" };
        const existing = createProvider(draft, first);
        existing.models.push({ ...existing.models[0], code: "removed", name: "Removed" });
        const client = fakeClient([existing], [createAgent("default", true)]);

        await persistWizardSetup(client, [draft], [first, second], selectedModelId(second));

        expect(client.calls).toEqual([
            "provider.list",
            "agent.list",
            "provider.removeModel",
            "provider.addModel",
            "provider.addModel",
            "agent.update",
            "initialize.complete",
        ]);
        expect(client.deletedModels).toEqual([{ providerCode: "custom", modelCode: "removed" }]);
        expect(client.createdModels.map(({ modelCode, isDefault }) => ({ modelCode, isDefault }))).toEqual([
            { modelCode: "first", isDefault: false },
            { modelCode: "second", isDefault: true },
        ]);
    });

    test("allows multiple models per provider and rejects duplicate model codes", () => {
        const draft = createDraft();
        const first = { ...createModel(draft), id: `${draft.id}:first` };
        const second = { ...createModel(draft), id: `${draft.id}:second`, code: "second" };

        expect(validateWizardDrafts([draft], [first, second], selectedModelId(second))).toBeNull();
        expect(validateWizardDrafts(
            [draft],
            [first, { ...second, code: first.code }],
            selectedModelId(second),
        )).toContain("already configured");
    });

    test("updates only the API key when a built-in provider is selected", async () => {
        const provider = { ...createProviderResource(), apiKey: "test-key" };
        const draft = createBuiltinDraft(provider);
        const models = modelDraftsForProvider(draft, provider);
        const client = fakeClient([provider]);

        await persistWizardSetup(client, [draft], models, selectedModelId(models[0]));

        expect(client.calls).toEqual([
            "provider.list",
            "agent.list",
            "agent.create",
            "initialize.complete",
        ]);
    });

    test("does not persist an untouched discovered model cache", async () => {
        const draft = createDraft();
        const provider = {
            ...createProvider(draft),
            models: [{ ...createProvider(draft).models[0], cached: true }],
            modelsCached: true,
        };
        const models = modelDraftsForProvider(draft, provider);
        const client = fakeClient([provider], [createAgent("default", true)]);

        expect(models[0].source).toBe("cached");
        await persistWizardSetup(client, [draft], models, selectedModelId(models[0]));

        expect(client.createdModels).toEqual([]);
        expect(client.deletedModels).toEqual([]);
        expect(client.calls).toEqual([
            "provider.list",
            "agent.list",
            "agent.update",
            "initialize.complete",
        ]);
    });

    test("removes configured models while preserving discovered cache entries", async () => {
        const draft = createDraft();
        const provider = {
            ...createProvider(draft),
            models: [
                { ...createProvider(draft).models[0], code: "configured", name: "Configured", isDefault: false },
                { ...createProvider(draft).models[0], code: "cached", name: "Cached", cached: true, isDefault: true },
            ],
            modelsCached: true,
        };
        const models = modelDraftsForProvider(draft, provider);
        const cached = models.find((model) => model.code === "cached");
        const client = fakeClient([provider], [createAgent("default", true)]);

        expect(models.map((model) => ({ code: model.code, source: model.source }))).toEqual([
            { code: "configured", source: "configured" },
            { code: "cached", source: "cached" },
        ]);
        expect(cached).toBeDefined();
        await persistWizardSetup(client, [draft], [cached!], selectedModelId(cached!));

        expect(client.deletedModels).toEqual([{ providerCode: "custom", modelCode: "configured" }]);
        expect(client.createdModels).toEqual([]);
    });

    test("preserves a default model for each provider", async () => {
        const firstDraft = createDraft();
        const secondDraft = {
            ...createDraft(),
            id: "custom:1",
            code: "second",
            name: "Second",
        };
        const first = { ...createModel(firstDraft), id: "custom:0:first", code: "first", isDefault: true };
        const second = { ...createModel(secondDraft), id: "custom:1:second", code: "second", isDefault: true };
        const client = fakeClient(
            [createProvider(firstDraft, first), createProvider(secondDraft, second)],
            [createAgent("default", true)],
        );

        await persistWizardSetup(client, [firstDraft, secondDraft], [first, second], selectedModelId(first));

        expect(client.createdModels.map(({ modelCode, isDefault }) => ({ modelCode, isDefault }))).toEqual([
            { modelCode: "first", isDefault: true },
            { modelCode: "second", isDefault: true },
        ]);
    });
});
