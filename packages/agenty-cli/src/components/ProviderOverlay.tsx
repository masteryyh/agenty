import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type {
    APIType,
    CoreModelDto,
    CreateModelDto,
    ModelProviderDto,
    UpdateModelProviderDto,
} from "../api/types";
import { providerDefaultBaseURLs, providerTypes } from "../consts/providerTypes";
import { useInput } from "../hooks/useInput";
import { useAppStore } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";
import { ConfirmDialog } from "./ConfirmDialog";
import type { FormField } from "./FormPanel";
import { FormPanel } from "./FormPanel";
import { List, useListNavigation } from "./List";
import { Panel } from "./Panel";
import {
    buildProviderRows,
    defaultExpandedProviderCodes,
    type ProviderListRow,
    type ProviderRowEnterAction,
    providerRowEnterAction,
    providerRowLabel,
    providerRowState,
    shouldExpandProvider,
} from "./providerRows";
import {
    createTableLayout,
    type TableColumn,
    TableHeader,
    TableRow,
} from "./Table";
import { Box, Spinner, Text } from "./ui";

const PROVIDER_TYPE_OPTIONS = providerTypes.map((type) => ({ label: type, value: type }));

function buildCreateProviderFields(formType: string): FormField[] {
    const baseUrl = providerDefaultBaseURLs[formType] ?? "";
    return [
        { key: "code", label: "Provider Code", kind: "text", value: "", placeholder: "my-provider" },
        { key: "name", label: "Name", kind: "text", value: "", placeholder: "my-provider" },
        { key: "type", label: "Type", kind: "select", value: formType, options: PROVIDER_TYPE_OPTIONS },
        { key: "baseUrl", label: "Base URL", kind: "text", value: baseUrl },
        {
            key: "freeFormTool",
            label: "Free-form apply_patch",
            kind: "boolean",
            value: "false",
            visible: formType === "openai",
        },
        { key: "apiKey", label: "API Key", kind: "text", value: "", placeholder: "sk-...", secret: true },
    ];
}

type ProviderFormMode = "edit" | "configure";

export function buildProviderFields(
    target: ModelProviderDto,
    mode: ProviderFormMode,
): FormField[] {
    const configuringBuiltin = mode === "configure";
    return [
        {
            key: "code",
            label: "Provider Code",
            kind: "text",
            value: target.code,
            readOnly: true,
            focusable: !configuringBuiltin,
        },
        {
            key: "name",
            label: "Name",
            kind: "text",
            value: target.name,
            readOnly: configuringBuiltin,
            focusable: !configuringBuiltin,
        },
        {
            key: "type",
            label: "Type",
            kind: "select",
            value: target.type,
            options: PROVIDER_TYPE_OPTIONS,
            readOnly: configuringBuiltin,
            focusable: !configuringBuiltin,
        },
        {
            key: "baseUrl",
            label: "Base URL",
            kind: "text",
            value: target.baseUrl,
            readOnly: configuringBuiltin,
            focusable: !configuringBuiltin,
        },
        {
            key: "apiKey",
            label: "API Key",
            kind: "text",
            value: "",
            placeholder: "leave blank to keep",
            secret: true,
        },
        {
            key: "freeFormTool",
            label: "Free-form apply_patch",
            kind: "boolean",
            value: String(target.freeFormTool === true),
            readOnly: configuringBuiltin || target.type !== "openai",
            focusable: !configuringBuiltin && target.type === "openai",
            visible: target.type === "openai",
        },
    ];
}

export function buildBuiltinProviderUpdate(
    values: Record<string, string>,
): UpdateModelProviderDto | null {
    const apiKey = values.apiKey?.trim() ?? "";
    return apiKey ? { apiKey } : null;
}

function buildCreateModelFields(): FormField[] {
    return [
        { key: "code", label: "Model Code", kind: "text", value: "", placeholder: "model-code or org/model-code" },
        { key: "name", label: "Model name", kind: "text", value: "", placeholder: "Model name" },
        { key: "contextWindow", label: "Context window", kind: "text", value: "128000", placeholder: "128000" },
        { key: "maxOutputTokens", label: "Max output tokens", kind: "text", value: "8192", placeholder: "8192" },
        { key: "multiModal", label: "Multimodal", kind: "boolean", value: "false" },
        { key: "light", label: "Light", kind: "boolean", value: "false" },
        { key: "reasoning", label: "Reasoning", kind: "boolean", value: "true" },
    ];
}

