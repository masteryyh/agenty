import { describe, expect, test } from "bun:test";

import { findCommand } from "./registry";

describe("command registry", () => {
    test("exposes effort and Codex Mode command arguments", async () => {
        expect(findCommand("/effort")?.usage).toBe("/effort [off|on|low|medium|high|xhigh|max]");
        expect(findCommand("/think")).toBeUndefined();
        expect(findCommand("/permissions")).toBeUndefined();
        expect(findCommand("/codex-mode")?.usage).toBe("/codex-mode");
        expect(findCommand("/new")?.usage).toBe("/new [codex]");
        await expect(findCommand("/new")?.completeArgs?.({ listModels: async () => [] })).resolves.toEqual(["codex"]);
        expect(findCommand("/help")).toBeUndefined();
    });
});
