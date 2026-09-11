export type BuildEnvironment = Record<string, string | undefined>;

export declare function resolveBuildVersion(
    environment?: BuildEnvironment,
    repositoryRoot?: string,
): string;

