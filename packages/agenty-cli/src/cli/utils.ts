import { AgentyClient } from "@/api/client";
import { loadOptions } from "@/config";
import { startLocalServer } from "@/localServer";
import type { ModelDto, ModelProviderDto } from "@/api/types";

export interface CommandResult {
    handled: boolean
    exitCode: number
}

export interface LocalServerConnectResult {
    client: AgentyClient
    stop?: () => Promise<void>
}

type FlagValue = string | boolean | string[];

export interface ParsedArgs {
	positionals: string[];
	flags: Map<string, FlagValue>;
}

export class CliError extends Error {
	constructor(message: string, readonly exitCode = 2) {
		super(message);
	}
}

export interface PageOptions {
    page: number
    pageSize: number
}

const BOOLEAN_FLAGS = new Set([
	"json",
	"quiet",
	"no-color",
	"yes",
	"chat-only",
	"embedding-only",
	"clear-models",
	"clear-thinking-levels",
	"debug",
	"help",
	"version",
]);

export async function connect(): Promise<LocalServerConnectResult> {
    const options = loadOptions();
    if (!options.localMode) {
        return {
            client: new AgentyClient(
                options.serverURL,
                options.username,
                options.password
            )
        }
    };

	const local = await startLocalServer({
		databasePath: options.databasePath,
		debug: options.backendDebug,
	});
    return {
        client: new AgentyClient(local.url),
        stop: local.stop
    };
}

export function requirePositionals(args: ParsedArgs, count: number, usage: string): string[] {
	if (args.positionals.length !== count) {
        throw new CliError(`usage: ${usage}`);
    }
	return args.positionals;
}

export function flag(args: ParsedArgs, name: string): string | undefined {
	const value = args.flags.get(name);
	if (Array.isArray(value)) {
        return value.at(-1);
    }
	return typeof value === "string" ? value : undefined;
}

export function requireFlag(args: ParsedArgs, name: string): string {
	const value = flag(args, name)?.trim();
	if (!value) {
		throw new CliError(`--${name} is required`);
	}
	return value;
}

export function secret(args: ParsedArgs, direct: string, env: string, label: string): string | undefined {
	const value = flag(args, direct)?.trim();
	const envName = flag(args, env)?.trim();

	if (value && envName) {
        throw new CliError(`${label} cannot use both direct value and env var`);
    }
	if (value) {
        return value;
    }
	if (!envName) {
        return undefined;
    }

	const resolved = process.env[envName]?.trim();
	if (!resolved) {
        throw new CliError(`environment variable ${envName} is empty or not set`);
    }
	return resolved;
}

export async function listAll<T>(fetch: (page: number, pageSize: number) => Promise<{ data: T[]; total: number }>): Promise<T[]> {
	const all: T[] = [];
	for (let page = 1;; page += 1) {
		const result = await fetch(page, 100);
		all.push(...result.data);

		if (result.data.length === 0 || all.length >= result.total) {
            return all;
        }
	}
}

export async function resolveProvider(client: AgentyClient, reference: string): Promise<ModelProviderDto> {
    const providers = await listAll((page, pageSize) =>
		client.listProvidersPage(page, pageSize));
    const lower = reference.toLowerCase();

    const matched = providers.filter((provider) =>
		provider.id === reference || provider.name.toLowerCase() === lower);
    if (matched.length === 0) {
		throw new CliError(`provider not found: ${reference}`);
	}
    if (matched.length > 1) {
		throw new CliError(`provider name is ambiguous: ${reference}; use provider ID instead`);
	}
    return matched[0];
}

export function displayModel(model: ModelDto): string {
	return `${model.provider?.name ?? "?"}/${model.name}`;
}

export async function resolveModel(client: AgentyClient, reference: string, embedding?: boolean): Promise<ModelDto> {
	const models = (await listAll((page, pageSize) =>
		client.listModelsPage(page, pageSize))).filter((model) =>
			embedding === undefined || model.embeddingModel === embedding);
    const lower = reference.toLowerCase();

    const matched = models.filter((model) =>
        model.id === reference ||
        model.code.toLowerCase() === lower ||
        model.name.toLowerCase() === lower ||
        displayModel(model).toLowerCase() === lower,
    );
    if (matched.length === 0) {
        throw new CliError(`model not found: ${reference}`);
    }
    if (matched.length > 1) {
        throw new CliError(`model reference is ambiguous: ${reference}; use provider/name or model ID instead`);
    }
    return matched[0];
}

