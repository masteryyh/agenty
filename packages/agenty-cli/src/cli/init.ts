import type { AgentyClient } from "@/api/client";
import type { APIType } from "@/api/types";

import {
    CliError,
    flag,
    outputFields,
    type ParsedArgs,
    render,
    requireFlag,
    requirePositionals,
    secret
} from "./utils";

export async function handleInit(client: AgentyClient, args: ParsedArgs): Promise<void> {
    requirePositionals(args, 1, "init [options]");
    const providerCode = requireFlag(args, "provider");
    const modelCode = requireFlag(args, "model");
    const agentCode = flag(args, "agent")?.trim() || "default";
    const contextWindow = positiveInteger(flag(args, "context-window") ?? "128000", "--context-window");
    const apiKey = secret(args, "api-key", "api-key-env", "provider API key") ?? "";

    await client.createProvider({
        code: providerCode,
        name: flag(args, "provider-name")?.trim() || providerCode,
        type: requireFlag(args, "type") as APIType,
        baseUrl: flag(args, "base-url")?.trim() || "",
        apiKey,
    });
    await client.createModel({
        providerCode,
        modelCode,
        name: flag(args, "model-name")?.trim() || modelCode,
        contextWindow,
        isDefault: true,
    });
    await client.createAgent({
        code: agentCode,
        name: flag(args, "agent-name")?.trim() || agentCode,
        soul: flag(args, "soul") ?? "",
        defaultModel: { providerCode, modelCode },
        defaultContextWindow: contextWindow,
        isDefault: true,
    });
    const result = await client.completeInitialization({ agentCode, providerCode, modelCode });
    render(args, result, () => outputFields([
        ["Initialized", String(result.initialized)],
        ["Provider", providerCode],
        ["Model", `${providerCode}/${modelCode}`],
        ["Agent", agentCode],
    ]));
}

function positiveInteger(raw: string, label: string): number {
    const value = Number(raw);
    if (!Number.isSafeInteger(value) || value <= 0) {
        throw new CliError(`${label} must be a positive integer`);
    }
    return value;
}
