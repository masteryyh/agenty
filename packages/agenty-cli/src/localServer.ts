import { existsSync } from "node:fs";
import { createServer } from "node:net";
import { homedir } from "node:os";
import { join, resolve } from "node:path";

export interface LocalServer {
	url: string;
	stop: () => Promise<void>;
}

export interface LocalServerOptions {
	databasePath?: string;
	debug?: boolean;
}

/**
 * Runtime path installed and maintained by the agenty launcher
 * (`packages/agenty-bootstrap`). Kept in sync with `artifact_paths` in
 * `packages/agenty-bootstrap/src/lib.rs`.
 */
export const MANAGED_RUNTIME_PATH = join(
	homedir(),
	".agenty",
	"bin",
	process.platform === "win32" ? "runtime.exe" : "runtime",
);

/**
 * Runtime built inside this repository. Only resolvable when the CLI runs
 * from a source checkout; standalone builds never carry this path.
 */
const REPO_RUNTIME_PATH = resolve(import.meta.dir, "../../agenty-runtime/bin/agenty");

export interface RuntimePathCandidates {
	envBin?: string;
	repoBin: string;
	managedBin: string;
}

/**
 * Picks the agenty runtime binary by priority: an explicit `AGENTY_BIN`
 * override, the in-repository build used for development, then the managed
 * path installed by the launcher.
 */
export function pickRuntimePath(
	candidates: RuntimePathCandidates,
	exists: (path: string) => boolean,
): string | null {
	if (candidates.envBin && exists(candidates.envBin)) {
		return candidates.envBin;
	}
	if (exists(candidates.repoBin)) {
		return candidates.repoBin;
	}
	if (exists(candidates.managedBin)) {
		return candidates.managedBin;
	}
	return null;
}

function resolveAgentyBinary(): string {
	const bin = pickRuntimePath(
		{
			envBin: process.env.AGENTY_BIN,
			repoBin: REPO_RUNTIME_PATH,
			managedBin: MANAGED_RUNTIME_PATH,
		},
		existsSync,
	);
	if (!bin) {
		throw new Error(
			"agenty runtime not found; start through the agenty launcher, set AGENTY_BIN, or build packages/agenty-runtime first",
		);
	}
	return bin;
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
		} catch {}

		await Bun.sleep(300);
	}
	throw new Error(`local agenty server did not become healthy at ${url}`);
}

export async function startLocalServer(options: LocalServerOptions = {}): Promise<LocalServer> {
	const bin = resolveAgentyBinary();
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
