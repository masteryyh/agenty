import { mkdirSync } from "node:fs";
import { resolve, join } from "node:path";

import { resolveArch, resolveBunTarget, resolveOpenTUILibc, resolveOS } from "./target";

const PKG = resolve(import.meta.dir, "..");
const DIST = join(PKG, "bin");

const os = resolveOS();
const arch = resolveArch();
const bunTarget = resolveBunTarget(os, arch);
const opentuiLibc = resolveOpenTUILibc(os);

const version = process.env.AGENTY_VERSION?.trim() || "dev";
mkdirSync(DIST, { recursive: true });
const outfile = join(DIST, `agenty-cli-${os}-${arch}${os === "windows" ? ".exe" : ""}`);

const result = await Bun.build({
	entrypoints: [join(PKG, "src/index.tsx")],
	compile: { outfile, target: bunTarget },
	target: "bun",
	define: {
		"process.env.AGENTY_VERSION": JSON.stringify(version),
		...(opentuiLibc
			? { "process.env.OPENTUI_LIBC": JSON.stringify(opentuiLibc) }
			: {}),
	},
});
if (!result.success) {
	for (const log of result.logs) {
		console.error(log.message);
	}
	process.exit(1);
}

console.log(
	`agenty-cli single executable built -> ${outfile} (${bunTarget}${opentuiLibc ? `, ${opentuiLibc}` : ""})`,
);
