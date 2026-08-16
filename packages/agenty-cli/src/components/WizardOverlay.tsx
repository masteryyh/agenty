import { useEffect, useMemo, useRef, useState } from "react";

import type { APIType } from "../api/types";
import {
    compatibleProviderTypes,
    createCustomDraft,
    createPresetDraft,
    draftForProvider,
    type ModelDraft,
    modelDraftForProvider,
    type ProviderDraft,
    providerPresets,
    validateModelDraft,
    validateProviderDraft,
} from "../consts/providerPresets";
import { useInput } from "../hooks/useInput";
import { useWindowSize } from "../hooks/useWindowSize";
import { useAppStore } from "../state/store";
import { useTuiRuntime } from "../tui/runtime";
import { BottomDialog, useBottomDialogSize } from "./BottomDialog";
import type { FormField, FormOption } from "./FormPanel";
import { FormPanel } from "./FormPanel";
import { Box, Spinner, Text } from "./ui";
import {
    moveWizardListFocus,
    rowIndexForFocus,
    type WizardListFocus,
} from "./wizardNavigation";
import {
    persistWizardSetup,
    selectedModelId,
    validateWizardDrafts,
} from "./wizardSetup";

type WizardStep = "welcome" | "providers" | "provider-form" | "models" | "model-form" | "saving" | "done";

interface ProviderRow {
    kind: "preset" | "custom";
    presetKey?: string;
    draft?: ProviderDraft;
    label: string;
    description: string;
}

function trunc(value: string, width: number): string {
    if (width <= 0) {
        return "";
    }
    if (value.length <= width) {
        return value;
    }
    if (width === 1) {
        return "…";
    }
    return `${value.slice(0, width - 1)}…`;
}

function pad(value: string, width: number): string {
    const clipped = trunc(value, width);
    return `${clipped}${" ".repeat(Math.max(width - clipped.length, 0))}`;
}

function providerTypeLabel(type: APIType): string {
    return compatibleProviderTypes.find((option) => option.value === type)?.label ?? type;
}

function isAPIType(value: string): value is APIType {
    return compatibleProviderTypes.some((option) => option.value === value);
}

function providerFields(draft: ProviderDraft): FormField[] {
    const typeOptions: FormOption[] = compatibleProviderTypes.map((option) => ({
        label: option.label,
        value: option.value,
    }));
    return [
        {
            key: "name",
            label: "Provider name",
            kind: "text",
            value: draft.name,
            placeholder: "My provider",
        },
        {
            key: "slug",
            label: "Provider slug",
            kind: "text",
            value: draft.slug,
            placeholder: "my-provider",
            readOnly: draft.source === "preset",
        },
        {
            key: "type",
            label: "API protocol",
            kind: "select",
            value: draft.type,
            options: typeOptions,
            readOnly: draft.source === "preset",
        },
        {
            key: "baseUrl",
            label: "Base URL",
            kind: "text",
            value: draft.baseUrl,
            placeholder: "https://api.example.com/v1",
        },
        {
            key: "apiKey",
            label: "API key",
            kind: "text",
            value: draft.apiKey,
            placeholder: "paste a key",
            secret: true,
        },
    ];
}

function modelFields(model: ModelDraft): FormField[] {
    return [
        {
            key: "slug",
            label: "Model ID",
            kind: "text",
            value: model.slug,
            placeholder: "model-id or org/model-id",
        },
        {
            key: "name",
            label: "Model name",
            kind: "text",
            value: model.name,
            placeholder: "Model name",
        },
        {
            key: "contextWindow",
            label: "Context window",
            kind: "text",
            value: String(model.contextWindow),
            placeholder: "128000",
        },
        {
            key: "maxOutputTokens",
            label: "Max output tokens",
            kind: "text",
            value: String(model.maxOutputTokens),
            placeholder: "16384",
        },
    ];
}

function rowsForDrafts(drafts: ProviderDraft[]): ProviderRow[] {
    const rows: ProviderRow[] = providerPresets.map((preset) => ({
        kind: "preset",
        presetKey: preset.key,
        draft: drafts.find((draft) => draft.presetKey === preset.key),
        label: preset.label,
        description: preset.description,
    }));
    rows.push(
        ...drafts
            .filter((draft) => draft.source === "custom")
            .map((draft) => ({
                kind: "custom" as const,
                draft,
                label: draft.name || draft.slug || "Custom provider",
                description: providerTypeLabel(draft.type),
            })),
    );
    return rows;
}

