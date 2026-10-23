import { useCallback, useEffect, useRef, useState } from "react";

import type {
    HubModel,
    ModelDownload,
    ModelHubSource,
    ModelStorage
} from "../api/types";
import { theme } from "../consts/theme";
import { useInput } from "../hooks/useInput";
import { useAppStore } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";
import { type FormField, FormPanel } from "./FormPanel";
import { List, useListNavigation } from "./List";
import { ModelDownloadDialog } from "./ModelDownloadDialog";
import {
    formatModelBytes,
    modelDownloadProgress,
    modelFileSizeLabel,
    modelParameterLabel
} from "./modelDownloadFormat";
import { Panel } from "./Panel";
import { Box, Pressable, Spinner, Text, TextInput } from "./ui";
export { formatModelBytes, modelDownloadProgress, modelFileSizeLabel, modelParameterLabel } from "./modelDownloadFormat";

function ModelRow({ name, parameters, sizeBytes, selected }: Pick<HubModel, "name" | "parameters" | "sizeBytes"> & { selected: boolean }) {
    const color = selected ? theme.selection : theme.textMuted;
    return <Box flexGrow={1} flexBasis={0}>
        <Box flexGrow={1} flexBasis={0} overflow="hidden"><Text wrap="truncate" color={selected ? theme.selection : theme.text} bold={selected}>{name || "UNKNOWN"}</Text></Box>
        <Box width={9} justifyContent="flex-end"><Text color={color}>{modelParameterLabel({ parameters })}</Text></Box>
        <Box width={11} justifyContent="flex-end"><Text color={color}>{modelFileSizeLabel(sizeBytes)}</Text></Box>
    </Box>;
}

function StorageSummary({ storage }: { storage: ModelStorage | null }) {
    const size = useBottomDialogSize();
    if (!storage) {
        return <Text dimColor>Checking model storage…</Text>;
    }
    return <Box flexDirection="column" width="100%">
        <Text dimColor wrap="truncate">Save to: {storage.directory}</Text>
        {size.width < 64 ? <Text dimColor wrap="truncate">{formatModelBytes(storage.availableBytes)} available · {storage.totalBytes > 0 ? Math.round(storage.usedBytes / storage.totalBytes * 100) : 0}% used</Text>
            : <Text dimColor wrap="truncate">Volume {storage.volume} · {formatModelBytes(storage.usedBytes)} / {formatModelBytes(storage.totalBytes)} used · {formatModelBytes(storage.availableBytes)} available</Text>}
    </Box>;
}

