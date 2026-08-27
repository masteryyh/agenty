import type { APIType, CoreModelDto, ModelProviderDto, ReasoningEffort } from "../api/types";
import { STANDARD_REASONING_EFFORTS } from "../api/types";

export const compatibleProviderTypes: readonly { label: string; value: APIType }[] = [
    { label: "OpenAI Responses API", value: "openai" },
    { label: "OpenAI Chat Completions", value: "openai_completions" },
    { label: "Anthropic Messages API", value: "anthropic" },
    { label: "Google Gemini API", value: "gemini" },
];

export function isBuiltinProvider(provider: Pick<ModelProviderDto, "builtin"> | string): boolean {
    return typeof provider === "string" ? false : provider.builtin === true;
}

export interface ProviderDraft {
    id: string;
    source: "builtin" | "custom";
    originalCode?: string;
    code: string;
    name: string;
    type: APIType;
    baseUrl: string;
    apiKey: string;
    freeFormTool: boolean;
    builtin: boolean;
}

export interface ModelDraft {
    source: "configured" | "cached" | "new";
    id: string;
    providerId: string;
    providerCode: string;
    providerName: string;
    originalCode?: string;
    code: string;
    name: string;
    contextWindow: number;
    maxOutputTokens: number;
    multiModal: boolean;
    light: boolean;
    reasoning?: boolean;
    reasoningEfforts: ReasoningEffort[];
    isDefault: boolean;
    isBuiltin: boolean;
}

export function createBuiltinDraft(provider: ModelProviderDto, existing?: ModelProviderDto): ProviderDraft {
    const source = existing ?? provider;
    return {
        id: `builtin:${provider.code}`,
        source: "builtin",
        originalCode: provider.code,
        code: provider.code,
        name: provider.name,
        type: provider.type,
        baseUrl: provider.baseUrl,
        apiKey: source.apiKey ?? "",
        freeFormTool: provider.freeFormTool === true,
        builtin: true,
    };
}

export function createCustomDraft(id: string, existing?: ModelProviderDto): ProviderDraft {
    return {
        id,
        source: "custom",
        originalCode: existing?.code,
        code: existing?.code ?? "",
        name: existing?.name ?? "",
        type: existing?.type ?? "openai_completions",
        baseUrl: existing?.baseUrl ?? "",
        apiKey: existing?.apiKey ?? "",
        freeFormTool: existing?.freeFormTool === true,
        builtin: false,
    };
}

export function draftForProvider(provider: ModelProviderDto): ProviderDraft {
    return provider.builtin === true
        ? createBuiltinDraft(provider)
        : createCustomDraft(`provider:${provider.code}`, provider);
}

export function createModelDraft(
    provider: ProviderDraft,
    id: string,
    existing?: CoreModelDto,
    source: ModelDraft["source"] = existing === undefined ? "new" : "configured",
): ModelDraft {
    return {
        source,
        id,
        providerId: provider.id,
        providerCode: provider.code,
        providerName: provider.name,
        originalCode: existing?.code,
        code: existing?.code ?? "",
        name: existing?.name ?? "",
        contextWindow: existing?.contextWindow ?? 128_000,
        maxOutputTokens: existing?.maxOutputTokens ?? 8_192,
        multiModal: existing?.multiModal ?? false,
        light: existing?.light ?? false,
        reasoning: existing === undefined
            ? true
            : existing.reasoning !== false && (existing.reasoning === true || (existing.reasoningEfforts?.length ?? 0) > 0),
        reasoningEfforts: existing?.reasoning === false
            ? []
            : existing?.reasoningEfforts ?? [...STANDARD_REASONING_EFFORTS],
        isDefault: existing?.isDefault ?? false,
        isBuiltin: provider.builtin === true,
    };
}

export function modelDraftsForProvider(
    provider: ProviderDraft,
    existing?: ModelProviderDto,
): ModelDraft[] {
    return (existing?.models ?? []).map((model) =>
        createModelDraft(
            provider,
            `${provider.id}:model:${model.code}`,
            model,
            model.cached === true ? "cached" : "configured",
        ),
    );
}

export function validateProviderDraft(draft: ProviderDraft): string | null {
    if (!draft.code.trim()) {
        return "Provider code is required.";
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
    if (!draft.code.trim()) {
        return `Model Code is required for ${draft.providerName.trim() || draft.providerCode}.`;
    }
    if (!draft.name.trim()) {
        return `Model name is required for ${draft.providerName.trim() || draft.providerCode}.`;
    }
    if (!Number.isSafeInteger(draft.contextWindow) || draft.contextWindow <= 0) {
        return `Context window for ${draft.name.trim()} must be a positive integer.`;
    }
    if (!Number.isSafeInteger(draft.maxOutputTokens) || draft.maxOutputTokens <= 0) {
        return `Max output tokens for ${draft.name.trim()} must be a positive integer.`;
    }
    return null;
}
