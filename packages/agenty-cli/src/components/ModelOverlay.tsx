import { useCallback, useEffect, useRef, useState } from "react";

import type {
    CoreModelDto,
    CreateModelDto,
    ModelProviderDto,
    ReasoningEffort,
    UpdateModelDto,
} from "../api/types";
import { isBuiltinProvider } from "../consts/providerPresets";
import { useInput } from "../hooks/useInput";
import { useAppStore } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";
import type { FormField } from "./FormPanel";
import { FormPanel } from "./FormPanel";
import { Box, Spinner, Text } from "./ui";

const REASONING_EFFORTS: readonly ReasoningEffort[] = [
    "off",
    "low",
    "medium",
    "high",
    "xhigh",
    "max",
];

type Mode =
    | { kind: "list" }
    | { kind: "create"; provider: ModelProviderDto }
    | { kind: "edit"; provider: ModelProviderDto; target: CoreModelDto }
    | { kind: "confirm-delete"; provider: ModelProviderDto; target: CoreModelDto };

export function initialProviderIndex(
    providers: ModelProviderDto[],
    providerCode?: string,
): number {
    if (providers.length === 0) {
        return 0;
    }
    const index = providerCode
        ? providers.findIndex((provider) => provider.code === providerCode)
        : -1;
    return index >= 0 ? index : 0;
}

export function initialModelIndex(
    provider: ModelProviderDto | undefined,
    modelCode?: string,
): number {
    const models = provider?.models ?? [];
    if (models.length === 0) {
        return 0;
    }
    const requested = modelCode
        ? models.findIndex((model) => model.code === modelCode)
        : -1;
    if (requested >= 0) {
        return requested;
    }
    const defaultIndex = models.findIndex((model) => model.isDefault);
    return defaultIndex >= 0 ? defaultIndex : 0;
}

export function parseReasoningMapping(raw: string): Record<string, ReasoningEffort> | undefined {
    const trimmed = raw.trim();
    if (!trimmed) {
        return undefined;
    }

    let parsed: unknown;
    try {
        parsed = JSON.parse(trimmed);
    } catch {
        throw new Error("Reasoning mapping must be a JSON object.");
    }
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        throw new Error("Reasoning mapping must be a JSON object.");
    }

    const mapping: Record<string, ReasoningEffort> = {};
    for (const [nativeEffort, agentyEffort] of Object.entries(parsed)) {
        if (!nativeEffort.trim()) {
            throw new Error("Reasoning mapping keys cannot be empty.");
        }
        if (typeof agentyEffort !== "string" || !REASONING_EFFORTS.includes(agentyEffort as ReasoningEffort)) {
            throw new Error(`Invalid reasoning effort for ${nativeEffort}.`);
        }
        mapping[nativeEffort] = agentyEffort as ReasoningEffort;
    }
    return mapping;
}

function serializeReasoningMapping(mapping?: Record<string, ReasoningEffort>): string {
    return mapping && Object.keys(mapping).length > 0 ? JSON.stringify(mapping) : "";
}

function parsePositiveInteger(raw: string, label: string): number {
    const value = Number(raw.trim());
    if (!Number.isSafeInteger(value) || value <= 0) {
        throw new Error(`${label} must be a positive integer.`);
    }
    return value;
}

export function modelInputFromValues(
    providerCode: string,
    values: Record<string, string>,
): CreateModelDto {
    const modelCode = values.code.trim();
    const name = values.name.trim();
    if (!modelCode) {
        throw new Error("Model Code is required.");
    }
    if (!name) {
        throw new Error("Model name is required.");
    }

    return {
        providerCode,
        modelCode,
        name,
        contextWindow: parsePositiveInteger(values.contextWindow, "Context window"),
        multiModal: values.multiModal === "true",
        light: values.light === "true",
        isDefault: values.isDefault === "true",
        reasoningEffortMapping: parseReasoningMapping(values.reasoningMapping),
    };
}

