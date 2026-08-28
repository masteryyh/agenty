import type { AgentyClient } from "@/api/client";
import { type ReasoningEffort, STANDARD_REASONING_EFFORTS, type UpdateModelDto } from "@/api/types";

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
    resolveModelInput,
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
        const [, , reference] = requirePositionals(args, 3, "model get <provider-code>/<model-code>");
        const model = await resolveModelInput(client, reference);
        render(args, model, () => outputFields([
            ["Model", displayModel(model)], ["Name", model.name], ["Default", String(model.isDefault)],
            ["Multimodal", String(model.multiModal)],
            ["Light", String(model.light)], ["Context window", String(model.contextWindow)],
            ["Max output tokens", String(model.maxOutputTokens)],
            ["Reasoning", String(model.reasoning === true || (model.reasoningEfforts?.length ?? 0) > 0)],
            ["Reasoning efforts", (model.reasoningEfforts ?? []).join(", ")],
        ]));
        return;
    }
    if (command === "add") {
        const [, , modelCode] = requirePositionals(args, 3, "model add <code> --provider <ref> [options]");
        const provider = await resolveProvider(client, requireFlag(args, "provider"));
        const created = await client.createModel({
            providerCode: provider.code,
            modelCode,
            name: flag(args, "name")?.trim() || modelCode,
            contextWindow: positiveInteger(flag(args, "context-window") ?? "0", "--context-window", true),
            multiModal: booleanFlag(args, "multi-modal"),
            light: booleanFlag(args, "light"),
            isDefault: booleanFlag(args, "default"),
            reasoning: hasFlag(args, "reasoning") ? booleanFlag(args, "reasoning") : true,
            reasoningEfforts: parseReasoningEfforts(flag(args, "reasoning-efforts")),
        });
        action(args, created, `Model added: ${displayModel(created)}`);
        return;
    }
    if (command === "update") {
        const [, , reference] = requirePositionals(args, 3, "model update <provider-code>/<model-code> [options]");
        const current = await resolveModelInput(client, reference);
        const reasoning = hasFlag(args, "reasoning")
            ? booleanFlag(args, "reasoning")
            : current.reasoning === true || (current.reasoningEfforts?.length ?? 0) > 0;
        const update: UpdateModelDto = {
            name: hasFlag(args, "name") ? requireFlag(args, "name") : current.name,
            contextWindow: hasFlag(args, "context-window") ? positiveInteger(requireFlag(args, "context-window"), "--context-window", true) : current.contextWindow,
            maxOutputTokens: current.maxOutputTokens,
            multiModal: hasFlag(args, "multi-modal") ? booleanFlag(args, "multi-modal") : current.multiModal,
            light: hasFlag(args, "light") ? booleanFlag(args, "light") : current.light,
            isDefault: hasFlag(args, "default") ? booleanFlag(args, "default") : current.isDefault,
            reasoning,
            reasoningEfforts: hasFlag(args, "reasoning-efforts")
                ? parseReasoningEfforts(requireFlag(args, "reasoning-efforts"))
                : reasoning ? current.reasoningEfforts : [],
        };
        const updated = await client.updateModel(current.providerCode, current.code, update);
        action(args, updated, `Model updated: ${displayModel(updated)}`);
        return;
    }
    if (command === "remove") {
        const [, , reference] = requirePositionals(args, 3, "model remove <provider-code>/<model-code> --yes");
        if (!hasFlag(args, "yes")) {
            throw new CliError("use --yes to remove a model non-interactively");
        }
        const current = await resolveModelInput(client, reference);
        await client.deleteModel(current.providerCode, current.code);
        action(args, { providerCode: current.providerCode, modelCode: current.code, deleted: true }, `Model removed: ${displayModel(current)}`);
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

function parseReasoningEfforts(raw?: string): ReasoningEffort[] {
    if (!raw) {
        return [];
    }
    const values = raw.split(",").map((value) => value.trim().toLowerCase()).filter(Boolean);
    if (values.some((value) => !(STANDARD_REASONING_EFFORTS as readonly string[]).includes(value))) {
        throw new CliError(`--reasoning-efforts must contain only ${STANDARD_REASONING_EFFORTS.join(", ")}`);
    }
    return Array.from(new Set(values)) as ReasoningEffort[];
}
