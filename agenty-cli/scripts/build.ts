/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import { existsSync, copyFileSync, mkdirSync, readFileSync } from "node:fs";
import { resolve, join } from "node:path";

const ROOT = resolve(import.meta.dir, "../..");
const PKG = resolve(import.meta.dir, "..");
const EMBEDDED_DIR = join(PKG, "src/_embedded");
const EMBEDDED_BIN = join(EMBEDDED_DIR, "agenty-bin");
const GO_BIN = join(ROOT, "bin/agenty");
const DIST = join(PKG, "dist");

function readCliVersion(): string {
	const fromEnv = process.env.AGENTY_CLI_VERSION;
	if (fromEnv && fromEnv !== "undefined") return fromEnv;
	const envPath = join(PKG, ".env");
	if (existsSync(envPath)) {
		try {
			const text = readFileSync(envPath, "utf8");
			const match = text.match(/^AGENTY_CLI_VERSION\s*=\s*(.+?)\s*$/m);
			if (match) return match[1].replace(/^["']|["']$/g, "");
		} catch {
			// fall through
		}
	}
	return "dev";
}

// 1. Build the agenty Go backend binary.
const build = Bun.spawn(["make", "build"], {
	cwd: ROOT,
	stdout: "inherit",
	stderr: "inherit",
});
const code = await build.exited;
if (code !== 0) {
	console.error(`\`make build\` failed with exit code ${code}`);
	process.exit(1);
}

// 2. Copy the binary into src/_embedded so `with { type: "file" }` can embed it
//    into the compiled single executable.
mkdirSync(EMBEDDED_DIR, { recursive: true });
copyFileSync(GO_BIN, EMBEDDED_BIN);

// 3. Compile agenty-cli into a single standalone executable with the embedded
//    agenty server binary baked in.
const version = readCliVersion();
mkdirSync(DIST, { recursive: true });
const outfile = join(DIST, "agenty-cli");
const result = await Bun.build({
	entrypoints: [join(PKG, "src/index.tsx")],
	compile: { outfile },
	target: "bun",
	define: {
		"process.env.AGENTY_CLI_VERSION": JSON.stringify(version),
	},
});
if (!result.success) {
	for (const log of result.logs) console.error(log.message);
	process.exit(1);
}

console.log(
	`agenty-cli single executable built -> dist/agenty-cli (cli ${version}, embedded agenty server)`,
);
