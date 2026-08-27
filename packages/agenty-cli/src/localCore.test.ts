import { delimiter } from "node:path";

import { describe, expect, test } from "bun:test";

import { MANAGED_BIN_DIR, pickCorePath, prependCoreDirectoryToPath } from "./localCore";

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

describe("prependCoreDirectoryToPath", () => {
    test("places the core directory before the inherited PATH", () => {
        expect(prependCoreDirectoryToPath("/managed/bin/core", "/usr/bin")).toBe(
            ["/managed/bin", MANAGED_BIN_DIR, "/usr/bin"].join(delimiter),
        );
    });

    test("keeps the managed helper directory reachable for an override", () => {
        expect(prependCoreDirectoryToPath(
            "/custom/bin/core",
            "/usr/bin:/managed/bin",
            "/managed/bin",
            ":",
        )).toBe("/custom/bin:/managed/bin:/usr/bin");
    });

    test("uses the Windows path delimiter when assembling an override PATH", () => {
        expect(prependCoreDirectoryToPath(
            "C:/custom/core.exe",
            "C:/Windows/System32;C:/managed/bin",
            "C:/managed/bin",
            ";",
        )).toBe("C:/custom;C:/managed/bin;C:/Windows/System32");
    });
});
