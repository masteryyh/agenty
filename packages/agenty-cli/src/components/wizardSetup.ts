import type {
    CreateModelDto,
    CreateModelProviderDto,
    ModelProviderDto,
    ReasoningEffort,
    UpdateModelProviderDto,
} from "../api/types";
import {
    type ModelDraft,
    type ProviderDraft,
    validateModelDraft,
    validateProviderDraft,
} from "../consts/providerPresets";

export interface WizardSetupClient {
    listProviders(): Promise<ModelProviderDto[]>;
    listProviderModels(providerCode: string): Promise<unknown>;
    createProvider(input: CreateModelProviderDto): Promise<ModelProviderDto>;
    updateProvider(code: string, input: UpdateModelProviderDto): Promise<ModelProviderDto>;
    createModel(input: CreateModelDto): Promise<unknown>;
    deleteModel(providerCode: string, modelCode: string): Promise<void>;
    completeInitialization(input: {
        providerCode: string;
        modelCode: string;
        reasoningEffort?: ReasoningEffort;
    }): Promise<{ initialized: boolean }>;
}

export function selectedModelId(model: ModelDraft): string {
    return model.id;
}

export function validateWizardDrafts(
    drafts: ProviderDraft[],
    models: ModelDraft[],
    selectedId: string,
): string | null {
    if (drafts.length === 0) {
        return "Configure at least one provider to continue.";
    }
    if (models.length === 0) {
        return "Configure or select at least one model to continue.";
    }

    const providerCodes = new Set<string>();
    const providersById = new Map(drafts.map((draft) => [draft.id, draft]));
    for (const draft of drafts) {
        const providerError = validateProviderDraft(draft);
        if (providerError) {
            return providerError;
        }
        const code = draft.code.trim();
        if (providerCodes.has(code)) {
            return `Provider code already configured: ${code}`;
        }
        providerCodes.add(code);
    }

    const modelCodesByProvider = new Map<string, Set<string>>();
    for (const model of models) {
        const provider = providersById.get(model.providerId);
        if (!provider) {
            return "A model is attached to an unknown provider.";
        }
        if (model.providerCode.trim() !== provider.code.trim()) {
            return `Model provider does not match ${provider.name.trim() || provider.code.trim()}.`;
        }
        const modelError = validateModelDraft(model);
        if (modelError) {
            return modelError;
        }

        const modelCodes = modelCodesByProvider.get(model.providerId) ?? new Set<string>();
        const modelCode = model.code.trim();
        if (modelCodes.has(modelCode)) {
            return `Model Code already configured: ${model.code.trim()}`;
        }
        modelCodes.add(modelCode);
        modelCodesByProvider.set(model.providerId, modelCodes);
    }

    if (!models.some((model) => selectedModelId(model) === selectedId)) {
        return "Select a default model to continue.";
    }
    return null;
}

export async function persistWizardSetup(
    client: WizardSetupClient,
    drafts: ProviderDraft[],
    models: ModelDraft[],
    selectedId: string,
): Promise<void> {
    const validationError = validateWizardDrafts(drafts, models, selectedId);
    if (validationError) {
        throw new Error(validationError);
    }

    const existingProviders = (await client.listProviders()) ?? [];
    const modelsByProvider = new Map<string, ModelDraft[]>();
    for (const model of models) {
        const providerModels = modelsByProvider.get(model.providerId) ?? [];
        providerModels.push(model);
        modelsByProvider.set(model.providerId, providerModels);
    }
    const selectedModel = models.find((model) => selectedModelId(model) === selectedId);
    if (!selectedModel) {
        throw new Error("Select a default model to continue.");
    }

    for (const draft of drafts) {
        const providerCode = draft.code.trim();
        const existing = existingProviders.find((provider) => provider.code === providerCode);
        let providerChanged = false;
        if (draft.builtin) {
            providerChanged = existing?.apiKey !== draft.apiKey.trim();
            if (providerChanged) {
                await client.updateProvider(providerCode, { apiKey: draft.apiKey.trim() });
            }
        } else if (existing) {
            providerChanged = existing.name !== draft.name.trim() ||
                existing.type !== draft.type ||
                existing.baseUrl !== draft.baseUrl.trim() ||
                existing.apiKey !== draft.apiKey.trim() ||
                (existing.freeFormTool === true) !== (draft.type === "openai" && draft.freeFormTool);
            if (providerChanged) {
                await client.updateProvider(providerCode, {
                    name: draft.name.trim(),
                    type: draft.type,
                    baseUrl: draft.baseUrl.trim(),
                    apiKey: draft.apiKey.trim(),
                    freeFormTool: draft.type === "openai" && draft.freeFormTool,
                });
            }
        } else {
            const providerInput: CreateModelProviderDto = {
                code: providerCode,
                name: draft.name.trim(),
                type: draft.type,
                baseUrl: draft.baseUrl.trim(),
                apiKey: draft.apiKey.trim(),
                freeFormTool: draft.type === "openai" && draft.freeFormTool,
            };
            await client.createProvider(providerInput);
        }

        if (existing?.modelsCached === true && providerChanged) {
            await client.listProviderModels(providerCode);
        }

        if (!draft.builtin) {
            const providerModels = modelsByProvider.get(draft.id) ?? [];
            const desiredModelCodes = new Set(providerModels.map((model) => model.code.trim()));
            for (const model of existing?.models ?? []) {
                if (model.cached !== true && !desiredModelCodes.has(model.code)) {
                    await client.deleteModel(providerCode, model.code);
                }
            }

            const selectedProviderModel = providerModels.find((model) => selectedModelId(model) === selectedId);
            const persistSelectedProviderDefault = selectedProviderModel !== undefined && selectedProviderModel.source !== "cached";

            for (const model of providerModels) {
                if (model.source === "cached") {
                    continue;
                }
                await client.createModel({
                    providerCode,
                    modelCode: model.code.trim(),
                    name: model.name.trim(),
                    contextWindow: model.contextWindow,
                    maxOutputTokens: model.maxOutputTokens,
                    multiModal: model.multiModal,
                    light: model.light,
                    reasoning: model.reasoning !== false && (model.reasoning === true || model.reasoningEfforts.length > 0),
                    reasoningEfforts: model.reasoningEfforts,
                    isDefault: persistSelectedProviderDefault
                        ? selectedModelId(model) === selectedId
                        : model.isDefault,
                });
            }

            if (existing?.modelsCached === true && !providerChanged && providerModels.some((model) => model.source !== "cached")) {
                await client.listProviderModels(providerCode);
            }
        }
    }

    const selectedProvider = drafts.find((draft) => draft.id === selectedModel.providerId);
    if (!selectedProvider) {
        throw new Error("Selected model provider is missing.");
    }
    const defaultModel = {
        providerCode: selectedProvider.code.trim(),
        modelCode: selectedModel.code.trim(),
    };
    await client.completeInitialization({
        providerCode: defaultModel.providerCode,
        modelCode: defaultModel.modelCode,
        reasoningEffort: "off",
    });
}
