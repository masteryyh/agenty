import type { McpServerDto } from "../api/types";

export const ADD_MCP_SERVER_KEY = "add-mcp-server" as const;

export type McpListRow =
    | {
        kind: "server";
        key: string;
        server: McpServerDto;
    }
    | {
        kind: "add-server";
        key: typeof ADD_MCP_SERVER_KEY;
        label: "Add more MCP servers...";
    };

export function buildMcpRows(servers: McpServerDto[]): McpListRow[] {
    return [
        ...servers.map((server) => ({
            kind: "server" as const,
            key: `mcp:${server.name}`,
            server,
        })),
        {
            kind: "add-server",
            key: ADD_MCP_SERVER_KEY,
            label: "Add more MCP servers...",
        },
    ];
}

export function mcpRowKey(row: McpListRow): string {
    return row.kind === "server" ? row.server.name : row.key;
}
