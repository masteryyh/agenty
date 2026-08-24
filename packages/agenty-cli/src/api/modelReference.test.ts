import { describe, expect, test } from "bun:test";

import {
    findModelByRef,
    formatModelRef,
    modelRefFromModel,
    resolveModelInput,
    sameModelRef,
} from "./modelReference";
import type { ModelDto } from "./types";

function model(providerCode: string, code: string, name = code, providerName = providerCode): ModelDto {
    return {
        code,
        providerCode,
        providerName,
        name,
        contextWindow: 128_000,
        maxOutputTokens: 8_192,
        multiModal: false,
        light: false,
        reasoningEfforts: [],
        isDefault: false,
    };
}

describe("model references", () => {
    test("formats and compares structured references without losing nested model slashes", () => {
        const candidate = model("openrouter", "deepseek/deepseek-v4-pro");
        const ref = modelRefFromModel(candidate);

        expect(formatModelRef(ref)).toBe("openrouter/deepseek/deepseek-v4-pro");
        expect(findModelByRef([candidate], ref)).toBe(candidate);
        expect(sameModelRef(ref, { providerCode: "openrouter", modelCode: "deepseek/deepseek-v4-pro" })).toBe(true);
    });

    test("gives an explicitly scoped reference precedence over another model's bare code", () => {
        const deepSeek = model("deepseek", "deepseek-v4-pro", "DeepSeek V4 Pro", "DeepSeek");
        const openRouter = model(
            "openrouter",
            "deepseek/deepseek-v4-pro",
            "DeepSeek: DeepSeek V4 Pro 0423",
            "OpenRouter",
        );

        expect(resolveModelInput([deepSeek, openRouter], "deepseek/deepseek-v4-pro")).toBe(deepSeek);
        expect(resolveModelInput([deepSeek, openRouter], "openrouter/deepseek/deepseek-v4-pro")).toBe(openRouter);
    });

    test("still rejects an unscoped code when providers share it", () => {
        const first = model("first", "shared-model");
        const second = model("second", "shared-model");

        expect(() => resolveModelInput([first, second], "shared-model"))
            .toThrow("model reference is ambiguous: shared-model; use <provider-code>/<model-code>");
    });

    test("allows unique name aliases while keeping canonical references deterministic", () => {
        const candidate = model("openai", "gpt-test", "GPT Test", "OpenAI");

        expect(resolveModelInput([candidate], "gpt test")).toBe(candidate);
        expect(resolveModelInput([candidate], "GPT Test")).toBe(candidate);
        expect(resolveModelInput([candidate], "OpenAI/GPT Test")).toBe(candidate);
    });
});
