import { resolve } from "node:path";

import { resolveBuildVersion } from "./build-version.mjs";
import { packageManagerCommand, spawnSyncCommand } from "./command.mjs";

const REPOSITORY_ROOT = resolve(import.meta.dirname, "..");

export function resolveTurboPlan(
    task,
    filter,
    environment = process.env,
    hostPlatform = process.platform,
    repositoryRoot = REPOSITORY_ROOT,
) {
    const args = ["exec", "turbo", "run", task];
    if (filter) {
        args.push(`--filter=${filter}`);
    }

    return {
        args,
        buildEnvironment: {
            ...environment,
            AGENTY_VERSION: resolveBuildVersion(environment, repositoryRoot),
        },
        packageManager: packageManagerCommand(hostPlatform),
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
    const task = process.argv[2]?.trim() || "test";
    const filter = process.argv[3]?.trim();
    const plan = resolveTurboPlan(task, filter);
    const result = spawnSyncCommand(plan.packageManager, plan.args, {
        cwd: REPOSITORY_ROOT,
        env: plan.buildEnvironment,
        stdio: "inherit",
    });
    return exitCode(`turbo ${task}`, result);
}

if (import.meta.main) {
    try {
        process.exitCode = run();
    } catch (error) {
        console.error(error instanceof Error ? error.message : error);
        process.exitCode = 1;
    }
}
