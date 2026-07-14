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

// Ensures src/_embedded/agenty-bin exists before `bun run src/index.tsx` in dev
// mode. The file is imported with `with { type: "file" }` in localServer.ts, so
// it must exist at module-load time even when not compiling a single binary.
import { existsSync, copyFileSync, mkdirSync } from "node:fs";
import { resolve, join } from "node:path";

const ROOT = resolve(import.meta.dir, "../..");
const EMBEDDED_DIR = resolve(import.meta.dir, "../src/_embedded");
const EMBEDDED_BIN = join(EMBEDDED_DIR, "agenty-bin");
const GO_BIN = join(ROOT, "bin/agenty");

function isRealBinary(path: string): boolean {
	try {
		return existsSync(path) && Bun.file(path).size > 1_000_000;
	} catch {
		return false;
	}
}

if (isRealBinary(EMBEDDED_BIN)) {
	process.exit(0);
}

mkdirSync(EMBEDDED_DIR, { recursive: true });
if (isRealBinary(GO_BIN)) {
	copyFileSync(GO_BIN, EMBEDDED_BIN);
	process.exit(0);
}

// Build the Go binary first, then copy it into place.
const build = Bun.spawn(["make", "build"], {
	cwd: ROOT,
	stdout: "inherit",
	stderr: "inherit",
});
const code = await build.exited;
if (code !== 0) process.exit(code);
if (!isRealBinary(GO_BIN)) {
	console.error("`make build` did not produce bin/agenty");
	process.exit(1);
}
copyFileSync(GO_BIN, EMBEDDED_BIN);
console.log("prepared src/_embedded/agenty-bin for dev mode");
