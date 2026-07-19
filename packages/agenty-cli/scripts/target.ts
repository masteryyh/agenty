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

export type TargetOS = "linux" | "macos" | "windows";
export type TargetArch = "amd64" | "arm64";

export function resolveArch(): TargetArch {
	const rawArch = process.env.ARCH?.trim() || process.arch;
	const lowerArch = rawArch.toLowerCase();
	if (lowerArch === "x64" || lowerArch === "x86_64" || lowerArch === "amd64") {
		return "amd64";
	}
	if (lowerArch === "arm64" || lowerArch === "aarch64") {
		return "arm64";
	}
	throw new Error(`unsupported CLI architecture: ${rawArch}`);
}

export function resolveOS(): TargetOS {
	const rawOS = process.env.OS?.trim() || process.platform;
	const lowerOS = rawOS.toLowerCase();
	if (lowerOS === "darwin" || lowerOS === "macos") {
		return "macos";
	}
	if (lowerOS.startsWith("win")) {
		return "windows";
	}
	if (lowerOS === "linux") {
		return "linux";
	}
	throw new Error(`unsupported CLI operating system: ${rawOS}`);
}

export function resolveBunTarget(os: TargetOS, arch: TargetArch): string {
	const bunArch = arch === "amd64" ? "x64" : "arm64";
	const bunOS = os === "macos" ? "darwin" : os;
	return `bun-${bunOS}-${bunArch}`;
}

export function resolveOpenTUILibc(os: TargetOS): string | null {
	if (os !== "linux") {
		return null;
	}
	const libc = process.env.OPENTUI_LIBC?.trim() || "glibc";
	if (libc !== "glibc" && libc !== "musl") {
		throw new Error(`unsupported OPENTUI_LIBC: ${libc} (expected glibc or musl)`);
	}
	return libc;
}
