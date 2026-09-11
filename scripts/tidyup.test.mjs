import assert from "node:assert/strict";
import { relative, resolve } from "node:path";
import test from "node:test";

import { resolveTidyupPlan } from "./tidyup.mjs";

const REPOSITORY_ROOT = resolve(import.meta.dirname, "..");

test("runs fmt, vet, and mod tidy for every Go module", () => {
    const steps = resolveTidyupPlan(REPOSITORY_ROOT);

    assert.deepEqual(
        steps.map(({ args, cwd }) => ({
            args,
            cwd: relative(REPOSITORY_ROOT, cwd),
        })),
        [
            { args: ["fmt", "./..."], cwd: "packages/agenty-core" },
            { args: ["vet", "./..."], cwd: "packages/agenty-core" },
            { args: ["mod", "tidy"], cwd: "packages/agenty-core" },
            { args: ["fmt", "./..."], cwd: "packages/agenty-inspector" },
            { args: ["vet", "./..."], cwd: "packages/agenty-inspector" },
            { args: ["mod", "tidy"], cwd: "packages/agenty-inspector" },
        ],
    );
});

test("supports a focused Go module tidyup", () => {
    const steps = resolveTidyupPlan(REPOSITORY_ROOT, "agenty-core");
    const coreDirectory = resolve(REPOSITORY_ROOT, "packages/agenty-core");

    assert.equal(steps.length, 3);
    assert.ok(steps.every(({ cwd }) => cwd === coreDirectory));
    assert.throws(
        () => resolveTidyupPlan(REPOSITORY_ROOT, "missing-module"),
        /Go module not found in workspace/u,
    );
});