export function WizardOverlay() {
    const { columns, rows } = useWindowSize();
    const width = Math.max(columns - 2, 1);
    const height = Math.max(rows - 1, 12);

    return (
        <BottomDialog width={width} height={height}>
            <WizardContent />
        </BottomDialog>
    );
}

function WizardContent() {
    const client = useAppStore((state) => state.client);
    const finishWizard = useAppStore((state) => state.finishWizard);
    const { exit } = useTuiRuntime();
    const [step, setStep] = useState<WizardStep>("welcome");
    const [drafts, setDrafts] = useState<ProviderDraft[]>([]);
    const [models, setModels] = useState<ModelDraft[]>([]);
    const [editing, setEditing] = useState<ProviderDraft | null>(null);
    const [editingModel, setEditingModel] = useState<ModelDraft | null>(null);
    const [providerFocus, setProviderFocus] = useState<WizardListFocus>({ kind: "row", index: 0 });
    const [modelFocus, setModelFocus] = useState<WizardListFocus>({ kind: "row", index: 0 });
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);
    const draftCounter = useRef(0);

    const rows = useMemo(() => rowsForDrafts(drafts), [drafts]);

    useEffect(() => {
        if (!client) {
            return;
        }
        let cancelled = false;
        setLoading(true);
        void client
            .listProviders()
            .then((providers) => {
                if (cancelled) {
                    return;
                }
                const knownPresetSlugs = new Set(providerPresets.map((preset) => preset.slug));
                const restored = (providers ?? []).map((provider) => {
                    const preset = providerPresets.find((candidate) => candidate.slug === provider.slug);
                    const draft = draftForProvider(provider, preset);
                    return {
                        draft,
                        model: modelDraftForProvider(draft, preset, provider),
                    };
                });
                const configured = restored.filter(({ draft }) =>
                    draft.source === "custom" || knownPresetSlugs.has(draft.slug),
                );
                setDrafts(configured.map(({ draft }) => draft));
                setModels(configured.map(({ model }) => model));
                setError(null);
            })
            .catch((cause: unknown) => {
                if (!cancelled) {
                    setError(cause instanceof Error ? cause.message : String(cause));
                }
            })
            .finally(() => {
                if (!cancelled) {
                    setLoading(false);
                }
            });
        return () => {
            cancelled = true;
        };
    }, [client]);

    const begin = () => {
        setError(null);
        setStep("providers");
    };

    const openPreset = (presetKey: string) => {
        const preset = providerPresets.find((candidate) => candidate.key === presetKey);
        if (!preset) {
            return;
        }
        const draft = drafts.find((candidate) => candidate.presetKey === preset.key) ?? createPresetDraft(preset);
        setEditing(draft);
        setError(null);
        setStep("provider-form");
    };

    const openCustom = (draft?: ProviderDraft) => {
        const next = draft ?? createCustomDraft(`custom:${draftCounter.current++}`);
        setEditing(next);
        setError(null);
        setStep("provider-form");
    };

    const saveDraft = (values: Record<string, string>) => {
        if (!editing) {
            return;
        }
        if (!isAPIType(values.type)) {
            setError("Choose a supported API protocol.");
            return;
        }
        const next: ProviderDraft = {
            ...editing,
            name: values.name.trim(),
            slug: values.slug.trim(),
            type: values.type,
            baseUrl: values.baseUrl.trim(),
            apiKey: values.apiKey.trim(),
        };
        const duplicate = drafts.some(
            (draft) => draft.id !== next.id && draft.slug.trim() !== "" && draft.slug === next.slug,
        );
        if (duplicate) {
            setError(`Provider slug already configured: ${next.slug}`);
            return;
        }
        const validationError = validateProviderDraft(next);
        if (validationError) {
            setError(validationError);
            return;
        }
        setDrafts((current) => {
            const index = current.findIndex((draft) => draft.id === next.id);
            if (index < 0) {
                return [...current, next];
            }
            return current.map((draft, draftIndex) => draftIndex === index ? next : draft);
        });
        setModels((current) => {
            const preset = next.presetKey
                ? providerPresets.find((candidate) => candidate.key === next.presetKey)
                : undefined;
            const index = current.findIndex((model) => model.providerId === next.id);
            if (index < 0) {
                return [...current, modelDraftForProvider(next, preset)];
            }
            return current.map((model, modelIndex) => modelIndex === index
                ? { ...model, providerSlug: next.slug, providerName: next.name }
                : model);
        });
        setProviderFocus({ kind: "row", index: 0 });
        setError(null);
        setStep("providers");
    };

    const continueToModels = () => {
        if (drafts.length === 0) {
            setError("Configure at least one provider to continue.");
            return;
        }
        setModelFocus((current) => ({
            kind: "row",
            index: Math.min(rowIndexForFocus(current), Math.max(models.length - 1, 0)),
        }));
        setError(null);
        setStep("models");
    };

    const saveModel = (values: Record<string, string>) => {
        if (!editingModel) {
            return;
        }
        const next: ModelDraft = {
            ...editingModel,
            slug: values.slug.trim(),
            name: values.name.trim(),
            contextWindow: Number(values.contextWindow),
            maxOutputTokens: Number(values.maxOutputTokens),
        };
        const validationError = validateModelDraft(next);
        if (validationError) {
            setError(validationError);
            return;
        }
        setModels((current) => current.map((model) => model.id === next.id ? next : model));
        setEditingModel(null);
        setError(null);
        setStep("models");
    };

    const saveSetup = async (selectedId: string) => {
        if (!client) {
            return;
        }
        const validationError = validateWizardDrafts(drafts, models, selectedId);
        if (validationError) {
            setError(validationError);
            return;
        }
        setStep("saving");
        setError(null);
        try {
            await persistWizardSetup(client, drafts, models, selectedId);
            setStep("done");
            await finishWizard();
        } catch (cause: unknown) {
            setError(cause instanceof Error ? cause.message : String(cause));
            setStep("models");
        }
    };

    if (step === "welcome") {
        return <WelcomeStep onBegin={begin} onExit={() => exit()} />;
    }
    if (step === "provider-form" && editing) {
        return (
            <Box flexDirection="column" flexGrow={1}>
                {error ? <Text color="red">{error}</Text> : null}
                <FormPanel
                    key={editing.id}
                    title={editing.source === "preset" ? `Configure ${editing.name}` : "Add compatible provider"}
                    fields={providerFields(editing)}
                    actions={[{ key: "save", label: "Save provider" }, { key: "cancel", label: "Back" }]}
                    onAction={(action, values) => {
                        if (action === "save") {
                            saveDraft(values);
                        } else {
                            setError(null);
                            setStep("providers");
                        }
                    }}
                    onClose={() => {
                        setError(null);
                        setStep("providers");
                    }}
                />
            </Box>
        );
    }
    if (step === "model-form" && editingModel) {
        return (
            <Box flexDirection="column" flexGrow={1}>
                {error ? <Text color="red">{error}</Text> : null}
                <FormPanel
                    key={editingModel.id}
                    title={`Configure model for ${editingModel.providerName || editingModel.providerSlug}`}
                    fields={modelFields(editingModel)}
                    actions={[{ key: "save", label: "Save model" }, { key: "cancel", label: "Back" }]}
                    onAction={(action, values) => {
                        if (action === "save") {
                            saveModel(values);
                        } else {
                            setError(null);
                            setEditingModel(null);
                            setStep("models");
                        }
                    }}
                    onClose={() => {
                        setError(null);
                        setEditingModel(null);
                        setStep("models");
                    }}
                />
            </Box>
        );
    }
    if (step === "models") {
        return (
            <ModelStep
                models={models}
                focus={modelFocus}
                error={error}
                onFocus={setModelFocus}
                onEdit={(model) => {
                    setEditingModel(model);
                    setError(null);
                    setStep("model-form");
                }}
                onConfirm={(model) => void saveSetup(selectedModelId(model))}
                onBack={() => {
                    setError(null);
                    setStep("providers");
                }}
            />
        );
    }
    if (step === "saving" || step === "done") {
        return (
            <Box flexDirection="column" padding={1} gap={1}>
                {step === "done" ? (
                    <Text color="green" bold>Setup complete. Starting agenty-cli…</Text>
                ) : (
                    <Spinner label="Saving providers, model, and default agent…" />
                )}
                {error ? <Text color="red">{error}</Text> : null}
            </Box>
        );
    }

    return (
        <ProviderStep
            rows={rows}
            focus={providerFocus}
            loading={loading}
            error={error}
            configuredCount={drafts.length}
            onFocus={setProviderFocus}
            onPreset={openPreset}
            onCustom={openCustom}
            onAddCustom={() => openCustom()}
            onContinue={continueToModels}
            onExit={() => exit()}
        />
    );
}

