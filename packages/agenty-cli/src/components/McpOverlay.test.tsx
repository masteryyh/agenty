import { describe, expect, test } from "bun:test";

import type { McpServerDto } from "../api/types";
import type { FormValues } from "./FormPanel";
import { buildConfig, buildFields } from "./McpOverlay";

describe("MCP form configuration", () => {
    test("shows headers under collapsed advanced options", () => {
        const fields = buildFields(undefined, "http");
        const labels = fields.filter((field) => field.visible !== false).map((field) => field.label);

        expect(labels).toEqual(["Name", "Transport", "Enabled", "URL", "Advanced Options"]);
        expect(fields.find((field) => field.key === "headers")?.visible).toBe(false);
    });

    test("builds headers only when the advanced field is submitted", () => {
        const target: McpServerDto = {
            name: "remote",
            config: {
                type: "http",
                enabled: true,
                url: "https://example.com/mcp",
                headers: { "X-Region": "us-east-1" },
            },
            status: "connected",
            toolCount: 1,
        };
        const values: FormValues = {
            type: "http",
            enabled: "true",
            url: "https://example.com/updated",
            headers: "{\"X-Region\":\"us-west-2\"}",
        };

        expect(buildConfig({ type: "http", enabled: "true", url: "https://example.com/updated" }, target)).toEqual({
            type: "http",
            enabled: true,
            url: "https://example.com/updated",
            headers: { "X-Region": "us-east-1" },
        });
        expect(buildConfig(values, target)).toEqual({
            type: "http",
            enabled: true,
            url: "https://example.com/updated",
            headers: { "X-Region": "us-west-2" },
        });
    });
});
