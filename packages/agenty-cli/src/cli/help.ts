import type { ParsedArgs } from "./utils";

const HELP = `Usage: agenty-cli [connection options] <command> <subcommand> [options]

Commands:
  init                         Initialize provider, models, web search, and default agent
  agent list|get|add|update|remove
                               Manage agents
  provider list|get|add|update|remove
                               Manage model providers
  model list|get|add|update|remove
                               Manage models
  settings get|update          Show or update system settings
  mcp list|get|add|update|remove|connect|disconnect
                               Manage MCP servers
  skill list|rescan            List skills or rescan global skills

Connection options:
  --db <path>                  SQLite path for a locally spawned agenty server
  --debug                      Enable debug logging in the local server
  --server <url>               Use an existing remote agenty server
  --client-config <path>       Optional remote client connection file
  --user <name> --password <p> Basic-auth credentials for a remote server
  --json                       Emit machine-readable JSON
  --quiet                      Suppress action confirmation output

Destructive remove commands require --yes.`;

export function handleHelp(_: ParsedArgs): void {
    process.stdout.write(`${HELP}\n`);
}
