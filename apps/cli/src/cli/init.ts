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

import type { AgentyClient } from "@/api/client";
import {
	CliError,
	configured,
	displayModel,
	flag,
	hasFlag,
	listAll,
	outputFields,
	render,
	requirePositionals,
	resolveModel,
	resolveProvider,
	secret,
	type ParsedArgs
} from "./utils";

export async function handleInit(client: AgentyClient, args: ParsedArgs): Promise<void> {
	requirePositionals(args, 1, "init [options]");
	await runInit(client, args);
}

async function runInit(client: AgentyClient, args: ParsedArgs): Promise<void> {
	const providerRef = flag(args, "provider")?.trim();
	const modelRef = flag(args, "model")?.trim();
	if (!providerRef) {
		throw new CliError("--provider is required");
	}
	if (!modelRef) {
		throw new CliError("--model is required");
	}

	const apiKey = secret(args, "api-key", "api-key-env", "provider API key");
	const webSearchKey = secret(args, "web-search-api-key", "web-search-api-key-env", "web search API key");

	let provider = await resolveProvider(client, providerRef);
	if (provider.apiKeyCensored === "<not set>" && !apiKey) {
		throw new CliError(`provider ${JSON.stringify(provider.name)} is not configured; use --api-key or --api-key-env`);
	}

	const baseUrl = flag(args, "base-url")?.trim();
	if (apiKey || baseUrl) {
		provider = await client.updateProvider(provider.id, { ...(apiKey ? { apiKey } : {}), ...(baseUrl ? { baseUrl } : {}) });
	}

	const chatModel = await resolveModel(client, modelRef, false);
	if (chatModel.provider?.id !== provider.id) {
		throw new CliError(`model ${JSON.stringify(displayModel(chatModel))} does not belong to provider ${JSON.stringify(provider.name)}`);
	}
	if (!configured(chatModel)) {
		throw new CliError(`model ${JSON.stringify(displayModel(chatModel))} is not configured because its provider API key is missing`);
	}
	await client.updateModel(chatModel.id, { defaultModel: true });

	const agents = await listAll((page, pageSize) =>
		client.listAgentsPage(page, pageSize));
	const defaultAgent = agents.find((agent) => agent.isDefault);
	let agentName: string;
	if (defaultAgent) {
		await client.updateAgent(defaultAgent.id, { isDefault: true, modelIds: [chatModel.id] });
		agentName = defaultAgent.name;
	} else {
		const created = await client.createAgent({ name: "default", soul: "", isDefault: true, modelIds: [chatModel.id] });
		agentName = created.name;
	}

	const embeddingRef = flag(args, "embedding-model")?.trim();
	let embeddingModel = "";
	if (embeddingRef) {
		const model = await resolveModel(client, embeddingRef, true);
		if (!configured(model)) {
			throw new CliError(`model ${JSON.stringify(displayModel(model))} is not configured because its provider API key is missing`);
		}
		await client.updateConfig({ embeddingModelId: model.id });
		embeddingModel = displayModel(model);
	}

	const webSearchProvider = flag(args, "web-search-provider")?.trim().toLowerCase();
	if (webSearchProvider) {
		if (!["disabled", "tavily", "brave", "firecrawl"].includes(webSearchProvider)) {
			throw new CliError(`unsupported web search provider: ${webSearchProvider}`);
		}

		const settings = await client.getConfig();
		const keyField = webSearchProvider === "tavily"
			? "tavilyApiKey"
			: webSearchProvider === "brave"
				? "braveApiKey"
				: "firecrawlApiKey";
		if (webSearchProvider !== "disabled" && !webSearchKey && !settings[keyField]) {
			throw new CliError(`web search provider ${webSearchProvider} requires --web-search-api-key or --web-search-api-key-env`);
		}
		await client.updateConfig({
			webSearchProvider,
			...(webSearchKey ? { [keyField]: webSearchKey } : {}),
			...(webSearchProvider === "firecrawl"
				&& flag(args, "firecrawl-base-url")
				? { firecrawlBaseUrl: flag(args, "firecrawl-base-url") }
				: {}),
		});
	}

	await client.setInitialized();
	const result = {
		initialized: true,
		provider: provider.name,
		defaultModel: displayModel(chatModel),
		...(embeddingModel ? { embeddingModel } : {}),
		...(webSearchProvider ? { webSearchProvider } : {}),
		defaultAgent: agentName
	};
	render(args, result, () =>
		outputFields(Object.entries(result).map(([key, value]) => [key, String(value)])));

	if (!hasFlag(args, "quiet") && !hasFlag(args, "json")) {
		process.stdout.write("Initialization completed.\n");
	}
}
