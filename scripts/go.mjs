import { spawnSync } from "node:child_process";

import { codecLinkEnvironment } from "./codec-link-env.mjs";
import { resolveCoreBuildEnvironment } from "./core-build-env.mjs";

const result = spawnSync("go", process.argv.slice(2), {
    env: codecLinkEnvironment(resolveCoreBuildEnvironment()),
    stdio: "inherit",
});
if (result.error) {
    throw result.error;
}
process.exitCode = result.status ?? 1;
