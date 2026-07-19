export const providerTypes = [
	"openai",
	"anthropic",
	"gemini",
	"kimi",
	"bigmodel",
	"qwen",
	"deepseek",
] as const;

export type ProviderType = (typeof providerTypes)[number];

export const providerDefaultBaseURLs: Record<string, string> = {
	openai: "https://api.openai.com/v1",
	"openai-legacy": "https://api.openai.com/v1",
	anthropic: "https://api.anthropic.com",
	gemini: "https://generativelanguage.googleapis.com/v1beta",
	kimi: "https://api.moonshot.cn/v1",
	bigmodel: "https://open.bigmodel.cn/api/paas/v4",
	qwen: "https://dashscope.aliyuncs.com/compatible-mode/v1",
	deepseek: "https://api.deepseek.com",
};
