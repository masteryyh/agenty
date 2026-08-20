import type { AgentyClient } from "@/api/client";
import type { APIType, UpdateModelProviderDto } from "@/api/types";

import {
    action,
    CliError,
    flag,
    hasFlag,
    outputFields,
    outputTable,
    pageOptions,
    type ParsedArgs,
    render,
    requireFlag,
    requirePositionals,
    resolveProvider,
    secret
} from "./utils";

export async function handleProvider(client: AgentyClient, args: ParsedArgs): Promise<void> {
    const command = args.positionals[1];
    if (command === "list") {
        requirePositionals(args, 2, "provider list");
        const { page, pageSize } = pageOptions(args);
        const result = await client.listProvidersPage(page, pageSize);
        render(args, result, () => result.data.length === 0
            ? process.stdout.write("No providers.\n")
            : outputTable(["Provider Code", "Name", "Type", "Base URL", "Models"], result.data.map((provider) => [
                provider.code, provider.name, provider.type, provider.baseUrl, String(provider.models.length),
            ])));
        return;
    }
    if (command === "get") {
        const [, , reference] = requirePositionals(args, 3, "provider get <code-or-name>");
        const provider = await resolveProvider(client, reference);
        render(args, provider, () => outputFields([
            ["Provider Code", provider.code], ["Name", provider.name], ["Type", provider.type],
            ["Base URL", provider.baseUrl], ["API Key", provider.apiKey ? "<set>" : "<not set>"],
            ["Models", String(provider.models.length)],
        ]));
        return;
    }
    if (command === "add") {
        const [, , code] = requirePositionals(args, 3, "provider add <code> --type <type> [options]");
        const created = await client.createProvider({
            code,
            name: flag(args, "name")?.trim() || code,
            type: requireFlag(args, "type") as APIType,
            baseUrl: flag(args, "base-url")?.trim() || "",
            apiKey: secret(args, "api-key", "api-key-env", "provider API key") ?? "",
        });
        action(args, created, `Provider added: ${created.code}`);
        return;
    }
    if (command === "update") {
        const [, , reference] = requirePositionals(args, 3, "provider update <code-or-name> [options]");
        const current = await resolveProvider(client, reference);
        const update: UpdateModelProviderDto = {};
        if (hasFlag(args, "name")) {
            update.name = requireFlag(args, "name");
        }
        if (hasFlag(args, "type")) {
            update.type = requireFlag(args, "type") as APIType;
        }
        if (hasFlag(args, "base-url")) {
            update.baseUrl = flag(args, "base-url") ?? "";
        }
        const apiKey = secret(args, "api-key", "api-key-env", "provider API key");
        if (apiKey !== undefined) {
            update.apiKey = apiKey;
        }
        if (Object.keys(update).length === 0) {
            throw new CliError("no changes specified");
        }
        const updated = await client.updateProvider(current.code, update);
        action(args, updated, `Provider updated: ${updated.code}`);
        return;
    }
    if (command === "remove") {
        const [, , reference] = requirePositionals(args, 3, "provider remove <code-or-name> --yes");
        if (!hasFlag(args, "yes")) {
            throw new CliError("use --yes to remove a provider non-interactively");
        }
        const current = await resolveProvider(client, reference);
        await client.deleteProvider(current.code);
        action(args, { code: current.code, deleted: true }, `Provider removed: ${current.code}`);
        return;
    }
    throw new CliError("usage: provider <list|get|add|update|remove>");
}
