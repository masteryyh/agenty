import type { CoreModelDto, ModelProviderDto } from "../api/types";

export function shouldExpandProvider(
    provider: Pick<ModelProviderDto, "builtin" | "apiKey" | "models" | "modelsCached">,
): boolean {
    if (provider.modelsCached === true || provider.models.length === 0) {
        return false;
    }
    if (provider.builtin === true && provider.apiKey.trim() === "") {
        return false;
    }
    return true;
}

export function defaultExpandedProviderCodes(providers: ModelProviderDto[]): Set<string> {
    return new Set(
        providers
            .filter(shouldExpandProvider)
            .map((provider) => provider.code),
    );
}

export type ProviderListRow =
    | {
        kind: "provider";
        key: string;
        depth: 0;
        provider: ModelProviderDto;
        expanded: boolean;
    }
    | {
        kind: "model";
        key: string;
        depth: 1;
        provider: ModelProviderDto;
        model: CoreModelDto;
    }
    | {
        kind: "add-model";
        key: string;
        depth: 1;
        provider: ModelProviderDto;
        label: "Add more models...";
    }
    | {
        kind: "add-provider";
        key: "add-provider";
        depth: 0;
        label: "Add more providers...";
    };

export type ProviderRowEnterAction =
    | { kind: "configure-provider"; provider: ModelProviderDto }
    | { kind: "edit-provider"; provider: ModelProviderDto }
    | { kind: "view-model"; provider: ModelProviderDto; model: CoreModelDto }
    | { kind: "edit-model"; provider: ModelProviderDto; model: CoreModelDto }
    | { kind: "add-provider" }
    | { kind: "add-model"; provider: ModelProviderDto }
    | { kind: "none" };

export function buildProviderRows(
    providers: ModelProviderDto[],
    expandedProviderCodes: ReadonlySet<string>,
): ProviderListRow[] {
    const rows: ProviderListRow[] = [];

    for (const provider of providers) {
        const expanded = expandedProviderCodes.has(provider.code);
        rows.push({
            kind: "provider",
            key: `provider:${provider.code}`,
            depth: 0,
            provider,
            expanded,
        });

        if (!expanded) {
            continue;
        }

        rows.push(...provider.models.map((model) => ({
            kind: "model" as const,
            key: `model:${provider.code}:${model.code}`,
            depth: 1 as const,
            provider,
            model,
        })));

        if (provider.builtin !== true) {
            rows.push({
                kind: "add-model",
                key: `add-model:${provider.code}`,
                depth: 1,
                provider,
                label: "Add more models...",
            });
        }
    }

    rows.push({
        kind: "add-provider",
        key: "add-provider",
        depth: 0,
        label: "Add more providers...",
    });
    return rows;
}

export function providerRowEnterAction(row: ProviderListRow | undefined): ProviderRowEnterAction {
    if (!row) {
        return { kind: "none" };
    }
    if (row.kind === "add-provider") {
        return { kind: "add-provider" };
    }
    if (row.kind === "add-model") {
        return { kind: "add-model", provider: row.provider };
    }
    if (row.kind === "provider") {
        return row.provider.builtin === true
            ? { kind: "configure-provider", provider: row.provider }
            : { kind: "edit-provider", provider: row.provider };
    }
    return row.provider.builtin === true
        ? { kind: "view-model", provider: row.provider, model: row.model }
        : { kind: "edit-model", provider: row.provider, model: row.model };
}

export function providerRowLabel(row: ProviderListRow): string {
    const prefix = row.depth === 1 ? "  └ " : "";
    if (row.kind === "provider") {
        const marker = row.expanded ? "▾" : "▸";
        return `${marker} ${row.provider.name} (${row.provider.code})`;
    }
    if (row.kind === "model") {
        const modelLabel = row.model.name
            ? `${row.model.name} · ${row.model.code}`
            : row.model.code;
        return `${prefix}${modelLabel}`;
    }
    return `${prefix}${row.label}`;
}

export function providerRowState(row: ProviderListRow): string {
    if (row.kind !== "provider") {
        return "";
    }
    if (row.provider.builtin === true) {
        return row.provider.apiKey.trim() ? "configured" : "unconfigured";
    }
    return "custom";
}
