import assert from "node:assert/strict";
import { resolve } from "node:path";
import test from "node:test";

import { packageManagerCommand, resolveDevPlan } from "./dev.mjs";

test("selects the launcher for the host target and forwards arguments", () => {
    const plan = resolveDevPlan(["--version"], {}, "darwin", "arm64", "/repo");

    assert.deepEqual(plan.buildArgs, ["exec", "turbo", "run", "build", "--filter=agenty-bootstrap"]);
    assert.equal(plan.launcher, resolve("/repo/packages/agenty-bootstrap/bin/agenty-macos-arm64"));
    assert.deepEqual(plan.launcherArgs, ["--version"]);
});

test("uses a Windows launcher extension and package manager command", () => {
    const plan = resolveDevPlan([], { ARCH: "amd64", OS: "Windows_NT" }, "win32", "x64", "/repo");

    assert.equal(plan.launcher, resolve("/repo/packages/agenty-bootstrap/bin/agenty-windows-amd64.exe"));
    assert.equal(packageManagerCommand("win32"), "pnpm.cmd");
    assert.equal(packageManagerCommand("linux"), "pnpm");
});
