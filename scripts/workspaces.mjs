import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";

export const REPOSITORY_ROOT = resolve(import.meta.dirname, "..");

export function workspacePackages(repositoryRoot = REPOSITORY_ROOT) {
    const packagesRoot = resolve(repositoryRoot, "packages");
    if (!existsSync(packagesRoot)) {
        return [];
    }

    return readdirSync(packagesRoot, { withFileTypes: true })
        .filter((entry) => entry.isDirectory())
        .map((entry) => ({
            directory: join(packagesRoot, entry.name),
            relativeDirectory: relative(repositoryRoot, join(packagesRoot, entry.name)),
        }))
        .filter(({ directory }) => existsSync(join(directory, "package.json")))
        .map(({ directory, relativeDirectory }) => {
            const packageJson = JSON.parse(readFileSync(join(directory, "package.json"), "utf8"));
            return {
                directory,
                name: packageJson.name,
                relativeDirectory,
            };
        })
        .filter((workspace) => typeof workspace.name === "string" && workspace.name !== "")
        .sort((left, right) => left.name.localeCompare(right.name));
}

export function workspaceModules(manifest, repositoryRoot = REPOSITORY_ROOT) {
    return workspacePackages(repositoryRoot).filter(({ directory }) =>
        existsSync(resolve(directory, manifest)),
    );
}