export function configured(model: ModelDto): boolean {
	return model.provider?.apiKeyCensored !== "<not set>";
}

export function hasFlag(args: ParsedArgs, name: string): boolean {
	return args.flags.has(name);
}

export function outputJSON(value: unknown): void {
	process.stdout.write(`${JSON.stringify(value, null, 2)}\n`);
}

export function render(args: ParsedArgs, value: unknown, text: () => void): void {
	if (hasFlag(args, "json")) {
        outputJSON(value);
    } else {
        text();
    }
}

export function outputFields(rows: Array<[string, string]>): void {
	const width = Math.max(...rows.map(([key]) => key.length));
	for (const [key, value] of rows) {
        process.stdout.write(`${key.padEnd(width)}  ${value}\n`);
    }
}

export function pageOptions(args: ParsedArgs): PageOptions {
	const page = Number(flag(args, "page") ?? "1");
	const pageSize = Number(flag(args, "page-size") ?? "50");

	if (!Number.isInteger(page) || page < 1 || !Number.isInteger(pageSize) || pageSize < 1 || pageSize > 100) {
		throw new CliError("--page must be >= 1 and --page-size must be between 1 and 100");
	}
	return { page, pageSize };
}

export function parsePairs(values: string[]): Record<string, string> | undefined {
	if (values.length === 0) {
        return undefined;
    }

	const result: Record<string, string> = {};
	for (const value of values) {
		const equal = value.indexOf("=");
		if (equal <= 0) {
            throw new CliError(`invalid key/value pair ${JSON.stringify(value)}, expected KEY=VALUE`);
        }
		result[value.slice(0, equal).trim()] = value.slice(equal + 1);
	}
	return result;
}

export function parseBoolean(value: string | undefined, name: string): boolean {
	if (value === "true") {
        return true;
    }
	if (value === "false") {
        return false;
    }

	throw new CliError(`${name} must be true or false`);
}

export function parseArgs(argv: string[]): ParsedArgs {
	const positionals: string[] = [];
	const flags = new Map<string, FlagValue>();
	for (let index = 0; index < argv.length; index += 1) {
		const raw = argv[index];
		if (!raw.startsWith("--")) {
			positionals.push(raw);
			continue;
		}

		const [key, inline] = raw.slice(2).split("=", 2);
		let value: FlagValue = true;
		if (inline !== undefined) {
			value = inline;
		} else if (!BOOLEAN_FLAGS.has(key) && argv[index + 1] !== undefined && !argv[index + 1].startsWith("--")) {
			value = argv[index + 1];
			index += 1;
		}

		const previous = flags.get(key);
		if (previous === undefined) {
			flags.set(key, value);
		}
		else {
			flags.set(key, [...(Array.isArray(previous) ?
				previous : [String(previous)]), String(value)]);
		}
	}
	return { positionals, flags };
}

export function outputTable(headers: string[], rows: string[][]): void {
	const clean = (value: string) =>
        value.replaceAll(/[\r\n\t]/g, " ");
	const widths = headers.map((header, index) =>
        Math.max(header.length, ...rows.map((row) => clean(row[index] ?? "").length)));

	const write = (row: string[]) =>
        process.stdout.write(`${row.map((value, index) =>
            clean(value ?? "").padEnd(widths[index])).join("  ").trimEnd()}\n`);

	write(headers);
	for (const row of rows) {
        write(row);
    }
}

export function repeatedFlag(args: ParsedArgs, name: string): string[] {
	const value = args.flags.get(name);
	if (Array.isArray(value)) {
        return value;
    }
	return typeof value === "string" ? [value] : [];
}

export function action(args: ParsedArgs, value: unknown, message: string): void {
	if (hasFlag(args, "json")) {
        outputJSON(value);
    } else if (!hasFlag(args, "quiet")) {
        process.stdout.write(`${message}\n`);
    }
}
