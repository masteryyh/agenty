import { describe, expect, test } from "bun:test";

import type { ModelProviderDto } from "../api/types";
import {
    buildBuiltinProviderUpdate,
    buildProviderFields,
} from "./ProviderOverlay";

function builtinProvider(): ModelProviderDto {
    return {
        code: "openai",
        name: "OpenAI",
        type: "openai",
        baseUrl: "https://api.openai.com/v1",
        apiKey: "existing-key",
        builtin: true,
        official: true,
        freeFormTool: true,
        models: [],
        createdAt: "",
        updatedAt: "",
    };
}

describe("provider overlay builtin configuration", () => {
    test("keeps only the API key focusable for a built-in provider", () => {
        const fields = buildProviderFields(builtinProvider(), "configure");

        expect(fields.filter((field) => field.focusable !== false).map((field) => field.key)).toEqual([
            "apiKey",
        ]);
        expect(fields.filter((field) => field.key !== "apiKey").every((field) => field.readOnly)).toBe(true);
        const apiKeyField = fields.find((field) => field.key === "apiKey");
        expect(apiKeyField?.readOnly).toBeUndefined();
        expect(apiKeyField?.value).toBe("");
    });

    test("builds an API-key-only update and ignores blank input", () => {
        expect(buildBuiltinProviderUpdate({ apiKey: "  next-key  " })).toEqual({
            apiKey: "next-key",
        });
        expect(buildBuiltinProviderUpdate({ apiKey: "   " })).toBeNull();
    });

    test("exposes the free-form setting for custom Responses providers", () => {
        const fields = buildProviderFields({
            ...builtinProvider(),
            builtin: false,
        }, "edit");
        const freeFormField = fields.find((field) => field.key === "freeFormTool");

        expect(freeFormField?.value).toBe("true");
        expect(freeFormField?.readOnly).toBe(false);
    });
});
