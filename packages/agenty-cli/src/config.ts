import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { parse } from "yaml";

export type ThinkingFlag = "off" | "on" | string;

export interface CliOptions {
	serverURL: string;
	localMode: boolean;
	agentRef?: string;
	modelRef?: string;
	username?: string;
	password?: string;
	thinking: ThinkingFlag;
	databasePath?: string;
	backendDebug: boolean;
	newSession: boolean;
}

// Used only as a remote fallback hint in error messages; local mode resolves
// the URL dynamically via startLocalServer().
const DEFAULT_SERVER_URL = "http://localhost:8081";

interface ClientYaml {
	server?: {
		url?: string;
	};
	auth?: {
		username?: string;
		password?: string;
	};
}

const BOOLEAN_FLAGS = new Set(["debug", "new-session"]);

function parseArgs(argv: string[]): Record<string, string | boolean> {
	const flags: Record<string, string | boolean> = {};
	for (let i = 0; i < argv.length; i++) {
		const arg = argv[i];
		if (!arg.startsWith("--")) continue;
		const key = arg.slice(2);
		const next = argv[i + 1];
		if (!BOOLEAN_FLAGS.has(key) && next !== undefined && !next.startsWith("--")) {
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

	const clientConfigPath =
		typeof flags["client-config"] === "string"
			? flags["client-config"]
			: findDefaultConfig();
	const yaml: ClientYaml = clientConfigPath ? (loadYaml(clientConfigPath) ?? {}) : {};
	// Local mode (default) spins up an embedded `agenty` server subprocess.
	// Remote mode is opted into by providing any explicit server URL.
	const explicitServer =
		(typeof flags.server === "string" && flags.server) ||
		process.env.AGENTY_SERVER_URL ||
		yaml.server?.url;
	const localMode = !explicitServer;
	const serverURL = explicitServer ? explicitServer.replace(/\/+$/, "") : "";

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
		serverURL,
		localMode,
		agentRef: typeof flags.agent === "string" ? flags.agent : undefined,
		modelRef: typeof flags.model === "string" ? flags.model : undefined,
		username,
		password,
		thinking,
		databasePath: typeof flags.db === "string" ? flags.db : undefined,
		backendDebug: flags.debug === true,
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

export { DEFAULT_SERVER_URL };
