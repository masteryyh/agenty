import { mkdir, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, describe, expect, test } from "bun:test";

import { loadInputHistory } from "../history/inputHistory";
import { useAppStore } from "./store";

const temporaryDirectories: string[] = [];

afterEach(async () => {
    useAppStore.setState({
        inputHistoryPath: null,
        promptHistory: [],
        toast: null,
    });
    for (const directory of temporaryDirectories.splice(0)) {
        await rm(directory, { recursive: true, force: true });
    }
});

async function historyPath(): Promise<string> {
    const directory = await mkdtemp(join(tmpdir(), "agenty-store-history-"));
    temporaryDirectories.push(directory);
    return join(directory, "input-history");
}

describe("input history store", () => {
    test("records input on disk before exposing it to navigation", async () => {
        const path = await historyPath();
        useAppStore.setState({ inputHistoryPath: path, promptHistory: [] });

        expect(await useAppStore.getState().recordInput("/status")).toBe(true);
        expect(useAppStore.getState().promptHistory).toEqual(["/status"]);
        expect((await loadInputHistory(path)).entries).toEqual(["/status"]);
    });

    test("keeps navigation history unchanged when persistence fails", async () => {
        const path = await historyPath();
        await mkdir(path);
        useAppStore.setState({ inputHistoryPath: path, promptHistory: ["existing"] });

        expect(await useAppStore.getState().recordInput("lost")).toBe(false);
        expect(useAppStore.getState().promptHistory).toEqual(["existing"]);
        expect(useAppStore.getState().toast).toMatchObject({ error: true });
    });
});
