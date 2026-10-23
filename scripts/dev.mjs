import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { packageManagerCommand, spawnSyncCommand } from "./command.mjs";
import { executableExtension, resolveArch, resolveOS } from "./platform.mjs";
import { resolveTurboPlan } from "./turbo.mjs";

const REPOSITORY_ROOT = resolve(import.meta.dirname, "..");

export { packageManagerCommand };

export function resolveDevPlan(
    args,
    environment = process.env,
    hostPlatform = process.platform,
    hostArch = process.arch,
    repositoryRoot = REPOSITORY_ROOT,
) {
    const os = resolveOS(environment, hostPlatform);
    const arch = resolveArch(environment, hostArch);
    const launcher = resolve(
        repositoryRoot,
        "packages/agenty-bootstrap/bin",
        `agenty-${os}-${arch}${executableExtension(os)}`,
    );
    const turboPlan = resolveTurboPlan(
        "build",
        "agenty-bootstrap",
        environment,
        hostPlatform,
        repositoryRoot,
    );
    if (os !== resolveOS({}, hostPlatform) || arch !== resolveArch({}, hostArch)) {
        throw new Error("local debugging must build for the current operating system and architecture");
    }
    const workers = [{ option: "--llama-cpp-worker", path: resolve(repositoryRoot, "packages/llama-cpp-worker/bin", `llama-cpp-worker${executableExtension(os)}`) }];
    if (os === "macos" && arch === "arm64") {
        workers.push({ option: "--mlx-worker", path: resolve(repositoryRoot, "packages/mlx-worker/bin/mlx-worker") });
    }
    const launcherArgs = [...workers.flatMap((worker) => [worker.option, worker.path]), ...args];
    return {
        buildEnvironment: turboPlan.buildEnvironment,
        buildArgs: turboPlan.args,
        launcher,
        launcherArgs,
        packageManager: turboPlan.packageManager,
        workers,
    };
}

function exitCode(label, result) {
    if (result.error) {
        throw result.error;
    }
    if (result.status === null) {
        const detail = result.signal ? ` (signal ${result.signal})` : "";
        throw new Error(`${label} did not return an exit code${detail}`);
    }
    return result.status;
}

function run() {
    const plan = resolveDevPlan(process.argv.slice(2));
    const build = spawnSyncCommand(plan.packageManager, plan.buildArgs, {
        cwd: REPOSITORY_ROOT,
        env: plan.buildEnvironment,
        stdio: "inherit",
    });
    const buildExitCode = exitCode("bootstrap build", build);
    if (buildExitCode !== 0) {
        return buildExitCode;
    }
    if (!existsSync(plan.launcher)) {
        throw new Error(`bootstrap launcher not found at ${plan.launcher}`);
    }

    for (const worker of plan.workers) {
        if (!existsSync(worker.path)) {
            throw new Error(`worker executable not found at ${worker.path}`);
        }
    }

    const launcher = spawnSync(plan.launcher, plan.launcherArgs, {
        cwd: REPOSITORY_ROOT,
        env: plan.buildEnvironment,
        stdio: "inherit",
    });
    return exitCode("bootstrap launcher", launcher);
}

const currentFile = fileURLToPath(import.meta.url);
if (process.argv[1] && resolve(process.argv[1]) === currentFile) {
    try {
        process.exitCode = run();
    } catch (error) {
        console.error(error instanceof Error ? error.message : error);
        process.exitCode = 1;
    }
}
