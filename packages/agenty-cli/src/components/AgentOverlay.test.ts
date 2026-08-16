import { describe, expect, test } from "bun:test";

import { parseModelRef } from "./AgentOverlay";

describe("parseModelRef", () => {
    test("keeps slashes inside a model ID", () => {
        expect(parseModelRef("openai/org/model_name[v2]"))
            .toEqual({ providerSlug: "openai", modelSlug: "org/model_name[v2]" });
    });

    test("rejects references without both sides", () => {
        expect(parseModelRef("openai")).toBeUndefined();
        expect(parseModelRef("/model")).toBeUndefined();
        expect(parseModelRef("openai/")).toBeUndefined();
    });
});
