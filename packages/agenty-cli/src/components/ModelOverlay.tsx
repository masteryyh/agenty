import { useCallback, useEffect, useRef, useState } from "react";

import type { CoreModelDto, ModelProviderDto, ModelRef } from "../api/types";
import { useInput } from "../hooks/useInput";
import { useAppStore } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";
import { List, useListNavigation } from "./List";
import { Panel } from "./Panel";
import {
    createTableLayout,
    type TableColumn,
    TableHeader,
    TableRow,
} from "./Table";
import { Box, Pressable, Spinner, Text, TextInput } from "./ui";

export function isConfiguredProvider(provider: Pick<ModelProviderDto, "apiKey">): boolean {
    return provider.apiKey.trim() !== "";
}

export function configuredProviders(providers: ModelProviderDto[]): ModelProviderDto[] {
    return providers
        .filter(isConfiguredProvider)
        .map((provider) => ({
            ...provider,
            models: sortModelsByCode(provider.models),
        }));
}

export function sortModelsByCode(models: CoreModelDto[]): CoreModelDto[] {
    return [...models].sort((left, right) => {
        const insensitiveOrder = left.code.localeCompare(right.code, "en", {
            sensitivity: "base",
        });
        return insensitiveOrder !== 0
            ? insensitiveOrder
            : left.code.localeCompare(right.code, "en");
    });
}

export function filterModelsByCodeLike(
    models: CoreModelDto[],
    query: string,
): CoreModelDto[] {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    if (normalizedQuery === "") {
        return models;
    }
    return models.filter((model) => model.code.toLocaleLowerCase().includes(normalizedQuery));
}

export function normalizeModelSearchInput(input: string): string {
    return input.replace(/[^A-Za-z0-9_./\\:-]/g, "");
}

export function isModelSearchInput(input: string): boolean {
    return input !== "" && normalizeModelSearchInput(input) === input;
}

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

export function ModelOverlay() {
    const client = useAppStore((state) => state.client);
    const sessionModel = useAppStore((state) => state.session?.currentModel);
    const setToast = useAppStore((state) => state.setToast);
    const setOverlay = useAppStore((state) => state.setOverlay);
    const switchModel = useAppStore((state) => state.switchModel);
    const [providers, setProviders] = useState<ModelProviderDto[] | null>(null);
    const [providerCursor, setProviderCursor] = useState(0);
    const [modelCursor, setModelCursor] = useState(0);
    const [loadingProviderCode, setLoadingProviderCode] = useState<string | null>(null);
    const selectedProviderCodeRef = useRef(sessionModel?.providerCode);
    const selectedModelCodeRef = useRef(sessionModel?.modelCode);
    const requestIdRef = useRef(0);

    const reload = useCallback(async (providerCode?: string) => {
        if (!client) {
            return;
        }
        const requestId = ++requestIdRef.current;
        if (providerCode) {
            setLoadingProviderCode(providerCode);
        }
        try {
            const list = configuredProviders(await client.listProviders(providerCode));
            if (requestId !== requestIdRef.current) {
                return;
            }
            const nextProviderIndex = initialProviderIndex(
                list,
                selectedProviderCodeRef.current,
            );
            const nextProvider = list[nextProviderIndex];
            const nextModelIndex = initialModelIndex(
                nextProvider,
                selectedModelCodeRef.current,
            );
            selectedProviderCodeRef.current = nextProvider?.code;
            selectedModelCodeRef.current = nextProvider?.models[nextModelIndex]?.code;
            setProviders(list);
            setProviderCursor(nextProviderIndex);
            setModelCursor(nextModelIndex);
        } catch (error) {
            if (requestId !== requestIdRef.current) {
                return;
            }
            setToast(`failed to load models: ${(error as Error).message}`, true);
            setProviders([]);
        } finally {
            if (requestId === requestIdRef.current) {
                setLoadingProviderCode(null);
            }
        }
    }, [client, setToast]);

    useEffect(() => {
        void reload(selectedProviderCodeRef.current);
    }, [reload]);

    const close = () => setOverlay(null);
    const selectProvider = (index: number) => {
        const provider = providers?.[index];
        if (!provider) {
            return;
        }
        const nextModelIndex = initialModelIndex(provider);
        selectedProviderCodeRef.current = provider.code;
        selectedModelCodeRef.current = provider.models[nextModelIndex]?.code;
        setProviderCursor(index);
        setModelCursor(nextModelIndex);
        void reload(provider.code);
    };
    const selectModelCursor = (index: number) => {
        const model = providers?.[providerCursor]?.models[index];
        if (model) {
            selectedModelCodeRef.current = model.code;
        }
        setModelCursor(index);
    };
    const handleSwitch = async (model: CoreModelDto) => {
        const provider = providers?.[providerCursor];
        if (!provider) {
            return;
        }
        await switchModel({
            ...model,
            providerCode: provider.code,
            providerName: provider.name,
        });
    };

    useInput((_input, key) => {
        if (providers !== null && providers.length > 0) {
            return;
        }
        if (key.escape) {
            close();
        }
    });

    return (
        <Panel title="Models">
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
                    loadingProviderCode={loadingProviderCode}
                    onProviderCursor={selectProvider}
                    onModelCursor={selectModelCursor}
                    onSwitch={(model) => void handleSwitch(model)}
                    onClose={close}
                />
            )}
        </Panel>
    );
}

