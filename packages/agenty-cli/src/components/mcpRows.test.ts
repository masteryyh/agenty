import { describe, expect, test } from "bun:test";

import type { McpServerDto } from "../api/types";
import {
    ADD_MCP_SERVER_KEY,
    buildMcpRows,
    mcpRowKey,
} from "./mcpRows";

function server(name: string): McpServerDto {
    return {
        name,
        config: {
            type: "stdio",
            enabled: true,
            command: "node",
            args: ["server.mjs"],
        },
        status: "connected",
        toolCount: 2,
    };
}

describe("MCP list rows", () => {
    test("keeps the add row at the bottom and preserves server identity", () => {
        const rows = buildMcpRows([server("filesystem"), server("git")]);

        expect(rows.map(mcpRowKey)).toEqual(["filesystem", "git", ADD_MCP_SERVER_KEY]);
        expect(rows.at(-1)).toMatchObject({
            kind: "add-server",
            label: "Add more MCP servers...",
        });
    });
});
