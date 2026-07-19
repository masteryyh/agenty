/**
 * Payload footer format shared with the Rust bootstrap (`src/lib.rs`).
 *
 * The footer is a fixed 108-byte little-endian trailer appended after the
 * two compressed payloads; `footer.test.ts` and the Rust golden-bytes test
 * assert the exact same layout to lock the cross-language contract.
 */

export const MAGIC = [0xca, 0xfe, 0xba, 0xbe, 0x10, 0x13, 0x66, 0x66] as const;
export const FORMAT_VERSION = 1;
export const FOOTER_SIZE = 108;

export interface PayloadSpec {
	/** Absolute offset of the compressed payload inside the packed file. */
	offset: bigint;
	/** Length of the compressed payload in bytes. */
	len: bigint;
	/** SHA3-256 digest of the decompressed payload. */
	sha3_256: Uint8Array;
}

export function encodeFooter(cli: PayloadSpec, runtime: PayloadSpec): Uint8Array {
	if (cli.sha3_256.length !== 32 || runtime.sha3_256.length !== 32) {
		throw new Error("payload SHA3-256 digests must be 32 bytes");
	}

	const out = new Uint8Array(FOOTER_SIZE);
	const view = new DataView(out.buffer, out.byteOffset, out.byteLength);
	view.setBigUint64(0, cli.offset, true);
	view.setBigUint64(8, cli.len, true);
	out.set(cli.sha3_256, 16);
	view.setBigUint64(48, runtime.offset, true);
	view.setBigUint64(56, runtime.len, true);
	out.set(runtime.sha3_256, 64);
	view.setUint32(96, FORMAT_VERSION, true);
	out.set(MAGIC, 100);
	return out;
}
