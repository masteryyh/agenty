import { useEffect, useMemo, useRef, useState } from "react";

import { type APIType, type ModelProviderDto, STANDARD_REASONING_EFFORTS } from "../api/types";
import {
    compatibleProviderTypes,
    createBuiltinDraft,
    createCustomDraft,
    createModelDraft,
    draftForProvider,
    type ModelDraft,
    modelDraftsForProvider,
    type ProviderDraft,
    validateModelDraft,
    validateProviderDraft,
} from "../consts/providerPresets";
import { useInput } from "../hooks/useInput";
import { useWindowSize } from "../hooks/useWindowSize";
import { useAppStore } from "../state/store";
import { useTuiRuntime } from "../tui/runtime";
import { BottomDialog, useBottomDialogSize } from "./BottomDialog";
import { ConfirmDialog } from "./ConfirmDialog";
import type { FormField, FormOption } from "./FormPanel";
import { FormPanel } from "./FormPanel";
import { List } from "./List";
import {
    createTableLayout,
    type TableColumn,
    TableHeader,
    TableRow,
} from "./Table";
import { TreeList } from "./TreeList";
import { ActionBar, Box, Spinner, Text } from "./ui";
import {
    moveWizardListFocus,
    rowIndexForFocus,
    type WizardListFocus,
} from "./wizardNavigation";
import {
    buildWizardModelRows,
    buildWizardProviderRows,
    wizardModelEnterAction,
    type WizardModelRow,
    type WizardProviderRow,
} from "./wizardRows";
import {
    persistWizardSetup,
    selectedModelId,
    validateWizardDrafts,
} from "./wizardSetup";