function buildModelFields(
    model: CoreModelDto,
    readOnly: boolean,
): FormField[] {
    return [
        { key: "code", label: "Model Code", kind: "text", value: model.code, readOnly: true },
        { key: "name", label: "Model name", kind: "text", value: model.name, readOnly },
        {
            key: "contextWindow",
            label: "Context window",
            kind: "text",
            value: String(model.contextWindow),
            readOnly,
        },
        {
            key: "maxOutputTokens",
            label: "Max output tokens",
            kind: "text",
            value: String(model.maxOutputTokens),
            readOnly,
        },
        { key: "multiModal", label: "Multimodal", kind: "boolean", value: String(model.multiModal), readOnly },
        { key: "light", label: "Light", kind: "boolean", value: String(model.light), readOnly },
        {
            key: "reasoning",
            label: "Reasoning",
            kind: "boolean",
            value: String((model.reasoningEfforts ?? []).length > 0),
            readOnly,
        },
    ];
}

function parseModelValues(values: Record<string, string>): CreateModelDto | string {
    const modelCode = values.code?.trim() ?? "";
    const name = values.name?.trim() ?? "";
    const contextWindow = Number(values.contextWindow);
    const maxOutputTokens = Number(values.maxOutputTokens);
    if (!modelCode) {
        return "Model Code is required.";
    }
    if (!name) {
        return "Model name is required.";
    }
    if (!Number.isSafeInteger(contextWindow) || contextWindow <= 0) {
        return "Context window must be a positive integer.";
    }
    if (!Number.isSafeInteger(maxOutputTokens) || maxOutputTokens <= 0) {
        return "Max output tokens must be a positive integer.";
    }
    return {
        providerCode: "",
        modelCode,
        name,
        contextWindow,
        maxOutputTokens,
        multiModal: values.multiModal === "true",
        light: values.light === "true",
        reasoning: values.reasoning === "true",
    };
}

type Mode =
    | { kind: "list" }
    | { kind: "create-provider" }
    | { kind: "edit-provider"; target: ModelProviderDto }
    | { kind: "configure-provider"; target: ModelProviderDto }
    | { kind: "confirm-delete-provider"; target: ModelProviderDto }
    | { kind: "create-model"; provider: ModelProviderDto }
    | { kind: "edit-model"; provider: ModelProviderDto; target: CoreModelDto }
    | { kind: "view-model"; provider: ModelProviderDto; target: CoreModelDto }
    | { kind: "confirm-delete-model"; provider: ModelProviderDto; target: CoreModelDto };

function errorMessage(cause: unknown): string {
    return cause instanceof Error ? cause.message : String(cause);
}

