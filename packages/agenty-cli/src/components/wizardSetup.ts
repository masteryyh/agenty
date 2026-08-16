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

const DEFAULT_AGENT_SLUG = "default";
const DEFAULT_AGENT_NAME = "Default";
const DEFAULT_AGENT_SOUL = "Be helpful, concise, and accurate.";

export interface WizardSetupClient {
    listProviders(): Promise<ModelProviderDto[]>;
    listAgents(): Promise<AgentDto[]>;
    createProvider(input: CreateModelProviderDto): Promise<ModelProviderDto>;
    updateProvider(slug: string, input: UpdateModelProviderDto): Promise<ModelProviderDto>;
    createModel(input: {
        providerSlug: string;
        modelSlug: string;
        name: string;
        contextWindow?: number;
        maxOutputTokens: number;
        reasoningEffortMapping?: Record<string, ReasoningEffort>;
        isDefault?: boolean;
    }): Promise<unknown>;
    createAgent(input: {
        slug: string;
        name: string;
        soul?: string;
        defaultModel?: { providerSlug: string; modelSlug: string };
        defaultContextWindow?: number;
        defaultReasoningEffort?: ReasoningEffort;
        isDefault?: boolean;
    }): Promise<AgentDto>;
    updateAgent(slug: string, input: UpdateAgentDto): Promise<AgentDto>;
    completeInitialization(input: {
        agentSlug: string;
        providerSlug: string;
        modelSlug: string;
    }): Promise<{ initialized: boolean }>;
}

export function selectedModelId(model: ModelDraft): string {
    return `${model.providerId}:${model.slug.trim()}`;
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

    const providerSlugs = new Set<string>();
    const providerIds = new Set(drafts.map((draft) => draft.id));
    for (const draft of drafts) {
        const providerError = validateProviderDraft(draft);
        if (providerError) {
            return providerError;
        }
        const slug = draft.slug.trim();
        if (providerSlugs.has(slug)) {
            return `Provider slug already configured: ${slug}`;
        }
        providerSlugs.add(slug);
    }

    const modelIds = new Set<string>();
    for (const model of models) {
        if (!providerIds.has(model.providerId)) {
            return "A model is attached to an unknown provider.";
        }
        const modelError = validateModelDraft(model);
        if (modelError) {
            return modelError;
        }
        const modelId = selectedModelId(model);
        if (modelIds.has(modelId)) {
            return `Model ID already configured: ${model.slug.trim()}`;
        }
        modelIds.add(modelId);
    }

    if (!modelIds.has(selectedId)) {
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
            throw new Error(`No model configured for provider ${draft.name || draft.slug}.`);
        }

        const providerSlug = draft.slug.trim();
        const providerInput: CreateModelProviderDto = {
            slug: providerSlug,
            name: draft.name.trim(),
            type: draft.type,
            baseUrl: draft.baseUrl.trim(),
            apiKey: draft.apiKey.trim(),
        };
        const existing = existingProviders.find((provider) => provider.slug === providerSlug);
        if (existing) {
            await client.updateProvider(providerSlug, {
                name: providerInput.name,
                type: providerInput.type,
                baseUrl: providerInput.baseUrl,
                apiKey: providerInput.apiKey,
            });
        } else {
            await client.createProvider(providerInput);
        }

        const modelSlug = model.slug.trim();
        const isSelected = selectedModelId(model) === selectedId;
        if (isSelected) {
            selectedModel = model;
        }
        await client.createModel({
            providerSlug,
            modelSlug,
            name: model.name.trim(),
            contextWindow: model.contextWindow,
            maxOutputTokens: model.maxOutputTokens,
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
        providerSlug: selectedProvider.slug.trim(),
        modelSlug: selectedModel.slug.trim(),
    };
    const existingAgent = existingAgents.find((agent) => agent.slug === DEFAULT_AGENT_SLUG) ??
        existingAgents.find((agent) => agent.isDefault);
    let agentSlug = DEFAULT_AGENT_SLUG;
    const agentInput = {
        name: DEFAULT_AGENT_NAME,
        soul: DEFAULT_AGENT_SOUL,
        defaultModel,
        defaultContextWindow: selectedModel.contextWindow,
        defaultReasoningEffort: "off" as const,
        isDefault: true,
    };
    if (existingAgent) {
        agentSlug = existingAgent.slug;
        await client.updateAgent(agentSlug, agentInput);
    } else {
        await client.createAgent({ slug: DEFAULT_AGENT_SLUG, ...agentInput });
    }

    await client.completeInitialization({
        agentSlug,
        providerSlug: defaultModel.providerSlug,
        modelSlug: defaultModel.modelSlug,
    });
}