type WizardStep = "welcome" | "providers" | "provider-form" | "models" | "model-form" | "saving" | "done";

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
            readOnly: draft.source === "builtin",
        },
        {
            key: "code",
            label: "Provider Code",
            kind: "text",
            value: draft.code,
            placeholder: "my-provider",
            readOnly: draft.source === "builtin" || draft.originalCode !== undefined,
        },
        {
            key: "type",
            label: "API protocol",
            kind: "select",
            value: draft.type,
            options: typeOptions,
            readOnly: draft.source === "builtin",
        },
        {
            key: "baseUrl",
            label: "Base URL",
            kind: "text",
            value: draft.baseUrl,
            placeholder: "https://api.example.com/v1",
            readOnly: draft.source === "builtin",
        },
        {
            key: "freeFormTool",
            label: "Free-form apply_patch",
            kind: "boolean",
            value: String(draft.freeFormTool),
            readOnly: draft.source === "builtin" || draft.type !== "openai",
            focusable: draft.source !== "builtin" && draft.type === "openai",
            visible: draft.type === "openai",
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
            key: "code",
            label: "Model Code",
            kind: "text",
            value: model.code,
            placeholder: "model-code or org/model-code",
            readOnly: model.isBuiltin || model.originalCode !== undefined,
        },
        {
            key: "name",
            label: "Model name",
            kind: "text",
            value: model.name,
            placeholder: "Model name",
            readOnly: model.isBuiltin,
        },
        {
            key: "contextWindow",
            label: "Context window",
            kind: "text",
            value: String(model.contextWindow),
            placeholder: "128000",
            readOnly: model.isBuiltin,
        },
        {
            key: "maxOutputTokens",
            label: "Max output tokens",
            kind: "text",
            value: String(model.maxOutputTokens),
            placeholder: "8192",
            readOnly: model.isBuiltin,
        },
        {
            key: "multiModal",
            label: "Multimodal",
            kind: "boolean",
            value: model.multiModal ? "true" : "false",
            readOnly: model.isBuiltin,
        },
        {
            key: "light",
            label: "Light",
            kind: "boolean",
            value: model.light ? "true" : "false",
            readOnly: model.isBuiltin,
        },
        {
            key: "reasoning",
            label: "Reasoning",
            kind: "boolean",
            value: model.reasoningEfforts.length > 0 ? "true" : "false",
            readOnly: model.isBuiltin,
        },
    ];
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
    const [builtinProviders, setBuiltinProviders] = useState<ModelProviderDto[]>([]);
    const [models, setModels] = useState<ModelDraft[]>([]);
    const [editing, setEditing] = useState<ProviderDraft | null>(null);
    const [editingModel, setEditingModel] = useState<ModelDraft | null>(null);
    const [deletingModel, setDeletingModel] = useState<ModelDraft | null>(null);
    const [selectedModelDraftId, setSelectedModelDraftId] = useState<string | null>(null);
    const [providerFocus, setProviderFocus] = useState<WizardListFocus>({ kind: "row", index: 0 });
    const [modelFocus, setModelFocus] = useState<WizardListFocus>({ kind: "row", index: 0 });
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);
    const draftCounter = useRef(0);
    const modelCounter = useRef(0);

    const providerRows = useMemo(
        () => buildWizardProviderRows(drafts, builtinProviders),
        [drafts, builtinProviders],
    );
    const modelRows = useMemo(
        () => buildWizardModelRows(drafts, models),
        [drafts, models],
    );

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
                const allProviders = providers ?? [];
                setBuiltinProviders(allProviders.filter((provider) => provider.builtin === true));
                const restored = allProviders.map((provider) => {
                    const draft = draftForProvider(provider);
                    return { provider, draft };
                });
                const configured = restored.filter(({ provider, draft }) =>
                    provider.builtin !== true || draft.apiKey.trim() !== "",
                );
                const restoredModels = configured.flatMap(({ provider, draft }) =>
                    modelDraftsForProvider(draft, provider),
                );
                setDrafts(configured.map(({ draft }) => draft));
                setModels(restoredModels);
                setSelectedModelDraftId(
                    restoredModels.find((model) => model.isDefault)?.id ?? restoredModels[0]?.id ?? null,
                );
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

    const openBuiltin = (code: string) => {
        const provider = builtinProviders.find((candidate) => candidate.code === code);
        if (!provider) {
            return;
        }
        const draft = drafts.find((candidate) => candidate.code === code) ?? createBuiltinDraft(provider);
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
            code: values.code.trim(),
            type: values.type,
            baseUrl: values.baseUrl.trim(),
            apiKey: values.apiKey.trim(),
            freeFormTool: values.type === "openai" && values.freeFormTool === "true",
        };
        const duplicate = drafts.some(
            (draft) => draft.id !== next.id && draft.code.trim() !== "" && draft.code === next.code,
        );
        if (duplicate) {
            setError(`Provider code already configured: ${next.code}`);
            return;
        }
        const validationError = validateProviderDraft(next);
        if (validationError) {
            setError(validationError);
            return;
        }
        const existingDraft = drafts.find((draft) => draft.id === next.id);
        const builtinProvider = builtinProviders.find((candidate) => candidate.code === next.code);
        const addedModels = !existingDraft && builtinProvider
            ? modelDraftsForProvider(next, builtinProvider)
            : [];
        setDrafts((current) => {
            const index = current.findIndex((draft) => draft.id === next.id);
            if (index < 0) {
                return [...current, next];
            }
            return current.map((draft, draftIndex) => draftIndex === index ? next : draft);
        });
        setModels((current) => {
            if (!existingDraft) {
                return [...current, ...addedModels];
            }
            return current.map((model) => model.providerId === next.id
                ? { ...model, providerCode: next.code, providerName: next.name }
                : model);
        });
        if (!selectedModelDraftId && addedModels[0]) {
            setSelectedModelDraftId(addedModels[0].id);
        }
        setProviderFocus({ kind: "row", index: 0 });
        setError(null);
        setStep("providers");
    };

    const continueToModels = () => {
        if (drafts.length === 0) {
            setError("Configure at least one provider to continue.");
            return;
        }
        const selectedIndex = modelRows.findIndex((row) =>
            row.kind === "model" && row.model.id === selectedModelDraftId,
        );
        setModelFocus({ kind: "row", index: selectedIndex >= 0 ? selectedIndex : 0 });
        setError(null);
        setStep("models");
    };

    const addModel = (provider: ProviderDraft) => {
        if (provider.builtin) {
            return;
        }
        const model = createModelDraft(
            provider,
            `${provider.id}:draft-model:${modelCounter.current++}`,
        );
        setEditingModel(model);
        setError(null);
        setStep("model-form");
    };

    const editModel = (model: ModelDraft) => {
        if (model.isBuiltin) {
            return;
        }
        setEditingModel(model);
        setError(null);
        setStep("model-form");
    };

    const saveModel = (values: Record<string, string>) => {
        if (!editingModel) {
            return;
        }
        const next: ModelDraft = {
            ...editingModel,
            code: values.code.trim(),
            name: values.name.trim(),
            contextWindow: Number(values.contextWindow),
            maxOutputTokens: Number(values.maxOutputTokens),
            multiModal: values.multiModal === "true",
            light: values.light === "true",
            reasoningEfforts: values.reasoning === "true" ? [...STANDARD_REASONING_EFFORTS] : [],
        };
        const validationError = validateModelDraft(next);
        if (validationError) {
            setError(validationError);
            return;
        }
        const duplicate = models.some((model) =>
            model.id !== next.id &&
            model.providerId === next.providerId &&
            model.code.trim() === next.code,
        );
        if (duplicate) {
            setError(`Model Code already configured: ${next.code}`);
            return;
        }
        const existing = models.some((model) => model.id === next.id);
        setModels((current) => existing
            ? current.map((model) => model.id === next.id ? next : model)
            : [...current, next]);
        if (!selectedModelDraftId) {
            setSelectedModelDraftId(next.id);
        }
        setEditingModel(null);
        setError(null);
        setStep("models");
    };

    const removeModel = (target: ModelDraft) => {
        const remaining = models.filter((model) => model.id !== target.id);
        setModels(remaining);
        if (selectedModelDraftId === target.id) {
            setSelectedModelDraftId(remaining[0]?.id ?? null);
        }
        setDeletingModel(null);
        setModelFocus({ kind: "row", index: 0 });
        setError(null);
        setStep("models");
    };

    const saveSetup = async () => {
        if (!client) {
            return;
        }
        const selectedId = selectedModelDraftId ?? "";
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
                    title={editing.source === "builtin" ? `Configure ${editing.name}` : "Add compatible provider"}
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
            <Box flexDirection="column" flexGrow={1} width="100%" position="relative">
                <FormPanel
                    key={editingModel.id}
                    title={editingModel.originalCode
                        ? `Edit model: ${editingModel.name}`
                        : `Add model to ${editingModel.providerName || editingModel.providerCode}`}
                    fields={modelFields(editingModel)}
                    active={!deletingModel}
                    error={error}
                    shortcutHint={editingModel.originalCode ? "d delete" : undefined}
                    onShortcut={(input) => {
                        if (input.toLowerCase() !== "d" || !editingModel.originalCode) {
                            return false;
                        }
                        setDeletingModel(editingModel);
                        return true;
                    }}
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
                {deletingModel ? (
                    <ConfirmDialog
                        title={`Delete model "${deletingModel.name}"?`}
                        message="The model will be removed when setup is completed."
                        onConfirm={() => removeModel(deletingModel)}
                        onCancel={() => setDeletingModel(null)}
                    />
                ) : null}
            </Box>
        );
    }
    if (step === "models") {
        return (
            <ModelStep
                rows={modelRows}
                focus={modelFocus}
                error={error}
                selectedModelId={selectedModelDraftId}
                onFocus={setModelFocus}
                onSelect={(model) => {
                    setSelectedModelDraftId(model.id);
                    setError(null);
                }}
                onAdd={addModel}
                onEdit={editModel}
                onComplete={() => void saveSetup()}
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
            rows={providerRows}
            focus={providerFocus}
            loading={loading}
            error={error}
            configuredCount={drafts.length}
            onFocus={setProviderFocus}
            onBuiltin={openBuiltin}
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
            <Text>Connect a model provider, configure its models, and choose the default agent model.</Text>
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
                    <Text dimColor wrap="truncate">Enter the model code and limits used by the core loop.</Text>
                </Box>
                <Box height={1}>
                    <Text color="cyan" bold>03  Default agent</Text>
                </Box>
                <Box height={1}>
                    <Text dimColor wrap="truncate">Pick the model used when a new session starts.</Text>
                </Box>
            </Box>
            <ActionBar
                actions={[
                    { key: "begin", label: "Begin setup" },
                    { key: "exit", label: "Exit" },
                ]}
                activeKey="begin"
                gap={3}
                onAction={(key) => {
                    if (key === "begin") {
                        onBegin();
                    } else {
                        onExit();
                    }
                }}
            />
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
    onBuiltin,
    onCustom,
    onAddCustom,
    onContinue,
    onExit,
}: {
    rows: WizardProviderRow[];
    focus: WizardListFocus;
    loading: boolean;
    error: string | null;
    configuredCount: number;
    onFocus: (focus: WizardListFocus) => void;
    onBuiltin: (code: string) => void;
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
                        if (row.kind === "builtin") {
                            onBuiltin(row.code);
                        } else if (row.kind === "custom") {
                            onCustom(row.draft);
                        } else {
                            onAddCustom();
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
    rows: WizardProviderRow[];
    focus: WizardListFocus;
    onFocus: (focus: WizardListFocus) => void;
    onActivate: (row: WizardProviderRow) => void;
    onAddCustom: () => void;
    onContinue: () => void;
    onExit: () => void;
    canContinue: boolean;
}) {
    const dialogSize = useBottomDialogSize();
    const maxVisible = Math.max(dialogSize.height - 8, 1);
    const providerStatus = (row: WizardProviderRow): string => {
        if (row.kind === "add") {
            return "";
        }
        if (!row.draft) {
            return "○ not set";
        }
        return row.draft.apiKey.trim() ? "✓ ready" : "! key";
    };
    const columns: Array<TableColumn<WizardProviderRow>> = [
        {
            key: "provider",
            header: "Provider",
            value: (row) => row.label,
            render: (row, selected) => (
                <Text color={selected ? "cyan" : "white"} bold={selected} wrap="truncate">
                    {row.label}
                </Text>
            ),
        },
        {
            key: "protocol",
            header: "Protocol",
            value: (row) => row.kind === "add" ? "" : row.description,
            render: (row, selected) => (
                <Text color={selected ? "cyan" : "gray"} wrap="truncate">
                    {row.kind === "add" ? "" : row.description}
                </Text>
            ),
        },
        {
            key: "status",
            header: "Status",
            value: providerStatus,
            render: (row) => (
                <Text color={providerStatus(row) === "✓ ready" ? "green" : "gray"}>
                    {providerStatus(row)}
                </Text>
            ),
        },
    ];
    const tableLayout = createTableLayout(
        columns,
        rows,
        Math.max(dialogSize.width - 2, 0),
    );

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
                <Box width={2} height={1}><Text> </Text></Box>
                <TableHeader columns={tableLayout} />
            </Box>
            <List
                items={rows}
                cursor={rowIndexForFocus(focus)}
                visibleCount={maxVisible}
                active={focus.kind === "row"}
                getKey={(row) => row.key}
                onCursor={(index) => onFocus({ kind: "row", index })}
                onActivate={onActivate}
                renderItem={(row, { selected }) => (
                    <TableRow
                        columns={tableLayout}
                        row={row}
                        selected={selected}
                    />
                )}
            />
            <ActionBar
                actions={[
                    { key: "continue", label: "Continue to model", disabled: !canContinue },
                    { key: "exit", label: "Exit" },
                ]}
                activeKey={focus.kind === "action"
                    ? focus.index === 0 ? "continue" : "exit"
                    : undefined}
                gap={3}
                onAction={(key) => {
                    if (key === "continue") {
                        onContinue();
                    } else {
                        onExit();
                    }
                }}
            />
            <Text dimColor wrap="truncate">
                ↑↓ rows/actions · ←→ choose action · Enter select · a add provider · c continue · Esc exit
            </Text>
        </Box>
    );
}

