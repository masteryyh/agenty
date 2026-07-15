/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
