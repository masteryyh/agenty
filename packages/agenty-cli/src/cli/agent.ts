import type { AgentyClient } from "@/api/client";
import type { AgentDto, UpdateAgentDto } from "@/api/types";
import {
	action,
	CliError,
	displayModel,
	flag,
	hasFlag,
	listAll,
	outputFields,
	outputTable,
	pageOptions,
	parseBoolean,
	render,
	repeatedFlag,
	requirePositionals,
	resolveModel,
	type ParsedArgs,
} from "./utils";

const subcommandHandlers: Record<
	string,
	(client: AgentyClient, args: ParsedArgs) => Promise<void>
> = {
	list: handleListAgent,
	get: handleGetAgent,
	add: handleAddAgent,
	update: handleUpdateAgent,
	remove: handleRemoveAgent,
};

export async function handleAgent(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const subcommand = args.positionals[1];
	const handler = subcommand ? subcommandHandlers[subcommand] : undefined;
	if (!handler) {
		throw new CliError("usage: agent <list|get|add|update|remove>");
	}
	await handler(client, args);
}

async function handleListAgent(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	requirePositionals(args, 2, "agent list");
	const { page, pageSize } = pageOptions(args);
	const result = await client.listAgentsPage(page, pageSize);

	render(args, result, () =>
		result.data.length === 0
			? process.stdout.write("No agents.\n")
			: outputTable(
					["ID", "Name", "Default", "Models"],
					result.data.map((agent) => [
						agent.id,
						agent.name,
						String(agent.isDefault),
						(agent.models ?? []).map(displayModel).join(", "),
					]),
				),
	);
}

async function handleGetAgent(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , reference] = requirePositionals(
		args,
		3,
		"agent get <name-or-id>",
	);
	const agent = await resolveAgent(client, reference);
	render(args, agent, () =>
		outputFields([
			["ID", agent.id],
			["Name", agent.name],
			["Soul", agent.soul],
			["Default", String(agent.isDefault)],
			["Models", (agent.models ?? []).map(displayModel).join(", ")],
		]),
	);
}

async function handleAddAgent(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , name] = requirePositionals(args, 3, "agent add <name> [options]");
	const modelIds = await resolveModelIds(client, repeatedFlag(args, "model"));
	const created = await client.createAgent({
		name,
		isDefault: hasFlag(args, "default")
			? parseBoolean(flag(args, "default"), "--default")
			: false,
		...(hasFlag(args, "soul") ? { soul: flag(args, "soul") ?? "" } : {}),
		...(modelIds.length > 0 ? { modelIds } : {}),
	});
	action(args, created, `Agent added: ${created.name}`);
}

async function handleUpdateAgent(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , reference] = requirePositionals(
		args,
		3,
		"agent update <name-or-id> [options]",
	);
	if (hasFlag(args, "model") && hasFlag(args, "clear-models")) {
		throw new CliError("--model and --clear-models cannot be used together");
	}

	const current = await resolveAgent(client, reference);
	const dto: UpdateAgentDto = {};
	if (hasFlag(args, "name")) {
		const name = flag(args, "name")?.trim();
		if (!name) {
			throw new CliError("--name cannot be empty");
		}
		dto.name = name;
	}
	if (hasFlag(args, "soul")) {
		dto.soul = flag(args, "soul") ?? "";
	}
	if (hasFlag(args, "default")) {
		dto.isDefault = parseBoolean(flag(args, "default"), "--default");
	}
	if (hasFlag(args, "clear-models")) {
		dto.modelIds = [];
	} else if (hasFlag(args, "model")) {
		dto.modelIds = await resolveModelIds(
			client,
			repeatedFlag(args, "model"),
		);
	}
	if (Object.keys(dto).length === 0) {
		throw new CliError("no changes specified");
	}

	await client.updateAgent(current.id, dto);
	action(
		args,
		{ id: current.id, name: dto.name ?? current.name, updated: true },
		`Agent updated: ${dto.name ?? current.name}`,
	);
}

async function handleRemoveAgent(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const [, , reference] = requirePositionals(
		args,
		3,
		"agent remove <name-or-id> --yes",
	);
	if (!hasFlag(args, "yes")) {
		throw new CliError("use --yes to remove an agent non-interactively");
	}
	const current = await resolveAgent(client, reference);
	await client.deleteAgent(current.id);
	action(
		args,
		{ id: current.id, name: current.name, deleted: true },
		`Agent removed: ${current.name}`,
	);
}

async function resolveAgent(
	client: AgentyClient,
	reference: string,
): Promise<AgentDto> {
	const agents = await listAll((page, pageSize) =>
		client.listAgentsPage(page, pageSize),
	);
	const lower = reference.toLowerCase();
	const matched = agents.filter(
		(agent) => agent.id === reference || agent.name.toLowerCase() === lower,
	);
	if (matched.length === 0) {
		throw new CliError(`agent not found: ${reference}`);
	}
	if (matched.length > 1) {
		throw new CliError(
			`agent name is ambiguous: ${reference}; use agent ID instead`,
		);
	}
	return matched[0];
}

async function resolveModelIds(
	client: AgentyClient,
	references: string[],
): Promise<string[]> {
	const ids: string[] = [];
	for (const raw of references) {
		const reference = raw.trim();
		if (!reference) {
			throw new CliError("--model cannot be empty");
		}
		ids.push((await resolveModel(client, reference, false)).id);
	}
	return [...new Set(ids)];
}
