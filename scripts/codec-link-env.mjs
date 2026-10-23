import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";

import { resolveCodecTarget } from "../packages/llm-codec/scripts/build.mjs";

export function codecLinkEnvironment(environment = process.env, repositoryRoot = resolve(import.meta.dirname, "..")) {
    const { target } = resolveCodecTarget(environment);
    const directory = resolve(repositoryRoot, "packages/llm-codec/bin", target);
    const native = readFileSync(join(directory, "native-static-libs.txt"), "utf8").trim();
    return {
        ...environment,
        CGO_ENABLED: "1",
        CGO_LDFLAGS: `"${join(directory, "libllm_codec.a")}" ${native} ${environment.CGO_LDFLAGS || ""}`,
    };
}
