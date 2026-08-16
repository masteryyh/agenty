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
    const providerSlug = requireFlag(args, "provider");
    const modelSlug = requireFlag(args, "model");
    const agentSlug = flag(args, "agent")?.trim() || "default";
    const contextWindow = positiveInteger(flag(args, "context-window") ?? "128000", "--context-window");
    const maxOutputTokens = positiveInteger(flag(args, "max-output-tokens") ?? "16384", "--max-output-tokens");
    const apiKey = secret(args, "api-key", "api-key-env", "provider API key") ?? "";

    await client.createProvider({
        slug: providerSlug,
        name: flag(args, "provider-name")?.trim() || providerSlug,
        type: requireFlag(args, "type") as APIType,
        baseUrl: flag(args, "base-url")?.trim() || "",
        apiKey,
    });
    await client.createModel({
        providerSlug,
        modelSlug,
        name: flag(args, "model-name")?.trim() || modelSlug,
        contextWindow,
        maxOutputTokens,
        isDefault: true,
    });
    await client.createAgent({
        slug: agentSlug,
        name: flag(args, "agent-name")?.trim() || agentSlug,
        soul: flag(args, "soul") ?? "",
        defaultModel: { providerSlug, modelSlug },
        defaultContextWindow: contextWindow,
        isDefault: true,
    });
    const result = await client.completeInitialization({ agentSlug, providerSlug, modelSlug });
    render(args, result, () => outputFields([
        ["Initialized", String(result.initialized)],
        ["Provider", providerSlug],
        ["Model", `${providerSlug}/${modelSlug}`],
        ["Agent", agentSlug],
    ]));
}

function positiveInteger(raw: string, label: string): number {
    const value = Number(raw);
    if (!Number.isSafeInteger(value) || value <= 0) {
        throw new CliError(`${label} must be a positive integer`);
    }
    return value;
}
