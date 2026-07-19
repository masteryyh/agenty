import type { AgentyClient } from "@/api/client";
import type {
	SystemConfigDto,
	UpdateSystemConfigDto,
} from "@/api/types";
import {
	action,
	CliError,
	flag,
	hasFlag,
	outputFields,
	parseBoolean,
	render,
	requirePositionals,
	resolveModel,
	secret,
	type ParsedArgs,
} from "./utils";

const WEB_SEARCH_PROVIDERS = new Set([
	"disabled",
	"tavily",
	"brave",
	"firecrawl",
]);

export async function handleSettings(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const subcommand = args.positionals[1];
	if (subcommand === "get") {
		requirePositionals(args, 2, "settings get");
		const settings = await client.getConfig();
		render(args, settings, () => renderSettings(settings));
		return;
	}
	if (subcommand === "update") {
		await handleUpdateSettings(client, args);
		return;
	}
	throw new CliError("usage: settings <get|update>");
}

async function handleUpdateSettings(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	requirePositionals(args, 2, "settings update [options]");
	const dto: UpdateSystemConfigDto = {};

	if (hasFlag(args, "initialized")) {
		dto.initialized = parseBoolean(flag(args, "initialized"), "--initialized");
	}
	if (hasFlag(args, "embedding-model")) {
		const reference = flag(args, "embedding-model")?.trim();
		if (!reference) {
			throw new CliError("--embedding-model cannot be empty");
		}
		dto.embeddingModelId = (await resolveModel(client, reference, true)).id;
	}
	if (hasFlag(args, "context-compression-model")) {
		const reference = flag(args, "context-compression-model")?.trim();
		if (!reference) {
			throw new CliError("--context-compression-model cannot be empty");
		}
		const model = await resolveModel(client, reference, false);
		if (!model.contextCompressionModel) {
			throw new CliError(
				`model ${JSON.stringify(reference)} is not a context compression model`,
			);
		}
		dto.contextCompressionModelId = model.id;
	}
	if (hasFlag(args, "web-search-provider")) {
		const provider = flag(args, "web-search-provider")?.trim().toLowerCase();
		if (!provider || !WEB_SEARCH_PROVIDERS.has(provider)) {
			throw new CliError(`unsupported web search provider: ${provider ?? ""}`);
		}
		dto.webSearchProvider = provider;
	}

	for (const [direct, env, field, label] of [
		["brave-api-key", "brave-api-key-env", "braveApiKey", "Brave API key"],
		[
			"tavily-api-key",
			"tavily-api-key-env",
			"tavilyApiKey",
			"Tavily API key",
		],
		[
			"firecrawl-api-key",
			"firecrawl-api-key-env",
			"firecrawlApiKey",
			"Firecrawl API key",
		],
	] as const) {
		const value = secret(args, direct, env, label);
		if (value) {
			dto[field] = value;
		}
	}
	if (hasFlag(args, "firecrawl-base-url")) {
		dto.firecrawlBaseUrl = flag(args, "firecrawl-base-url") ?? "";
	}
	if (Object.keys(dto).length === 0) {
		throw new CliError("no changes specified");
	}

	const updated = await client.updateConfig(dto);
	action(args, updated, "System settings updated.");
}

function renderSettings(settings: SystemConfigDto): void {
	outputFields([
		["Initialized", String(settings.initialized)],
		["Embedding model ID", settings.embeddingModelId ?? ""],
		[
			"Context compression model ID",
			settings.contextCompressionModelId ?? "",
		],
		["Web search provider", settings.webSearchProvider],
		[
			"Configured web search providers",
			(settings.configuredWebSearchProviders ?? []).join(", "),
		],
		[
			"Last configured web search provider",
			settings.lastConfiguredWebSearchProvider ?? "",
		],
		["Brave API key", settings.braveApiKey ?? ""],
		["Tavily API key", settings.tavilyApiKey ?? ""],
		["Firecrawl API key", settings.firecrawlApiKey ?? ""],
		["Firecrawl base URL", settings.firecrawlBaseUrl ?? ""],
	]);
}