export function ProviderOverlay() {
    const client = useAppStore((state) => state.client);
    const setToast = useAppStore((state) => state.setToast);
    const setOverlay = useAppStore((state) => state.setOverlay);
    const [providers, setProviders] = useState<ModelProviderDto[] | null>(null);
    const [mode, setMode] = useState<Mode>({ kind: "list" });
    const [selectedKey, setSelectedKey] = useState<string | null>(null);
    const [expandedProviderCodes, setExpandedProviderCodes] = useState<Set<string>>(new Set());
    const [formType, setFormType] = useState<string>(providerTypes[0]);
    const modeRef = useRef(mode);
    const expansionInitializedRef = useRef(false);
    const previousProviderCodesRef = useRef<Set<string>>(new Set());
    modeRef.current = mode;

    const reload = useCallback(async () => {
        if (!client) {
            return;
        }
        try {
            const list = await client.listProviders();
            setProviders(list);
            const providerCodes = new Set(list.map((provider) => provider.code));
            setExpandedProviderCodes((current) => {
                if (!expansionInitializedRef.current) {
                    expansionInitializedRef.current = true;
                    previousProviderCodesRef.current = providerCodes;
                    return defaultExpandedProviderCodes(list);
                }
                const next = new Set(
                    list
                        .filter((provider) => current.has(provider.code))
                        .map((provider) => provider.code),
                );
                for (const provider of list) {
                    if (
                        !previousProviderCodesRef.current.has(provider.code) &&
                        shouldExpandProvider(provider)
                    ) {
                        next.add(provider.code);
                    }
                }
                previousProviderCodesRef.current = providerCodes;
                return next;
            });
        } catch (cause: unknown) {
            setToast(`failed to load providers: ${errorMessage(cause)}`, true);
            setProviders([]);
        }
    }, [client, setToast]);

    useEffect(() => {
        void reload();
    }, [reload]);

    useInput((_input, key) => {
        if (modeRef.current.kind === "list" && providers === null && key.escape) {
            setOverlay(null);
        }
    });

    const close = () => setOverlay(null);
    const returnToList = () => setMode({ kind: "list" });

    const openAction = (action: ProviderRowEnterAction) => {
        switch (action.kind) {
            case "configure-provider":
                setMode({ kind: "configure-provider", target: action.provider });
                return;
            case "edit-provider":
                setMode({ kind: "edit-provider", target: action.provider });
                return;
            case "view-model":
                setMode({ kind: "view-model", provider: action.provider, target: action.model });
                return;
            case "edit-model":
                setMode({ kind: "edit-model", provider: action.provider, target: action.model });
                return;
            case "add-provider":
                setFormType(providerTypes[0]);
                setMode({ kind: "create-provider" });
                return;
            case "add-model":
                setMode({ kind: "create-model", provider: action.provider });
                return;
            case "none":
                return;
        }
    };

    const toggleProvider = (providerCode: string, expanded: boolean) => {
        setExpandedProviderCodes((current) => {
            const next = new Set(current);
            if (expanded) {
                next.add(providerCode);
            } else {
                next.delete(providerCode);
            }
            return next;
        });
    };

    const handleCreateProvider = async (values: Record<string, string>) => {
        if (!client) {
            return;
        }
        try {
            const type = values.type as APIType;
            await client.createProvider({
                code: values.code.trim(),
                name: values.name.trim(),
                type,
                baseUrl: values.baseUrl.trim(),
                apiKey: values.apiKey.trim(),
                freeFormTool: type === "openai" && values.freeFormTool === "true",
            });
            setToast(`Provider created: ${values.name.trim()}`);
            await reload();
            returnToList();
        } catch (cause: unknown) {
            setToast(`create provider failed: ${errorMessage(cause)}`, true);
        }
    };

    const handleEditProvider = async (target: ModelProviderDto, values: Record<string, string>) => {
        if (!client || target.builtin === true) {
            returnToList();
            return;
        }
        try {
            const type = values.type as APIType;
            const update: UpdateModelProviderDto = {
                name: values.name.trim(),
                type,
                baseUrl: values.baseUrl.trim(),
                freeFormTool: type === "openai" && values.freeFormTool === "true",
            };
            if (values.apiKey.trim()) {
                update.apiKey = values.apiKey.trim();
            }
            await client.updateProvider(target.code, update);
            setToast(`Provider updated: ${values.name.trim()}`);
            await reload();
            returnToList();
        } catch (cause: unknown) {
            setToast(`update provider failed: ${errorMessage(cause)}`, true);
        }
    };

    const handleConfigureProvider = async (
        target: ModelProviderDto,
        values: Record<string, string>,
    ) => {
        if (!client || target.builtin !== true) {
            returnToList();
            return;
        }
        const update = buildBuiltinProviderUpdate(values);
        if (!update) {
            returnToList();
            return;
        }
        try {
            await client.updateProvider(target.code, update);
            setToast(`Provider API key updated: ${target.name}`);
            await reload();
            returnToList();
        } catch (cause: unknown) {
            setToast(`update provider API key failed: ${errorMessage(cause)}`, true);
        }
    };

    const handleDeleteProvider = async (target: ModelProviderDto) => {
        if (!client || target.builtin === true) {
            returnToList();
            return;
        }
        try {
            await client.deleteProvider(target.code);
            setToast(`Provider deleted: ${target.name}`);
            await reload();
            returnToList();
        } catch (cause: unknown) {
            setToast(`delete provider failed: ${errorMessage(cause)}`, true);
        }
    };

    const handleCreateModel = async (provider: ModelProviderDto, values: Record<string, string>) => {
        if (!client || provider.builtin === true) {
            returnToList();
            return;
        }
        const parsed = parseModelValues(values);
        if (typeof parsed === "string") {
            setToast(parsed, true);
            return;
        }
        try {
            await client.createModel({ ...parsed, providerCode: provider.code });
            setToast(`Model created: ${parsed.name}`);
            await reload();
            returnToList();
        } catch (cause: unknown) {
            setToast(`create model failed: ${errorMessage(cause)}`, true);
        }
    };

    const handleEditModel = async (
        provider: ModelProviderDto,
        target: CoreModelDto,
        values: Record<string, string>,
    ) => {
        if (!client || provider.builtin === true) {
            returnToList();
            return;
        }
        const parsed = parseModelValues(values);
        if (typeof parsed === "string") {
            setToast(parsed, true);
            return;
        }
        try {
            await client.updateModel(provider.code, target.code, {
                name: parsed.name,
                contextWindow: parsed.contextWindow,
                maxOutputTokens: parsed.maxOutputTokens,
                multiModal: parsed.multiModal,
                light: parsed.light,
                reasoning: parsed.reasoning,
            });
            setToast(`Model updated: ${parsed.name}`);
            await reload();
            returnToList();
        } catch (cause: unknown) {
            setToast(`update model failed: ${errorMessage(cause)}`, true);
        }
    };

    const handleDeleteModel = async (provider: ModelProviderDto, target: CoreModelDto) => {
        if (!client || provider.builtin === true) {
            returnToList();
            return;
        }
        try {
            await client.deleteModel(provider.code, target.code);
            setToast(`Model deleted: ${target.name || target.code}`);
            await reload();
            returnToList();
        } catch (cause: unknown) {
            setToast(`delete model failed: ${errorMessage(cause)}`, true);
        }
    };

    if (mode.kind === "create-provider") {
        return (
            <FormPanel
                title="Add Provider"
                fields={buildCreateProviderFields(formType)}
                onChange={(key, values) => {
                    if (key === "type") {
                        setFormType(values.type);
                    }
                }}
                onAction={(action, values) => {
                    if (action === "save") {
                        void handleCreateProvider(values);
                    } else {
                        returnToList();
                    }
                }}
                onClose={returnToList}
            />
        );
    }

    if (mode.kind === "edit-provider" || mode.kind === "configure-provider") {
        const configuringBuiltin = mode.kind === "configure-provider";
        const target = mode.target;
        return (
            <FormPanel
                title={`${configuringBuiltin ? "Configure" : "Edit"}: ${target.name}`}
                fields={buildProviderFields(target, configuringBuiltin ? "configure" : "edit")}
                hint={configuringBuiltin ? "↑↓ navigate · type to edit · Esc back" : undefined}
                shortcutHint={!configuringBuiltin ? "d delete" : undefined}
                onShortcut={(input) => {
                    if (!configuringBuiltin && input.toLowerCase() === "d") {
                        setMode({ kind: "confirm-delete-provider", target });
                        return true;
                    }
                    return false;
                }}
                onAction={(action, values) => {
                    if (action !== "save") {
                        returnToList();
                    } else if (configuringBuiltin) {
                        void handleConfigureProvider(target, values);
                    } else {
                        void handleEditProvider(target, values);
                    }
                }}
                onClose={returnToList}
            />
        );
    }

    if (mode.kind === "create-model") {
        const provider = mode.provider;
        return (
            <FormPanel
                title={`Add model to ${provider.name}`}
                fields={buildCreateModelFields()}
                onAction={(action, values) => {
                    if (action === "save") {
                        void handleCreateModel(provider, values);
                    } else {
                        returnToList();
                    }
                }}
                onClose={returnToList}
            />
        );
    }

    if (mode.kind === "edit-model" || mode.kind === "view-model") {
        const readOnly = mode.kind === "view-model";
        const { provider, target } = mode;
        return (
            <FormPanel
                title={`${readOnly ? "View" : "Edit"} model: ${target.name || target.code}`}
                fields={buildModelFields(target, readOnly)}
                actions={readOnly ? [{ key: "back", label: "Back" }] : undefined}
                hint={readOnly ? "↑↓ view · Esc back" : undefined}
                shortcutHint={!readOnly ? "d delete" : undefined}
                onShortcut={(input) => {
                    if (!readOnly && input.toLowerCase() === "d") {
                        setMode({ kind: "confirm-delete-model", provider, target });
                        return true;
                    }
                    return false;
                }}
                onAction={(action, values) => {
                    if (readOnly || action !== "save") {
                        returnToList();
                    } else {
                        void handleEditModel(provider, target, values);
                    }
                }}
                onClose={returnToList}
            />
        );
    }

    if (mode.kind === "confirm-delete-provider") {
        return (
            <ConfirmDialog
                title={`Delete provider "${mode.target.name}"?`}
                message="This also deletes all its models."
                onConfirm={() => void handleDeleteProvider(mode.target)}
                onCancel={returnToList}
            />
        );
    }

    if (mode.kind === "confirm-delete-model") {
        return (
            <ConfirmDialog
                title={`Delete model "${mode.target.name || mode.target.code}"?`}
                message={`This removes it from ${mode.provider.name}.`}
                onConfirm={() => void handleDeleteModel(mode.provider, mode.target)}
                onCancel={returnToList}
            />
        );
    }

    return (
        <Panel title="Providers" hint="↑↓ navigate · ←→ expand/collapse · Enter open · Esc back">
            {providers === null ? (
                <Spinner label="Loading..." />
            ) : (
                <ProviderList
                    providers={providers}
                    expandedProviderCodes={expandedProviderCodes}
                    selectedKey={selectedKey}
                    onSelectedKey={setSelectedKey}
                    onToggleProvider={toggleProvider}
                    onActivate={openAction}
                    onClose={close}
                />
            )}
        </Panel>
    );
}

