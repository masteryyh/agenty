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

import {
	existsSync,
	copyFileSync,
	mkdirSync,
	readFileSync,
	readdirSync,
} from "node:fs";
import { resolve, join } from "node:path";

const PKG = resolve(import.meta.dir, "..");
const REPO_ROOT = resolve(import.meta.dir, "../../..");
const EMBEDDED_DIR = join(PKG, "src/_embedded");
const EMBEDDED_BIN = join(EMBEDDED_DIR, "agenty-bin");
const RUNTIME_BIN_DIR = join(REPO_ROOT, "packages/agenty-runtime/bin");
const DIST = join(PKG, "dist");

function readCliVersion(): string {
	const envVal = process.env.AGENTY_CLI_VERSION;
	if (envVal && envVal !== "undefined") {
		return envVal;
	}

	const envPath = join(PKG, ".env");
	if (existsSync(envPath)) {
		try {
			const text = readFileSync(envPath, "utf8");
			const match = text.match(/^AGENTY_CLI_VERSION\s*=\s*(.+?)\s*$/m);
			if (match) return match[1].replace(/^["']|["']$/g, "");
		} catch {}
	}
	return "dev";
}

function resolveArch(): string {
	const rawArch = process.env.ARCH?.trim();
	if (!rawArch) {
		return null;
	}

	const lowerArch = rawArch.toLowerCase();
	if (lowerArch === "x64" || lowerArch === "x86_64" || lowerArch === "amd64") {
		return "amd64";
	}
	if (lowerArch === "arm64" || lowerArch === "aarch64") {
		return "arm64";
	}
	return lowerArch;
}

function resolveOS(): string | null {
	const rawOS = process.env.OS?.trim();
	if (!rawOS) {
		return null;
	}

	const lowerOS = rawOS.toLowerCase();
	if (lowerOS === "darwin" || lowerOS === "macos") {
		return "macos";
	}
	if (lowerOS.startsWith("win")) {
		return "windows";
	}
	return lowerOS;
}

function findAgentyBinary(dir: string): string | null {
	let names: string[];
	try {
		names = readdirSync(dir);
	} catch {
		return null;
	}

	const hits = names
		.filter((n) => n.startsWith("agenty"))
		.map((n) => join(dir, n));
	if (hits.length === 0) {
		return null;
	}

	if (hits.length > 1) {
		console.error('multiple runtime binary found, exiting.');
		process.exit(1);
	}
	return hits[0];
}

function resolveRuntimeBinary(os: string | null, arch: string | null): string {
	if (os && arch) {
		const target = `${os}_${arch}`;
		const dir = join(RUNTIME_BIN_DIR, target);
		const bin = findAgentyBinary(dir);
		if (!bin) {
			console.error(`agenty-runtime binary not found for OS=${os} ARCH=${arch}\n`);
			process.exit(1);
		}
		return bin!;
	}

	const flat = join(RUNTIME_BIN_DIR, "agenty");
	if (!existsSync(flat)) {
		console.error(`agenty-runtime binary not found at ${flat}\n`);
		process.exit(1);
	}
	return flat;
}

const os = resolveOS();
const arch = resolveArch();
const runtimeBin = resolveRuntimeBinary(os, arch);
mkdirSync(EMBEDDED_DIR, { recursive: true });
copyFileSync(runtimeBin, EMBEDDED_BIN);
console.log(`embedded agenty runtime <- ${runtimeBin}`);

const version = readCliVersion();
mkdirSync(DIST, { recursive: true });
const outfile = join(DIST, `agenty-cli-${os}-${arch}${os === "windows" ? ".exe" : ""}`);

const result = await Bun.build({
	entrypoints: [join(PKG, "src/index.tsx")],
	compile: { outfile },
	target: "bun",
	define: {
		"process.env.AGENTY_CLI_VERSION": JSON.stringify(version),
	},
});
if (!result.success) {
	for (const log of result.logs) {
		console.error(log.message);
	}
	process.exit(1);
}

console.log(`agenty-cli single executable built -> dist/agenty-cli`);
