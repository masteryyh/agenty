import { spawn } from "node:child_process";
import { resolve } from "node:path";

import { packageDir, runtimeEnv } from "./runtime.mjs";

const env = runtimeEnv();
process.chdir(packageDir);
const binary = resolve("bin", process.platform === "win32" ? "agenty-inspector.exe" : "agenty-inspector");
const child = spawn(binary, process.argv.slice(2).filter((arg) => arg !== "--"), { stdio: "inherit", env });
for (const signal of ["SIGINT", "SIGTERM"]) {
    process.on(signal, () => child.kill(signal));
}

child.on("error", (error) => {
    console.error(error.message + "\nRun pnpm inspector:build first.");
    process.exitCode = 1;
});
child.on("exit", (code) => {
    process.exitCode = code ?? 1;
});
