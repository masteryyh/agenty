import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const packageDir = fileURLToPath(new URL("..", import.meta.url));

// pnpm changes cwd to the package. Resolve user-supplied relative data paths
// against the invocation directory before launching either child process.
export function runtimeEnv(source = process.env, invocationDir = process.cwd()) {
    const env = { ...source };
    if (env.AGENTY_DATA_DIR) {
        env.AGENTY_DATA_DIR = resolve(env.INIT_CWD ?? invocationDir, env.AGENTY_DATA_DIR);
    }
    return env;
}

export function hostGoEnv(source = process.env) {
    const env = { ...source };
    delete env.GOOS;
    delete env.GOARCH;
    return env;
}