function WelcomeStep({ onBegin, onExit }: { onBegin: () => void; onExit: () => void }) {
    useInput((input, key) => {
        if (key.return || input.toLowerCase() === "y") {
            onBegin();
        } else if (key.escape || input.toLowerCase() === "n") {
            onExit();
        }
    });

    return (
        <Box flexDirection="column" flexGrow={1} padding={1} gap={1}>
            <Text color="magenta" bold>AGENTY / FIRST RUN</Text>
            <Text color="cyan" bold>Welcome to agenty</Text>
            <Text>Connect a model provider, add one model, and choose the default agent model.</Text>
            <Box flexDirection="column" height={10} borderStyle="single" borderColor="cyan" padding={1} marginTop={1}>
                <Box height={1}>
                    <Text color="cyan" bold>01  Provider access</Text>
                </Box>
                <Box height={1}>
                    <Text dimColor wrap="truncate">Configure OpenAI, Anthropic, Google, or a compatible endpoint.</Text>
                </Box>
                <Box height={1}>
                    <Text color="cyan" bold>02  Model details</Text>
                </Box>
                <Box height={1}>
                    <Text dimColor wrap="truncate">Enter the model ID and limits used by the core loop.</Text>
                </Box>
                <Box height={1}>
                    <Text color="cyan" bold>03  Default agent</Text>
                </Box>
                <Box height={1}>
                    <Text dimColor wrap="truncate">Pick the model used when a new session starts.</Text>
                </Box>
            </Box>
            <Box marginTop={1} gap={3}>
                <Text color="cyan" bold onMouseClick={onBegin}>[Begin setup]</Text>
                <Text color="gray" onMouseClick={onExit}>[Exit]</Text>
            </Box>
            <Text dimColor>Enter/y to begin · Esc/n to exit</Text>
        </Box>
    );
}

