import { spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const PACKAGE_ROOT = resolve(import.meta.dirname, "..");

function targetForGoOS(rawOS) {
    const lowerOS = rawOS.toLowerCase();
    if (lowerOS === "darwin" || lowerOS === "macos") {
        return { artifactOS: "macos", extension: "", goOS: "darwin" };
    }
    if (lowerOS.startsWith("win")) {
        return { artifactOS: "windows", extension: ".exe", goOS: "windows" };
    }
    if (lowerOS === "linux") {
        return { artifactOS: "linux", extension: "", goOS: "linux" };
    }
    throw new Error(`unsupported Go operating system: ${rawOS}`);
}

function hostGoOS(hostPlatform) {
    if (hostPlatform === "darwin") {
        return "darwin";
    }
    if (hostPlatform === "win32") {
        return "windows";
    }
    if (hostPlatform === "linux") {
        return "linux";
    }
    throw new Error(`unsupported host operating system: ${hostPlatform}`);
}

function outputPath(packageRoot, path) {
    return isAbsolute(path) ? path : resolve(packageRoot, path);
}

function executableName(name, extension) {
    if (extension === "" || name.toLowerCase().endsWith(extension)) {
        return name;
    }
    return `${name}${extension}`;
}

export function resolveCoreBuildPlan(
    environment = process.env,
    hostPlatform = process.platform,
    packageRoot = PACKAGE_ROOT,
) {
    const requestedGoOS = environment.GOOS?.trim() || hostGoOS(hostPlatform);
    const target = targetForGoOS(requestedGoOS);
    const outputDirectory = outputPath(packageRoot, environment.PACKAGE_DIR?.trim() || "bin");
    const coreName = executableName(environment.BIN_NAME?.trim() || "agenty-core", target.extension);
    const helperName = `apply_patch${target.extension}`;
    const repositoryRoot = resolve(packageRoot, "../..");
    return {
        corePath: join(outputDirectory, coreName),
        goArgs: ["build", "-o", join(outputDirectory, coreName), "./cmd"],
        helperDestination: join(outputDirectory, helperName),
        helperSource: join(repositoryRoot, "packages/patch-applier/target/release", helperName),
        outputDirectory,
        packageRoot,
        target,
    };
}

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
    const plan = resolveCoreBuildPlan();
    mkdirSync(plan.outputDirectory, { recursive: true });
    const build = spawnSync("go", plan.goArgs, {
        cwd: plan.packageRoot,
        env: process.env,
        stdio: "inherit",
    });
    const buildExitCode = exitCode("go build", build);
    if (buildExitCode !== 0) {
        return buildExitCode;
    }
    if (!existsSync(plan.helperSource)) {
        throw new Error(`apply_patch binary not found at ${plan.helperSource}`);
    }
    copyFileSync(plan.helperSource, plan.helperDestination);
    console.log(`agenty-core built -> ${plan.corePath}`);
    return 0;
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
