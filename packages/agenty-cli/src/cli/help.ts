import type { ParsedArgs } from "./utils";

const HELP = `Usage: agenty-cli [core options] <command> <subcommand> [options]

Commands:
  init                         Initialize one provider, model, and default session model
  provider list|get|add|update|remove
                               Manage model providers
  model list|get|add|update|remove
                               Manage models
  local status|enable|disable|configure|install|installations|cancel-installation
                               Manage local inference and worker installation

Core options:
  --data-dir <path>             Override the core data directory
  --llama-cpp-worker <path>     Use a local llama.cpp worker executable
  --mlx-worker <path>           Use a local MLX worker executable
  --json                       Emit machine-readable JSON
  --quiet                      Suppress action confirmation output

Destructive remove commands require --yes.`;

export function handleHelp(_: ParsedArgs): void {
    process.stdout.write(`${HELP}\n`);
}
