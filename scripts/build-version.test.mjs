import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { resolve } from "node:path";
import test from "node:test";

import { resolveBuildVersion } from "./build-version.mjs";

function withRepositoryDotEnv(contents, callback) {
    const repositoryRoot = mkdtempSync(resolve(tmpdir(), "agenty-build-version-"));
    try {
        writeFileSync(resolve(repositoryRoot, ".env"), contents);
        return callback(repositoryRoot);
    } finally {
        rmSync(repositoryRoot, { recursive: true, force: true });
    }
}

test("prefers AGENTY_VERSION from the process environment", () => {
    withRepositoryDotEnv("AGENTY_VERSION=from-file\n", (repositoryRoot) => {
        assert.equal(
            resolveBuildVersion({ AGENTY_VERSION: " from-process " }, repositoryRoot),
            "from-process",
        );
    });
});

test("reads AGENTY_VERSION from the root .env", () => {
    withRepositoryDotEnv("export AGENTY_VERSION=\"from-file\"\n", (repositoryRoot) => {
        assert.equal(resolveBuildVersion({}, repositoryRoot), "from-file");
    });
});

test("falls back to dev when no version is configured", () => {
    const repositoryRoot = mkdtempSync(resolve(tmpdir(), "agenty-build-version-"));
    try {
        assert.equal(resolveBuildVersion({}, repositoryRoot), "dev");
    } finally {
        rmSync(repositoryRoot, { recursive: true, force: true });
    }
});

