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

import type { AgentyClient } from "../api/client";

export interface Command {
	name: string;
	description: string;
	usage: string;
	argHint?: string;
	completeArgs?: (client: AgentyClient) => Promise<string[]>;
}

export const commands: Command[] = [
	{
		name: "/help",
		description: "Show available commands",
		usage: "/help",
	},
	{
		name: "/model",
		description: "Switch the chat model",
		usage: "/model [provider/model]",
		argHint: "provider/model",
		completeArgs: async (client) => {
			const models = await client.listModels();
			return models
				.filter((m) => !m.embeddingModel)
				.map((m) => `${m.provider?.name ?? "?"}/${m.name}`);
		},
	},
	{
		name: "/new",
		description: "Start a new empty session",
		usage: "/new",
	},
	{
		name: "/provider",
		description: "Manage model providers",
		usage: "/provider",
	},
	{
		name: "/agents",
		description: "Manage agents and switch current agent",
		usage: "/agents [<name>]",
		argHint: "name",
		completeArgs: async (client) => {
			const agents = await client.listAgents();
			return agents.map((a) => a.name);
		},
	},
	{
		name: "/config",
		description: "View and edit system config",
		usage: "/config",
	},
	{
		name: "/resume",
		description: "Resume a previous session",
		usage: "/resume",
	},
	{
		name: "/exit",
		description: "Quit agenty-cli",
		usage: "/exit",
	},
	{
		name: "/mcp",
		description: "Manage MCP servers",
		usage: "/mcp",
	},
	{
		name: "/think",
		description: "Set thinking mode (off/on/low/medium/high/xhigh)",
		usage: "/think [off|on|low|medium|high|xhigh]",
	},
	{
		name: "/compact",
		description: "Compact the current conversation context",
		usage: "/compact",
	},
	{
		name: "/skill",
		description: "Browse available skills",
		usage: "/skill",
	},
	{
		name: "/status",
		description: "Show current session status",
		usage: "/status",
	},
	{
		name: "/cwd",
		description: "Set or show the session working directory",
		usage: "/cwd [<path>|clear]",
	},
];

export function findCommand(name: string): Command | undefined {
	const lower = name.toLowerCase();
	return commands.find((c) => c.name === lower);
}

export function parseCommandTokens(input: string): string[] {
	const parts: string[] = [];
	let current = "";
	let inSingle = false;
	let inDouble = false;
	for (const ch of input) {
		if (ch === "'" && !inDouble) {
			inSingle = !inSingle;
			continue;
		}
		if (ch === '"' && !inSingle) {
			inDouble = !inDouble;
			continue;
		}
		if ((ch === " " || ch === "\t") && !inSingle && !inDouble) {
			if (current.length > 0) {
				parts.push(current);
				current = "";
			}
			continue;
		}
		current += ch;
	}
	if (current.length > 0) parts.push(current);
	return parts;
}

export function quoteArg(arg: string): string {
	return arg.includes(" ") ? `"${arg}"` : arg;
}

export function matchingCommands(input: string): Command[] {
	const trimmed = input.trim().toLowerCase();
	if (trimmed === "" || !trimmed.startsWith("/")) return [];
	return commands.filter((c) => c.name.toLowerCase().startsWith(trimmed));
}

export function longestCommonPrefix(names: string[]): string {
	if (names.length === 0) return "";
	if (names.length === 1) return names[0];
	let prefix = names[0];
	for (const name of names.slice(1)) {
		let i = 0;
		while (i < prefix.length && i < name.length && prefix[i] === name[i]) {
			i += 1;
		}
		prefix = prefix.slice(0, i);
		if (prefix === "") break;
	}
	return prefix;
}
