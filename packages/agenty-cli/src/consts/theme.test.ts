import { RGBA } from "@opentui/core";
import { describe, expect, test } from "bun:test";

import { theme } from "./theme";

const HEX6 = /^#[0-9A-F]{6}$/;
// OpenTUI's parseColor falls back to magenta (with a console.warn that
// consoleMode: "disabled" swallows), so an invalid token would render
// silently as magenta. Guard both the literal shape and the parse result.
const MAGENTA = RGBA.fromInts(255, 0, 255, 255);
// Quoted terminal color names are banned outside the theme module; the whole
// token set is absolute hex.
const TERMINAL_COLOR_NAME
    = /["'](cyan|magenta|red|green|yellow|white|gray|grey|blue|black|orange|purple)["']/;

describe("theme", () => {
    test("every token is a #RRGGBB literal", () => {
        const bad = Object.entries(theme).filter(([, value]) => !HEX6.test(value));
        expect(bad).toEqual([]);
    });

    test("no token parses to OpenTUI's magenta fallback", () => {
        for (const value of Object.values(theme)) {
            expect(RGBA.fromHex(value).toInts()).not.toEqual(MAGENTA.toInts());
        }
    });

    test("no color literals outside the theme module", async () => {
        const glob = new Bun.Glob("src/**/*.{ts,tsx}");
        const packageRoot = `${import.meta.dir}/../..`;
        const offenders: string[] = [];
        for await (const path of glob.scan({ cwd: packageRoot })) {
            if (path === "src/consts/theme.ts" || path.includes(".test.")) {
                continue;
            }
            const source = await Bun.file(`${packageRoot}/${path}`).text();
            if (/#[0-9a-fA-F]{6}\b/.test(source)) {
                offenders.push(`${path}: hex literal`);
            }
            const lines = source.split("\n");
            for (const [index, line] of lines.entries()) {
                if (TERMINAL_COLOR_NAME.test(line)) {
                    offenders.push(`${path}:${index + 1}: ${line.trim()}`);
                }
            }
        }
        expect(offenders).toEqual([]);
    });
});
