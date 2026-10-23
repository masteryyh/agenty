import { useCallback, useEffect, useState } from "react";

import type {
    CoreModelDto,
    LocalInferenceConfig,
    LocalModel,
    LocalResource,
    LocalRuntimeSnapshot,
    LocalWorker,
    ModelProviderDto,
    WorkerInstallation
} from "../api/types";
import { theme } from "../consts/theme";
import { useInput } from "../hooks/useInput";
import { useAppStore } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";
import { ConfirmDialog } from "./ConfirmDialog";
import { type FormField, FormPanel, formString, type FormValues } from "./FormPanel";
import { List, useListNavigation } from "./List";
import { ModelDownloadPanel } from "./ModelDownloadPanel";
import { Panel } from "./Panel";
import { Box, Text } from "./ui";

export function localWorkerAvailable(backend: LocalWorker["backend"], platform = process.platform, arch = process.arch): boolean {
    return backend !== "mlx" || platform === "darwin" && arch === "arm64";
}

export function localWorkerStatus(backend: LocalWorker["backend"], config: LocalInferenceConfig, runtime: LocalRuntimeSnapshot | null, platform = process.platform, arch = process.arch): string {
    if (!localWorkerAvailable(backend, platform, arch)) {
        return "UNAVAILABLE";
    }
    const workers = config.workers.filter((worker) => worker.backend === backend && worker.command !== "");
    if (!workers.length) {
        return "NOT INSTALLED";
    }
    if (!workers.some((worker) => worker.enabled)) {
        return "DISABLED";
    }
    const state = runtime?.workers.find((worker) => workers.some((installed) => installed.id === worker.id));
    if (state && ["ready", "generating", "running"].includes(state.status)) {
        return "RUNNING";
    }
    if (state && state.status !== "stopped") {
        return state.status.toUpperCase();
    }
    return "INSTALLED";
}

interface LocalSettingsRow {
    key: string;
    label: string;
    value?: string;
    disabled?: boolean;
}

export function localModelFromForm(values: FormValues, previous?: LocalModel): LocalModel {
    const codecDirectory = formString(values, "codecDirectory").trim();
    const mlx = formString(values, "mlxPath").trim();
    const gguf = formString(values, "ggufPath").trim();
    const resources: LocalResource[] = [];
    if (mlx) {
        resources.push({ resourceId: "mlx", backend: "mlx", format: "safetensors", weightsPath: mlx, codecDirectory });
    }
    if (gguf) {
        resources.push({ resourceId: "gguf", backend: "llamaCpp", format: "gguf", weightsPath: gguf,
            codecDirectory: formString(values, "ggufCodecDirectory").trim() || codecDirectory });
    }
    if (resources.length === 0 || !codecDirectory) {
        throw new Error("Enter a tokenizer directory and at least one model weight path.");
    }
    const parser = formString(values, "parser");
    if (parser !== "qwen3" && parser !== "qwen3_5") {
        throw new Error("Choose a supported model format.");
    }
    const backendPreference = formString(values, "backendPreference") || previous?.backendPreference || "auto";
    if (backendPreference !== "auto" && backendPreference !== "mlx" && backendPreference !== "llamaCpp") {
        throw new Error("Choose a supported backend.");
    }
    return {
        backendPreference,
        outputParser: parser,
        templateName: formString(values, "templateName").trim() || undefined,
        thinkingEffort: formString(values, "thinking") === "true" ? "medium" : undefined,
        resources: resources.map((resource) => ({ ...resource,
            resourceId: previous?.resources.find((item) => item.backend === resource.backend)?.resourceId ?? resource.resourceId })),
    };
}

