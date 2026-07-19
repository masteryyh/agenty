import type { AgentyClient } from "@/api/client";
import {
	action,
	CliError,
	flag,
	outputTable,
	render,
	requirePositionals,
	type ParsedArgs,
} from "./utils";

export async function handleSkill(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	const subcommand = args.positionals[1];
	if (subcommand === "list") {
		await handleListSkill(client, args);
		return;
	}
	if (subcommand === "rescan") {
		requirePositionals(args, 2, "skill rescan");
		await client.rescanGlobalSkills();
		action(args, { rescanned: true }, "Global skills rescanned.");
		return;
	}
	throw new CliError("usage: skill <list|rescan>");
}

async function handleListSkill(
	client: AgentyClient,
	args: ParsedArgs,
): Promise<void> {
	requirePositionals(args, 2, "skill list [--session-id <id>]");
	const sessionId = flag(args, "session-id")?.trim();
	const skills = await client.listSkills(sessionId || undefined);
	render(args, skills, () =>
		skills.length === 0
			? process.stdout.write("No skills.\n")
			: outputTable(
					["ID", "Name", "Scope", "Description", "Path"],
					skills.map((skill) => [
						skill.id,
						skill.name,
						skill.scope ?? "global",
						skill.description,
						skill.skillMdPath,
					]),
				),
	);
}
