import { describe, expect, test } from "bun:test";

import type { ModelProviderDto } from "../api/types";
import {
    createBuiltinDraft,
    createCustomDraft,
    createModelDraft,
} from "../consts/providerPresets";
import {
    buildWizardModelRows,
    buildWizardProviderRows,
    wizardModelEnterAction,
} from "./wizardRows";

function provider(code: string, builtin: boolean): ModelProviderDto {
    return {
        code,
        name: code === "openai" ? "OpenAI" : "Custom",
        type: builtin ? "openai" : "openai_completions",
        baseUrl: "https://example.invalid/v1",
        apiKey: builtin ? "" : "test-key",
        builtin,
        official: builtin,
        models: [],
        createdAt: "",
        updatedAt: "",
    };
}

describe("wizard hierarchical rows", () => {
    test("keeps Add more provider as the last selectable provider row", () => {
        const builtinProvider = provider("openai", true);
        const customProvider = provider("custom", false);
        const rows = buildWizardProviderRows(
            [createCustomDraft("provider:custom", customProvider)],
            [builtinProvider],
        );

        expect(rows.map((row) => row.kind)).toEqual(["builtin", "custom", "add"]);
        expect(rows.at(-1)).toEqual({
            kind: "add",
            key: "add-provider",
            label: "Add more provider...",
        });
    });

    test("places each provider before all of its model rows", () => {
        const builtin = createBuiltinDraft(provider("openai", true));
        const custom = createCustomDraft("provider:custom", provider("custom", false));
        const first = createModelDraft(custom, "custom:first", {
            code: "first",
            name: "First",
            contextWindow: 128_000,
            maxOutputTokens: 8_192,
            multiModal: false,
            light: false,
            isDefault: true,
        });
        const second = createModelDraft(custom, "custom:second", {
            ...first,
            code: "second",
            name: "Second",
            isDefault: false,
        });

        const rows = buildWizardModelRows([builtin, custom], [first, second]);

        expect(rows.map((row) => row.kind)).toEqual([
            "provider",
            "provider",
            "model",
            "model",
            "add-model",
        ]);
        expect(rows.map((row) => row.key)).toEqual([
            builtin.id,
            custom.id,
            first.id,
            second.id,
            `${custom.id}:add-model`,
        ]);
        expect(rows.at(-1)).toMatchObject({
            kind: "add-model",
            provider: custom,
            label: "Add more models...",
        });
    });

    test("routes Enter to select built-ins, edit custom models, and add model rows", () => {
        const builtin = createBuiltinDraft(provider("openai", true));
        const custom = createCustomDraft("provider:custom", provider("custom", false));
        const builtinModel = createModelDraft(builtin, "openai:model", {
            code: "builtin",
            name: "Built-in",
            contextWindow: 128_000,
            maxOutputTokens: 8_192,
            multiModal: false,
            light: false,
            isDefault: true,
        });
        const customModel = createModelDraft(custom, "custom:model", {
            ...builtinModel,
            code: "custom",
            name: "Custom",
            isDefault: false,
        });
        const rows = buildWizardModelRows(
            [builtin, custom],
            [builtinModel, customModel],
        );

        expect(wizardModelEnterAction(rows.find((row) => row.key === builtinModel.id)))
            .toEqual({ kind: "select", model: builtinModel });
        expect(wizardModelEnterAction(rows.find((row) => row.key === customModel.id)))
            .toEqual({ kind: "edit", model: customModel });
        expect(wizardModelEnterAction(rows.find((row) => row.kind === "add-model")))
            .toEqual({ kind: "add", provider: custom });
    });
});
