import assert from "node:assert/strict";
import { join, resolve } from "node:path";
import test from "node:test";

import { resolveCoreBuildPlan } from "./build.mjs";

const repositoryRoot = resolve("/repo");
const packageRoot = join(repositoryRoot, "packages/agenty-core");

test("uses host defaults for a macOS core build", () => {
    const plan = resolveCoreBuildPlan({}, "darwin", packageRoot);

    assert.deepEqual(plan.target, { artifactOS: "macos", extension: "", goOS: "darwin" });
    assert.equal(plan.buildEnvironment.CGO_ENABLED, "1");
    assert.equal(plan.version, "dev");
    assert.equal(plan.corePath, join(packageRoot, "bin/agenty-core"));
    assert.deepEqual(plan.goArgs.slice(0, 3), [
        "build",
        "-ldflags",
        "-X github.com/masteryyh/agenty-core/pkg/buildinfo.Version=dev",
    ]);
    assert.equal(plan.helperSource, join(repositoryRoot, "packages/file-editor/target/release/fileedit"));
    assert.equal(plan.helperDestination, join(packageRoot, "bin/fileedit"));
});

test("uses the GOOS target and Windows extensions", () => {
    const plan = resolveCoreBuildPlan({
        BIN_NAME: "core",
        GOOS: "windows",
        PACKAGE_DIR: "bin/windows_amd64",
    }, "darwin", packageRoot);

    assert.deepEqual(plan.target, { artifactOS: "windows", extension: ".exe", goOS: "windows" });
    assert.equal(plan.version, "dev");
    assert.equal(plan.corePath, join(packageRoot, "bin/windows_amd64/core.exe"));
    assert.equal(plan.helperSource, join(repositoryRoot, "packages/file-editor/target/release/fileedit.exe"));
    assert.equal(plan.helperDestination, join(packageRoot, "bin/windows_amd64/fileedit.exe"));
});

test("detects a Windows host when GOOS is not set", () => {
    const plan = resolveCoreBuildPlan({}, "win32", packageRoot);

    assert.equal(plan.target.goOS, "windows");
    assert.equal(plan.buildEnvironment.CGO_ENABLED, "1");
    assert.equal(plan.corePath, join(packageRoot, "bin/agenty-core.exe"));
});

test("preserves an explicit cgo compiler configuration", () => {
    const plan = resolveCoreBuildPlan({
        CC: "C:\\Program Files\\LLVM-MinGW\\bin\\clang.exe",
        CGO_ENABLED: "1",
        CXX: "C:\\Program Files\\LLVM-MinGW\\bin\\clang++.exe",
    }, "win32", packageRoot);

    assert.equal(plan.buildEnvironment.CGO_ENABLED, "1");
    assert.equal(plan.buildEnvironment.CC, "C:\\Program Files\\LLVM-MinGW\\bin\\clang.exe");
    assert.equal(plan.buildEnvironment.CXX, "C:\\Program Files\\LLVM-MinGW\\bin\\clang++.exe");
});

test("rejects an explicit cgo-disabled build", () => {
    assert.throws(
        () => resolveCoreBuildPlan({ CGO_ENABLED: "0" }, "win32", packageRoot),
        /requires CGO_ENABLED=1/u,
    );
});

test("rejects non-gcc-compatible MSVC for Windows cgo builds", () => {
    assert.throws(
        () => resolveCoreBuildPlan({ CC: "C:\\Build Tools\\cl.exe" }, "win32", packageRoot),
        /MSVC cl\.exe is unsupported/u,
    );
});
