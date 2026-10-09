import assert from "node:assert/strict";
import test from "node:test";

import { resolveCodecTarget } from "./build.mjs";

test("matches the core release targets and Windows GNU ABI", () => {
    for (const [GOOS, GOARCH, target] of [
        ["darwin", "arm64", "aarch64-apple-darwin"],
        ["darwin", "amd64", "x86_64-apple-darwin"],
        ["linux", "arm64", "aarch64-unknown-linux-gnu"],
        ["linux", "amd64", "x86_64-unknown-linux-gnu"],
        ["windows", "amd64", "x86_64-pc-windows-gnu"],
    ]) {
        assert.equal(resolveCodecTarget({ GOOS, GOARCH }, "darwin", "arm64").target, target);
    }
});

test("core target takes priority over OS/ARCH aliases", () => {
    assert.equal(resolveCodecTarget({ GOOS: "linux", GOARCH: "amd64", OS: "macos", ARCH: "arm64" }).target, "x86_64-unknown-linux-gnu");
    assert.throws(() => resolveCodecTarget({ GOOS: "windows", GOARCH: "arm64" }), /MinGW/u);
});
