import { mkdir, mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, describe, expect, test } from "bun:test";

import {
    appendInputHistory,
    decodeInputHistoryEntry,
    encodeInputHistoryEntry,
    loadInputHistory,
    resolveInputHistoryPath,
} from "./inputHistory";

const temporaryDirectories: string[] = [];

afterEach(async () => {
    for (const directory of temporaryDirectories.splice(0)) {
        await rm(directory, { recursive: true, force: true });
    }
});

async function createHistoryPath(): Promise<string> {
    const directory = await mkdtemp(join(tmpdir(), "agenty-input-history-"));
    temporaryDirectories.push(directory);
    return join(directory, "input-history");
}

describe("input history", () => {
    test("round-trips arbitrary input through one encoded line", () => {
        const input = "中文\n100% \"quoted\" [$skill](/tmp/a path)";
        const encoded = encodeInputHistoryEntry(input);

        expect(encoded).not.toContain("\n");
        expect(decodeInputHistoryEntry(encoded)).toBe(input);
    });

    test("appends and reloads entries across instances", async () => {
        const path = await createHistoryPath();
        const entries = ["/model openai/gpt", "hello\nworld", "?"];

        for (const entry of entries) {
            await appendInputHistory(path, entry);
        }

        expect((await loadInputHistory(path)).entries).toEqual(entries);
        expect((await readFile(path, "utf8")).split("\n")).toHaveLength(entries.length + 1);
    });

    test("skips malformed lines while retaining valid entries", async () => {
        const path = await createHistoryPath();
        await appendInputHistory(path, "valid");
        await Bun.write(path, `${await readFile(path, "utf8")}%E0%A4\n`);

        await expect(loadInputHistory(path)).resolves.toEqual({
            entries: ["valid"],
            invalidLines: 1,
        });
    });

    test("resolves the history below the configured data directory", () => {
        expect(resolveInputHistoryPath("/tmp/agenty-data")).toBe("/tmp/agenty-data/input-history");
    });

    test("reports a write error without changing an existing directory target", async () => {
        const path = await createHistoryPath();
        await mkdir(path);

        await expect(appendInputHistory(path, "blocked")).rejects.toThrow("failed to write input history");
    });

    test("creates a private history file", async () => {
        const path = await createHistoryPath();
        await appendInputHistory(path, "secret");
        if (process.platform === "win32") {
            return;
        }
        const mode = (await Bun.file(path).stat()).mode & 0o777;

        expect(mode).toBe(0o600);
    });
});
