import { describe, expect, test } from "bun:test";

import type { McpServerDto } from "../api/types";
import type { FormValues } from "./FormPanel";
import { buildConfig, buildFields } from "./McpOverlay";

describe("MCP form configuration", () => {
    test("does not expose optional HTTP headers, bearer, or OAuth fields", () => {
        const fields = buildFields(undefined, "http");
        const labels = fields.filter((field) => field.visible !== false).map((field) => field.label);

        expect(labels).toEqual(["Name", "Transport", "Enabled", "URL"]);
    });

    test("preserves hidden HTTP configuration while editing a server", () => {
        const target: McpServerDto = {
            name: "remote",
            config: {
                type: "http",
                enabled: true,
                url: "https://example.com/mcp",
                headers: { "X-Region": "us-east-1" },
                bearerTokenEnvVar: "MCP_TOKEN",
                oauth: {
                    clientId: "client-id",
                    clientSecret: "client-secret",
                    issuer: "https://issuer.example.com",
                },
            },
            status: "connected",
            toolCount: 1,
        };
        const values: FormValues = {
            type: "http",
            enabled: "true",
            url: "https://example.com/updated",
        };

        expect(buildConfig(values, target)).toEqual({
            type: "http",
            enabled: true,
            url: "https://example.com/updated",
            headers: { "X-Region": "us-east-1" },
            bearerTokenEnvVar: "MCP_TOKEN",
            oauth: {
                clientId: "client-id",
                clientSecret: "client-secret",
                issuer: "https://issuer.example.com",
            },
        });
    });
});
