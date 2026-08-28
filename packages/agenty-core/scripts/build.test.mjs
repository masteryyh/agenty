import assert from "node:assert/strict";
import { join, resolve } from "node:path";
import test from "node:test";

import { resolveCoreBuildPlan } from "./build.mjs";

const repositoryRoot = resolve("/repo");
const packageRoot = join(repositoryRoot, "packages/agenty-core");

test("uses host defaults for a macOS core build", () => {
    const plan = resolveCoreBuildPlan({}, "darwin", packageRoot);

    assert.deepEqual(plan.target, { artifactOS: "macos", extension: "", goOS: "darwin" });
    assert.equal(plan.corePath, join(packageRoot, "bin/agenty-core"));
    assert.equal(plan.helperSource, join(repositoryRoot, "packages/patch-applier/target/release/apply_patch"));
    assert.equal(plan.helperDestination, join(packageRoot, "bin/apply_patch"));
});

test("uses the GOOS target and Windows extensions", () => {
    const plan = resolveCoreBuildPlan({
        BIN_NAME: "core",
        GOOS: "windows",
        PACKAGE_DIR: "bin/windows_amd64",
    }, "darwin", packageRoot);

    assert.deepEqual(plan.target, { artifactOS: "windows", extension: ".exe", goOS: "windows" });
    assert.equal(plan.corePath, join(packageRoot, "bin/windows_amd64/core.exe"));
    assert.equal(plan.helperSource, join(repositoryRoot, "packages/patch-applier/target/release/apply_patch.exe"));
    assert.equal(plan.helperDestination, join(packageRoot, "bin/windows_amd64/apply_patch.exe"));
});

test("detects a Windows host when GOOS is not set", () => {
    const plan = resolveCoreBuildPlan({}, "win32", packageRoot);

    assert.equal(plan.target.goOS, "windows");
    assert.equal(plan.corePath, join(packageRoot, "bin/agenty-core.exe"));
});
