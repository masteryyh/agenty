import { existsSync, rmSync } from "node:fs";
import { relative, resolve } from "node:path";

import { REPOSITORY_ROOT, workspacePackages } from "./workspaces.mjs";

const ROOT_BUILD_PATHS = ["bin", "dist"];
const MODULE_BUILD_PATHS = ["bin", "dist", "target", "web/dist"];
const ROOT_DEEPCLEAN_PATHS = [
    ".cache",
    ".eslintcache",
    ".pnpm-store",
    ".turbo",
    "coverage",
    "node_modules",
];
const MODULE_DEEPCLEAN_PATHS = [
    ".cache",
    ".eslintcache",
    ".turbo",
    "agenty.log",
    "coverage",
    "node_modules",
];
const STATIC_DEEPCLEAN_PATHS = ["packages/agenty-cli/bun.lockb"];

function unique(values) {
    return [...new Set(values)];
}

function joinWorkspacePath(relativeDirectory, path) {
    return `${relativeDirectory}/${path}`;
}

export function resolveCleanupPaths(
    mode = "clean",
    repositoryRoot = REPOSITORY_ROOT,
) {
    if (mode !== "clean" && mode !== "deepclean") {
        throw new Error(`unsupported cleanup mode: ${mode}`);
    }

    const relativePaths = [...ROOT_BUILD_PATHS];
    const workspaces = workspacePackages(repositoryRoot);
    for (const workspace of workspaces) {
        relativePaths.push(
            ...MODULE_BUILD_PATHS.map((path) => joinWorkspacePath(workspace.relativeDirectory, path)),
        );
    }

    if (mode === "deepclean") {
        relativePaths.push(...ROOT_DEEPCLEAN_PATHS, ...STATIC_DEEPCLEAN_PATHS);
        for (const workspace of workspaces) {
            relativePaths.push(
                ...MODULE_DEEPCLEAN_PATHS.map((path) =>
                    joinWorkspacePath(workspace.relativeDirectory, path),
                ),
            );
        }
    }

    return unique(relativePaths.map((path) => resolve(repositoryRoot, path)));
}

export function runCleanup(mode = "clean", repositoryRoot = REPOSITORY_ROOT) {
    const paths = resolveCleanupPaths(mode, repositoryRoot);
    let removedCount = 0;
    for (const path of paths) {
        if (!existsSync(path)) {
            continue;
        }
        rmSync(path, { force: true, recursive: true });
        removedCount += 1;
        console.log(`removed ${relative(repositoryRoot, path)}`);
    }
    console.log(`${mode}: removed ${removedCount} path(s)`);
}

if (import.meta.main) {
    try {
        runCleanup(process.argv[2]?.trim() || "clean");
    } catch (error) {
        console.error(error instanceof Error ? error.message : error);
        process.exitCode = 1;
    }
}
