import { describe, expect, test } from "bun:test";

import type { CoreModelDto, ModelProviderDto } from "../api/types";
import {
    buildProviderRows,
    defaultExpandedProviderCodes,
    providerRowEnterAction,
    providerRowLabel,
    providerRowState,
} from "./providerRows";

function model(code: string, name = code): CoreModelDto {
    return {
        code,
        name,
        contextWindow: 128_000,
        maxOutputTokens: 8_192,
        multiModal: false,
        light: false,
        reasoningEfforts: ["low"],
        isDefault: false,
    };
}

function provider(
    code: string,
    builtin: boolean,
    models: CoreModelDto[] = [],
    apiKey = "",
): ModelProviderDto {
    return {
        code,
        name: builtin ? "OpenAI" : "Custom",
        type: builtin ? "openai" : "openai_completions",
        baseUrl: "https://example.invalid/v1",
        apiKey,
        builtin,
        models,
        createdAt: "",
        updatedAt: "",
    };
}

describe("provider hierarchical rows", () => {
    test("starts expanded and adds model creation only for custom providers", () => {
        const builtin = provider("openai", true, [model("gpt-5")]);
        const custom = provider("custom", false, [model("custom-model")]);
        const rows = buildProviderRows([builtin, custom], new Set(["openai", "custom"]));

        expect(rows.map((row) => row.kind)).toEqual([
            "provider",
            "model",
            "provider",
            "model",
            "add-model",
            "add-provider",
        ]);
        expect(rows.at(-1)).toMatchObject({
            kind: "add-provider",
            label: "Add more providers...",
        });
    });

    test("removes child rows when a provider is collapsed", () => {
        const custom = provider("custom", false, [model("custom-model")]);
        const rows = buildProviderRows([custom], new Set());

        expect(rows.map((row) => row.kind)).toEqual(["provider", "add-provider"]);
        expect(providerRowLabel(rows[0]!)).toContain("▸");
    });

    test("does not auto-expand a provider populated from the core cache", () => {
        const discovered = provider("openrouter", true, [model("openai/gpt-test")], "key");
        discovered.modelsCached = true;

        expect(defaultExpandedProviderCodes([discovered])).toEqual(new Set());
    });

    test("routes built-ins to API key configuration and custom resources to edit", () => {
        const builtin = provider("openai", true, [model("gpt-5")]);
        const custom = provider("custom", false, [model("custom-model")]);
        const rows = buildProviderRows([builtin, custom], new Set(["openai", "custom"]));

        expect(providerRowEnterAction(rows[0])).toEqual({
            kind: "configure-provider",
            provider: builtin,
        });
        expect(providerRowEnterAction(rows[1])).toEqual({
            kind: "view-model",
            provider: builtin,
            model: builtin.models[0],
        });
        expect(providerRowEnterAction(rows[2])).toEqual({
            kind: "edit-provider",
            provider: custom,
        });
        expect(providerRowEnterAction(rows[3])).toEqual({
            kind: "edit-model",
            provider: custom,
            model: custom.models[0],
        });
        expect(providerRowEnterAction(rows[4])).toEqual({
            kind: "add-model",
            provider: custom,
        });
        expect(providerRowEnterAction(rows[5])).toEqual({ kind: "add-provider" });
    });

    test("shows configuration state only on provider rows", () => {
        const configuredBuiltin = provider("openai", true, [model("gpt-5")], "test-key");
        const unconfiguredBuiltin = provider("anthropic", true);
        const custom = provider("custom", false, [model("custom-model")]);
        const rows = buildProviderRows(
            [configuredBuiltin, unconfiguredBuiltin, custom],
            new Set(["openai", "anthropic", "custom"]),
        );

        expect(rows.map(providerRowState)).toEqual([
            "configured",
            "",
            "unconfigured",
            "custom",
            "",
            "",
            "",
        ]);
    });
});
