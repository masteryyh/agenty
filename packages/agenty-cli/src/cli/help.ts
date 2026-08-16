import type { ParsedArgs } from "./utils";

const HELP = `Usage: agenty-cli [core options] <command> <subcommand> [options]

Commands:
  init                         Initialize one provider, model, and default agent
  agent list|get|add|update|remove
                               Manage agents
  provider list|get|add|update|remove
                               Manage model providers
  model list|get|add|update|remove
                               Manage models

Core options:
  --data-dir <path>            Override the core data directory
  AGENTY_CORE_BIN=<path>       Override the core executable
  --json                       Emit machine-readable JSON
  --quiet                      Suppress action confirmation output

Destructive remove commands require --yes.`;

export function handleHelp(_: ParsedArgs): void {
    process.stdout.write(`${HELP}\n`);
}
