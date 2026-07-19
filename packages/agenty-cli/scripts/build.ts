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

import { mkdirSync } from "node:fs";
import { resolve, join } from "node:path";

import { resolveArch, resolveBunTarget, resolveOpenTUILibc, resolveOS } from "./target";

const PKG = resolve(import.meta.dir, "..");
const DIST = join(PKG, "dist");

function readCliVersion(): string {
	const envVal = process.env.AGENTY_CLI_VERSION;
	if (envVal && envVal !== "undefined") {
		return envVal;
	}
	return "dev";
}

const os = resolveOS();
const arch = resolveArch();
const bunTarget = resolveBunTarget(os, arch);
const opentuiLibc = resolveOpenTUILibc(os);

const version = readCliVersion();
mkdirSync(DIST, { recursive: true });
const outfile = join(DIST, `agenty-cli-${os}-${arch}${os === "windows" ? ".exe" : ""}`);

const result = await Bun.build({
	entrypoints: [join(PKG, "src/index.tsx")],
	compile: { outfile, target: bunTarget },
	target: "bun",
	define: {
		"process.env.AGENTY_CLI_VERSION": JSON.stringify(version),
		...(opentuiLibc
			? { "process.env.OPENTUI_LIBC": JSON.stringify(opentuiLibc) }
			: {}),
	},
});
if (!result.success) {
	for (const log of result.logs) {
		console.error(log.message);
	}
	process.exit(1);
}

console.log(
	`agenty-cli single executable built -> ${outfile} (${bunTarget}${opentuiLibc ? `, ${opentuiLibc}` : ""})`,
);
