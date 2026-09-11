import assert from "node:assert/strict";
import { relative, resolve } from "node:path";
import test from "node:test";

import { resolveUpdatePlan } from "./update.mjs";

const REPOSITORY_ROOT = resolve(import.meta.dirname, "..");

test("updates pnpm, every Go module, and every Cargo module", () => {
    const steps = resolveUpdatePlan(REPOSITORY_ROOT, "darwin");

    assert.deepEqual(
        steps.map(({ command, args, cwd }) => ({
            args,
            command,
            cwd: relative(REPOSITORY_ROOT, cwd) || ".",
        })),
        [
            { args: ["update", "--recursive"], command: "pnpm", cwd: "." },
            { args: ["get", "-u", "./..."], command: "go", cwd: "packages/agenty-core" },
            { args: ["mod", "tidy"], command: "go", cwd: "packages/agenty-core" },
            { args: ["get", "-u", "./..."], command: "go", cwd: "packages/agenty-inspector" },
            { args: ["mod", "tidy"], command: "go", cwd: "packages/agenty-inspector" },
            { args: ["update"], command: "cargo", cwd: "packages/agenty-bootstrap" },
            { args: ["update"], command: "cargo", cwd: "packages/patch-applier" },
        ],
    );
});

test("uses the Windows package-manager executable", () => {
    const [step] = resolveUpdatePlan(REPOSITORY_ROOT, "win32");

    assert.equal(step.command, "pnpm.cmd");
});
