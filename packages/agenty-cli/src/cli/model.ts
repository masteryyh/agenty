import type { AgentyClient } from "@/api/client";
import type { CreateModelDto, ModelDto, UpdateModelDto } from "@/api/types";
import {
	action,
	CliError,
	configured,
	displayModel,
	flag,
	hasFlag,
	outputFields,
	outputTable,
	pageOptions,
	parseBoolean,
	render,
	repeatedFlag,
	requireFlag,
	requirePositionals,
	resolveModel,
	resolveProvider,
	type ParsedArgs,
} from "./utils";

const subcommandHandlers: Record<
	string,
	(client: AgentyClient, args: ParsedArgs) => Promise<void>
> = {
	list: handleListModel,
	get: handleGetModel,
	add: handleAddModel,
	update: handleUpdateModel,
	remove: handleRemoveModel,
};

interface ModelCapabilityUpdate {
	embeddingModel?: boolean;
	contextCompressionModel?: boolean;
	multiModal?: boolean;
	light?: boolean;
	thinking?: boolean;
	anthropicAdaptiveThinking?: boolean;
}

export async function handleModel(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const subcommand = args.positionals[1];
	const handler = subcommand ? subcommandHandlers[subcommand] : undefined;
	if (!handler) {
		throw new CliError("usage: model <list|get|add|update|remove>");
	}
	await handler(client, args);
}

async function handleListModel(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	requirePositionals(args, 2, "model list");
	if (hasFlag(args, "chat-only") && hasFlag(args, "embedding-only")) {
		throw new CliError(
			"--chat-only and --embedding-only cannot be used together",
		);
	}
	const { page, pageSize } = pageOptions(args);
	const pageResult = await client.listModelsPage(page, pageSize);
	const data = pageResult.data
		.filter((model) => !hasFlag(args, "chat-only") || !model.embeddingModel)
		.filter(
			(model) => !hasFlag(args, "embedding-only") || model.embeddingModel,
		);
	const result = { ...pageResult, total: data.length, data };

	render(args, result, () =>
		data.length === 0
			? process.stdout.write("No models.\n")
			: outputTable(
					["ID", "Model", "Code", "Kind", "Default", "Configured"],
					data.map((model) => [
						model.id,
						displayModel(model),
						model.code,
						model.embeddingModel ? "embedding" : "chat",
						String(model.defaultModel),
						String(configured(model)),
					]),
				),
	);
}

async function handleGetModel(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , reference] = requirePositionals(
		args,
		3,
		"model get <name-code-or-id>",
	);
	const model = await resolveModel(client, reference);
	render(args, model, () => renderModel(model));
}

async function handleAddModel(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , code] = requirePositionals(
		args,
		3,
		"model add <code> --provider <ref> --name <name>",
	);
	const provider = await resolveProvider(client, requireFlag(args, "provider"));
	const dto: CreateModelDto = {
		providerId: provider.id,
		name: requireFlag(args, "name"),
		code,
	};
	applyCreateBooleanFlags(dto, args);
	if (hasFlag(args, "thinking-level")) {
		dto.thinkingLevels = parseThinkingLevels(args);
	}
	const created = await client.createModel(dto);
	action(args, created, `Model added: ${displayModel(created)}`);
}

async function handleUpdateModel(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , reference] = requirePositionals(
		args,
		3,
		"model update <name-code-or-id> [options]",
	);
	if (hasFlag(args, "thinking-level") && hasFlag(args, "clear-thinking-levels")) {
		throw new CliError(
			"--thinking-level and --clear-thinking-levels cannot be used together",
		);
	}
	const current = await resolveModel(client, reference);
	const dto: UpdateModelDto = {};
	if (hasFlag(args, "name")) {
		dto.name = requireFlag(args, "name");
	}
	applyUpdateBooleanFlags(dto, args);
	if (hasFlag(args, "clear-thinking-levels")) {
		dto.thinkingLevels = [];
	} else if (hasFlag(args, "thinking-level")) {
		dto.thinkingLevels = parseThinkingLevels(args);
	}
	if (Object.keys(dto).length === 0) {
		throw new CliError("no changes specified");
	}

	await client.updateModel(current.id, dto);
	action(
		args,
		{ id: current.id, name: dto.name ?? current.name, updated: true },
		`Model updated: ${dto.name ?? displayModel(current)}`,
	);
}

async function handleRemoveModel(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , reference] = requirePositionals(
		args,
		3,
		"model remove <name-code-or-id> --yes",
	);
	if (!hasFlag(args, "yes")) {
		throw new CliError("use --yes to remove a model non-interactively");
	}
	const current = await resolveModel(client, reference);
	await client.deleteModel(current.id);
	action(
		args,
		{ id: current.id, name: current.name, deleted: true },
		`Model removed: ${displayModel(current)}`,
	);
}

function renderModel(model: ModelDto): void {
	outputFields([
		["ID", model.id],
		["Model", displayModel(model)],
		["Code", model.code],
		["Default", String(model.defaultModel)],
		["Embedding", String(model.embeddingModel)],
		["Context compression", String(model.contextCompressionModel)],
		["Multimodal", String(model.multiModal)],
		["Light", String(model.light)],
		["Thinking", String(model.thinking)],
		["Thinking levels", model.thinkingLevels.join(", ")],
		["Anthropic adaptive thinking", String(model.anthropicAdaptiveThinking)],
		["Preset", String(model.isPreset)],
		["Context window", String(model.contextWindow)],
	]);
}

function applyCreateBooleanFlags(
	dto: ModelCapabilityUpdate,
	args: ParsedArgs,
): void {
	if (hasFlag(args, "embedding")) {
		dto.embeddingModel = parseBoolean(flag(args, "embedding"), "--embedding");
	}
	if (hasFlag(args, "context-compression")) {
		dto.contextCompressionModel = parseBoolean(
			flag(args, "context-compression"),
			"--context-compression",
		);
	}
	if (hasFlag(args, "multi-modal")) {
		dto.multiModal = parseBoolean(flag(args, "multi-modal"), "--multi-modal");
	}
	if (hasFlag(args, "light")) {
		dto.light = parseBoolean(flag(args, "light"), "--light");
	}
	if (hasFlag(args, "thinking")) {
		dto.thinking = parseBoolean(flag(args, "thinking"), "--thinking");
	}
	if (hasFlag(args, "anthropic-adaptive-thinking")) {
		dto.anthropicAdaptiveThinking = parseBoolean(
			flag(args, "anthropic-adaptive-thinking"),
			"--anthropic-adaptive-thinking",
		);
	}
}

function applyUpdateBooleanFlags(dto: UpdateModelDto, args: ParsedArgs): void {
	if (hasFlag(args, "default")) {
		dto.defaultModel = parseBoolean(flag(args, "default"), "--default");
	}
	applyCreateBooleanFlags(dto, args);
}

function parseThinkingLevels(args: ParsedArgs): string[] {
	const levels = repeatedFlag(args, "thinking-level")
		.map((level) => level.trim())
		.filter(Boolean);
	if (levels.length === 0) {
		throw new CliError("--thinking-level cannot be empty");
	}
	return [...new Set(levels)];
}
