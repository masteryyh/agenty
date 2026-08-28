import { describe, expect, test } from "bun:test";

import { findCommand } from "./registry";

describe("command registry", () => {
    test("exposes effort without the old think command", () => {
        expect(findCommand("/effort")?.usage).toBe("/effort [off|on|low|medium|high|xhigh|max]");
        expect(findCommand("/think")).toBeUndefined();
    });
});
