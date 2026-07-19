import { describe, expect, test } from "bun:test";

import { pickRuntimePath } from "./localServer";

const CANDIDATES = {
	envBin: "/override/agenty",
	repoBin: "/repo/packages/agenty-runtime/bin/agenty",
	managedBin: "/home/tester/.agenty/bin/runtime",
};

function existing(...paths: string[]): (path: string) => boolean {
	return (path) => paths.includes(path);
}

describe("pickRuntimePath", () => {
	test("prefers the explicit AGENTY_BIN override", () => {
		const bin = pickRuntimePath(
			CANDIDATES,
			existing(CANDIDATES.envBin, CANDIDATES.repoBin, CANDIDATES.managedBin),
		);
		expect(bin).toBe(CANDIDATES.envBin);
	});

	test("falls back to the in-repository build for development", () => {
		const bin = pickRuntimePath(
			{ ...CANDIDATES, envBin: undefined },
			existing(CANDIDATES.repoBin, CANDIDATES.managedBin),
		);
		expect(bin).toBe(CANDIDATES.repoBin);
	});

	test("ignores a missing override and keeps resolving", () => {
		const bin = pickRuntimePath(
			CANDIDATES,
			existing(CANDIDATES.repoBin, CANDIDATES.managedBin),
		);
		expect(bin).toBe(CANDIDATES.repoBin);
	});

	test("uses the launcher-managed path for standalone builds", () => {
		const bin = pickRuntimePath(
			{ ...CANDIDATES, envBin: undefined },
			existing(CANDIDATES.managedBin),
		);
		expect(bin).toBe(CANDIDATES.managedBin);
	});

	test("returns null when no candidate exists", () => {
		expect(pickRuntimePath(CANDIDATES, () => false)).toBeNull();
	});
});
