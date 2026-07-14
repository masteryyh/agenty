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
import type { UpdateModelProviderDto } from "@/api/types";
import {
	action,
	CliError,
	flag,
	hasFlag,
	outputFields,
	outputTable,
	pageOptions,
	render,
	requireFlag,
	requirePositionals,
	resolveProvider,
	secret,
	type ParsedArgs,
} from "./utils";

const subcommandHandlers: Record<
	string,
	(client: AgentyClient, args: ParsedArgs) => Promise<void>
> = {
	list: handleListProvider,
	get: handleGetProvider,
	add: handleAddProvider,
	update: handleUpdateProvider,
	remove: handleRemoveProvider,
};

export async function handleProvider(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const subcommand = args.positionals[1];
	const handler = subcommand ? subcommandHandlers[subcommand] : undefined;
	if (!handler) {
		throw new CliError("usage: provider <list|get|add|update|remove>");
	}
	await handler(client, args);
}

async function handleListProvider(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	requirePositionals(args, 2, "provider list");
	const { page, pageSize } = pageOptions(args);
	const result = await client.listProvidersPage(page, pageSize);

	render(args, result, () =>
		result.data.length === 0
			? process.stdout.write("No providers.\n")
			: outputTable(
					["ID", "Name", "Type", "Base URL", "API Key"],
					result.data.map((provider) => [
						provider.id,
						provider.name,
						provider.type,
						provider.baseUrl,
						provider.apiKeyCensored,
					]),
				),
	);
}

async function handleGetProvider(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , reference] = requirePositionals(
		args,
		3,
		"provider get <name-or-id>",
	);
	const provider = await resolveProvider(client, reference);
	render(args, provider, () =>
		outputFields([
			["ID", provider.id],
			["Name", provider.name],
			["Type", provider.type],
			["Base URL", provider.baseUrl],
			[
				"Bailian multimodal embedding base URL",
				provider.bailianMultiModalEmbeddingBaseUrl ?? "",
			],
			["API Key", provider.apiKeyCensored],
			["Preset", String(provider.isPreset)],
		]),
	);
}

async function handleAddProvider(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , name] = requirePositionals(
		args,
		3,
		"provider add <name> --type <type> --base-url <url> --api-key <key>",
	);
	const apiKey = secret(args, "api-key", "api-key-env", "provider API key");
	if (!apiKey) {
		throw new CliError("--api-key or --api-key-env is required");
	}
	const created = await client.createProvider({
		name,
		type: requireFlag(args, "type"),
		baseUrl: requireFlag(args, "base-url"),
		apiKey,
		...(hasFlag(args, "bailian-multimodal-embedding-base-url")
			? {
					bailianMultiModalEmbeddingBaseUrl: requireFlag(
						args,
						"bailian-multimodal-embedding-base-url",
					),
				}
			: {}),
	});
	action(args, created, `Provider added: ${created.name}`);
}

async function handleUpdateProvider(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , reference] = requirePositionals(
		args,
		3,
		"provider update <name-or-id> [options]",
	);
	const current = await resolveProvider(client, reference);
	const dto: UpdateModelProviderDto = {};
	for (const [flagName, field] of [
		["name", "name"],
		["type", "type"],
		["base-url", "baseUrl"],
		[
			"bailian-multimodal-embedding-base-url",
			"bailianMultiModalEmbeddingBaseUrl",
		],
	] as const) {
		if (hasFlag(args, flagName)) {
			dto[field] = requireFlag(args, flagName);
		}
	}
	const apiKey = secret(args, "api-key", "api-key-env", "provider API key");
	if (apiKey) {
		dto.apiKey = apiKey;
	}
	if (Object.keys(dto).length === 0) {
		throw new CliError("no changes specified");
	}

	const updated = await client.updateProvider(current.id, dto);
	action(args, updated, `Provider updated: ${updated.name}`);
}

async function handleRemoveProvider(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , reference] = requirePositionals(
		args,
		3,
		"provider remove <name-or-id> --yes",
	);
	if (!hasFlag(args, "yes")) {
		throw new CliError("use --yes to remove a provider non-interactively");
	}
	const current = await resolveProvider(client, reference);
	await client.deleteProvider(current.id);
	action(
		args,
		{ id: current.id, name: current.name, deleted: true },
		`Provider removed: ${current.name}`,
	);
}
