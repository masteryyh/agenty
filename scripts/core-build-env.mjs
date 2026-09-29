export function resolveCoreBuildEnvironment(environment = process.env, goOS = process.platform) {
    const configuredCgo = environment.CGO_ENABLED?.trim();
    if (configuredCgo === "0") {
        throw new Error("agenty-core requires CGO_ENABLED=1 because its SQLite driver uses cgo");
    }
    if (configuredCgo && configuredCgo !== "1") {
        throw new Error(`invalid CGO_ENABLED value ${JSON.stringify(configuredCgo)}; agenty-core requires 1`);
    }

    const normalizedGoOS = goOS === "win32" ? "windows" : goOS;
    const configuredCompiler = environment.CC?.trim() || "";
    const usesMSVC = /(?:^|[\\/ ])cl(?:\.exe)?(?:$|[\s"'])/iu.test(configuredCompiler);
    if (normalizedGoOS === "windows" && usesMSVC) {
        throw new Error(
            "agenty-core cgo requires a GCC-compatible CC on Windows; MSVC cl.exe is unsupported. " +
            "Install or select MinGW-w64/LLVM-MinGW and set CC to its gcc.exe or clang.exe.",
        );
    }

    return {
        ...environment,
        CGO_ENABLED: "1",
    };
}