function ProviderStep({
    rows,
    focus,
    loading,
    error,
    configuredCount,
    onFocus,
    onPreset,
    onCustom,
    onAddCustom,
    onContinue,
    onExit,
}: {
    rows: ProviderRow[];
    focus: WizardListFocus;
    loading: boolean;
    error: string | null;
    configuredCount: number;
    onFocus: (focus: WizardListFocus) => void;
    onPreset: (presetKey: string) => void;
    onCustom: (draft?: ProviderDraft) => void;
    onAddCustom: () => void;
    onContinue: () => void;
    onExit: () => void;
}) {
    return (
        <Box flexDirection="column" flexGrow={1} gap={1}>
            <Box flexDirection="column">
                <Text color="magenta" bold>01 / Provider connections</Text>
                <Text dimColor>Choose a row to configure it. You can add more than one provider.</Text>
            </Box>
            {loading ? <Spinner label="Loading existing providers…" /> : null}
            {error ? <Text color="red">{error}</Text> : null}
            {!loading ? (
                <ProviderTable
                    rows={rows}
                    focus={focus}
                    onFocus={onFocus}
                    onActivate={(row) => {
                        if (row.kind === "preset" && row.presetKey) {
                            onPreset(row.presetKey);
                        } else if (row.kind === "custom") {
                            onCustom(row.draft);
                        }
                    }}
                    onAddCustom={onAddCustom}
                    onContinue={onContinue}
                    onExit={onExit}
                    canContinue={configuredCount > 0}
                />
            ) : null}
        </Box>
    );
}

