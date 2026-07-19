import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";

function readVersionFromEnvFile(): string | null {
	const envPath = resolve(import.meta.dir, "../.env");
	if (!existsSync(envPath)) return null;
	try {
		const text = readFileSync(envPath, "utf8");
		const match = text.match(/^AGENTY_CLI_VERSION\s*=\s*(.+?)\s*$/m);
		if (!match) return null;
		return match[1].replace(/^["']|["']$/g, "");
	} catch {
		return null;
	}
}

function resolveCliVersion(): string {
	const fromEnv = process.env.AGENTY_CLI_VERSION;
	if (fromEnv && fromEnv !== "undefined") return fromEnv;
	return readVersionFromEnvFile() ?? "dev";
}

export const CLI_VERSION = resolveCliVersion();
