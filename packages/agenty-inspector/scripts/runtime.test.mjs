import assert from "node:assert/strict";
import { resolve } from "node:path";
import test from "node:test";

import { hostGoEnv, runtimeEnv } from "./runtime.mjs";

test("relative data directory follows the pnpm invocation directory", () => {
    const env = runtimeEnv({ AGENTY_DATA_DIR: "fixtures", INIT_CWD: "/workspace/root" }, "/workspace/root/packages/agenty-inspector");
    assert.equal(env.AGENTY_DATA_DIR, resolve("/workspace/root", "fixtures"));
});

test("unset data directory remains unset for core default resolution", () => {
    assert.equal(runtimeEnv({}, "/workspace").AGENTY_DATA_DIR, undefined);
});

test("absolute data directory remains absolute", () => {
    const path = resolve("fixtures");
    assert.equal(runtimeEnv({ AGENTY_DATA_DIR: path }, "/unrelated").AGENTY_DATA_DIR, path);
});

test("code generation runs on host even during a cross build", () => {
    const env = hostGoEnv({ GOOS: "windows", GOARCH: "amd64", GOCACHE: "/tmp/go" });
    assert.equal(env.GOOS, undefined);
    assert.equal(env.GOARCH, undefined);
    assert.equal(env.GOCACHE, "/tmp/go");
});