function ModelStep({
    rows,
    focus,
    error,
    selectedModelId,
    onFocus,
    onSelect,
    onAdd,
    onEdit,
    onComplete,
    onBack,
}: {
    rows: WizardModelRow[];
    focus: WizardListFocus;
    error: string | null;
    selectedModelId: string | null;
    onFocus: (focus: WizardListFocus) => void;
    onSelect: (model: ModelDraft) => void;
    onAdd: (provider: ProviderDraft) => void;
    onEdit: (model: ModelDraft) => void;
    onComplete: () => void;
    onBack: () => void;
}) {
    const dialogSize = useBottomDialogSize();
    const currentRow = rows[rowIndexForFocus(focus)];
    const currentModel = currentRow?.kind === "model" ? currentRow.model : undefined;
    const maxVisible = Math.max(dialogSize.height - 10, 1);
    const rowLabel = (row: WizardModelRow): string => {
        const providerLabel = `${row.provider.name} (${row.provider.code})`;
        if (row.kind === "provider") {
            return providerLabel;
        }
        const childLabel = row.kind === "model"
            ? `${row.model.name} · ${row.model.code}`
            : row.label;
        return `└ ${childLabel}`;
    };
    const rowContext = (row: WizardModelRow): string => {
        if (row.kind === "model") {
            return row.model.contextWindow.toLocaleString();
        }
        if (row.kind === "provider") {
            return `${row.modelCount} model${row.modelCount === 1 ? "" : "s"}`;
        }
        return "";
    };
    const rowState = (row: WizardModelRow): string =>
        row.kind === "model" && row.model.id === selectedModelId ? "selected" : "";
    const columns: Array<TableColumn<WizardModelRow>> = [
        {
            key: "model",
            header: "Provider / model",
            value: rowLabel,
            render: (row, active) => (
                <Text
                    color={rowState(row) ? "green" : active ? "cyan" : row.kind === "provider" ? "white" : "gray"}
                    bold={active || row.kind === "provider"}
                    wrap="truncate"
                >
                    {rowLabel(row)}
                </Text>
            ),
        },
        {
            key: "context",
            header: "Context",
            value: rowContext,
            render: (row, active) => (
                <Text color={active ? "cyan" : "gray"}>{rowContext(row)}</Text>
            ),
        },
        {
            key: "state",
            header: "State",
            value: rowState,
            render: (row) => (
                <Text color={rowState(row) ? "green" : "gray"}>{rowState(row)}</Text>
            ),
        },
    ];
    const tableLayout = createTableLayout(
        columns,
        rows,
        Math.max(dialogSize.width - 2, 0),
    );

    useInput((input, key) => {
        if (key.escape) {
            onBack();
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
                if (focus.index === 0) {
                    onComplete();
                } else if (focus.index === 1) {
                    onBack();
                }
                return;
            }
            const row = rows[focus.index];
            const action = wizardModelEnterAction(row);
            if (action.kind === "select") {
                onSelect(action.model);
            } else if (action.kind === "edit") {
                onEdit(action.model);
            } else if (action.kind === "add") {
                onAdd(action.provider);
            }
            return;
        }
        const lower = input.toLowerCase();
        if (lower === "c") {
            onComplete();
        } else if (lower === "s" && currentModel) {
            onSelect(currentModel);
        }
    });

    return (
        <Box flexDirection="column" flexGrow={1} gap={1}>
            <Box flexDirection="column">
                <Text color="magenta" bold>02 / Default agent model</Text>
                <Text dimColor>Choose a model. Custom providers also allow model management here.</Text>
            </Box>
            {error ? <Text color="red">{error}</Text> : null}
            <Box height={1} overflow="hidden">
                <Box width={2} height={1}><Text> </Text></Box>
                <TableHeader columns={tableLayout} />
            </Box>
            <TreeList
                items={rows.map((row) => ({
                    key: row.key,
                    depth: row.kind === "provider" ? 0 : 1,
                    value: row,
                }))}
                cursor={rowIndexForFocus(focus)}
                visibleCount={maxVisible}
                active={focus.kind === "row"}
                onCursor={(index) => onFocus({ kind: "row", index })}
                onActivate={(row) => {
                    if (row.kind === "model") {
                        onSelect(row.model);
                    } else if (row.kind === "add-model") {
                        onAdd(row.provider);
                    }
                }}
                renderItem={(row, { selected: active }) => (
                    <TableRow
                        columns={tableLayout}
                        row={row}
                        selected={active}
                    />
                )}
            />
            <ActionBar
                actions={[
                    { key: "complete", label: "Complete setup" },
                    { key: "back", label: "Back" },
                ]}
                activeKey={focus.kind === "action"
                    ? focus.index === 0 ? "complete" : "back"
                    : undefined}
                gap={3}
                onAction={(key) => {
                    if (key === "complete") {
                        onComplete();
                    } else {
                        onBack();
                    }
                }}
            />
            <Text dimColor wrap="truncate">
                ↑↓ rows/actions · Enter edit/add · click/s select · c complete · Esc back
            </Text>
        </Box>
    );
}
