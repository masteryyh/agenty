import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { join } from "node:path";

import { packageDir } from "./runtime.mjs";

process.chdir(packageDir);
execFileSync(process.execPath, ["scripts/generate.mjs", "--check"], { stdio: "inherit" });
execFileSync(process.execPath, ["node_modules/typescript/bin/tsc", "--noEmit"], { stdio: "inherit" });
execFileSync(process.execPath, ["node_modules/vite/bin/vite.js", "build"], { stdio: "inherit" });
mkdirSync("bin", { recursive: true });
const windows = (process.env.GOOS ?? process.platform) === "windows" || (!process.env.GOOS && process.platform === "win32");
execFileSync("go", ["build", "-tags", "production", "-ldflags", "-X github.com/masteryyh/agenty-inspector/internal/inspection.BuildVersion=" + (process.env.AGENTY_VERSION ?? "dev"), "-o", join("bin", windows ? "agenty-inspector.exe" : "agenty-inspector"), "./cmd/agenty-inspector"], { stdio: "inherit" });