export function modelUpdateFromValues(
    target: CoreModelDto,
    values: Record<string, string>,
): UpdateModelDto {
    const input = modelInputFromValues(target.code, values);
    return {
        name: input.name,
        contextWindow: input.contextWindow,
        multiModal: input.multiModal,
        light: input.light,
        isDefault: input.isDefault,
        reasoningEffortMapping: input.reasoningEffortMapping,
    };
}

function modelFields(target?: CoreModelDto): FormField[] {
    return [
        {
            key: "code",
            label: "Model Code",
            kind: "text",
            value: target?.code ?? "",
            placeholder: "model-code or org/model-code",
            readOnly: !!target,
        },
        {
            key: "name",
            label: "Model name",
            kind: "text",
            value: target?.name ?? "",
            placeholder: "Model name",
        },
        {
            key: "contextWindow",
            label: "Context window",
            kind: "text",
            value: target ? String(target.contextWindow) : "128000",
            placeholder: "128000",
        },
        {
            key: "multiModal",
            label: "Multimodal",
            kind: "boolean",
            value: target?.multiModal ? "true" : "false",
        },
        {
            key: "light",
            label: "Light",
            kind: "boolean",
            value: target?.light ? "true" : "false",
        },
        {
            key: "isDefault",
            label: "Default",
            kind: "boolean",
            value: target?.isDefault ? "true" : "false",
        },
        {
            key: "reasoningMapping",
            label: "Reasoning map",
            kind: "text",
            value: serializeReasoningMapping(target?.reasoningEffortMapping),
            placeholder: "{\"native\":\"medium\"}",
        },
    ];
}

