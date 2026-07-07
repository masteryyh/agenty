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

import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { parse } from "yaml";

export type ThinkingFlag = "off" | "on" | string;

export interface CliOptions {
	serverURL: string;
	agentRef?: string;
	modelRef?: string;
	username?: string;
	password?: string;
	thinking: ThinkingFlag;
	configPath?: string;
	newSession: boolean;
}

const DEFAULT_SERVER_URL = "http://localhost:8080";

interface ClientYaml {
	server?: {
		url?: string;
	};
	auth?: {
		username?: string;
		password?: string;
	};
}

function parseArgs(argv: string[]): Record<string, string | boolean> {
	const flags: Record<string, string | boolean> = {};
	for (let i = 0; i < argv.length; i++) {
		const arg = argv[i];
		if (!arg.startsWith("--")) continue;
		const key = arg.slice(2);
		const next = argv[i + 1];
		if (next !== undefined && !next.startsWith("--")) {
			flags[key] = next;
			i++;
		} else {
			flags[key] = true;
		}
	}
	return flags;
}

function loadYaml(path: string): ClientYaml | null {
	const abs = resolve(path);
	if (!existsSync(abs)) return null;
	try {
		const text = readFileSync(abs, "utf8");
		return parse(text) as ClientYaml;
	} catch {
		return null;
	}
}

function findDefaultConfig(): string | null {
	let dir = process.cwd();
	for (let i = 0; i < 6; i++) {
		const candidate = resolve(dir, "agenty-client.yaml");
		if (existsSync(candidate)) return candidate;
		const parent = resolve(dir, "..");
		if (parent === dir) break;
		dir = parent;
	}
	return null;
}

export function loadOptions(): CliOptions {
	const flags = parseArgs(process.argv.slice(2));

	const configPath =
		typeof flags.config === "string" ? flags.config : findDefaultConfig();
	const yaml: ClientYaml = configPath ? (loadYaml(configPath) ?? {}) : {};

	const serverURL =
		(typeof flags.server === "string" && flags.server) ||
		process.env.AGENTY_SERVER_URL ||
		yaml.server?.url ||
		DEFAULT_SERVER_URL;

	const username =
		(typeof flags.user === "string" && flags.user) ||
		process.env.AGENTY_USER ||
		yaml.auth?.username ||
		undefined;

	const password =
		(typeof flags.password === "string" && flags.password) ||
		process.env.AGENTY_PASSWORD ||
		yaml.auth?.password ||
		undefined;

	const thinking =
		(typeof flags.thinking === "string" && flags.thinking) || "off";

	return {
		serverURL: serverURL.replace(/\/+$/, ""),
		agentRef: typeof flags.agent === "string" ? flags.agent : undefined,
		modelRef: typeof flags.model === "string" ? flags.model : undefined,
		username,
		password,
		thinking,
		configPath: configPath ?? undefined,
		newSession: flags["new-session"] === true,
	};
}

export function parseThinking(flag: ThinkingFlag): {
	thinking: boolean;
	thinkingLevel: string;
} {
	const value = flag.trim().toLowerCase();
	if (value === "" || value === "off" || value === "false") {
		return { thinking: false, thinkingLevel: "" };
	}
	if (value === "on" || value === "true") {
		return { thinking: true, thinkingLevel: "" };
	}
	return { thinking: true, thinkingLevel: flag.trim() };
}
