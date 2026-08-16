import type { AgentyClient } from "@/api/client";
import type { UpdateAgentDto } from "@/api/types";

import {
    action,
    CliError,
    displayModel,
    flag,
    hasFlag,
    outputFields,
    outputTable,
    pageOptions,
    parseBoolean,
    type ParsedArgs,
    render,
    requirePositionals,
    resolveModel
} from "./utils";

export async function handleAgent(client: AgentyClient, args: ParsedArgs): Promise<void> {
    const command = args.positionals[1];
    if (command === "list") {
        requirePositionals(args, 2, "agent list");
        const { page, pageSize } = pageOptions(args);
        const result = await client.listAgentsPage(page, pageSize);
        render(args, result, () => result.data.length === 0
            ? process.stdout.write("No agents.\n")
            : outputTable(["Slug", "Name", "Default", "Model"], result.data.map((agent) => [
                agent.slug, agent.name, String(agent.isDefault), agent.defaultModel ? `${agent.defaultModel.providerSlug}/${agent.defaultModel.modelSlug}` : "",
            ])));
        return;
    }
    if (command === "get") {
        const [, , reference] = requirePositionals(args, 3, "agent get <slug-or-name>");
        const agent = await client.resolveAgent(reference);
        render(args, agent, () => outputFields([
            ["Slug", agent.slug], ["Name", agent.name], ["Soul", agent.soul], ["Default", String(agent.isDefault)],
            ["Model", agent.defaultModel ? `${agent.defaultModel.providerSlug}/${agent.defaultModel.modelSlug}` : ""],
        ]));
        return;
    }
    if (command === "add") {
        const [, , slug] = requirePositionals(args, 3, "agent add <slug> [options]");
        const model = flag(args, "model") ? await resolveModel(client, flag(args, "model")!) : undefined;
        const created = await client.createAgent({
            slug, name: flag(args, "name")?.trim() || slug, soul: flag(args, "soul") ?? "",
            isDefault: hasFlag(args, "default") ? parseBoolean(flag(args, "default"), "--default") : false,
            defaultModel: model ? { providerSlug: model.providerSlug, modelSlug: model.slug } : undefined,
            defaultContextWindow: model?.contextWindow ?? 0,
        });
        action(args, created, `Agent added: ${created.slug}`);
        return;
    }
    if (command === "update") {
        const [, , reference] = requirePositionals(args, 3, "agent update <slug-or-name> [options]");
        const current = await client.resolveAgent(reference);
        const update: UpdateAgentDto = {};
        if (hasFlag(args, "name")) {
            update.name = flag(args, "name")?.trim() || "";
        }
        if (hasFlag(args, "soul")) {
            update.soul = flag(args, "soul") ?? "";
        }
        if (hasFlag(args, "default")) {
            update.isDefault = parseBoolean(flag(args, "default"), "--default");
        }
        if (hasFlag(args, "model")) {
            const model = await resolveModel(client, flag(args, "model")!);
            update.defaultModel = { providerSlug: model.providerSlug, modelSlug: model.slug };
            update.defaultContextWindow = model.contextWindow;
        }
        if (Object.keys(update).length === 0) {
            throw new CliError("no changes specified");
        }
        const updated = await client.updateAgent(current.slug, update);
        action(args, updated, `Agent updated: ${updated.slug}`);
        return;
    }
    if (command === "remove") {
        const [, , reference] = requirePositionals(args, 3, "agent remove <slug-or-name> --yes");
        if (!hasFlag(args, "yes")) {
            throw new CliError("use --yes to remove an agent non-interactively");
        }
        const current = await client.resolveAgent(reference);
        await client.deleteAgent(current.slug);
        action(args, { slug: current.slug, deleted: true }, `Agent removed: ${current.slug}`);
        return;
    }
    throw new CliError("usage: agent <list|get|add|update|remove>");
}