export function LocalInferencePanel({ onDone, wizard = false }: { onDone: (provider?: ModelProviderDto) => void; wizard?: boolean }) {
    const client = useAppStore((state) => state.client);
    const runtime = useAppStore((state) => state.localRuntime);
    const installations = useAppStore((state) => state.workerInstallations);
    const [config, setConfig] = useState<LocalInferenceConfig>({ enabled: false, workers: [] });
    const [provider, setProvider] = useState<ModelProviderDto>();
    const [loaded, setLoaded] = useState(false);
    const [editing, setEditing] = useState<CoreModelDto | null>(null);
    const [downloading, setDownloading] = useState(false);
    const [deleting, setDeleting] = useState<CoreModelDto | null>(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [pathsOpen, setPathsOpen] = useState(false);
    const [limitsOpen, setLimitsOpen] = useState(false);
    const [cursor, setCursor] = useState(0);
    const size = useBottomDialogSize();
    const reload = useCallback(async () => {
        if (!client) {
            return;
        }
        const [next, providers] = await Promise.all([client.localInferenceConfig(), client.listProviders("local")]);
        setConfig({ ...next, workers: next.workers ?? [] });
        setProvider(providers.find((item) => item.code === "local"));
        setLoaded(true);
        return providers.find((item) => item.code === "local");
    }, [client]);
    useEffect(() => {
        let active = true;
        void reload().catch((cause: unknown) => {
            if (active) {
                setError(cause instanceof Error ? cause.message : String(cause));
            }
        });
        return () => {
            active = false;
        };
    }, [reload, installations.filter((item) => item.status === "installed").length]);

    const run = async (operation: () => Promise<void>) => {
        if (busy) {
            return;
        }
        setBusy(true);
        setError(null);
        try {
            await operation();
        } catch (cause: unknown) {
            setError(cause instanceof Error ? cause.message : String(cause));
        } finally {
            setBusy(false);
        }
    };
    const close = () => {
        if (!busy) {
            onDone(provider);
        }
    };
    if (downloading) {
        return <ModelDownloadPanel wizard={wizard} onClose={() => {
            setDownloading(false);
            void reload().catch((cause: unknown) => setError(cause instanceof Error ? cause.message : String(cause)));
        }} />;
    }
    if (!loaded) {
        return <FormPanel key="loading" title="Local inference" fields={[]} actions={[{ key: "back", label: "Back" }]} error={error}
            showHint={!wizard} onClose={close} onAction={close} />;
    }
    if (deleting) {
        return <ConfirmDialog title="Delete local model" letterShortcuts={!wizard} message={`Delete ${deleting.name}? Model files will remain on disk.`}
            onConfirm={() => {
                void run(async () => {
                    await client?.deleteModel("local", deleting.code);
                    setDeleting(null);
                    await reload();
                });
            }}
            onCancel={() => setDeleting(null)} />;
    }
    if (editing) {
        const model = editing;
        const fields: FormField[] = [
            { key: "code", label: "Model code", kind: "text", value: model?.code ?? "qwen3.5-0.8b", readOnly: model !== undefined },
            { key: "name", label: "Name", kind: "text", value: model?.name ?? "Qwen3.5 0.8B" },
            { key: "parser", label: "Model format", kind: "select", value: model?.local?.outputParser ?? "qwen3_5", options: [{ label: "Qwen3.5", value: "qwen3_5" }, { label: "Qwen3", value: "qwen3" }] },
            { key: "backendPreference", label: "Preferred backend", kind: "select", value: model?.local?.backendPreference || "auto", options: [{ label: "Automatic", value: "auto" }, { label: "MLX", value: "mlx" }, { label: "llama.cpp", value: "llamaCpp" }] },
            { key: "templateName", label: "Template name", kind: "text", value: model?.local?.templateName ?? "", placeholder: "Optional named template" },
            { key: "thinking", label: "Enable thinking", kind: "boolean", value: String(model?.local?.thinkingEffort !== undefined) },
            { key: "section:paths", label: "Paths", kind: "disclosure", value: String(pathsOpen) },
            { key: "codecDirectory", label: "Tokenizer directory", kind: "text", value: model?.local?.resources[0]?.codecDirectory ?? "", placeholder: "Absolute path containing tokenizer.json and chat template", visible: pathsOpen },
            { key: "mlxPath", label: "MLX weights dir", kind: "text", value: model?.local?.resources.find((item) => item.backend === "mlx")?.weightsPath ?? "", placeholder: "Optional absolute Safetensors directory", visible: pathsOpen },
            { key: "ggufPath", label: "GGUF file", kind: "text", value: model?.local?.resources.find((item) => item.backend === "llamaCpp")?.weightsPath ?? "", placeholder: "Optional absolute .gguf file", visible: pathsOpen },
            { key: "ggufCodecDirectory", label: "GGUF tokenizer override", kind: "text", value: model?.local?.resources.find((item) => item.backend === "llamaCpp")?.codecDirectory ?? "", placeholder: "Leave empty to use the tokenizer directory above", visible: pathsOpen },
            { key: "section:limits", label: "Limits", kind: "disclosure", value: String(limitsOpen) },
            { key: "contextWindow", label: "Context window", kind: "text", value: String(model?.contextWindow ?? 8192), visible: limitsOpen },
            { key: "maxOutputTokens", label: "Output limit", kind: "text", value: String(model?.maxOutputTokens ?? 512), visible: limitsOpen },
        ];
        return <FormPanel key={`model:${model?.code ?? "new"}`} title="Local model" showHint={!wizard} fields={fields} error={error} active={!busy}
            onChange={(key, values) => {
                if (key === "section:paths") {
                    setPathsOpen(values["section:paths"] === "true");
                } else if (key === "section:limits") {
                    setLimitsOpen(values["section:limits"] === "true");
                }
            }}
            actions={[{ key: "save", label: busy ? "Checking…" : "Check and save" }, ...(model ? [{ key: "delete", label: "Delete" }] : []), { key: "back", label: "Back" }]}
            onClose={() => setEditing(null)} onAction={(key, values) => {
                if (key === "back") {
                    setEditing(null);
                    return;
                }
                if (key === "delete" && model) {
                    setDeleting(model);
                    return;
                }
                void run(async () => {
                    if (!client) {
                        return;
                    }
                    if (model?.local?.resources.some((item, index, all) => all.findIndex((other) => other.backend === item.backend) !== index)) {
                        throw new Error("Edit models with several resources per backend using model update --local-config.");
                    }
                    const local = localModelFromForm(values, model?.local);
                    const contextWindow = Number(formString(values, "contextWindow"));
                    const maxOutputTokens = Number(formString(values, "maxOutputTokens"));
                    const code = formString(values, "code").trim();
                    const name = formString(values, "name").trim();
                    if (!code || !name || !Number.isSafeInteger(contextWindow) || !Number.isSafeInteger(maxOutputTokens) || maxOutputTokens <= 0 || contextWindow <= maxOutputTokens) {
                        throw new Error("Enter a name, model code, and positive token limits with output below context.");
                    }
                    if (wizard) {
                        const current = await client.localInferenceConfig();
                        await client.configureLocalInference({ workers: (current.workers ?? []).map((worker) => ({ ...worker, enabled: localWorkerAvailable(worker.backend) })) });
                    }
                    await client.probeLocalModel(local);
                    await client.createModel({ providerCode: "local", modelCode: code, name, contextWindow, maxOutputTokens, local,
                        reasoning: local.thinkingEffort !== undefined, reasoningEfforts: local.thinkingEffort ? ["medium"] : [], multiModal: false });
                    setEditing(null);
                    await reload();
                });
            }} />;
    }
    const handleAction = (key: string) => {
        if (key === "download-model") {
            if (wizard) {
                void run(async () => {
                    if (!client) {
                        return;
                    }
                    await client.configureLocalInference({ workers: config.workers.map((worker) => ({ ...worker, enabled: localWorkerAvailable(worker.backend) })) });
                    setDownloading(true);
                });
            } else {
                setDownloading(true);
            }
            return;
        }
        if (key.startsWith("model:")) {
            setEditing(provider?.models.find((item) => item.code === key.slice(6)) ?? null);
            return;
        }
        void run(async () => {
            if (!client) {
                return;
            }
            if (key.startsWith("cancel:")) {
                await client.cancelWorkerInstallation(key.slice(7));
                return;
            }
            if (key.startsWith("download:")) {
                const backend = key.slice(9);
                if (backend === "mlx" && !localWorkerAvailable("mlx")) {
                    return;
                }
                const bundles = await client.workerBundles();
                const bundle = bundles.find((item) => item.backend === backend);
                if (!bundle) {
                    throw new Error("No worker package is available for this platform.");
                }
                await client.installWorker(bundle.url, bundle.sha256);
                return;
            }
            if (installations.some((item) => item.status === "downloading" || item.status === "installing")) {
                throw new Error("Wait for the worker installation to finish or cancel it before saving settings.");
            }
            if (wizard) {
                const [current, providers] = await Promise.all([client.localInferenceConfig(), client.listProviders("local")]);
                const workers = (current.workers ?? []).map((worker) => ({ ...worker, enabled: localWorkerAvailable(worker.backend) }));
                if (!workers.some((worker) => worker.enabled && worker.command !== "")) {
                    throw new Error("Install at least one compatible worker before saving.");
                }
                const models = providers.find((item) => item.code === "local")?.models ?? [];
                if (models.length === 0) {
                    throw new Error("Download at least one supported model before saving.");
                }
                await client.configureLocalInference({ workers });
                let ready = false;
                let failure: unknown;
                for (const model of models) {
                    if (!model.local) {
                        continue;
                    }
                    try {
                        await client.probeLocalModel(model.local);
                        ready = true;
                        break;
                    } catch (cause: unknown) {
                        failure = cause;
                    }
                }
                if (!ready) {
                    throw failure instanceof Error ? failure : new Error("No local model matches the installed workers.");
                }
                await client.configureLocalInference({ enabled: true });
                const configured = await reload();
                if (!configured) {
                    throw new Error("Could not reload the local inference configuration.");
                }
                onDone(configured);
                return;
            }
            await client.configureLocalInference({ ...config, workers: config.workers.map((worker) => ({ ...worker, enabled: worker.enabled && localWorkerAvailable(worker.backend) })) });
            await reload();
        });
    };
    return <LocalSettingsList config={config} runtime={runtime} provider={provider} wizard={wizard} busy={busy} error={error}
        installations={installations} cursor={cursor} onCursor={setCursor} height={size.height} onClose={close}
        onAction={(key) => {
            if (key === "enabled") {
                setConfig((value) => ({ ...value, enabled: !value.enabled }));
            } else if (key.startsWith("reinstall:")) {
                handleAction(`download:${key.slice(10)}`);
            } else if (key.startsWith("download:") && !wizard && config.workers.some((worker) => worker.backend === key.slice(9) && worker.command)) {
                const backend = key.slice(9);
                const enabled = !config.workers.some((worker) => worker.backend === backend && worker.enabled);
                setConfig((value) => ({ ...value, workers: value.workers.map((worker) => worker.backend === backend ? { ...worker, enabled } : worker) }));
            } else {
                handleAction(key);
            }
        }} />;
}

interface LocalSettingsListProps {
    config: LocalInferenceConfig;
    runtime: LocalRuntimeSnapshot | null;
    provider?: ModelProviderDto;
    wizard: boolean;
    busy: boolean;
    error: string | null;
    installations: WorkerInstallation[];
    cursor: number;
    onCursor: (cursor: number) => void;
    height: number;
    onClose: () => void;
    onAction: (key: string) => void;
}

function LocalSettingsList({ config, runtime, provider, wizard, busy, error, installations, cursor, onCursor, height, onClose, onAction }: LocalSettingsListProps) {
    const workers: LocalSettingsRow[] = [
        ...(!wizard ? [{ key: "enabled", label: "Local inference", value: config.enabled ? "[ENABLED]" : "[DISABLED]" }] : []),
        ...(["mlx", "llamaCpp"] as const).map((backend) => ({ key: `download:${backend}`,
            label: backend === "mlx" ? "MLX worker" : "llama.cpp worker",
            value: `[${localWorkerStatus(backend, config, runtime)}]`,
            disabled: !localWorkerAvailable(backend) || wizard && config.workers.some((worker) => worker.backend === backend && worker.command !== ""),
        })),
    ];
    const models: LocalSettingsRow[] = (provider?.models ?? []).map((model) => ({ key: `model:${model.code}`, label: model.name,
        value: [...new Set(model.local?.resources.map((resource) => resource.backend === "mlx" ? "MLX" : "GGUF"))].join(" / ") }));
    const actions: LocalSettingsRow[] = [
        ...installations.filter((item) => item.status === "downloading" || item.status === "installing").map((item) => ({ key: `cancel:${item.id}`, label: "Cancel worker download" })),
        { key: "download-model", label: "Download model from Hugging Face / ModelScope…" },
        { key: "save", label: busy ? "Saving…" : wizard ? "Save and back" : "Save settings" },
    ];
    const rows = [...workers, ...models, ...actions];
    const navigation = rows.filter((row) => !row.disabled);
    const bounded = Math.min(cursor, Math.max(navigation.length - 1, 0));
    const selectedKey = navigation[bounded]?.key;
    useListNavigation({ items: navigation, cursor: bounded, onCursor, onActivate: (row) => onAction(row.key), onClose, active: !busy });
    const selectedBackend = selectedKey?.startsWith("download:") ? selectedKey.slice(9) : null;
    const reinstall = !wizard && selectedBackend && config.workers.some((worker) => worker.backend === selectedBackend && worker.command);
    useInput((_input, key, event) => {
        if (reinstall && key.ctrl && event.name === "r") {
            onAction(`reinstall:${selectedBackend}`);
        }
    }, { isActive: !busy });
    const renderRow = (row: LocalSettingsRow, selected: boolean) => <Box flexGrow={1} flexBasis={0}>
        <Box flexGrow={1} flexBasis={0} overflow="hidden"><Text wrap="truncate" bold={selected} color={row.disabled ? theme.textFaint : selected ? theme.selection : theme.text}>{row.label}</Text></Box>
        {row.value ? <Box flexShrink={0} paddingLeft={1}><Text color={selected ? theme.selection : row.value === "[RUNNING]" ? theme.success : theme.textMuted}>{row.value}</Text></Box> : null}
    </Box>;
    const list = (rows: LocalSettingsRow[], visibleCount: number) => <List items={rows} cursor={rows.findIndex((row) => row.key === selectedKey)}
        onCursor={(index) => {
            const position = navigation.findIndex((row) => row.key === rows[index]?.key);
            if (position >= 0) {
                onCursor(position);
            }
        }} onActivate={(row) => {
            if (!busy && !row.disabled) {
                onAction(row.key);
            }
        }}
        active={!busy} getKey={(row) => row.key} visibleCount={visibleCount}
        renderItem={(row, { selected }) => renderRow(row, selected)} emptyHint={<Text dimColor>No models yet. Download a supported model to get started.</Text>} />;
    if (height < 12) {
        return <Panel title="Local inference" error={error}>{list(rows, Math.max(height - 3, 1))}</Panel>;
    }
    return <Panel title="Local inference" error={error} hint={reinstall ? "↑/↓ select · Enter toggle · Ctrl+R reinstall · Esc back" : "↑/↓ select · Enter act · Esc back"}>
        <Box height={workers.length} width="100%" flexShrink={0}>{list(workers, workers.length)}</Box>
        {runtime?.workers.filter((worker) => worker.error).map((worker) => <Text key={worker.id} color={theme.danger} wrap="truncate">{worker.error}</Text>)}
        {installations.map((item) => <Text key={item.id} dimColor wrap="truncate">{item.error ?? `${item.status} · ${Math.round(item.downloadedBytes / 1024 ** 2)} / ${item.totalBytes > 0 ? Math.round(item.totalBytes / 1024 ** 2) : "?"} MiB`}</Text>)}
        <Box marginTop={1} marginBottom={1}><Text bold color={theme.textMuted}>Registered models ({models.length})</Text></Box>
        {list(models, Math.max(height - workers.length - actions.length - installations.length - 8, 1))}
        <Box flexDirection="column" height={actions.length + 2} flexShrink={0} paddingTop={1} paddingBottom={1} width="100%">{list(actions, actions.length)}</Box>
    </Panel>;
}
