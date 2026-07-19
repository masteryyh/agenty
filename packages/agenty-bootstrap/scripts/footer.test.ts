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

import { encodeFooter, FOOTER_SIZE } from "./footer";

/**
 * Golden footer bytes shared with the Rust `footer_golden_bytes` test in
 * `src/lib.rs`; both sides assert the exact same layout to lock the
 * cross-language format.
 */
const GOLDEN_FOOTER_HEX =
	"88776655443322110807060504030201000102030405060708090a0b0c0d0e0f" +
	"101112131415161718191a1b1c1d1e1f1122334455667788010203040506070820212223242526" +
	"2728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f01000000cafebabe10136666";

function toHex(bytes: Uint8Array): string {
	return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

describe("encodeFooter", () => {
	test("matches the golden layout shared with the Rust bootstrap", () => {
		const footer = encodeFooter(
			{
				offset: 0x1122334455667788n,
				len: 0x0102030405060708n,
				sha3_256: Uint8Array.from({ length: 32 }, (_, i) => i),
			},
			{
				offset: 0x8877665544332211n,
				len: 0x0807060504030201n,
				sha3_256: Uint8Array.from({ length: 32 }, (_, i) => 0x20 + i),
			},
		);

		expect(footer.length).toBe(FOOTER_SIZE);
		expect(toHex(footer)).toBe(GOLDEN_FOOTER_HEX);
	});

	test("rejects non-32-byte digests", () => {
		const spec = { offset: 0n, len: 0n, sha3_256: new Uint8Array(31) };
		expect(() => encodeFooter(spec, spec)).toThrow("32 bytes");
	});
});
