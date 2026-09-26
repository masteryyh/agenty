import { resolve } from "node:path";

import { packageManagerCommand, spawnSyncCommand } from "./command.mjs";
import { REPOSITORY_ROOT, workspaceModules } from "./workspaces.mjs";

export function resolveUpdatePlan(
    repositoryRoot = REPOSITORY_ROOT,
    hostPlatform = process.platform,
) {
    const steps = [
        {
            args: ["update", "--recursive"],
            command: packageManagerCommand(hostPlatform),
            cwd: repositoryRoot,
            label: "pnpm workspace dependencies",
        },
    ];

    for (const module of workspaceModules("go.mod", repositoryRoot)) {
        steps.push(
            {
                args: ["get", "-u", "./..."],
                command: "go",
                cwd: module.directory,
                label: `${module.name} Go dependencies`,
            },
            {
                args: ["mod", "tidy"],
                command: "go",
                cwd: module.directory,
                label: `${module.name} Go module tidy`,
            },
        );
    }

    for (const module of workspaceModules("Cargo.toml", repositoryRoot)) {
        steps.push({
            args: ["update"],
            command: "cargo",
            cwd: module.directory,
            label: `${module.name} Cargo dependencies`,
        });
    }

    return steps;
}

function exitCode(step, result) {
    if (result.error) {
        throw result.error;
    }
    if (result.status === null) {
        const detail = result.signal ? ` (signal ${result.signal})` : "";
        throw new Error(`${step.label} did not return an exit code${detail}`);
    }
    return result.status;
}

export function runUpdate(repositoryRoot = REPOSITORY_ROOT, hostPlatform = process.platform) {
    for (const step of resolveUpdatePlan(repositoryRoot, hostPlatform)) {
        console.log(`\n==> ${step.label}`);
        const result = spawnSyncCommand(step.command, step.args, {
            cwd: resolve(step.cwd),
            stdio: "inherit",
        });
        const status = exitCode(step, result);
        if (status !== 0) {
            return status;
        }
    }
    return 0;
}

if (import.meta.main) {
    try {
        process.exitCode = runUpdate();
    } catch (error) {
        console.error(error instanceof Error ? error.message : error);
        process.exitCode = 1;
    }
}
