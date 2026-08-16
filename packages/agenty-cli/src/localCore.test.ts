import { describe, expect, test } from "bun:test";

import { pickCorePath } from "./localCore";

const candidates = {
    repoBin: "/repo/packages/agenty-core/bin/agenty-core",
    managedBin: "/home/tester/.agenty/bin/core",
};

describe("pickCorePath", () => {
    test("prefers the explicit override", () => {
        expect(pickCorePath({ ...candidates, override: "/custom/core" }, () => false)).toBe("/custom/core");
    });

    test("uses the repository build before the managed binary", () => {
        expect(pickCorePath(candidates, () => true)).toBe(candidates.repoBin);
    });

    test("falls back to the launcher-managed core", () => {
        expect(pickCorePath(candidates, (path) => path === candidates.managedBin)).toBe(candidates.managedBin);
    });

    test("returns null when no core binary exists", () => {
        expect(pickCorePath(candidates, () => false)).toBeNull();
    });
});
