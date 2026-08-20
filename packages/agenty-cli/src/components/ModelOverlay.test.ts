import { describe, expect, test } from "bun:test";

import type { CoreModelDto, ModelProviderDto } from "../api/types";
import { isBuiltinProvider } from "../consts/providerPresets";
import {
    initialModelIndex,
    initialProviderIndex,
    modelInputFromValues,
    modelUpdateFromValues,
    parseReasoningMapping,
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

function provider(code: string, models: CoreModelDto[] = []): ModelProviderDto {
    return {
        code,
        name: code,
        type: "openai_completions",
        baseUrl: "https://example.invalid/v1",
        apiKey: "test-key",
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
        expect(isBuiltinProvider("openai")).toBe(true);
        expect(isBuiltinProvider("custom")).toBe(false);
    });

    test("starts on the current model, then provider default, then first model", () => {
        const models = [model("first"), model("default", true), model("current")];
        const target = provider("custom", models);

        expect(initialModelIndex(target, "current")).toBe(2);
        expect(initialModelIndex(target, "missing")).toBe(1);
        expect(initialModelIndex(provider("empty"))).toBe(0);
    });

    test("parses and validates reasoning mappings", () => {
        expect(parseReasoningMapping("{\"fast\":\"low\",\"deep\":\"high\"}")).toEqual({
            fast: "low",
            deep: "high",
        });
        expect(parseReasoningMapping("{}")).toEqual({});
        expect(parseReasoningMapping(" ")).toBeUndefined();
        expect(() => parseReasoningMapping("[]")).toThrow("JSON object");
        expect(() => parseReasoningMapping("{\"fast\":\"unsupported\"}")).toThrow("Invalid reasoning effort");
    });

    test("builds create and update payloads without changing model codes", () => {
        const values = {
            code: "org/model_name[v2]",
            name: "Model v2",
            contextWindow: "64000",
            multiModal: "true",
            light: "false",
            isDefault: "true",
            reasoningMapping: "{\"deep\":\"xhigh\"}",
        };
        const created = modelInputFromValues("custom", values);
        expect(created).toMatchObject({
            providerCode: "custom",
            modelCode: "org/model_name[v2]",
            contextWindow: 64_000,
            multiModal: true,
            isDefault: true,
            reasoningEffortMapping: { deep: "xhigh" },
        });

        const updated = modelUpdateFromValues(model("old-id"), {
            ...values,
            name: "Updated model",
            code: "new-id",
        });
        expect(updated).toMatchObject({
            name: "Updated model",
            contextWindow: 64_000,
            multiModal: true,
        });
    });

});
