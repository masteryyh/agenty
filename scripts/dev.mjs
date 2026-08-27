import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { executableExtension, resolveArch, resolveOS } from "./platform.mjs";

const REPOSITORY_ROOT = resolve(import.meta.dirname, "..");

export function packageManagerCommand(hostPlatform = process.platform) {
    return hostPlatform === "win32" ? "pnpm.cmd" : "pnpm";
}

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
    return {
        buildArgs: ["exec", "turbo", "run", "build", "--filter=agenty-bootstrap"],
        launcher,
        launcherArgs: args,
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
    const build = spawnSync(packageManagerCommand(), plan.buildArgs, {
        cwd: REPOSITORY_ROOT,
        env: process.env,
        stdio: "inherit",
    });
    const buildExitCode = exitCode("bootstrap build", build);
    if (buildExitCode !== 0) {
        return buildExitCode;
    }
    if (!existsSync(plan.launcher)) {
        throw new Error(`bootstrap launcher not found at ${plan.launcher}`);
    }

    const launcher = spawnSync(plan.launcher, plan.launcherArgs, {
        cwd: REPOSITORY_ROOT,
        env: process.env,
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
