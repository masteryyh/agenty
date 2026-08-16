import type { AgentyClient } from "@/api/client";
import type { ReasoningEffort, UpdateModelDto } from "@/api/types";

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
    requireFlag,
    requirePositionals,
    resolveModel,
    resolveProvider
} from "./utils";

export async function handleModel(client: AgentyClient, args: ParsedArgs): Promise<void> {
    const command = args.positionals[1];
    if (command === "list") {
        requirePositionals(args, 2, "model list");
        const { page, pageSize } = pageOptions(args);
        const result = await client.listModelsPage(page, pageSize);
        const data = result.data;
        render(args, { ...result, data, total: data.length }, () => data.length === 0
            ? process.stdout.write("No models.\n")
            : outputTable(["Model", "Default", "Context", "Max output"], data.map((model) => [
                displayModel(model), String(model.isDefault),
                String(model.contextWindow), String(model.maxOutputTokens),
            ])));
        return;
    }
    if (command === "get") {
        const [, , reference] = requirePositionals(args, 3, "model get <provider/model-or-name>");
        const model = await resolveModel(client, reference);
        render(args, model, () => outputFields([
            ["Model", displayModel(model)], ["Name", model.name], ["Default", String(model.isDefault)],
            ["Multimodal", String(model.multiModal)],
            ["Light", String(model.light)], ["Context window", String(model.contextWindow)],
            ["Max output tokens", String(model.maxOutputTokens)],
        ]));
        return;
    }
    if (command === "add") {
        const [, , modelSlug] = requirePositionals(args, 3, "model add <slug> --provider <ref> --max-output-tokens <n> [options]");
        const provider = await resolveProvider(client, requireFlag(args, "provider"));
        const created = await client.createModel({
            providerSlug: provider.slug,
            modelSlug,
            name: flag(args, "name")?.trim() || modelSlug,
            contextWindow: positiveInteger(flag(args, "context-window") ?? "0", "--context-window", true),
            maxOutputTokens: positiveInteger(requireFlag(args, "max-output-tokens"), "--max-output-tokens"),
            multiModal: booleanFlag(args, "multi-modal"),
            light: booleanFlag(args, "light"),
            isDefault: booleanFlag(args, "default"),
            reasoningEffortMapping: reasoningMapping(args),
        });
        action(args, created, `Model added: ${displayModel(created)}`);
        return;
    }
    if (command === "update") {
        const [, , reference] = requirePositionals(args, 3, "model update <provider/model-or-name> [options]");
        const current = await resolveModel(client, reference);
        const update: UpdateModelDto = {
            name: hasFlag(args, "name") ? requireFlag(args, "name") : current.name,
            contextWindow: hasFlag(args, "context-window") ? positiveInteger(requireFlag(args, "context-window"), "--context-window", true) : current.contextWindow,
            maxOutputTokens: hasFlag(args, "max-output-tokens") ? positiveInteger(requireFlag(args, "max-output-tokens"), "--max-output-tokens") : current.maxOutputTokens,
            multiModal: hasFlag(args, "multi-modal") ? booleanFlag(args, "multi-modal") : current.multiModal,
            light: hasFlag(args, "light") ? booleanFlag(args, "light") : current.light,
            isDefault: hasFlag(args, "default") ? booleanFlag(args, "default") : current.isDefault,
            reasoningEffortMapping: hasFlag(args, "reasoning-map") ? reasoningMapping(args) : current.reasoningEffortMapping,
        };
        const updated = await client.updateModel(current.providerSlug, current.slug, update);
        action(args, updated, `Model updated: ${displayModel(updated)}`);
        return;
    }
    if (command === "remove") {
        const [, , reference] = requirePositionals(args, 3, "model remove <provider/model-or-name> --yes");
        if (!hasFlag(args, "yes")) {
            throw new CliError("use --yes to remove a model non-interactively");
        }
        const current = await resolveModel(client, reference);
        await client.deleteModel(current.providerSlug, current.slug);
        action(args, { providerSlug: current.providerSlug, modelSlug: current.slug, deleted: true }, `Model removed: ${displayModel(current)}`);
        return;
    }
    throw new CliError("usage: model <list|get|add|update|remove>");
}

function booleanFlag(args: ParsedArgs, name: string): boolean {
    return hasFlag(args, name) ? parseBoolean(flag(args, name), `--${name}`) : false;
}

function positiveInteger(raw: string, label: string, allowZero = false): number {
    const value = Number(raw);
    if (!Number.isSafeInteger(value) || value < (allowZero ? 0 : 1)) {
        throw new CliError(`${label} must be ${allowZero ? "a non-negative" : "a positive"} integer`);
    }
    return value;
}

function reasoningMapping(args: ParsedArgs): Record<string, ReasoningEffort> | undefined {
    const raw = flag(args, "reasoning-map")?.trim();
    if (!raw) {
        return undefined;
    }
    try {
        return JSON.parse(raw) as Record<string, ReasoningEffort>;
    } catch {
        throw new CliError("--reasoning-map must be a JSON object");
    }
}
