export const MAGIC = [0xca, 0xfe, 0xba, 0xbe, 0x10, 0x13, 0x66, 0x66] as const;
export const FORMAT_VERSION = 2;
export const FOOTER_SIZE = 156;

export interface PayloadSpec {
    offset: bigint;
    len: bigint;
    sha3_256: Uint8Array;
}

export function encodeFooter(cli: PayloadSpec, core: PayloadSpec, patchApplier: PayloadSpec): Uint8Array {
    if (cli.sha3_256.length !== 32 || core.sha3_256.length !== 32 || patchApplier.sha3_256.length !== 32) {
        throw new Error("payload SHA3-256 digests must be 32 bytes");
    }

    const out = new Uint8Array(FOOTER_SIZE);
    const view = new DataView(out.buffer, out.byteOffset, out.byteLength);
    view.setBigUint64(0, cli.offset, true);
    view.setBigUint64(8, cli.len, true);
    out.set(cli.sha3_256, 16);
    view.setBigUint64(48, core.offset, true);
    view.setBigUint64(56, core.len, true);
    out.set(core.sha3_256, 64);
    view.setBigUint64(96, patchApplier.offset, true);
    view.setBigUint64(104, patchApplier.len, true);
    out.set(patchApplier.sha3_256, 112);
    view.setUint32(144, FORMAT_VERSION, true);
    out.set(MAGIC, 148);
    return out;
}
