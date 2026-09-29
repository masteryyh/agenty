import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { win32 } from "node:path";

const WINDOWS_CMD_UNSAFE_ARGUMENT = /[&|<>^()%!"\r\n]/u;
const JAVASCRIPT_ENTRY = /\.(?:cjs|mjs|js)$/i;
const NATIVE_EXECUTABLE = /\.exe$/i;

function resolveWindowsPackageManagerCommand(environment) {
    const searchPath = environment.PATH || environment.Path || "";
    for (const directory of searchPath.split(win32.delimiter)) {
        if (!directory) {
            continue;
        }

        const candidate = win32.join(directory.replace(/^"|"$/gu, ""), "pnpm.cmd");
        if (existsSync(candidate)) {
            return candidate;
        }
    }

    return "pnpm.cmd";
}

export function packageManagerCommand(hostPlatform = process.platform, environment = process.env) {
    const packageManagerEntry = environment.npm_execpath?.trim();
    if (packageManagerEntry && (JAVASCRIPT_ENTRY.test(packageManagerEntry) || NATIVE_EXECUTABLE.test(packageManagerEntry))) {
        return packageManagerEntry;
    }

    return hostPlatform === "win32" ? resolveWindowsPackageManagerCommand(environment) : "pnpm";
}

export function spawnSyncCommand(command, args, options, hostPlatform = process.platform) {
    if (JAVASCRIPT_ENTRY.test(command)) {
        const environment = options?.env ?? process.env;
        const node = environment.npm_node_execpath?.trim() || process.execPath;
        return spawnSync(node, [command, ...args], options);
    }

    const isWindowsBatchFile = hostPlatform === "win32" && /\.(?:bat|cmd)$/i.test(command);
    if (!isWindowsBatchFile) {
        return spawnSync(command, args, options);
    }

    if ([command, ...args].some((argument) => WINDOWS_CMD_UNSAFE_ARGUMENT.test(argument))) {
        throw new Error(
            "Windows batch fallback rejects shell metacharacters; invoke the command from a package-manager script " +
                "to pass arbitrary arguments safely",
        );
    }

    const commandLine = [command, ...args].map((argument) => `"${argument}"`).join(" ");
    const commandProcessor = options?.env?.ComSpec || options?.env?.COMSPEC || process.env.ComSpec || "cmd.exe";
    return spawnSync(commandProcessor, ["/d", "/s", "/c", `"${commandLine}"`], {
        ...options,
        windowsVerbatimArguments: true,
    });
}