export function ModelOverlay() {
    const client = useAppStore((state) => state.client);
    const sessionModel = useAppStore((state) => state.session?.currentModel);
    const setToast = useAppStore((state) => state.setToast);
    const setOverlay = useAppStore((state) => state.setOverlay);
    const switchModel = useAppStore((state) => state.switchModel);

    const [providers, setProviders] = useState<ModelProviderDto[] | null>(null);
    const [providerCursor, setProviderCursor] = useState(0);
    const [modelCursor, setModelCursor] = useState(0);
    const [mode, setMode] = useState<Mode>({ kind: "list" });
    const modeRef = useRef(mode);
    modeRef.current = mode;
    const selectedProviderCodeRef = useRef(sessionModel?.providerCode);
    const selectedModelCodeRef = useRef(sessionModel?.modelCode);

    const reload = useCallback(async () => {
        if (!client) {
            return;
        }
        try {
            const list = await client.listProviders();
            const nextProviderIndex = initialProviderIndex(
                list,
                selectedProviderCodeRef.current,
            );
            const nextProvider = list[nextProviderIndex];
            const nextModelIndex = initialModelIndex(
                nextProvider,
                selectedModelCodeRef.current,
            );
            const nextModel = nextProvider?.models[nextModelIndex];
            selectedProviderCodeRef.current = nextProvider?.code;
            selectedModelCodeRef.current = nextModel?.code;
            setProviders(list);
            setProviderCursor(nextProviderIndex);
            setModelCursor(nextModelIndex);
        } catch (error) {
            setToast(`failed to load models: ${(error as Error).message}`, true);
            setProviders([]);
        }
    }, [client, setToast]);

    useEffect(() => {
        void reload();
    }, [reload]);

    const close = () => setOverlay(null);

    useInput((input, key) => {
        if (modeRef.current.kind !== "list") {
            return;
        }
        if (providers !== null && providers.length > 0) {
            return;
        }
        if (key.escape) {
            close();
            return;
        }
        if (input.toLowerCase() === "a") {
            setToast("No provider is available for a new model.", true);
        }
    });

    const selectProvider = (index: number) => {
        const provider = providers?.[index];
        if (!provider) {
            return;
        }
        selectedProviderCodeRef.current = provider.code;
        const nextModelIndex = initialModelIndex(provider);
        selectedModelCodeRef.current = provider.models[nextModelIndex]?.code;
        setProviderCursor(index);
        setModelCursor(nextModelIndex);
    };

    const selectModelCursor = (index: number) => {
        const provider = providers?.[providerCursor];
        const model = provider?.models[index];
        if (model) {
            selectedModelCodeRef.current = model.code;
        }
        setModelCursor(index);
    };

    const handleSwitch = async (model: CoreModelDto) => {
        if (!client || !providers) {
            return;
        }
        const provider = providers[providerCursor];
        if (!provider) {
            return;
        }
        const projected = {
            ...model,
            providerCode: provider.code,
            providerName: provider.name,
        };
        await switchModel(projected);
    };

    const handleCreate = async (provider: ModelProviderDto, values: Record<string, string>) => {
        if (!client) {
            return;
        }
        try {
            const input = modelInputFromValues(provider.code, values);
            const created = await client.createModel(input);
            selectedModelCodeRef.current = created.code;
            await reload();
            setMode({ kind: "list" });
            setToast(`Model added: ${provider.name} · ${created.name}`);
        } catch (error) {
            setToast(`add model failed: ${(error as Error).message}`, true);
        }
    };

    const handleUpdate = async (
        provider: ModelProviderDto,
        target: CoreModelDto,
        values: Record<string, string>,
    ) => {
        if (!client) {
            return;
        }
        try {
            const update = modelUpdateFromValues(target, values);
            const updated = await client.updateModel(provider.code, target.code, update);
            selectedModelCodeRef.current = updated.code;
            await reload();
            setMode({ kind: "list" });
            setToast(`Model updated: ${provider.name} · ${updated.name}`);
        } catch (error) {
            setToast(`update model failed: ${(error as Error).message}`, true);
        }
    };

    const handleDelete = async (provider: ModelProviderDto, target: CoreModelDto) => {
        if (!client) {
            return;
        }
        try {
            await client.deleteModel(provider.code, target.code);
            selectedModelCodeRef.current = undefined;
            await reload();
            setMode({ kind: "list" });
            setToast(`Model deleted: ${provider.name} · ${target.name}`);
        } catch (error) {
            setToast(`delete model failed: ${(error as Error).message}`, true);
        }
    };

    if (mode.kind === "create") {
        return (
            <ModelForm
                title={`Add model to ${mode.provider.name}`}
                fields={modelFields()}
                onSave={(values) => void handleCreate(mode.provider, values)}
                onClose={() => setMode({ kind: "list" })}
            />
        );
    }

    if (mode.kind === "edit") {
        return (
            <ModelForm
                title={`Edit: ${mode.target.name}`}
                fields={modelFields(mode.target)}
                onSave={(values) => void handleUpdate(mode.provider, mode.target, values)}
                onClose={() => setMode({ kind: "list" })}
            />
        );
    }

    if (mode.kind === "confirm-delete") {
        return (
            <DeleteConfirm
                target={mode.target}
                onConfirm={() => void handleDelete(mode.provider, mode.target)}
                onCancel={() => setMode({ kind: "list" })}
            />
        );
    }

    return (
        <Box flexDirection="column" flexGrow={1}>
            <Box marginBottom={1}>
                <Text color="magenta" bold>Models</Text>
            </Box>
            {providers === null ? (
                <Spinner label="Loading..." />
            ) : providers.length === 0 ? (
                <Text dimColor>No providers configured.</Text>
            ) : (
                <ModelList
                    providers={providers}
                    providerCursor={providerCursor}
                    modelCursor={modelCursor}
                    currentModel={sessionModel}
                    onProviderCursor={selectProvider}
                    onModelCursor={selectModelCursor}
                    onSwitch={(model) => void handleSwitch(model)}
                    onAdd={(provider) => setMode({ kind: "create", provider })}
                    onEdit={(provider, target) => setMode({ kind: "edit", provider, target })}
                    onDelete={(provider, target) => setMode({ kind: "confirm-delete", provider, target })}
                    onClose={close}
                />
            )}
        </Box>
    );
}

