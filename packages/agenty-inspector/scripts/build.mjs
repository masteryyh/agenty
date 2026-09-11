import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { join } from "node:path";

import { resolveBuildVersion } from "../../../scripts/build-version.mjs";
import { packageDir } from "./runtime.mjs";

const version = resolveBuildVersion();
const environment = {
    ...process.env,
    AGENTY_VERSION: version,
};

process.chdir(packageDir);
execFileSync(process.execPath, ["scripts/generate.mjs", "--check"], { env: environment, stdio: "inherit" });
execFileSync(process.execPath, ["node_modules/typescript/bin/tsc", "--noEmit"], { env: environment, stdio: "inherit" });
execFileSync(process.execPath, ["node_modules/vite/bin/vite.js", "build"], { env: environment, stdio: "inherit" });
mkdirSync("bin", { recursive: true });
const windows = (process.env.GOOS ?? process.platform) === "windows" || (!process.env.GOOS && process.platform === "win32");
execFileSync("go", ["build", "-tags", "production", "-ldflags", "-X github.com/masteryyh/agenty-inspector/internal/inspection.BuildVersion=" + version, "-o", join("bin", windows ? "agenty-inspector.exe" : "agenty-inspector"), "./cmd/agenty-inspector"], { env: environment, stdio: "inherit" });
