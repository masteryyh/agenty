import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";

import { resolveCoreBuildEnvironment } from "../../../scripts/core-build-env.mjs";
import { resolveCodecTarget } from "./build.mjs";

const packageRoot = resolve(import.meta.dirname, "..");
const { target } = resolveCodecTarget();
const directory = join(packageRoot, "bin", target);
const library = join(directory, "libllm_codec.a");
const nativeFlags = readFileSync(join(directory, "native-static-libs.txt"), "utf8");
const result = spawnSync("go", ["test", "-count=1", "./..."], {
    cwd: join(packageRoot, "tests/ffi-go"),
    env: {
        ...resolveCoreBuildEnvironment(),
        CGO_LDFLAGS: `"${library}" ${nativeFlags} ${process.env.CGO_LDFLAGS || ""}`,
    },
    stdio: "inherit",
});
if (result.error) {
    throw result.error;
}
process.exitCode = result.status ?? 1;
