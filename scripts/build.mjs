import { spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, readdirSync, statSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { resolveTurboPlan } from "./turbo.mjs";

const REPOSITORY_ROOT = resolve(import.meta.dirname, "..");

export function resolveBuildPlan(
    filter = "agenty-bootstrap",
    environment = process.env,
    hostPlatform = process.platform,
    repositoryRoot = REPOSITORY_ROOT,
) {
    const turboPlan = resolveTurboPlan("build", filter, environment, hostPlatform, repositoryRoot);
    return {
        buildArgs: turboPlan.args,
        buildEnvironment: turboPlan.buildEnvironment,
        copyBootstrap: filter === "agenty-bootstrap",
        packageManager: turboPlan.packageManager,
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

function copyBootstrapArtifacts() {
    const sourceDirectory = resolve(REPOSITORY_ROOT, "packages/agenty-bootstrap/bin");
    if (!existsSync(sourceDirectory)) {
        return;
    }

    const destinationDirectory = resolve(REPOSITORY_ROOT, "dist");
    mkdirSync(destinationDirectory, { recursive: true });
    for (const name of readdirSync(sourceDirectory)) {
        const source = resolve(sourceDirectory, name);
        if (statSync(source).isFile()) {
            copyFileSync(source, resolve(destinationDirectory, name));
        }
    }
}

function run() {
    const filter = process.argv[2]?.trim() || "agenty-bootstrap";
    const plan = resolveBuildPlan(filter);
    const build = spawnSync(plan.packageManager, plan.buildArgs, {
        cwd: REPOSITORY_ROOT,
        env: plan.buildEnvironment,
        stdio: "inherit",
    });
    const buildExitCode = exitCode("workspace build", build);
    if (buildExitCode !== 0) {
        return buildExitCode;
    }

    if (plan.copyBootstrap) {
        copyBootstrapArtifacts();
    }
    return 0;
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
