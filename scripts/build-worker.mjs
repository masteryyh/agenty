import { spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, renameSync, rmSync } from "node:fs";
import { resolve } from "node:path";

import { executableExtension, resolveArch, resolveOS } from "./platform.mjs";

const root = resolve(import.meta.dirname, "..");
const backend = process.argv[2];
const os = resolveOS();
const arch = resolveArch();
const hostOS = resolveOS({}, process.platform);
const hostArch = resolveArch({}, process.arch);

function run(command, args, capture = false) {
    const result = spawnSync(command, args, {
        cwd: root,
        stdio: capture ? "pipe" : "inherit",
        encoding: "utf8",
    });
    if (result.error) {
        throw result.error;
    }
    if (result.status !== 0) {
        throw new Error(`${command} failed (${result.status ?? result.signal})${result.stderr ? `: ${result.stderr}` : ""}`);
    }
    return result.stdout?.trim();
}

function build() {
    if (backend !== "mlx" && backend !== "llamaCpp") {
        throw new Error("expected mlx or llamaCpp worker backend");
    }
    if (os !== hostOS || arch !== hostArch) {
        console.log(`Skipping ${backend} worker: target ${os}/${arch} differs from host ${hostOS}/${hostArch}`);
        return;
    }
    if (backend === "mlx" && (os !== "macos" || arch !== "arm64")) {
        console.log("Skipping MLX worker: requires macOS arm64");
        return;
    }
    const directory = resolve(root, "packages", backend === "mlx" ? "mlx-worker" : "llama-cpp-worker");
    const bin = resolve(directory, "bin");
    if (backend === "llamaCpp") {
        const probe = spawnSync("cmake", ["--version"], { stdio: "pipe" });
        if (probe.error && probe.error.code !== "ENOENT") {
            throw probe.error;
        }
        if (!probe.error && probe.status !== 0) {
            throw new Error("the installed CMake cannot start");
        }
        const command = probe.error ? "uv" : "cmake";
        const prefix = probe.error ? ["tool", "run", "--from", "cmake==4.4.4", "cmake"] : [];
        const buildDirectory = resolve(directory, "build");
        run(command, [...prefix, "-S", directory, "-B", buildDirectory, "-DCMAKE_BUILD_TYPE=Release"]);
        run(command, [...prefix, "--build", buildDirectory, "--config", "Release", "--parallel", "4"]);
        const name = `llama-cpp-worker${executableExtension(os)}`;
        const binary = [resolve(buildDirectory, name), resolve(buildDirectory, "Release", name)]
            .find((candidate) => existsSync(candidate));
        if (!binary) {
            throw new Error(`native worker output is missing: ${name}`);
        }
        mkdirSync(resolve(bin, "licenses"), { recursive: true });
        copyFileSync(binary, resolve(bin, name));
        copyFileSync(resolve(buildDirectory, "_deps/llama-src/LICENSE"), resolve(bin, "licenses/llama.cpp-LICENSE"));
        copyFileSync(resolve(buildDirectory, "_deps/json-src/LICENSE.MIT"), resolve(bin, "licenses/nlohmann-json-LICENSE"));
        return;
    }

    run("uv", ["python", "install", "3.12.13"]);
    const python = run("uv", ["python", "find", "--managed-python", "3.12.13"], true);
    const runtime = run(python, ["-I", "-c", "import sys; print(sys.base_prefix)"], true);
    const stage = mkdtempSync(resolve(directory, ".worker-build-"));
    try {
        run(python, ["scripts/worker-bundle.py", "mlx", "--version", "dev", "--python-runtime", runtime, "--directory", stage]);
        rmSync(bin, { recursive: true, force: true });
        renameSync(stage, bin);
    } finally {
        rmSync(stage, { recursive: true, force: true });
    }
}

try {
    build();
} catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
}
