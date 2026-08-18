import type { APIType, CoreModelDto, ModelProviderDto, ReasoningEffort } from "../api/types";

export interface ModelPreset {
    slug: string;
    name: string;
    contextWindow: number;
    reasoningEffortMapping?: Record<string, ReasoningEffort>;
}

export interface ProviderPreset {
    key: string;
    label: string;
    description: string;
    slug: string;
    name: string;
    type: APIType;
    baseUrl: string;
    model: ModelPreset;
}

export const providerPresets: readonly ProviderPreset[] = [
    {
        key: "openai",
        label: "OpenAI",
        description: "Responses API",
        slug: "openai",
        name: "OpenAI",
        type: "openai",
        baseUrl: "https://api.openai.com/v1",
        model: {
            slug: "gpt-5-mini",
            name: "GPT-5 mini",
            contextWindow: 128_000,
            reasoningEffortMapping: {
                low: "low",
                medium: "medium",
                high: "high",
                xhigh: "xhigh",
            },
        },
    },
    {
        key: "anthropic",
        label: "Anthropic",
        description: "Messages API",
        slug: "anthropic",
        name: "Anthropic",
        type: "anthropic",
        baseUrl: "https://api.anthropic.com",
        model: {
            slug: "claude-haiku-4-5",
            name: "Claude Haiku 4.5",
            contextWindow: 200_000,
            reasoningEffortMapping: {
                low: "low",
                medium: "medium",
                high: "high",
                max: "max",
            },
        },
    },
    {
        key: "google",
        label: "Google",
        description: "Gemini API",
        slug: "google",
        name: "Google",
        type: "gemini",
        baseUrl: "https://generativelanguage.googleapis.com/v1beta",
        model: {
            slug: "gemini-2.5-flash",
            name: "Gemini 2.5 Flash",
            contextWindow: 128_000,
            reasoningEffortMapping: {
                low: "low",
                medium: "medium",
                high: "high",
            },
        },
    },
];

export const compatibleProviderTypes: readonly { label: string; value: APIType }[] = [
    { label: "OpenAI Responses API", value: "openai" },
    { label: "OpenAI Chat Completions", value: "openai_completions" },
    { label: "Anthropic Messages API", value: "anthropic" },
    { label: "Google Gemini API", value: "gemini" },
];

export interface ProviderDraft {
    id: string;
    source: "preset" | "custom";
    presetKey?: string;
    slug: string;
    name: string;
    type: APIType;
    baseUrl: string;
    apiKey: string;
}

export interface ModelDraft {
    id: string;
    providerId: string;
    providerSlug: string;
    providerName: string;
    slug: string;
    name: string;
    contextWindow: number;
    reasoningEffortMapping?: Record<string, ReasoningEffort>;
}

function preferredModel(existing?: ModelProviderDto): CoreModelDto | undefined {
    const models = existing?.models ?? [];
    return models.find((model) => model.isDefault) ?? models[0];
}

export function createPresetDraft(
    preset: ProviderPreset,
    existing?: ModelProviderDto,
): ProviderDraft {
    return {
        id: `preset:${preset.key}`,
        source: "preset",
        presetKey: preset.key,
        slug: existing?.slug ?? preset.slug,
        name: existing?.name ?? preset.name,
        type: existing?.type ?? preset.type,
        baseUrl: existing?.baseUrl ?? preset.baseUrl,
        apiKey: existing?.apiKey ?? "",
    };
}

export function createCustomDraft(
    id: string,
    existing?: ModelProviderDto,
): ProviderDraft {
    return {
        id,
        source: "custom",
        slug: existing?.slug ?? "",
        name: existing?.name ?? "",
        type: existing?.type ?? "openai_completions",
        baseUrl: existing?.baseUrl ?? "",
        apiKey: existing?.apiKey ?? "",
    };
}

export function draftForProvider(
    provider: ModelProviderDto,
    preset?: ProviderPreset,
): ProviderDraft {
    if (preset) {
        return createPresetDraft(preset, provider);
    }
    return createCustomDraft(`provider:${provider.slug}`, provider);
}

export function modelDraftForProvider(
    provider: ProviderDraft,
    preset?: ProviderPreset,
    existing?: ModelProviderDto,
): ModelDraft {
    const model = preferredModel(existing);
    const fallback = preset?.model;
    return {
        id: `${provider.id}:model`,
        providerId: provider.id,
        providerSlug: provider.slug,
        providerName: provider.name,
        slug: model?.slug ?? fallback?.slug ?? "",
        name: model?.name ?? fallback?.name ?? "",
        contextWindow: model?.contextWindow ?? fallback?.contextWindow ?? 128_000,
        reasoningEffortMapping: model?.reasoningEffortMapping ?? fallback?.reasoningEffortMapping,
    };
}

export function validateProviderDraft(draft: ProviderDraft): string | null {
    if (!draft.slug.trim()) {
        return "Provider slug is required.";
    }
    if (!draft.name.trim()) {
        return "Provider name is required.";
    }
    if (!draft.baseUrl.trim()) {
        return "Base URL is required.";
    }
    if (!draft.apiKey.trim()) {
        return `Enter an API key for ${draft.name.trim()}.`;
    }
    return null;
}

export function validateModelDraft(draft: ModelDraft): string | null {
    if (!draft.slug.trim()) {
        return `Model ID is required for ${draft.providerName.trim() || draft.providerSlug}.`;
    }
    if (!draft.name.trim()) {
        return `Model name is required for ${draft.providerName.trim() || draft.providerSlug}.`;
    }
    if (!Number.isSafeInteger(draft.contextWindow) || draft.contextWindow <= 0) {
        return `Context window for ${draft.name.trim()} must be a positive integer.`;
    }
    return null;
}
