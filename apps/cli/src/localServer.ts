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

import { existsSync, mkdirSync, chmodSync } from "node:fs";
import { join } from "node:path";
import { createServer } from "node:net";
import { tmpdir } from "node:os";

import agentyBinUrl from "./_embedded/agenty-bin" with { type: "file" };

export interface LocalServer {
	url: string;
	stop: () => Promise<void>;
}

export interface LocalServerOptions {
	databasePath?: string;
	debug?: boolean;
}

const RUNTIME_DIR = join(import.meta.dir, "../../../packages/agenty-runtime");
const DEV_BINARY = join(RUNTIME_DIR, "bin/agenty");

function embeddedPath(): string {
	return agentyBinUrl.startsWith("/") ? agentyBinUrl : join(import.meta.dir, agentyBinUrl);
}

function isCompiledBinary(): boolean {
	return agentyBinUrl.startsWith("/$bunfs");
}

async function isRealBinary(path: string): Promise<boolean> {
	try {
		const f = Bun.file(path);
		return (await f.exists()) && f.size > 1_000_000;
	} catch {
		return false;
	}
}

async function extractEmbedded(): Promise<string> {
	const src = embeddedPath();
	const blob = Bun.file(src);
	const size = blob.size;
	if (size < 1_000_000) {
		throw new Error(
			"embedded agenty binary is missing or too small; rebuild with `pnpm cli:build`",
		);
	}
	const cacheDir = join(tmpdir(), "agenty-cli");
	mkdirSync(cacheDir, { recursive: true });
	const name = src.split("/").pop() ?? `agenty-server-${size}`;
	const cachePath = join(cacheDir, name);
	const cached = Bun.file(cachePath);
	if (!(await cached.exists()) || cached.size !== size) {
		await Bun.write(cachePath, blob);
		chmodSync(cachePath, 0o755);
	}
	return cachePath;
}

async function buildGoBinary(): Promise<string> {
	if (!existsSync(join(RUNTIME_DIR, "Makefile"))) {
		throw new Error("agenty binary not found. Run `make build` or set AGENTY_BIN.");
	}
	process.stderr.write("agenty binary not found; building with `make build`…\n");
	const build = Bun.spawn(["make", "build"], {
		cwd: RUNTIME_DIR,
		stdout: "inherit",
		stderr: "inherit",
	});
	const code = await build.exited;
	if (code !== 0) throw new Error(`\`make build\` failed with exit code ${code}`);
	if (!existsSync(DEV_BINARY)) throw new Error("`make build` did not produce bin/agenty");
	return DEV_BINARY;
}

async function resolveAgentyBinary(): Promise<string> {
	const envBin = process.env.AGENTY_BIN;
	if (envBin && existsSync(envBin)) return envBin;

	if (isCompiledBinary()) {
		return await extractEmbedded();
	}

	// dev mode: src/_embedded/agenty-bin (kept in sync by ensure-embedded.ts)
	if (await isRealBinary(embeddedPath())) return embeddedPath();
	if (existsSync(DEV_BINARY)) return DEV_BINARY;
	const which = Bun.which("agenty");
	if (typeof which === "string" && which !== "" && existsSync(which)) return which;
	return await buildGoBinary();
}

function pickFreePort(): Promise<number> {
	return new Promise((resolve, reject) => {
		const srv = createServer();
		srv.unref();
		srv.on("error", reject);
		srv.listen(0, "127.0.0.1", () => {
			const addr = srv.address();
			if (addr && typeof addr === "object") {
				const port = addr.port;
				srv.close(() => resolve(port));
			} else {
				srv.close();
				reject(new Error("failed to pick a free port"));
			}
		});
	});
}

async function waitForHealth(url: string, timeoutMs = 20000): Promise<void> {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		try {
			const resp = await fetch(`${url}/api/v1/system/version`);
			if (resp.ok) return;
		} catch {
			// server not ready yet
		}
		await Bun.sleep(300);
	}
	throw new Error(`local agenty server did not become healthy at ${url}`);
}

export async function startLocalServer(options: LocalServerOptions = {}): Promise<LocalServer> {
	const bin = await resolveAgentyBinary();
	const port = await pickFreePort();
	const args = [bin, "--port", String(port)];
	if (options.databasePath) {
		args.push("--db", options.databasePath);
	}
	if (options.debug) {
		args.push("--debug");
	}

	const proc = Bun.spawn(args, {
		env: process.env,
		stdout: "ignore",
		stderr: "ignore",
	});

	const url = `http://127.0.0.1:${port}`;
	let stopped = false;
	const stop = async (): Promise<void> => {
		if (stopped) return;
		stopped = true;
		try {
			proc.kill("SIGTERM");
		} catch {}

		const forceKill = new Promise<void>((resolve) => {
			const timer = setTimeout(() => {
				try {
					proc.kill("SIGKILL");
				} catch {}
				resolve();
			}, 5000);
			timer.unref?.();
		});

		try {
			await Promise.race([proc.exited.catch(() => {}), forceKill]);
		} catch {}
	};

	process.on("exit", () => {
		void stop();
	});

	try {
		await waitForHealth(url);
	} catch (err) {
		void stop();
		throw err;
	}

	return { url, stop };
}
