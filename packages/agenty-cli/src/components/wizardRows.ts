import type { APIType, ModelProviderDto } from "../api/types";
import {
    compatibleProviderTypes,
    type ModelDraft,
    type ProviderDraft,
} from "../consts/providerPresets";

export type WizardProviderRow =
    | {
        kind: "builtin" | "custom";
        key: string;
        code: string;
        draft?: ProviderDraft;
        label: string;
        description: string;
    }
    | {
        kind: "add";
        key: "add-provider";
        label: "Add more provider...";
    };

export type WizardModelRow =
    | {
        kind: "provider";
        key: string;
        provider: ProviderDraft;
        modelCount: number;
    }
    | {
        kind: "model";
        key: string;
        provider: ProviderDraft;
        model: ModelDraft;
    }
    | {
        kind: "add-model";
        key: string;
        provider: ProviderDraft;
        label: "Add more models...";
    };

export type WizardModelEnterAction =
    | { kind: "select"; model: ModelDraft }
    | { kind: "edit"; model: ModelDraft }
    | { kind: "add"; provider: ProviderDraft }
    | { kind: "none" };

function providerTypeLabel(type: APIType): string {
    return compatibleProviderTypes.find((option) => option.value === type)?.label ?? type;
}

export function buildWizardProviderRows(
    drafts: ProviderDraft[],
    builtinProviders: ModelProviderDto[],
): WizardProviderRow[] {
    const rows: WizardProviderRow[] = builtinProviders.map((provider) => ({
        kind: "builtin",
        key: `builtin:${provider.code}`,
        code: provider.code,
        draft: drafts.find((draft) => draft.code === provider.code),
        label: provider.name,
        description: providerTypeLabel(provider.type),
    }));
    rows.push(
        ...drafts
            .filter((draft) => draft.source === "custom")
            .map((draft) => ({
                kind: "custom" as const,
                key: draft.id,
                code: draft.code,
                draft,
                label: draft.name || draft.code || "Custom provider",
                description: providerTypeLabel(draft.type),
            })),
        {
            kind: "add",
            key: "add-provider",
            label: "Add more provider...",
        },
    );
    return rows;
}

export function buildWizardModelRows(
    providers: ProviderDraft[],
    models: ModelDraft[],
): WizardModelRow[] {
    const rows: WizardModelRow[] = [];
    for (const provider of providers) {
        const providerModels = models.filter((model) => model.providerId === provider.id);
        rows.push({
            kind: "provider",
            key: provider.id,
            provider,
            modelCount: providerModels.length,
        });
        rows.push(...providerModels.map((model) => ({
            kind: "model" as const,
            key: model.id,
            provider,
            model,
        })));
        if (!provider.builtin) {
            rows.push({
                kind: "add-model",
                key: `${provider.id}:add-model`,
                provider,
                label: "Add more models...",
            });
        }
    }
    return rows;
}

export function wizardModelEnterAction(row: WizardModelRow | undefined): WizardModelEnterAction {
    if (!row || row.kind === "provider") {
        return { kind: "none" };
    }
    if (row.kind === "add-model") {
        return { kind: "add", provider: row.provider };
    }
    if (row.model.isBuiltin) {
        return { kind: "select", model: row.model };
    }
    return { kind: "edit", model: row.model };
}
