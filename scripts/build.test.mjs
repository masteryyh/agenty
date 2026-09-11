import assert from "node:assert/strict";
import test from "node:test";

import { resolveBuildPlan } from "./build.mjs";

test("builds the bootstrap dependency chain by default", () => {
    const plan = resolveBuildPlan("agenty-bootstrap", {}, "darwin", "/repo");

    assert.deepEqual(plan.buildArgs, ["exec", "turbo", "run", "build", "--filter=agenty-bootstrap"]);
    assert.equal(plan.packageManager, "pnpm");
    assert.equal(plan.buildEnvironment.AGENTY_VERSION, "dev");
    assert.equal(plan.copyBootstrap, true);
});

test("passes a selected workspace filter without copying the launcher", () => {
    const plan = resolveBuildPlan("agenty-inspector", { AGENTY_VERSION: "v1.2.3" }, "win32", "/repo");

    assert.deepEqual(plan.buildArgs, ["exec", "turbo", "run", "build", "--filter=agenty-inspector"]);
    assert.equal(plan.packageManager, "pnpm.cmd");
    assert.equal(plan.buildEnvironment.AGENTY_VERSION, "v1.2.3");
    assert.equal(plan.copyBootstrap, false);
});
