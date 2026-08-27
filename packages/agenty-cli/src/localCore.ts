import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { delimiter, dirname, join, resolve } from "node:path";

import { StdioRPCClient } from "./core/rpc";

export const MANAGED_BIN_DIR = join(homedir(), ".agenty", "bin");

export const MANAGED_CORE_PATH = join(
    MANAGED_BIN_DIR,
    process.platform === "win32" ? "core.exe" : "core",
);

const REPO_CORE_PATH = resolve(
    import.meta.dir,
    `../../agenty-core/bin/${process.platform === "win32" ? "agenty-core.exe" : "agenty-core"}`,
);

export interface CorePathCandidates {
    override?: string;
    repoBin: string;
    managedBin: string;
}

export function pickCorePath(
    candidates: CorePathCandidates,
    exists: (path: string) => boolean = existsSync,
): string | null {
    if (candidates.override) {
        return candidates.override;
    }
    if (exists(candidates.repoBin)) {
        return candidates.repoBin;
    }
    if (exists(candidates.managedBin)) {
        return candidates.managedBin;
    }
    return null;
}

export function prependCoreDirectoryToPath(
    binary: string,
    currentPath?: string,
    managedBinDirectory = MANAGED_BIN_DIR,
    pathDelimiter = delimiter,
): string {
    const entries = [
        dirname(binary),
        managedBinDirectory,
        ...(currentPath?.split(pathDelimiter) ?? []),
    ].filter(Boolean);
    return Array.from(new Set(entries)).join(pathDelimiter);
}

export interface LocalCore {
    rpc: StdioRPCClient;
    stop: () => Promise<void>;
}

export async function startLocalCore(options: { dataDir?: string } = {}): Promise<LocalCore> {
    const binary = pickCorePath({
        override: process.env.AGENTY_CORE_BIN,
        repoBin: REPO_CORE_PATH,
        managedBin: MANAGED_CORE_PATH,
    });
    if (!binary) {
        throw new Error(
            "agenty core not found; start through the agenty launcher, set AGENTY_CORE_BIN, or build packages/agenty-core first",
        );
    }

    const subprocess = Bun.spawn([binary], {
        stdin: "pipe",
        stdout: "pipe",
        stderr: "pipe",
        env: {
            ...process.env,
            PATH: prependCoreDirectoryToPath(binary, process.env.PATH),
            ...(options.dataDir ? { AGENTY_DATA_DIR: options.dataDir } : {}),
        },
    });
    const errors: string[] = [];
    void (async () => {
        const decoder = new TextDecoder();
        for await (const chunk of subprocess.stderr) {
            errors.push(decoder.decode(chunk, { stream: true }));
        }
    })();

    const rpc = new StdioRPCClient(subprocess.stdin, subprocess.stdout);
    let stopped = false;
    return {
        rpc,
        stop: async () => {
            if (stopped) {
                return;
            }
            stopped = true;
            rpc.close();
            await subprocess.stdin.end();
            const exitCode = await subprocess.exited;
            if (exitCode !== 0) {
                throw new Error(`agenty core exited with code ${exitCode}: ${errors.join("").trim()}`);
            }
        },
    };
}
