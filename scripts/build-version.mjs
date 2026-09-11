import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const REPOSITORY_ROOT = resolve(import.meta.dirname, "..");
const VERSION_ENV = "AGENTY_VERSION";

function parseValue(rawValue) {
    const value = rawValue.trim();
    if (value.length < 2) {
        return value;
    }

    const first = value[0];
    const last = value.at(-1);
    if ((first === "\"" || first === "'") && last === first) {
        return value.slice(1, -1).trim();
    }
    return value;
}

function readDotEnvVersion(envPath) {
    let contents;
    try {
        contents = readFileSync(envPath, "utf8");
    } catch (error) {
        if (error?.code === "ENOENT") {
            return undefined;
        }
        throw error;
    }

    for (const rawLine of contents.split(/\r?\n/u)) {
        const line = rawLine.trim();
        if (line === "" || line.startsWith("#")) {
            continue;
        }

        const match = line.match(/^(?:export\s+)?AGENTY_VERSION\s*=\s*(.*)$/u);
        if (match) {
            return parseValue(match[1]);
        }
    }

    return undefined;
}

export function resolveBuildVersion(
    environment = process.env,
    repositoryRoot = REPOSITORY_ROOT,
) {
    const configured = environment[VERSION_ENV]?.trim();
    if (configured) {
        return configured;
    }

    const fromDotEnv = readDotEnvVersion(resolve(repositoryRoot, ".env"));
    return fromDotEnv || "dev";
}
