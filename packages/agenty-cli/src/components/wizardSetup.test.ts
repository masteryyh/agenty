import { describe, expect, test } from "bun:test";

import type { AgentDto, ModelProviderDto } from "../api/types";
import {
    createPresetDraft,
    type ModelDraft,
    modelDraftForProvider,
    type ProviderDraft,
    providerPresets,
    validateModelDraft,
    validateProviderDraft,
} from "../consts/providerPresets";
import {
    persistWizardSetup,
    selectedModelCode,
    type WizardSetupClient,
} from "./wizardSetup";

function createDraft(): ProviderDraft {
    return {
        ...createPresetDraft(providerPresets[0]),
        apiKey: "test-key",
    };
}

function createModel(draft: ProviderDraft): ModelDraft {
    return modelDraftForProvider(draft, providerPresets[0]);
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
): WizardSetupClient & { calls: string[] } {
    const calls: string[] = [];
    return {
        calls,
        listProviders: async () => {
            calls.push("provider.list");
            return providers;
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
        createModel: async () => {
            calls.push("provider.addModel");
            return undefined;
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
    test("exposes the three supported preset providers", () => {
        expect(providerPresets.map((preset) => preset.key)).toEqual([
            "openai",
            "anthropic",
            "google",
        ]);
        expect(providerPresets.map((preset) => preset.type)).toEqual([
            "openai",
            "anthropic",
            "gemini",
        ]);
        expect(providerPresets.every((preset) => preset.model.code.length > 0)).toBe(true);
    });

    test("restores an existing provider and its preferred model", () => {
        const preset = providerPresets[0];
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
        const restoredProvider = createPresetDraft(preset, existing);
        const restoredModel = modelDraftForProvider(restoredProvider, preset, existing);

        expect(restoredProvider.name).toBe("OpenAI gateway");
        expect(restoredProvider.baseUrl).toBe("https://gateway.example/v1");
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

        await persistWizardSetup(client, [draft], [model], selectedModelCode(model));

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

        await persistWizardSetup(client, [draft], [model], selectedModelCode(model));

        expect(client.calls).toEqual([
            "provider.list",
            "agent.list",
            "provider.update",
            "provider.addModel",
            "agent.update",
            "initialize.complete",
        ]);
    });
});
