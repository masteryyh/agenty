import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";

import { hostGoEnv, packageDir } from "./runtime.mjs";

const file = new URL("../web/src/generated.ts", import.meta.url);
const output = execFileSync("go", ["run", "./cmd/generate"], { encoding: "utf8", cwd: packageDir, env: hostGoEnv() });
if (process.argv.includes("--check")) {
    if (readFileSync(file, "utf8") !== output) {
        throw new Error("Inspector DTOs are stale. Run pnpm --filter agenty-inspector generate.");
    }
} else {
    writeFileSync(file, output);
}
