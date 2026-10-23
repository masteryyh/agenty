import assert from "node:assert/strict";
import { resolve } from "node:path";
import test from "node:test";

import { resolveDevPlan } from "./dev.mjs";
import { resolveTurboPlan } from "./turbo.mjs";

const root = resolve("/tmp/agenty dev workspace");

for (const [platform, arch, workerNames] of [
    ["darwin", "arm64", ["llama-cpp-worker", "mlx-worker"]],
    ["darwin", "x64", ["llama-cpp-worker"]],
    ["linux", "arm64", ["llama-cpp-worker"]],
    ["win32", "x64", ["llama-cpp-worker.exe"]],
]) {
    test(`dev passes built workers on ${platform}/${arch} and retains user overrides`, () => {
        const args = ["--data-dir=/tmp/debug profile", "--llama-cpp-worker", "/custom/native worker", "local", "status", "--json"];
        const plan = resolveDevPlan(args, {}, platform, arch, root);
        assert.deepEqual(plan.workers.map((worker) => worker.path.split("/").at(-1)), workerNames);
        assert.deepEqual(plan.launcherArgs.slice(-args.length), args);
        for (const worker of plan.workers) {
            assert.ok(worker.path.startsWith(root));
            assert.ok(worker.path.includes("/bin/"));
            const index = plan.launcherArgs.indexOf(worker.option);
            assert.equal(plan.launcherArgs[index + 1], worker.path);
        }
        assert.ok(plan.buildArgs.includes("--filter=agenty-bootstrap"));
    });
}

test("dev rejects a target that cannot run on the host", () => {
    assert.throws(() => resolveDevPlan([], { OS: "linux" }, "darwin", "arm64", root), /current operating system/);
    assert.throws(() => resolveDevPlan([], { ARCH: "amd64" }, "darwin", "arm64", root), /current operating system/);
});

test("standalone worker builds do not require core's cgo environment", () => {
    for (const name of ["mlx-worker", "llama-cpp-worker"]) {
        const plan = resolveTurboPlan("build", name, { CGO_ENABLED: "0" }, "linux", root);
        assert.equal(plan.buildEnvironment.CGO_ENABLED, "0");
    }
});
