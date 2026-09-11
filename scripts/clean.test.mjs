import assert from "node:assert/strict";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test from "node:test";

import { resolveCleanupPaths, runCleanup } from "./clean.mjs";

const REPOSITORY_ROOT = resolve(import.meta.dirname, "..");

test("clean covers root and workspace build artifacts without duplicates", () => {
    const paths = resolveCleanupPaths("clean", REPOSITORY_ROOT);

    assert.equal(paths.length, new Set(paths).size);
    assert.ok(paths.includes(resolve(REPOSITORY_ROOT, "bin")));
    assert.ok(paths.includes(resolve(REPOSITORY_ROOT, "dist")));
    assert.ok(paths.includes(resolve(REPOSITORY_ROOT, "packages/agenty-core/bin")));
    assert.ok(paths.includes(resolve(REPOSITORY_ROOT, "packages/agenty-bootstrap/target")));
    assert.ok(paths.includes(resolve(REPOSITORY_ROOT, "packages/agenty-inspector/web/dist")));
    assert.ok(!paths.includes(resolve(REPOSITORY_ROOT, "node_modules")));
});

test("deepclean extends clean exactly once and preserves environment files", () => {
    const cleanPaths = resolveCleanupPaths("clean", REPOSITORY_ROOT);
    const deepcleanPaths = resolveCleanupPaths("deepclean", REPOSITORY_ROOT);

    assert.equal(deepcleanPaths.length, new Set(deepcleanPaths).size);
    for (const path of cleanPaths) {
        assert.ok(deepcleanPaths.includes(path));
    }
    assert.ok(deepcleanPaths.includes(resolve(REPOSITORY_ROOT, ".pnpm-store")));
    assert.ok(deepcleanPaths.includes(resolve(REPOSITORY_ROOT, ".turbo")));
    assert.ok(deepcleanPaths.includes(resolve(REPOSITORY_ROOT, "node_modules")));
    assert.ok(!deepcleanPaths.includes(resolve(REPOSITORY_ROOT, ".env")));
});

test("removes only the selected cleanup scope", () => {
    const repositoryRoot = mkdtempSync(join(tmpdir(), "agenty-clean-"));
    const workspaceRoot = resolve(repositoryRoot, "packages/agenty-core");
    try {
        mkdirSync(workspaceRoot, { recursive: true });
        writeFileSync(
            resolve(workspaceRoot, "package.json"),
            JSON.stringify({ name: "agenty-core" }),
        );
        for (const path of [
            "bin/core",
            "target/release/core",
            "node_modules/core",
            ".turbo/turbo-build.log",
        ]) {
            const target = resolve(workspaceRoot, path);
            mkdirSync(resolve(target, ".."), { recursive: true });
            writeFileSync(target, "generated");
        }
        mkdirSync(resolve(repositoryRoot, ".pnpm-store"), { recursive: true });
        mkdirSync(resolve(repositoryRoot, "node_modules"), { recursive: true });
        writeFileSync(resolve(repositoryRoot, ".env"), "AGENTY_VERSION=dev\n");
        mkdirSync(resolve(repositoryRoot, "dist"), { recursive: true });

        runCleanup("clean", repositoryRoot);
        assert.equal(existsSync(resolve(repositoryRoot, "dist")), false);
        assert.equal(existsSync(resolve(workspaceRoot, "bin")), false);
        assert.equal(existsSync(resolve(workspaceRoot, "target")), false);
        assert.equal(existsSync(resolve(workspaceRoot, "node_modules")), true);
        assert.equal(existsSync(resolve(workspaceRoot, ".turbo")), true);

        runCleanup("deepclean", repositoryRoot);
        assert.equal(existsSync(resolve(repositoryRoot, "node_modules")), false);
        assert.equal(existsSync(resolve(repositoryRoot, ".pnpm-store")), false);
        assert.equal(existsSync(resolve(workspaceRoot, ".turbo")), false);
        assert.equal(existsSync(resolve(repositoryRoot, ".env")), true);
        assert.equal(readFileSync(resolve(repositoryRoot, ".env"), "utf8"), "AGENTY_VERSION=dev\n");
    } finally {
        rmSync(repositoryRoot, { force: true, recursive: true });
    }
});