function ProviderTable({
    rows,
    focus,
    onFocus,
    onActivate,
    onAddCustom,
    onContinue,
    onExit,
    canContinue,
}: {
    rows: ProviderRow[];
    focus: WizardListFocus;
    onFocus: (focus: WizardListFocus) => void;
    onActivate: (row: ProviderRow) => void;
    onAddCustom: () => void;
    onContinue: () => void;
    onExit: () => void;
    canContinue: boolean;
}) {
    const dialogSize = useBottomDialogSize();
    const compact = dialogSize.width < 72;
    const nameWidth = compact ? Math.max(dialogSize.width - 8, 14) : 22;
    const protocolWidth = compact ? 0 : Math.max(dialogSize.width - nameWidth - 18, 18);

    useInput((input, key) => {
        if (key.escape) {
            onExit();
            return;
        }
        if (key.upArrow) {
            onFocus(moveWizardListFocus(focus, rows.length, "up"));
            return;
        }
        if (key.downArrow) {
            onFocus(moveWizardListFocus(focus, rows.length, "down"));
            return;
        }
        if (key.leftArrow) {
            onFocus(moveWizardListFocus(focus, rows.length, "left"));
            return;
        }
        if (key.rightArrow) {
            onFocus(moveWizardListFocus(focus, rows.length, "right"));
            return;
        }
        if (key.return) {
            if (focus.kind === "action") {
                if (focus.index === 0 && canContinue) {
                    onContinue();
                } else if (focus.index === 1) {
                    onExit();
                }
                return;
            }
            const row = rows[focus.index];
            if (row) {
                onActivate(row);
            }
            return;
        }
        if (input.toLowerCase() === "a") {
            onAddCustom();
        } else if (input.toLowerCase() === "c" && canContinue) {
            onContinue();
        }
    });

    return (
        <Box flexDirection="column" flexGrow={1}>
            <Box height={1} overflow="hidden">
                <Text dimColor>
                    {compact
                        ? `  ${pad("Provider", nameWidth)}  Status`
                        : `  ${pad("Provider", nameWidth)}  ${pad("Protocol", protocolWidth)}  Status`}
                </Text>
            </Box>
            <Box flexDirection="column" flexGrow={1} overflow="hidden">
                {rows.map((row, index) => {
                    const active = focus.kind === "row" && index === focus.index;
                    const configured = !!row.draft;
                    const status = configured
                        ? row.draft?.apiKey.trim()
                            ? "✓ ready"
                            : "! key"
                        : "○ not set";
                    return (
                        <Box
                            key={row.kind === "preset" ? row.presetKey : row.draft?.id}
                            height={1}
                            overflow="hidden"
                            onMouseOver={() => onFocus({ kind: "row", index })}
                            onMouseClick={() => {
                                onFocus({ kind: "row", index });
                                onActivate(row);
                            }}
                        >
                            <Text color={active ? "cyan" : "gray"}>{active ? "❯" : " "}</Text>
                            <Text> </Text>
                            <Box width={nameWidth}>
                                <Text color={active ? "cyan" : "white"} bold={active} wrap="truncate">
                                    {trunc(row.label, nameWidth)}
                                </Text>
                            </Box>
                            {compact ? null : (
                                <>
                                    <Text>  </Text>
                                    <Box width={protocolWidth}>
                                        <Text color={active ? "cyan" : "gray"} wrap="truncate">
                                            {trunc(row.description, protocolWidth)}
                                        </Text>
                                    </Box>
                                </>
                            )}
                            <Text color={configured ? "green" : "gray"}>{status}</Text>
                        </Box>
                    );
                })}
            </Box>
            <Box gap={3} marginTop={1}>
                <Box
                    onMouseOver={() => onFocus({ kind: "action", index: 0, rowIndex: rowIndexForFocus(focus) })}
                    onMouseClick={canContinue ? onContinue : undefined}
                >
                    <Text color={focus.kind === "action" && focus.index === 0 && canContinue ? "cyan" : "gray"} bold={focus.kind === "action" && focus.index === 0 && canContinue}>
                        {focus.kind === "action" && focus.index === 0 ? "[Continue to model]" : " Continue to model "}
                    </Text>
                </Box>
                <Box
                    onMouseOver={() => onFocus({ kind: "action", index: 1, rowIndex: rowIndexForFocus(focus) })}
                    onMouseClick={onExit}
                >
                    <Text color={focus.kind === "action" && focus.index === 1 ? "cyan" : "gray"} bold={focus.kind === "action" && focus.index === 1}>
                        {focus.kind === "action" && focus.index === 1 ? "[Exit]" : " Exit "}
                    </Text>
                </Box>
            </Box>
            <Text dimColor wrap="truncate">
                ↑↓ rows/actions · ←→ choose action · Enter select · a add provider · c continue · Esc exit
            </Text>
        </Box>
    );
}

