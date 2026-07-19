import type { AgentyClient } from "@/api/client";
import {
	action,
	CliError,
	flag,
	hasFlag,
	listAll,
	outputFields,
	outputTable,
	pageOptions,
	parseBoolean,
	parsePairs,
	render,
	repeatedFlag,
	requirePositionals,
	type ParsedArgs
} from "./utils";
import type { MCPServerDto } from "@/api/types";

const subcommandHandlers: Record<string, (client: AgentyClient, args: ParsedArgs) => Promise<void>> = {
	"list": handleListMCP,
	"get": handleGetMCP,
	"add": handleAddMCP,
	"update": handleUpdateMCP,
	"remove": handleRemoveMCP,
	"connect": handleConnectMCP,
	"disconnect": handleDisconnectMCP,
}

export async function handleMCP(client: AgentyClient, args: ParsedArgs): Promise<void> {
	const [_, subcommand] = args.positionals;
	if (!subcommand) {
		throw new CliError("usage: mcp <list|get|add|update|remove|connect|disconnect>");
	}

	const handler = subcommandHandlers[subcommand];
	if (!handler) {
		throw new CliError(`unknown MCP subcommand: ${subcommand}`);
	}
	await handler(client, args);
}

async function resolveMCP(client: AgentyClient, reference: string): Promise<MCPServerDto> {
	const servers = await listAll((page, pageSize) =>
		client.listMcpServersPage(page, pageSize));
	const lower = reference.toLowerCase();
	const matched = servers.filter((server) =>
		server.id === reference || server.name.toLowerCase() === lower);

	if (matched.length === 0) {
		throw new CliError(`MCP server not found: ${reference}`);
	}
	if (matched.length > 1) {
		throw new CliError(`MCP server name is ambiguous: ${reference}; use server ID instead`);
	}
	return matched[0];
}

async function handleListMCP(client: AgentyClient, args: ParsedArgs): Promise<void> {
	requirePositionals(args, 2, "mcp list");
	const { page, pageSize } = pageOptions(args);
	const result = await client.listMcpServersPage(page, pageSize);

	const data = result.data.map((server) =>
		[server.id, server.name, server.transport, server.transport === "stdio"
			? [server.command, ...(server.args ?? [])].filter(Boolean).join(" ")
			: server.url ?? "", String(server.enabled), server.status ?? "disconnected"]);
	render(args, result, () =>
		result.data.length === 0
			? process.stdout.write("No MCP servers.\n")
			: outputTable(["ID", "Name", "Transport", "Target", "Enabled", "Status"], data));
}

async function handleGetMCP(client: AgentyClient, args: ParsedArgs): Promise<void> {
	const [, , reference] = requirePositionals(args, 3, "mcp get <name-or-id>");
	const server = await resolveMCP(client, reference);

	render(args, server, () => outputFields([
		["ID", server.id],
		["Name", server.name],
		["Transport", server.transport],
		["Enabled", String(server.enabled)],
		["Status", server.status ?? "disconnected"],
		["Target", server.transport === "stdio"
			? [server.command, ...(server.args ?? [])].filter(Boolean).join(" ")
			: server.url ?? ""],
	]));
}

async function handleAddMCP(client: AgentyClient, args: ParsedArgs): Promise<void> {
	const [, , reference] = requirePositionals(args, 3, "mcp add <name> [options]");
	const stdio = flag(args, "stdio")?.trim();
	const sse = flag(args, "sse")?.trim();
	const http = flag(args, "http")?.trim();
	if ([stdio, sse, http].filter(Boolean).length !== 1) {
		throw new CliError("exactly one of --stdio, --sse, or --http is required");
	}

	const env = parsePairs(repeatedFlag(args, "env"));
	const headers = parsePairs(repeatedFlag(args, "header"));
	const command = stdio
		? {
			transport: "stdio" as const,
			command: stdio,
			args: repeatedFlag(args, "arg"),
			...(env ? { env } : {}),
		}
		: {
			transport: sse ? "sse" as const : "streamable-http" as const,
			url: sse ?? http!,
			...(headers ? { headers } : {}),
		};
	if (stdio && headers) {
		throw new CliError("--header can only be used with --sse or --http");
	}
	if (!stdio && (env || repeatedFlag(args, "arg").length > 0)) {
		throw new CliError("--arg and --env can only be used with --stdio");
	}

	const server = await client.createMcpServer({ name: reference, ...command });
	action(args, server, `MCP server added: ${server.name}`);
}

async function handleUpdateMCP(client: AgentyClient, args: ParsedArgs): Promise<void> {
	const [, , reference] = requirePositionals(args, 3, "mcp update <name-or-id> [options]");

	const dto: Record<string, unknown> = {};
	if (hasFlag(args, "name")) {
		const name = flag(args, "name")?.trim();
		if (!name) {
			throw new CliError("--name cannot be empty");
		}
		dto.name = name;
	}
	if (hasFlag(args, "enabled")) {
		dto.enabled = parseBoolean(flag(args, "enabled"), "--enabled");
	}

	const current = await resolveMCP(client, reference);
	if (current.transport === "stdio") {
		if (hasFlag(args, "url") || hasFlag(args, "header")) {
			throw new CliError("--url and --header can only be used with sse or streamable-http servers");
		}
		if (hasFlag(args, "command")) {
			const command = flag(args, "command")?.trim();
			if (!command) {
				throw new CliError("--command cannot be empty");
			}
			dto.command = command;
		}
		if (hasFlag(args, "arg")) {
			dto.args = repeatedFlag(args, "arg");
		}
		if (hasFlag(args, "env")) {
			dto.env = parsePairs(repeatedFlag(args, "env")) ?? {};
		}
	} else {
		if (hasFlag(args, "command") || hasFlag(args, "arg") || hasFlag(args, "env")) {
			throw new CliError("--command, --arg, and --env can only be used with stdio servers");
		}
		if (hasFlag(args, "url")) {
			const url = flag(args, "url")?.trim();
			if (!url) {
				throw new CliError("--url cannot be empty");
			}
			dto.url = url;
		}
		if (hasFlag(args, "header")) {
			dto.headers = parsePairs(repeatedFlag(args, "header")) ?? {};
		}
	}

	if (Object.keys(dto).length === 0) {
		throw new CliError("no changes specified");
	}

	const updated = await client.updateMcpServer(current.id, dto);
	action(args, updated, `MCP server updated: ${updated.name}`);
}

async function handleRemoveMCP(client: AgentyClient, args: ParsedArgs): Promise<void> {
	const [, , reference] = requirePositionals(args, 3, "mcp remove <name-or-id> --yes");
	if (!hasFlag(args, "yes")) {
		throw new CliError("use --yes to remove an MCP server non-interactively");
	}

	const current = await resolveMCP(client, reference);
	await client.deleteMcpServer(current.id);
	action(args, { id: current.id, name: current.name, deleted: true }, `MCP server removed: ${current.name}`);
}

async function handleConnectMCP(client: AgentyClient, args: ParsedArgs): Promise<void> {
	const [, , reference] = requirePositionals(args, 3, "mcp connect <name-or-id>");
	const current = await resolveMCP(client, reference);
	const connected = await client.connectMcpServer(current.id);
	action(args, connected, `MCP server connected: ${connected.name}`);
}

async function handleDisconnectMCP(client: AgentyClient, args: ParsedArgs): Promise<void> {
	const [, , reference] = requirePositionals(args, 3, "mcp disconnect <name-or-id>");
	const current = await resolveMCP(client, reference);
	const disconnected = await client.disconnectMcpServer(current.id);
	action(args, disconnected, `MCP server disconnected: ${disconnected.name}`);
}