function ProviderList({
    providers,
    expandedProviderCodes,
    selectedKey,
    onSelectedKey,
    onToggleProvider,
    onActivate,
    onClose,
}: {
    providers: ModelProviderDto[];
    expandedProviderCodes: ReadonlySet<string>;
    selectedKey: string | null;
    onSelectedKey: (key: string | null) => void;
    onToggleProvider: (code: string, expanded: boolean) => void;
    onActivate: (action: ProviderRowEnterAction) => void;
    onClose: () => void;
}) {
    const dialogSize = useBottomDialogSize();
    const rows = useMemo(
        () => buildProviderRows(providers, expandedProviderCodes),
        [providers, expandedProviderCodes],
    );
    const cursor = Math.max(rows.findIndex((row) => row.key === selectedKey), 0);
    const columns: Array<TableColumn<ProviderListRow>> = [
        {
            key: "resource",
            header: "Provider / model",
            value: providerRowLabel,
            render: (row, selected) => (
                <Text
                    color={selected ? "cyan" : row.kind === "provider" ? "white" : "gray"}
                    bold={selected || row.kind === "provider"}
                    wrap="truncate"
                >
                    {providerRowLabel(row)}
                </Text>
            ),
        },
        {
            key: "state",
            header: "State",
            value: providerRowState,
            render: (row) => (
                <Text
                    color={providerRowState(row) === "configured" ? "green" : "gray"}
                    dimColor={row.kind === "add-provider" || row.kind === "add-model"}
                >
                    {providerRowState(row)}
                </Text>
            ),
        },
    ];
    const tableLayout = createTableLayout(
        columns,
        rows,
        Math.max(dialogSize.width - 2, 0),
    );

    useListNavigation({
        items: rows,
        cursor,
        onCursor: (index) => onSelectedKey(rows[index]?.key ?? null),
        onActivate: (row) => onActivate(providerRowEnterAction(row)),
        onClose,
        onInput: (_input, key, _event, row) => {
            if (row?.kind !== "provider") {
                return;
            }
            if (key.leftArrow) {
                onToggleProvider(row.provider.code, false);
            } else if (key.rightArrow) {
                onToggleProvider(row.provider.code, true);
            }
        },
    });

    return (
        <Box flexDirection="column" flexGrow={1} width="100%">
            <Box width="100%" height={1} marginBottom={1} overflow="hidden">
                <Box width={2} height={1}><Text> </Text></Box>
                <TableHeader columns={tableLayout} />
            </Box>
            <List
                items={rows}
                cursor={cursor}
                visibleCount={Math.max(dialogSize.height - 6, 1)}
                getKey={(row) => row.key}
                onCursor={(index) => onSelectedKey(rows[index]?.key ?? null)}
                onActivate={(row) => onActivate(providerRowEnterAction(row))}
                renderItem={(row, { selected }) => (
                    <TableRow
                        columns={tableLayout}
                        row={row}
                        selected={selected}
                    />
                )}
            />
        </Box>
    );
}