function ModelStep({
    models,
    focus,
    error,
    onFocus,
    onEdit,
    onConfirm,
    onBack,
}: {
    models: ModelDraft[];
    focus: WizardListFocus;
    error: string | null;
    onFocus: (focus: WizardListFocus) => void;
    onEdit: (model: ModelDraft) => void;
    onConfirm: (model: ModelDraft) => void;
    onBack: () => void;
}) {
    const dialogSize = useBottomDialogSize();
    const compact = dialogSize.width < 72;
    const providerWidth = compact ? Math.max(dialogSize.width - 8, 14) : 20;
    const modelWidth = compact ? 0 : Math.max(dialogSize.width - providerWidth - 24, 18);

    useInput((input, key) => {
        if (key.escape) {
            onBack();
            return;
        }
        if (key.upArrow) {
            onFocus(moveWizardListFocus(focus, models.length, "up"));
            return;
        }
        if (key.downArrow) {
            onFocus(moveWizardListFocus(focus, models.length, "down"));
            return;
        }
        if (key.leftArrow) {
            onFocus(moveWizardListFocus(focus, models.length, "left"));
            return;
        }
        if (key.rightArrow) {
            onFocus(moveWizardListFocus(focus, models.length, "right"));
            return;
        }
        if (key.return) {
            if (focus.kind === "action") {
                if (focus.index === 0) {
                    const model = models[rowIndexForFocus(focus)];
                    if (model) {
                        onConfirm(model);
                    }
                } else if (focus.index === 1) {
                    onBack();
                }
                return;
            }
            const model = models[focus.index];
            if (model) {
                onEdit(model);
            }
            return;
        }
        if (input.toLowerCase() === "d") {
            const model = models[rowIndexForFocus(focus)];
            if (model) {
                onConfirm(model);
            }
        }
    });

    return (
        <Box flexDirection="column" flexGrow={1} gap={1}>
            <Box flexDirection="column">
                <Text color="magenta" bold>02 / Default agent model</Text>
                <Text dimColor>Edit each model, then choose the one used for new sessions with the default agent.</Text>
            </Box>
            {error ? <Text color="red">{error}</Text> : null}
            <Box height={1} overflow="hidden">
                <Text dimColor>
                    {compact
                        ? `  ${pad("Provider", providerWidth)}  Select`
                        : `  ${pad("Provider", providerWidth)}  ${pad("Model", modelWidth)}  Context`}
                </Text>
            </Box>
            <Box flexDirection="column" flexGrow={1} overflow="hidden">
                {models.map((model, index) => {
                    const active = focus.kind === "row" && index === focus.index;
                    const provider = `${model.providerName} (${model.providerSlug})`;
                    const modelLabel = model.name && model.slug ? `${model.name} · ${model.slug}` : "Configure model";
                    return (
                        <Box
                            key={model.id}
                            height={1}
                            overflow="hidden"
                            onMouseOver={() => onFocus({ kind: "row", index })}
                            onMouseClick={() => {
                                onFocus({ kind: "row", index });
                                onEdit(model);
                            }}
                        >
                            <Text color={active ? "cyan" : "gray"}>{active ? "❯" : " "}</Text>
                            <Text> </Text>
                            <Box width={providerWidth}>
                                <Text color={active ? "cyan" : "white"} bold={active} wrap="truncate">
                                    {trunc(provider, providerWidth)}
                                </Text>
                            </Box>
                            {compact ? null : (
                                <>
                                    <Text>  </Text>
                                    <Box width={modelWidth}>
                                        <Text color={active ? "cyan" : "white"} wrap="truncate">
                                            {trunc(modelLabel, modelWidth)}
                                        </Text>
                                    </Box>
                                    <Text>  </Text>
                                </>
                            )}
                            <Text color={active ? "cyan" : "gray"}>{model.contextWindow.toLocaleString()}</Text>
                        </Box>
                    );
                })}
            </Box>
            <Box gap={3} marginTop={1}>
                <Box
                    onMouseOver={() => onFocus({ kind: "action", index: 0, rowIndex: rowIndexForFocus(focus) })}
                    onMouseClick={() => {
                        const model = models[rowIndexForFocus(focus)];
                        if (model) {
                            onConfirm(model);
                        }
                    }}
                >
                    <Text color={focus.kind === "action" && focus.index === 0 ? "cyan" : "gray"} bold={focus.kind === "action" && focus.index === 0}>
                        {focus.kind === "action" && focus.index === 0 ? "[Use selected model]" : " Use selected model "}
                    </Text>
                </Box>
                <Box
                    onMouseOver={() => onFocus({ kind: "action", index: 1, rowIndex: rowIndexForFocus(focus) })}
                    onMouseClick={onBack}
                >
                    <Text color={focus.kind === "action" && focus.index === 1 ? "cyan" : "gray"} bold={focus.kind === "action" && focus.index === 1}>
                        {focus.kind === "action" && focus.index === 1 ? "[Back]" : " Back "}
                    </Text>
                </Box>
            </Box>
            <Text dimColor wrap="truncate">↑↓ rows/actions · ←→ choose action · Enter select · d choose default · Esc back</Text>
        </Box>
    );
}
