import { AgentyClient } from "../api/client";
import { handleAgent } from "./agent";
import { handleHelp } from "./help";
import { handleInit } from "./init";
import { handleMCP } from "./mcp";
import { handleModel } from "./model";
import { handleProvider } from "./provider";
import { handleSettings } from "./config";
import { handleSkill } from "./skill";
import {
	CliError,
	connect,
	parseArgs,
	type CommandResult,
	type ParsedArgs
} from "./utils";
import { handleVersion } from "./version";

const handlers: Record<string, (client: AgentyClient, args: ParsedArgs) => Promise<void> | void> = {
	"init": handleInit,
	"mcp": handleMCP,
	"skill": handleSkill,
	"agent": handleAgent,
	"provider": handleProvider,
	"model": handleModel,
	"settings": handleSettings,
};

export async function runCLICommand(argv: string[]): Promise<CommandResult> {
	const args = parseArgs(argv);
	try {
		const command = args.positionals[0];
		if (argv[0] === "-h" || args.flags.has("help")) {
			handleHelp(args);
			return { handled: true, exitCode: 0 };
		}
		if (argv[0] === "-v" || args.flags.has("version")) {
			handleVersion(args);
			return { handled: true, exitCode: 0 };
		}
		if (!command) {
			return { handled: false, exitCode: 0 };
		}
		if (command === "help") {
			handleHelp(args);
			return { handled: true, exitCode: 0 };
		}
		if (command === "version") {
			handleVersion(args);
			return { handled: true, exitCode: 0 };
		}

		const handler = handlers[command];
		if (!handler) {
			throw new CliError(`unknown command: ${command}`);
		}

		const { client, stop } = await connect();
		try {
			await handler(client, args);
		} finally {
			await stop?.();
		}
		return { handled: true, exitCode: 0 };
	} catch (error) {
		const err = error as Error;
		process.stderr.write(`${err.message}\n`);
		return { handled: true, exitCode: error instanceof CliError ? error.exitCode : 1 };
	}
}
