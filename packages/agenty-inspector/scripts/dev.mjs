import { spawn, spawnSync } from "node:child_process";
import { resolve } from "node:path";

import { packageDir, runtimeEnv } from "./runtime.mjs";

const env = runtimeEnv();
process.chdir(packageDir);
const binary = "bin/inspector-dev" + (process.platform === "win32" ? ".exe" : "");
const apiPort = process.env.INSPECTOR_API_PORT ?? "4318";
const build = spawnSync("go", ["build", "-o", binary, "./cmd/agenty-inspector"], { stdio: "inherit", env });
if (build.status !== 0) {
    process.exit(build.status ?? 1);
}
const children = [
    spawn(resolve(binary), ["--port", apiPort, "--dev-origin", "http://127.0.0.1:5173"], { stdio: "inherit", env }),
    spawn(process.execPath, ["node_modules/vite/bin/vite.js"], { stdio: "inherit", env }),
];
let stopping = false;
function stop(code) {
    if (stopping) {
        return;
    }
    stopping = true;
    process.exitCode = code;
    for (const child of children) {
        child.kill("SIGTERM");
    }
}
for (const child of children) {
    child.on("error", (error) => {
        console.error(error);
        stop(1);
    });
    child.on("exit", (code) => stop(code ?? 0));
}
process.on("SIGINT", () => stop(0));
process.on("SIGTERM", () => stop(0));