function ModelForm({
    title,
    fields,
    onSave,
    onClose,
}: {
    title: string;
    fields: FormField[];
    onSave: (values: Record<string, string>) => void;
    onClose: () => void;
}) {
    return (
        <FormPanel
            title={title}
            fields={fields}
            actions={[{ key: "save", label: "Save model" }, { key: "cancel", label: "Cancel" }]}
            onAction={(action, values) => {
                if (action === "save") {
                    onSave(values);
                } else {
                    onClose();
                }
            }}
            onClose={onClose}
        />
    );
}

function ModelList({
    providers,
    providerCursor,
    modelCursor,
    currentModel,
    onProviderCursor,
    onModelCursor,
    onSwitch,
    onAdd,
    onEdit,
    onDelete,
    onClose,
}: {
    providers: ModelProviderDto[];
    providerCursor: number;
    modelCursor: number;
    currentModel?: { providerCode: string; modelCode: string };
    onProviderCursor: (index: number) => void;
    onModelCursor: (index: number) => void;
    onSwitch: (model: CoreModelDto) => void;
    onAdd: (provider: ModelProviderDto) => void;
    onEdit: (provider: ModelProviderDto, model: CoreModelDto) => void;
    onDelete: (provider: ModelProviderDto, model: CoreModelDto) => void;
    onClose: () => void;
}) {
    const dialogSize = useBottomDialogSize();
    const provider = providers[providerCursor];
    const models = provider?.models ?? [];
    const manageable = provider ? !isBuiltinProvider(provider) : false;
    const compact = dialogSize.width < 66;
    const providerWidth = compact
        ? Math.max(dialogSize.width - 16, 16)
        : Math.max(Math.min(Math.floor(dialogSize.width * 0.34), 28), 18);
    const modelWidth = compact
        ? 0
        : Math.max(dialogSize.width - providerWidth - 22, 18);
    const maxVisible = Math.max(dialogSize.height - 8, 1);
    const maxVis = Math.min(maxVisible, models.length);
    const half = Math.floor(maxVis / 2);
    let start = modelCursor - half;
    if (start < 0) {
        start = 0;
    }
    if (start + maxVis > models.length) {
        start = Math.max(models.length - maxVis, 0);
    }
    const visible = models.slice(start, start + maxVis);

    useInput((input, key) => {
        if (key.escape) {
            onClose();
            return;
        }
        if (key.leftArrow || key.rightArrow) {
            const direction = key.leftArrow ? -1 : 1;
            const next = (providerCursor + direction + providers.length) % providers.length;
            onProviderCursor(next);
            return;
        }
        if (key.upArrow) {
            onModelCursor(Math.max(modelCursor - 1, 0));
            return;
        }
        if (key.downArrow) {
            onModelCursor(Math.min(modelCursor + 1, Math.max(models.length - 1, 0)));
            return;
        }

        const model = models[modelCursor];
        const lower = input.toLowerCase();
        if ((key.return || lower === "s") && model) {
            onSwitch(model);
            return;
        }
        if (!provider || !manageable) {
            return;
        }
        if (lower === "a") {
            onAdd(provider);
        } else if (lower === "e" && model) {
            onEdit(provider, model);
        } else if (lower === "d" && model) {
            onDelete(provider, model);
        }
    });

    return (
        <Box flexDirection="column" flexGrow={1}>
            <Box height={1} overflow="hidden">
                <Text dimColor>Provider: </Text>
                <Text color="cyan" bold onMouseClick={() => {
                    const next = (providerCursor - 1 + providers.length) % providers.length;
                    onProviderCursor(next);
                }}>
                    ‹
                </Text>
                <Text color="cyan" bold> {provider?.name ?? "—"} </Text>
                <Text color="cyan" bold onMouseClick={() => {
                    const next = (providerCursor + 1) % providers.length;
                    onProviderCursor(next);
                }}>
                    ›
                </Text>
                <Text dimColor>
                    {provider ? ` · ${manageable ? "custom" : "built-in"}` : ""}
                </Text>
            </Box>
            <Box marginBottom={1} height={1} overflow="hidden">
                <Text dimColor wrap="truncate">
                    {provider
                        ? `  ${compact ? "Model" : `${pad("Model", modelWidth)}  ${pad("Context", 10)}`}  ${models.length} model${models.length === 1 ? "" : "s"}`
                        : "No provider selected"}
                </Text>
            </Box>
            {models.length === 0 ? (
                <Box flexGrow={1}>
                    <Text dimColor>
                        {manageable
                            ? "No models. Press `a` to add one."
                            : "No models available for this built-in provider."}
                    </Text>
                </Box>
            ) : (
                <Box flexDirection="column" flexGrow={1} overflow="hidden">
                    {visible.map((model) => {
                        const index = models.indexOf(model);
                        const selected = index === modelCursor;
                        const current = currentModel?.providerCode === provider?.code &&
                            currentModel.modelCode === model.code;
                        const label = model.name && model.code
                            ? `${model.name} · ${model.code}`
                            : model.code;
                        return (
                            <Box
                                key={model.code}
                                height={1}
                                overflow="hidden"
                                onMouseOver={() => onModelCursor(index)}
                                onMouseClick={() => onSwitch(model)}
                            >
                                <Text color={selected ? "cyan" : "gray"}>
                                    {selected ? "❯" : " "}
                                </Text>
                                <Text> </Text>
                                <Box width={compact ? Math.max(dialogSize.width - 4, 12) : modelWidth}>
                                    <Text color={selected ? "cyan" : "white"} bold={selected} wrap="truncate">
                                        {label}
                                    </Text>
                                </Box>
                                {compact ? null : <Text>  </Text>}
                                {compact ? null : (
                                    <Text color={selected ? "cyan" : "gray"} dimColor={!selected}>
                                        {pad(model.contextWindow.toLocaleString(), 10)}
                                    </Text>
                                )}
                                <Text dimColor>
                                    {current ? " · current" : model.isDefault ? " · default" : ""}
                                </Text>
                            </Box>
                        );
                    })}
                </Box>
            )}
            <Box gap={3} height={1} marginTop={1} overflow="hidden">
                {manageable ? (
                    <>
                        <Text color="cyan" onMouseClick={() => provider && onAdd(provider)}>Add</Text>
                        <Text color="cyan" onMouseClick={() => {
                            const model = models[modelCursor];
                            if (provider && model) {
                                onEdit(provider, model);
                            }
                        }}>Edit</Text>
                        <Text color="cyan" onMouseClick={() => {
                            const model = models[modelCursor];
                            if (provider && model) {
                                onDelete(provider, model);
                            }
                        }}>Delete</Text>
                    </>
                ) : (
                    <Text dimColor>Built-in provider · switching only</Text>
                )}
            </Box>
            <Box height={1} overflow="hidden">
                <Text dimColor wrap="truncate">
                    {manageable
                        ? "←→ provider · ↑↓ model · Enter/s switch · a add · e edit · d delete · Esc back"
                        : "←→ provider · ↑↓ model · Enter/s switch · Esc back"}
                </Text>
            </Box>
        </Box>
    );
}

function DeleteConfirm({
    target,
    onConfirm,
    onCancel,
}: {
    target: CoreModelDto;
    onConfirm: () => void;
    onCancel: () => void;
}) {
    useInput((input, key) => {
        if (key.escape) {
            onCancel();
            return;
        }
        const lower = input.toLowerCase();
        if (lower === "y") {
            onConfirm();
        } else if (lower === "n") {
            onCancel();
        }
    });

    return (
        <Box flexDirection="column" flexGrow={1}>
            <Text color="red" bold>
                Delete model &quot;{target.name}&quot;?
            </Text>
            <Text dimColor>This cannot be undone.</Text>
            <Box marginTop={1} gap={2}>
                <Text color="red" bold onMouseClick={onConfirm}>[Delete]</Text>
                <Text color="cyan" onMouseClick={onCancel}>[Cancel]</Text>
            </Box>
        </Box>
    );
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
    return value.slice(0, width - 1) + "…";
}

function pad(value: string, width: number): string {
    const clipped = trunc(value, width);
    return clipped + " ".repeat(Math.max(width - clipped.length, 0));
}
