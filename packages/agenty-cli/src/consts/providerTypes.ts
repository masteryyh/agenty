export const providerTypes = [
    "openai",
    "openai_completions",
    "anthropic",
    "gemini",
] as const;

export type ProviderType = (typeof providerTypes)[number];

export const providerDefaultBaseURLs: Record<string, string> = {
    openai: "https://api.openai.com/v1",
    openai_completions: "https://api.openai.com/v1",
    anthropic: "https://api.anthropic.com",
    gemini: "https://generativelanguage.googleapis.com/v1beta",
};
