import type {
    AgentDto,
    CreateModelProviderDto,
    ModelProviderDto,
    ReasoningEffort,
    UpdateAgentDto,
    UpdateModelProviderDto,
} from "../api/types";
import {
    type ModelDraft,
    type ProviderDraft,
    validateModelDraft,
    validateProviderDraft,
} from "../consts/providerPresets";

const DEFAULT_AGENT_CODE = "default";
const DEFAULT_AGENT_NAME = "Default";
const DEFAULT_AGENT_SOUL = "Be helpful, concise, and accurate.";

export interface WizardSetupClient {
    listProviders(): Promise<ModelProviderDto[]>;
    listAgents(): Promise<AgentDto[]>;
    createProvider(input: CreateModelProviderDto): Promise<ModelProviderDto>;
    updateProvider(code: string, input: UpdateModelProviderDto): Promise<ModelProviderDto>;
    createModel(input: {
        providerCode: string;
        modelCode: string;
        name: string;
        contextWindow?: number;
        reasoningEffortMapping?: Record<string, ReasoningEffort>;
        isDefault?: boolean;
    }): Promise<unknown>;
    createAgent(input: {
        code: string;
        name: string;
        soul?: string;
        defaultModel?: { providerCode: string; modelCode: string };
        defaultContextWindow?: number;
        defaultReasoningEffort?: ReasoningEffort;
        isDefault?: boolean;
    }): Promise<AgentDto>;
    updateAgent(code: string, input: UpdateAgentDto): Promise<AgentDto>;
    completeInitialization(input: {
        agentCode: string;
        providerCode: string;
        modelCode: string;
    }): Promise<{ initialized: boolean }>;
}

export function selectedModelCode(model: ModelDraft): string {
    return `${model.providerId}:${model.code.trim()}`;
}

export function validateWizardDrafts(
    drafts: ProviderDraft[],
    models: ModelDraft[],
    selectedId: string,
): string | null {
    if (drafts.length === 0) {
        return "Configure at least one provider to continue.";
    }
    if (models.length !== drafts.length) {
        return "Configure one model for each provider to continue.";
    }

    const providerCodes = new Set<string>();
    const providerIds = new Set(drafts.map((draft) => draft.id));
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

    const modelCodes = new Set<string>();
    for (const model of models) {
        if (!providerIds.has(model.providerId)) {
            return "A model is attached to an unknown provider.";
        }
        const modelError = validateModelDraft(model);
        if (modelError) {
            return modelError;
        }
        const modelCode = selectedModelCode(model);
        if (modelCodes.has(modelCode)) {
            return `Model Code already configured: ${model.code.trim()}`;
        }
        modelCodes.add(modelCode);
    }

    if (!modelCodes.has(selectedId)) {
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
    const existingAgents = (await client.listAgents()) ?? [];
    const modelsByProvider = new Map(models.map((model) => [model.providerId, model]));
    let selectedModel: ModelDraft | undefined;

    for (const draft of drafts) {
        const model = modelsByProvider.get(draft.id);
        if (!model) {
            throw new Error(`No model configured for provider ${draft.name || draft.code}.`);
        }

        const providerCode = draft.code.trim();
        const providerInput: CreateModelProviderDto = {
            code: providerCode,
            name: draft.name.trim(),
            type: draft.type,
            baseUrl: draft.baseUrl.trim(),
            apiKey: draft.apiKey.trim(),
        };
        const existing = existingProviders.find((provider) => provider.code === providerCode);
        if (existing) {
            await client.updateProvider(providerCode, {
                name: providerInput.name,
                type: providerInput.type,
                baseUrl: providerInput.baseUrl,
                apiKey: providerInput.apiKey,
            });
        } else {
            await client.createProvider(providerInput);
        }

        const modelCode = model.code.trim();
        const isSelected = selectedModelCode(model) === selectedId;
        if (isSelected) {
            selectedModel = model;
        }
        await client.createModel({
            providerCode,
            modelCode,
            name: model.name.trim(),
            contextWindow: model.contextWindow,
            reasoningEffortMapping: model.reasoningEffortMapping,
            isDefault: isSelected,
        });
    }

    if (!selectedModel) {
        throw new Error("Select a default model to continue.");
    }

    const selectedProvider = drafts.find((draft) => draft.id === selectedModel?.providerId);
    if (!selectedProvider) {
        throw new Error("Selected model provider is missing.");
    }
    const defaultModel = {
        providerCode: selectedProvider.code.trim(),
        modelCode: selectedModel.code.trim(),
    };
    const existingAgent = existingAgents.find((agent) => agent.code === DEFAULT_AGENT_CODE) ??
        existingAgents.find((agent) => agent.isDefault);
    let agentCode = DEFAULT_AGENT_CODE;
    const agentInput = {
        name: DEFAULT_AGENT_NAME,
        soul: DEFAULT_AGENT_SOUL,
        defaultModel,
        defaultContextWindow: selectedModel.contextWindow,
        defaultReasoningEffort: "off" as const,
        isDefault: true,
    };
    if (existingAgent) {
        agentCode = existingAgent.code;
        await client.updateAgent(agentCode, agentInput);
    } else {
        await client.createAgent({ code: DEFAULT_AGENT_CODE, ...agentInput });
    }

    await client.completeInitialization({
        agentCode,
        providerCode: defaultModel.providerCode,
        modelCode: defaultModel.modelCode,
    });
}
