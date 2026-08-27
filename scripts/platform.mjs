/** @typedef {"linux" | "macos" | "windows"} TargetOS */
/** @typedef {"amd64" | "arm64"} TargetArch */

/**
 * @param {Record<string, string | undefined>} environment
 * @param {string} hostArch
 * @returns {TargetArch}
 */
export function resolveArch(environment = process.env, hostArch = process.arch) {
    const rawArch = environment.ARCH?.trim() || hostArch;
    const lowerArch = rawArch.toLowerCase();
    if (lowerArch === "x64" || lowerArch === "x86_64" || lowerArch === "amd64") {
        return "amd64";
    }
    if (lowerArch === "arm64" || lowerArch === "aarch64") {
        return "arm64";
    }
    throw new Error(`unsupported architecture: ${rawArch}`);
}

/**
 * @param {Record<string, string | undefined>} environment
 * @param {string} hostPlatform
 * @returns {TargetOS}
 */
export function resolveOS(environment = process.env, hostPlatform = process.platform) {
    const rawOS = environment.OS?.trim() || hostPlatform;
    const lowerOS = rawOS.toLowerCase();
    if (lowerOS === "darwin" || lowerOS === "macos") {
        return "macos";
    }
    if (lowerOS.startsWith("win")) {
        return "windows";
    }
    if (lowerOS === "linux") {
        return "linux";
    }
    throw new Error(`unsupported operating system: ${rawOS}`);
}

/** @param {TargetOS} os */
export function executableExtension(os) {
    return os === "windows" ? ".exe" : "";
}
