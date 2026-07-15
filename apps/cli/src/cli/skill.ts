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
