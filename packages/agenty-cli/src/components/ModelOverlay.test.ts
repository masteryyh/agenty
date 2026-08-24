import { describe, expect, test } from "bun:test";

import type { CoreModelDto, ModelProviderDto } from "../api/types";
import { isBuiltinProvider } from "../consts/providerPresets";
import {
    configuredProviders,
    filterModelsByCodeLike,
    initialModelIndex,
    initialProviderIndex,
    isConfiguredProvider,
    isModelSearchInput,
    normalizeModelSearchInput,
    sortModelsByCode,
} from "./ModelOverlay";

function model(code: string, isDefault = false): CoreModelDto {
    return {
        code,
        name: code.toUpperCase(),
        contextWindow: 128_000,
        maxOutputTokens: 8_192,
        multiModal: false,
        light: false,
        isDefault,
    };
}

function provider(
    code: string,
    models: CoreModelDto[] = [],
    apiKey = "test-key",
): ModelProviderDto {
    return {
        code,
        name: code,
        type: "openai_completions",
        baseUrl: "https://example.invalid/v1",
        apiKey,
        builtin: code === "openai",
        official: code === "openai",
        models,
        createdAt: "",
        updatedAt: "",
    };
}

describe("model overlay behavior", () => {
    test("starts on the provider used by the current session", () => {
        const providers = [provider("custom"), provider("openai")];

        expect(initialProviderIndex(providers, "openai")).toBe(1);
        expect(initialProviderIndex(providers, "missing")).toBe(0);
        expect(isBuiltinProvider(provider("openai"))).toBe(true);
        expect(isBuiltinProvider(provider("custom"))).toBe(false);
    });

    test("starts on the current model, then provider default, then first model", () => {
        const models = [model("first"), model("default", true), model("current")];
        const target = provider("custom", models);

        expect(initialModelIndex(target, "current")).toBe(2);
        expect(initialModelIndex(target, "missing")).toBe(1);
        expect(initialModelIndex(provider("empty"))).toBe(0);
    });

    test("sorts models by code without mutating the provider response", () => {
        const models = [model("zeta"), model("Alpha"), model("beta")];

        expect(sortModelsByCode(models).map((candidate) => candidate.code)).toEqual([
            "Alpha",
            "beta",
            "zeta",
        ]);
        expect(models.map((candidate) => candidate.code)).toEqual(["zeta", "Alpha", "beta"]);
        expect(configuredProviders([provider("custom", models)])[0]?.models.map(
            (candidate) => candidate.code,
        )).toEqual(["Alpha", "beta", "zeta"]);
    });

    test("filters sorted model codes with a case-insensitive contains match", () => {
        const models = sortModelsByCode([
            model("gpt-5"),
            model("GPT-4o"),
            model("openrouter:google/gemini-2.5-pro"),
            { ...model("claude-3"), name: "gpt display name" },
        ]);

        expect(filterModelsByCodeLike(models, "pt-").map((candidate) => candidate.code)).toEqual([
            "GPT-4o",
            "gpt-5",
        ]);
        expect(filterModelsByCodeLike(models, "AUdE").map((candidate) => candidate.code)).toEqual([
            "claude-3",
        ]);
        expect(filterModelsByCodeLike(models, ":GOOGLE/GEMINI-2.5").map(
            (candidate) => candidate.code,
        )).toEqual(["openrouter:google/gemini-2.5-pro"]);
        expect(filterModelsByCodeLike(models, "display")).toEqual([]);
        expect(filterModelsByCodeLike(models, "")).toBe(models);
    });

    test("only accepts model code characters as automatic search input", () => {
        const namespacedCode = "openrouter:google/gemini-2.5-pro";

        expect(isModelSearchInput("Az09-_ /\\:.".replace(" ", ""))).toBe(true);
        expect(isModelSearchInput(namespacedCode)).toBe(true);
        expect(normalizeModelSearchInput(namespacedCode)).toBe(namespacedCode);
        expect(normalizeModelSearchInput("gpt 4.1")).toBe("gpt4.1");
        expect(isModelSearchInput("model name")).toBe(false);
        expect(isModelSearchInput("")).toBe(false);
    });

    test("only treats providers with a non-blank API key as configured", () => {
        const providers = [
            provider("configured"),
            provider("empty", [], ""),
            provider("blank", [], "   "),
        ];

        expect(isConfiguredProvider(providers[0]!)).toBe(true);
        expect(isConfiguredProvider(providers[1]!)).toBe(false);
        expect(configuredProviders(providers).map((candidate) => candidate.code)).toEqual([
            "configured",
        ]);
    });

});
