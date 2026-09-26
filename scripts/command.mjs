import { spawnSync } from "node:child_process";

const WINDOWS_CMD_UNSAFE_ARGUMENT = /[\s&|<>^()%"\r\n]/u;

export function packageManagerCommand(hostPlatform = process.platform) {
    return hostPlatform === "win32" ? "pnpm.cmd" : "pnpm";
}

export function spawnSyncCommand(command, args, options, hostPlatform = process.platform) {
    const isWindowsBatchFile = hostPlatform === "win32" && /\.(?:bat|cmd)$/i.test(command);
    if (!isWindowsBatchFile) {
        return spawnSync(command, args, options);
    }

    // Windows needs cmd.exe for batch files; reject shell syntax in arguments before enabling it.
    if (args.some((argument) => WINDOWS_CMD_UNSAFE_ARGUMENT.test(argument))) {
        throw new Error("Windows batch commands cannot receive whitespace or shell control characters");
    }

    return spawnSync(command, args, {
        ...options,
        shell: true,
    });
}