function modelLabel(model: CoreModelDto): string {
    return model.name && model.code
        ? `${model.name} · ${model.code}`
        : model.code;
}

function ModelList({
    providers,
    providerCursor,
    modelCursor,
    currentModel,
    loadingProviderCode,
    onProviderCursor,
    onModelCursor,
    onSwitch,
    onClose,
}: {
    providers: ModelProviderDto[];
    providerCursor: number;
    modelCursor: number;
    currentModel?: ModelRef;
    loadingProviderCode: string | null;
    onProviderCursor: (index: number) => void;
    onModelCursor: (index: number) => void;
    onSwitch: (model: CoreModelDto) => void;
    onClose: () => void;
}) {
    const dialogSize = useBottomDialogSize();
    const [searchQuery, setSearchQuery] = useState("");
    const [searchFocused, setSearchFocused] = useState(false);
    const provider = providers[providerCursor];
    const models = provider?.models ?? [];
    const filteredModels = filterModelsByCodeLike(models, searchQuery);
    const selectedModel = models[modelCursor];
    const filteredCursor = selectedModel
        ? Math.max(
            filteredModels.findIndex((model) => model.code === selectedModel.code),
            0,
        )
        : 0;
    const selectFilteredModel = () => {
        const model = filteredModels[filteredCursor];
        if (model) {
            onSwitch(model);
        }
    };
    const selectFilteredCursor = (index: number) => {
        const model = filteredModels[index];
        if (model) {
            onModelCursor(models.indexOf(model));
        }
    };
    const moveProvider = (direction: -1 | 1) => {
        onProviderCursor(
            (providerCursor + direction + providers.length) % providers.length,
        );
    };
    const maxVisible = Math.max(dialogSize.height - 8, 1);
    const modelState = (model: CoreModelDto): string => {
        const current = currentModel?.providerCode === provider?.code &&
            currentModel.modelCode === model.code;
        return current ? "current" : model.isDefault ? "default" : "";
    };
    const columns: Array<TableColumn<CoreModelDto>> = [
        {
            key: "model",
            header: "Model",
            value: modelLabel,
            render: (model, selected) => (
                <Text color={selected ? "cyan" : "white"} bold={selected} wrap="truncate">
                    {modelLabel(model)}
                </Text>
            ),
        },
        {
            key: "context",
            header: "Context",
            value: (model) => model.contextWindow.toLocaleString(),
            render: (model, selected) => (
                <Text color={selected ? "cyan" : "gray"} dimColor={!selected}>
                    {model.contextWindow.toLocaleString()}
                </Text>
            ),
        },
        {
            key: "state",
            header: "State",
            value: modelState,
            render: (model) => (
                <Text color={modelState(model) === "current" ? "green" : "gray"} dimColor>
                    {modelState(model)}
                </Text>
            ),
        },
    ];
    const tableLayout = createTableLayout(
        columns,
        models,
        Math.max(dialogSize.width - 2, 0),
    );

    useListNavigation({
        items: filteredModels,
        cursor: filteredCursor,
        onCursor: selectFilteredCursor,
        onActivate: onSwitch,
        onClose,
        onInput: (input, key, event) => {
            if (key.leftArrow || key.rightArrow) {
                event.preventDefault();
                moveProvider(key.leftArrow ? -1 : 1);
                return;
            }
            if (isModelSearchInput(input)) {
                event.preventDefault();
                event.stopPropagation();
                setSearchQuery((query) => query + input);
                setSearchFocused(true);
            }
        },
        active: !searchFocused,
    });

    const hint = "←→ provider · ↑↓ model · type code to search · Enter select · Esc back";

    return (
        <Panel hint={hint}>
            <Box width="100%" height={1} marginBottom={1} overflow="hidden">
                <Box width={12} height={1} justifyContent="flex-end" overflow="hidden">
                    <Text dimColor>Provider:</Text>
                </Box>
                <Text> </Text>
                <Pressable height={1} onPress={() => moveProvider(-1)}>
                    <Text color="cyan" bold> ‹ </Text>
                </Pressable>
                <Text color="cyan" bold> {provider?.name ?? "—"} </Text>
                <Pressable height={1} onPress={() => moveProvider(1)}>
                    <Text color="cyan" bold> › </Text>
                </Pressable>
            </Box>
            <Box width="100%" height={1} marginBottom={1} overflow="hidden">
                <Box width={12} height={1} justifyContent="flex-end" overflow="hidden">
                    <Text dimColor>Search:</Text>
                </Box>
                <Text> </Text>
                <Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
                    <TextInput
                        value={searchQuery}
                        onChange={(next) => setSearchQuery(normalizeModelSearchInput(next))}
                        onSubmit={selectFilteredModel}
                        placeholder="filter by model code"
                        focus={searchFocused}
                        onMouseDown={() => setSearchFocused(true)}
                        onKeyDown={(event) => {
                            if (event.name === "escape") {
                                event.preventDefault();
                                event.stopPropagation();
                                onClose();
                            } else if (event.name === "up" || event.name === "down") {
                                event.preventDefault();
                                event.stopPropagation();
                                const direction = event.name === "up" ? -1 : 1;
                                const nextIndex = Math.min(
                                    Math.max(filteredCursor + direction, 0),
                                    Math.max(filteredModels.length - 1, 0),
                                );
                                selectFilteredCursor(nextIndex);
                            } else if (event.name === "left" || event.name === "right") {
                                event.preventDefault();
                                event.stopPropagation();
                                moveProvider(event.name === "left" ? -1 : 1);
                            }
                        }}
                    />
                </Box>
            </Box>
            <Box width="100%" height={1} overflow="hidden">
                <Box width={2} height={1}><Text> </Text></Box>
                <TableHeader columns={tableLayout} />
            </Box>
            <List
                items={filteredModels}
                cursor={filteredCursor}
                visibleCount={maxVisible}
                getKey={(model) => model.code}
                onCursor={selectFilteredCursor}
                onActivate={onSwitch}
                emptyHint={loadingProviderCode === provider?.code
                    ? <Spinner label="Loading models..." />
                    : searchQuery.trim() === ""
                        ? <Text dimColor>No models available for this provider.</Text>
                        : <Text dimColor>No model code contains “{searchQuery}”.</Text>}
                renderItem={(model, { selected }) => {
                    return (
                        <TableRow
                            columns={tableLayout}
                            row={model}
                            selected={selected}
                        />
                    );
                }}
            />
        </Panel>
    );
}
