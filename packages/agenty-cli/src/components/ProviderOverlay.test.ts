import { describe, expect, test } from "bun:test";

import type { ModelProviderDto } from "../api/types";
import {
    buildBuiltinProviderUpdate,
    buildCreateModelFields,
    buildProviderFields,
    parseModelValues,
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

describe("provider overlay model advanced options", () => {
    test("hides advanced model fields until expanded and supplies defaults", () => {
        const collapsed = buildCreateModelFields(false);
        expect(collapsed.find((field) => field.key === "maxOutputTokens")?.visible).toBe(false);
        expect(collapsed.find((field) => field.key === "reasoningEfforts")?.visible).toBe(false);

        const expanded = buildCreateModelFields(true);
        expect(expanded.find((field) => field.key === "maxOutputTokens")?.value).toBe("8192");
        expect(expanded.find((field) => field.key === "reasoningEfforts")?.kind).toBe("multiselect");

        expect(parseModelValues({
            code: "model",
            name: "Model",
            contextWindow: "128000",
            multiModal: "false",
            light: "false",
            reasoning: "true",
        })).toMatchObject({
            maxOutputTokens: 8192,
            reasoning: true,
            reasoningEfforts: [],
        });
    });

    test("uses the Light model label", () => {
        expect(buildCreateModelFields(false).find((field) => field.key === "light")?.label)
            .toBe("Light model");
    });
});
