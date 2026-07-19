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
