import assert from "node:assert/strict";
import test from "node:test";

import { resolveTurboPlan } from "./turbo.mjs";

test("resolves a root Turbo task with the shared build version", () => {
    const plan = resolveTurboPlan("test", "agenty-core", {}, "darwin", "/repo");

    assert.deepEqual(plan.args, ["exec", "turbo", "run", "test", "--filter=agenty-core"]);
    assert.equal(plan.buildEnvironment.AGENTY_VERSION, "dev");
    assert.equal(plan.packageManager, "pnpm");
});

