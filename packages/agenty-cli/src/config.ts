export type ThinkingFlag = "off" | "on" | string;

export interface CliOptions {
    agentRef?: string;
    modelInput?: string;
    thinking: ThinkingFlag;
    dataDir?: string;
    newSession: boolean;
}

const BOOLEAN_FLAGS = new Set(["new-session"]);

function parseArgs(argv: string[]): Record<string, string | boolean> {
    const flags: Record<string, string | boolean> = {};
    for (let i = 0; i < argv.length; i++) {
        const arg = argv[i];
        if (!arg.startsWith("--")) {
            continue;
        }
        const key = arg.slice(2);
        const next = argv[i + 1];
        if (!BOOLEAN_FLAGS.has(key) && next !== undefined && !next.startsWith("--")) {
            flags[key] = next;
            i++;
        } else {
            flags[key] = true;
        }
    }
    return flags;
}

export function loadOptions(): CliOptions {
    const flags = parseArgs(process.argv.slice(2));
    return {
        agentRef: typeof flags.agent === "string" ? flags.agent : undefined,
        modelInput: typeof flags.model === "string" ? flags.model : undefined,
        thinking: typeof flags.thinking === "string" ? flags.thinking : "off",
        dataDir: typeof flags["data-dir"] === "string" ? flags["data-dir"] : undefined,
        newSession: flags["new-session"] === true,
    };
}

export function parseThinking(flag: ThinkingFlag): {
    thinking: boolean;
    thinkingLevel: string;
} {
    const value = flag.trim().toLowerCase();
    if (value === "" || value === "off" || value === "false") {
        return { thinking: false, thinkingLevel: "" };
    }
    if (value === "on" || value === "true") {
        return { thinking: true, thinkingLevel: "medium" };
    }
    return { thinking: true, thinkingLevel: value };
}
