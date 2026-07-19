/**
 * Packs the final self-extracting `agenty-<os>-<arch>` binary:
 *
 *   [ agenty-bootstrap stub ][ compressed CLI ][ compressed runtime ][ footer ]
 *
 * Both payloads are compressed in memory with xz and appended directly, so no
 * intermediate archives are written to disk. The footer records each payload's
 * offset, compressed length and the SHA3-256 of its decompressed contents.
 */

import { compress } from "@napi-rs/lzma/xz";
import {
	chmodSync,
	existsSync,
	mkdirSync,
	readFileSync,
	readdirSync,
	statSync,
	writeFileSync,
} from "node:fs";
import { join, resolve } from "node:path";

import { encodeFooter } from "./footer";
import { resolveArch, resolveOS, type TargetArch, type TargetOS } from "./target";

const PKG = resolve(import.meta.dir, "..");
const REPO = resolve(PKG, "../..");
const RUNTIME_BIN_DIR = join(REPO, "packages/agenty-runtime/bin");
const CLI_DIST_DIR = join(REPO, "packages/agenty-cli/bin");
const DIST = join(PKG, "bin");

function findAgentyBinary(dir: string): string | null {
	let names: string[];
	try {
		names = readdirSync(dir);
	} catch {
		return null;
	}

	const hits = names
		.filter((name) => name.startsWith("agenty"))
		.map((name) => join(dir, name))
		.filter((path) => statSync(path).isFile());
	if (hits.length > 1) {
		throw new Error(`multiple agenty-runtime binaries found in ${dir}`);
	}
	return hits[0] ?? null;
}

function hostTarget(): { os: TargetOS; arch: TargetArch } {
	const os =
		process.platform === "darwin"
			? "macos"
			: process.platform === "win32"
				? "windows"
				: "linux";
	const arch = process.arch === "x64" ? "amd64" : "arm64";
	return { os, arch };
}

function resolveRuntimeBinary(os: TargetOS, arch: TargetArch): string {
	const targetBinary = findAgentyBinary(join(RUNTIME_BIN_DIR, `${os}_${arch}`));
	if (targetBinary) {
		return targetBinary;
	}

	const host = hostTarget();
	const flat = join(RUNTIME_BIN_DIR, "agenty");
	if (os === host.os && arch === host.arch && existsSync(flat)) {
		return flat;
	}

	throw new Error(`agenty-runtime binary not found for OS=${os} ARCH=${arch}`);
}

function resolveCliBinary(os: TargetOS, arch: TargetArch, ext: string): string {
	const path = join(CLI_DIST_DIR, `agenty-cli-${os}-${arch}${ext}`);
	if (!existsSync(path)) {
		throw new Error(`agenty-cli binary not found at ${path}; run the agenty-cli build first`);
	}
	return path;
}

function sha3_256(data: Uint8Array): Uint8Array {
	return new Bun.CryptoHasher("sha3-256").update(data).digest();
}

const os = resolveOS();
const arch = resolveArch();
const ext = os === "windows" ? ".exe" : "";

const stubPath = join(PKG, `target/release/agenty-bootstrap${ext}`);
if (!existsSync(stubPath)) {
	throw new Error(`bootstrap stub not found at ${stubPath}; run \`cargo build --release\` first`);
}
const cliPath = resolveCliBinary(os, arch, ext);
const runtimePath = resolveRuntimeBinary(os, arch);

const stub = readFileSync(stubPath);
const cli = readFileSync(cliPath);
const runtime = readFileSync(runtimePath);
if (stub.length === 0 || cli.length === 0 || runtime.length === 0) {
	throw new Error("stub, CLI or runtime binary is empty");
}

const cliSha3 = sha3_256(cli);
const runtimeSha3 = sha3_256(runtime);
const cliPayload = await compress(cli);
const runtimePayload = await compress(runtime);

const footer = encodeFooter(
	{
		offset: BigInt(stub.length),
		len: BigInt(cliPayload.length),
		sha3_256: cliSha3,
	},
	{
		offset: BigInt(stub.length + cliPayload.length),
		len: BigInt(runtimePayload.length),
		sha3_256: runtimeSha3,
	},
);

mkdirSync(DIST, { recursive: true });
const out = join(DIST, `agenty-${os}-${arch}${ext}`);
writeFileSync(out, Buffer.concat([stub, cliPayload, runtimePayload, footer]));
if (os !== "windows") {
	chmodSync(out, 0o755);
}

console.log(
	`packed agenty <- ${out}\n` +
		`  stub    ${stub.length} bytes (${stubPath})\n` +
		`  cli     ${cli.length} -> ${cliPayload.length} bytes (${cliPath})\n` +
		`  runtime ${runtime.length} -> ${runtimePayload.length} bytes (${runtimePath})\n` +
		`  total   ${stub.length + cliPayload.length + runtimePayload.length + footer.length} bytes`,
);
