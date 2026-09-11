import { afterEach, describe, expect, test } from "bun:test";

import { loadOptions, parseThinking } from "./config";

const originalArgv = process.argv.slice();

afterEach(() => {
    process.argv.splice(0, process.argv.length, ...originalArgv);
});

describe("CLI options", () => {
    test("keeps thinking omitted when no flag was provided", () => {
        process.argv.splice(0, process.argv.length, "bun", "agenty");

        expect(loadOptions()).toMatchObject({
            thinking: undefined,
            newSession: false,
        });
    });

    test("preserves an explicit thinking flag", () => {
        process.argv.splice(0, process.argv.length, "bun", "agenty", "--thinking", "off");

        expect(loadOptions().thinking).toBe("off");
        expect(parseThinking("medium")).toEqual({ thinking: true, thinkingLevel: "medium" });
    });
});
