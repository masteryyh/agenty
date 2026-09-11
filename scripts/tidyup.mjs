import { spawnSync } from "node:child_process";

import { REPOSITORY_ROOT, workspaceModules } from "./workspaces.mjs";

const GO_TIDYUP_COMMANDS = [
    ["fmt", "./..."],
    ["vet", "./..."],
    ["mod", "tidy"],
];

export function resolveTidyupPlan(
    repositoryRoot = REPOSITORY_ROOT,
    moduleName,
) {
    const modules = workspaceModules("go.mod", repositoryRoot).filter(
        (module) => !moduleName || module.name === moduleName,
    );
    if (moduleName && modules.length === 0) {
        throw new Error(`Go module not found in workspace: ${moduleName}`);
    }

    return modules.flatMap((module) =>
        GO_TIDYUP_COMMANDS.map((args) => ({
            args,
            command: "go",
            cwd: module.directory,
            label: `${module.name}: go ${args.join(" ")}`,
        })),
    );
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

export function runTidyup(repositoryRoot = REPOSITORY_ROOT, moduleName) {
    for (const step of resolveTidyupPlan(repositoryRoot, moduleName)) {
        console.log(`\n==> ${step.label}`);
        const result = spawnSync(step.command, step.args, {
            cwd: step.cwd,
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
        process.exitCode = runTidyup(REPOSITORY_ROOT, process.argv[2]?.trim());
    } catch (error) {
        console.error(error instanceof Error ? error.message : error);
        process.exitCode = 1;
    }
}
