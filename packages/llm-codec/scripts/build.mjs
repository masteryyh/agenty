import { spawnSync } from "node:child_process";
import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { resolveArch, resolveOS } from "../../../scripts/platform.mjs";

const packageRoot = resolve(import.meta.dirname, "..");

export function resolveCodecTarget(environment = process.env, hostPlatform = process.platform, hostArch = process.arch) {
    const os = resolveOS({ ...environment, OS: environment.GOOS || environment.OS }, hostPlatform);
    const arch = resolveArch({ ...environment, ARCH: environment.GOARCH || environment.ARCH }, hostArch);
    const cpu = arch === "amd64" ? "x86_64" : "aarch64";
    if (os === "windows" && arch !== "amd64") {
        throw new Error("llm-codec Windows cgo builds support amd64 with MinGW/LLVM-MinGW");
    }
    const target = `${cpu}-${os === "macos" ? "apple-darwin" : os === "windows" ? "pc-windows-gnu" : "unknown-linux-gnu"}`;
    return { arch, os, target };
}

export function buildCodec(environment = process.env) {
    const { target } = resolveCodecTarget(environment);
    const result = spawnSync("cargo", ["rustc", "--locked", "--release", "--target", target, "--lib", "--message-format=json", "--", "--print", "native-static-libs"], {
        cwd: packageRoot,
        encoding: "utf8",
        env: environment,
        maxBuffer: 16 * 1024 * 1024,
    });
    if (result.error) {
        throw result.error;
    }
    if (result.status !== 0) {
        process.stderr.write(result.stderr || result.stdout);
        throw new Error(`llm-codec build failed for ${target}`);
    }
    const messages = result.stdout.split("\n").filter(Boolean).map((line) => JSON.parse(line));
    const script = messages.find((message) => message.reason === "build-script-executed" && message.package_id.includes("llm-codec"));
    const artifact = messages.find((message) => message.reason === "compiler-artifact" && message.target.name === "llm_codec");
    const library = artifact?.filenames.find((path) => path.endsWith(".a"));
    if (!script || !library) {
        throw new Error("cargo did not report llm-codec header or static library");
    }
    const diagnostic = messages.filter((message) => message.reason === "compiler-message").map((message) => message.message.message).join("\n");
    const nativeFlags = /native-static-libs:\s*([^\r\n]*)/u.exec(`${diagnostic}\n${result.stderr}`)?.[1]?.trim();
    const destination = join(packageRoot, "bin", target);
    mkdirSync(destination, { recursive: true });
    mkdirSync(join(packageRoot, "include"), { recursive: true });
    copyFileSync(library, join(destination, "libllm_codec.a"));
    const header = readFileSync(join(script.out_dir, "llm_codec.h"));
    writeFileSync(join(packageRoot, "include/llm_codec.h"), header);
    if (nativeFlags) {
        writeFileSync(join(destination, "native-static-libs.txt"), nativeFlags);
    } else {
        readFileSync(join(destination, "native-static-libs.txt"), "utf8");
    }
    console.log(`llm-codec built -> ${destination}`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
    buildCodec();
}