export function ModelDownloadPanel({ onClose, wizard = false }: { onClose: () => void; wizard?: boolean }) {
    const client = useAppStore((state) => state.client);
    const downloads = useAppStore((state) => state.modelDownloads);
    const size = useBottomDialogSize();
    const compact = size.height < 18;
    const [source, setSource] = useState<ModelHubSource>("huggingface");
    const [query, setQuery] = useState("");
    const [models, setModels] = useState<HubModel[]>([]);
    const [next, setNext] = useState<string>();
    const [cursor, setCursor] = useState(0);
    const [searchFocused, setSearchFocused] = useState(true);
    const [loading, setLoading] = useState(false);
    const [busy, setBusy] = useState(false);
    const [storage, setStorage] = useState<ModelStorage | null>(null);
    const [selection, setSelection] = useState<{ model: HubModel; source: ModelHubSource; variant?: string } | null>(null);
    const [progressOpen, setProgressOpen] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const sequence = useRef(0);
    const pageRequest = useRef<number | null>(null);
    const operationRunning = useRef(false);
    const mounted = useRef(true);

    useEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
            sequence.current++;
        };
    }, []);
    const refreshStorage = useCallback(async () => {
        if (!client) {
            return;
        }
        try {
            const value = await client.modelStorage();
            if (mounted.current) {
                setStorage(value);
            }
        } catch (cause: unknown) {
            if (mounted.current) {
                setError(`Cannot inspect model partition: ${cause instanceof Error ? cause.message : String(cause)}`);
            }
        }
    }, [client]);
    useEffect(() => {
        void refreshStorage();
        const timer = setInterval(() => void refreshStorage(), 3000);
        return () => clearInterval(timer);
    }, [refreshStorage]);
    useEffect(() => {
        if (!client) {
            return;
        }
        void client.modelDownloads().then((items) => {
            if (!mounted.current || useAppStore.getState().client !== client) {
                return;
            }
            useAppStore.setState((state) => ({ modelDownloads: [...items.filter((item) => !state.modelDownloads.some((current) => current.id === item.id)), ...state.modelDownloads] }));
        }).catch((cause: unknown) => {
            if (mounted.current) {
                setError(cause instanceof Error ? cause.message : String(cause));
            }
        });
    }, [client]);

    const search = useCallback(async (after = "") => {
        if (!client || (after && pageRequest.current !== null)) {
            return;
        }
        const request = ++sequence.current;
        pageRequest.current = after ? request : null;
        setLoading(true);
        setError(null);
        try {
            const result = await client.searchHubModels(source, query.trim(), after);
            if (!mounted.current || request !== sequence.current) {
                return;
            }
            setModels((current) => after ? [...current, ...result.models.filter((item) => !current.some((old) => old.id === item.id))] : result.models);
            setNext(result.next === after ? undefined : result.next);
            if (!after) {
                setCursor(0);
            }
        } catch (cause: unknown) {
            if (mounted.current && request === sequence.current) {
                setError(cause instanceof Error ? cause.message : String(cause));
            }
        } finally {
            if (pageRequest.current === request) {
                pageRequest.current = null;
            }
            if (mounted.current && request === sequence.current) {
                setLoading(false);
            }
        }
    }, [client, source, query]);
    useEffect(() => {
        sequence.current++;
        pageRequest.current = null;
        setModels([]);
        setNext(undefined);
        const timer = setTimeout(() => void search(), 300);
        return () => {
            clearTimeout(timer);
            sequence.current++;
        };
    }, [search]);

    const run = async (operation: () => Promise<void>) => {
        if (operationRunning.current) {
            return;
        }
        operationRunning.current = true;
        setBusy(true);
        setError(null);
        try {
            await operation();
        } catch (cause: unknown) {
            if (mounted.current) {
                setError(cause instanceof Error ? cause.message : String(cause));
            }
        } finally {
            operationRunning.current = false;
            if (mounted.current) {
                setBusy(false);
            }
        }
    };
    const openModel = (model: HubModel, retry?: ModelDownload) => {
        setSelection({ model, source: retry?.source ?? source, variant: retry?.variant });
        setProgressOpen(false);
    };
    const backToModels = () => {
        setSelection(null);
        setProgressOpen(false);
        setSearchFocused(false);
        void search();
    };
    const selectModelCursor = (index: number) => {
        setCursor(index);
        setSearchFocused(false);
        if (models.length > 0 && index === models.length - 1 && next) {
            void search(next);
        }
    };

    useListNavigation({ items: models, cursor, onCursor: selectModelCursor, onActivate: (model) => openModel(model), onClose,
        active: !selection && !progressOpen && !searchFocused && !busy,
        onInput: (input, key, event) => {
            if (key.tab || key.leftArrow || key.rightArrow) {
                event.preventDefault();
                setSource((value) => value === "huggingface" ? "modelscope" : "huggingface");
            } else if (input && !key.ctrl && !key.meta) {
                setQuery((value) => value + input);
                setSearchFocused(true);
            }
        } });
    useInput((_input, key, event) => {
        if (!selection && !progressOpen && key.ctrl) {
            if (event.name === "p") {
                setProgressOpen(true);
            }
        }
        if (key.escape && !selection && !progressOpen && (searchFocused || busy)) {
            onClose();
        }
    });

    if (progressOpen) {
        const fields: FormField[] = downloads.flatMap((item) => [
            { key: `${item.id}:title`, kind: "text", label: item.repository, value: `${item.status} · ${item.variant}`, readOnly: true, focusable: false },
            { key: `${item.id}:total`, kind: "text", label: "Total", value: `${modelDownloadProgress(item.downloadedBytes, item.totalBytes)} · ${formatModelBytes(item.downloadedBytes)} / ${formatModelBytes(item.totalBytes)}`, readOnly: true, focusable: false },
            ...item.files.map((file): FormField => ({ key: `${item.id}:${file.path}`, label: file.path, kind: "text", value: `${modelDownloadProgress(file.downloadedBytes, file.totalBytes, 8)} · ${file.status}`, readOnly: true, focusable: false })),
            ...(item.error ? [{ key: `${item.id}:error`, kind: "text" as const, label: "Notice", value: item.error, readOnly: true, focusable: false }] : []),
            ...(isModelDownloading(item) ? [{ key: `cancel:${item.id}`, kind: "action" as const, label: "Cancel download", value: "" }]
                : item.status === "ready" ? wizard ? [] : [{ key: `enable:${item.id}`, kind: "action" as const, label: "Enable local inference", value: "" }]
                    : [{ key: `retry:${item.id}`, kind: "action" as const, label: "Retry / add model", value: "" }]),
        ]);
        return <FormPanel title="Model downloads" fields={fields} error={error} active={!busy}
            afterFields={<StorageSummary storage={storage} />} actions={[]} hint="↑/↓ select · Enter act · Esc back"
            onClose={backToModels} onAction={(key) => {
                if (key.startsWith("cancel:")) {
                    void run(async () => {
                        await client?.cancelModelDownload(key.slice(7));
                    });
                } else if (key.startsWith("enable:")) {
                    void run(async () => {
                        await client?.configureLocalInference({ enabled: true });
                        if (mounted.current) {
                            onClose();
                        }
                    });
                } else if (key.startsWith("retry:")) {
                    const item = downloads.find((download) => download.id === key.slice(6));
                    if (item) {
                        setSource(item.source);
                        openModel({ id: item.repository, name: item.repository, architecture: "Unknown", downloads: 0 }, item);
                    }
                }
            }} />;
    }
    return <Box flexDirection="column" flexGrow={1}>
        <Panel title={compact ? undefined : "Download local models"} error={error}
            hint={compact ? undefined : "Tab source · ↓ list · Enter download · Ctrl+P progress · Esc back"}
            footer={<Box width="100%" height={loading || busy ? 2 : 1} flexDirection="column" flexShrink={0}>
                {loading || busy ? <Spinner label={busy ? "Working…" : "Loading models…"} /> : null}
            </Box>}>
            <Box height={1} gap={1} marginBottom={compact ? 0 : 1}>
                {(["huggingface", "modelscope"] as const).map((item, index) => (
                    <Box key={item} flexDirection="row" height={1}>
                        {index > 0 ? <Text color={theme.border}> · </Text> : null}
                        <Pressable disabled={!!selection || busy} onPress={() => item === source ? void search() : setSource(item)}>
                            <Text bold={source === item} color={source === item ? theme.accent : theme.textFaint}>
                                {source === item ? `[${item === "huggingface" ? "Hugging Face" : "ModelScope"}]` : ` ${item === "huggingface" ? "Hugging Face" : "ModelScope"} `}
                            </Text>
                        </Pressable>
                    </Box>
                ))}
            </Box>
            <StorageSummary storage={storage} />
            <Box height={1} width="100%" marginTop={compact ? 0 : 1} marginBottom={compact ? 0 : 1}>
                <Text>Search: </Text>
                <Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
                    <TextInput value={query} onChange={setQuery} placeholder="Model name or owner/repository" focus={searchFocused && !busy && !selection}
                        onMouseDown={() => setSearchFocused(true)} onSubmit={() => {
                            if (!selection && models[cursor]) {
                                openModel(models[cursor]);
                            }
                        }} onKeyDown={(event) => {
                            if (event.name === "down") {
                                event.preventDefault();
                                selectModelCursor(cursor);
                            } else if (event.name === "tab") {
                                event.preventDefault();
                                setSource((value) => value === "huggingface" ? "modelscope" : "huggingface");
                            }
                        }} />
                </Box>
            </Box>
            {!compact ? <Box width="100%" height={1} paddingLeft={2}><Box flexGrow={1}><Text dimColor>Model</Text></Box><Box width={9} justifyContent="flex-end"><Text dimColor>Params</Text></Box><Box width={11} justifyContent="flex-end"><Text dimColor>Total size</Text></Box></Box> : null}
            <List items={models} cursor={cursor} onCursor={selection ? () => {} : selectModelCursor} onActivate={busy || selection ? undefined : (model) => openModel(model)}
                active={!searchFocused && !busy && !selection} visibleCount={Math.max(size.height - (compact ? 5 : 12) - (loading || busy ? 1 : 0), 1)} getKey={(model) => model.id}
                emptyHint={<Text dimColor>{loading ? "" : "No models found. Change the source or search query."}</Text>}
                renderItem={(model, { selected }) => <ModelRow name={model.name || model.id} parameters={model.parameters} sizeBytes={model.sizeBytes} selected={selected} />} />
        </Panel>
        {selection ? <ModelDownloadDialog key={`${selection.source}:${selection.model.id}`} model={selection.model} source={selection.source}
            initialVariant={selection.variant} onClose={() => setSelection(null)} onDownload={(item) => {
                useAppStore.setState((state) => ({ modelDownloads: state.modelDownloads.some((entry) => entry.id === item.id)
                    ? state.modelDownloads : [...state.modelDownloads, item] }));
                setSelection(null);
                setProgressOpen(true);
            }} /> : null}
    </Box>;
}

function isModelDownloading(item: ModelDownload): boolean {
    return item.status === "downloading" || item.status === "checking" || item.status === "registering";
}
