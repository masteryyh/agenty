import { spawnSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { resolveBuildVersion } from "../../../scripts/build-version.mjs";

const PACKAGE_ROOT = resolve(import.meta.dirname, "..");

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

function run() {
    const environment = {
        ...process.env,
        AGENTY_VERSION: resolveBuildVersion(),
    };
    const cargo = spawnSync("cargo", ["build", "--release"], {
        cwd: PACKAGE_ROOT,
        env: environment,
        stdio: "inherit",
    });
    const cargoExitCode = exitCode("cargo build", cargo);
    if (cargoExitCode !== 0) {
        return cargoExitCode;
    }

    const pack = spawnSync("bun", ["run", "scripts/pack.ts"], {
        cwd: PACKAGE_ROOT,
        env: environment,
        stdio: "inherit",
    });
    return exitCode("bootstrap pack", pack);
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

