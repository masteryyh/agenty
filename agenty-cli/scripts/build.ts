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

import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { CLI_VERSION } from "../src/version";

function readCliVersion(): string {
	const fromEnv = process.env.AGENTY_CLI_VERSION;
	if (fromEnv && fromEnv !== "undefined") return fromEnv;
	const envPath = resolve(import.meta.dir, "../.env");
	if (existsSync(envPath)) {
		try {
			const text = readFileSync(envPath, "utf8");
			const match = text.match(/^AGENTY_CLI_VERSION\s*=\s*(.+?)\s*$/m);
			if (match) return match[1].replace(/^["']|["']$/g, "");
		} catch {
			// fall through
		}
	}
	return CLI_VERSION;
}

const version = readCliVersion();

const result = await Bun.build({
	entrypoints: [resolve(import.meta.dir, "../src/index.tsx")],
	outdir: resolve(import.meta.dir, "../dist"),
	target: "bun",
	packages: "external",
	define: {
		"process.env.AGENTY_CLI_VERSION": JSON.stringify(version),
	},
});

if (!result.success) {
	for (const log of result.logs) console.error(log);
	process.exit(1);
}

console.log(`agenty-cli built (cli version ${version}) -> dist/`);
