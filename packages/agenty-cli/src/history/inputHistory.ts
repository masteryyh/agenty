import { appendFile, chmod, mkdir, readFile } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, isAbsolute, join, resolve } from "node:path";

export interface LoadedInputHistory {
    entries: string[];
    invalidLines: number;
}

export function resolveInputHistoryPath(dataDir?: string): string {
    const configured = dataDir ?? process.env.AGENTY_DATA_DIR;
    const root = configured && configured.trim() !== ""
        ? configured
        : join(homedir(), ".agenty");
    return join(isAbsolute(root) ? root : resolve(root), "input-history");
}

export function encodeInputHistoryEntry(text: string): string {
    return encodeURIComponent(text);
}

export function decodeInputHistoryEntry(line: string): string {
    return decodeURIComponent(line);
}

export async function loadInputHistory(path: string): Promise<LoadedInputHistory> {
    let content: string;
    try {
        content = await readFile(path, "utf8");
    } catch (error) {
        if ((error as NodeJS.ErrnoException).code === "ENOENT") {
            return { entries: [], invalidLines: 0 };
        }
        throw new Error(`failed to read input history: ${(error as Error).message}`, { cause: error });
    }

    const entries: string[] = [];
    let invalidLines = 0;
    for (const line of content.split("\n")) {
        if (line === "") {
            continue;
        }
        try {
            entries.push(decodeInputHistoryEntry(line));
        } catch {
            invalidLines += 1;
        }
    }
    return { entries, invalidLines };
}

export async function appendInputHistory(path: string, text: string): Promise<void> {
    try {
        await mkdir(dirname(path), { recursive: true, mode: 0o700 });
        await appendFile(path, `${encodeInputHistoryEntry(text)}\n`, {
            encoding: "utf8",
            mode: 0o600,
            flag: "a",
        });
        await chmod(path, 0o600);
    } catch (error) {
        throw new Error(`failed to write input history: ${(error as Error).message}`, { cause: error });
    }
}
