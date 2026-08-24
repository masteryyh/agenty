import { useCallback, useEffect, useRef, useState } from "react";

import { formatModelRef, modelRefFromModel } from "../api/modelReference";
import type { AgentDto, ModelDto } from "../api/types";
import { useInput } from "../hooks/useInput";
import { useAppStore } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";
import { ConfirmDialog } from "./ConfirmDialog";
import type { FormField, FormOption } from "./FormPanel";
import { FormPanel } from "./FormPanel";
import { List, useListNavigation } from "./List";
import { Panel } from "./Panel";
import {
    createTableLayout,
    type TableColumn,
    TableHeader,
    TableRow,
} from "./Table";
import { ActionBar, Box, Spinner, Text } from "./ui";

type Mode =
    | { kind: "list" }
    | { kind: "create" }
    | { kind: "edit"; target: AgentDto }
    | { kind: "confirm-delete"; target: AgentDto };

export function AgentOverlay() {
    const client = useAppStore((s) => s.client);
    const setToast = useAppStore((s) => s.setToast);
    const setOverlay = useAppStore((s) => s.setOverlay);
    const currentAgent = useAppStore((s) => s.agent);
    const switchAgent = useAppStore((s) => s.switchAgent);

    const [agents, setAgents] = useState<AgentDto[] | null>(null);
    const [modelOptions, setModelOptions] = useState<FormOption[]>([]);
    const [models, setModels] = useState<ModelDto[]>([]);
    const [cursor, setCursor] = useState(0);
    const [mode, setMode] = useState<Mode>({ kind: "list" });
    const modeRef = useRef(mode);
    modeRef.current = mode;

    const reload = useCallback(async () => {
        if (!client) {
            return;
        }
        try {
            const list = await client.listAgents();
            setAgents(list);
            setCursor((c) => Math.min(c, Math.max(list.length - 1, 0)));
        } catch (e) {
            setToast(`failed to load agents: ${(e as Error).message}`, true);
            setAgents([]);
        }
    }, [client, setToast]);

    const reloadModels = useCallback(async () => {
        if (!client) {
            return;
        }
        try {
            const models = await client.listModels();
            setModels(models);
            setModelOptions(
                models.map((m) => ({
                    label: `${m.providerName} · ${m.name}`,
                    value: formatModelRef(modelRefFromModel(m)),
                })),
            );
        } catch {
            setModelOptions([]);
        }
    }, [client]);

    useEffect(() => {
        void reload();
        void reloadModels();
    }, [reload, reloadModels]);

    const close = () => setOverlay(null);

    // Top-level input handler - ensures empty / loading list states respond to keyboard.
    useInput((input, key) => {
        if (modeRef.current.kind !== "list") {
            return;
        }
        if (agents !== null && agents.length > 0) {
            return;
        }
        if (key.escape) {
            close();
            return;
        }
        if (input === "a") {
            setMode({ kind: "create" });
            return;
        }
    });

    const buildFields = (target?: AgentDto): FormField[] => {
        const modelValue = target?.defaultModel
            ? formatModelRef(target.defaultModel)
            : modelOptions[0]?.value ?? "";
        return [
            { key: "code", label: "Agent Code", kind: "text" as const, value: target?.code ?? "", placeholder: "my-agent", readOnly: !!target },
            { key: "name", label: "Name", kind: "text" as const, value: target?.name ?? "", placeholder: "my-agent" },
            { key: "soul", label: "Soul", kind: "text" as const, value: target?.soul ?? "", placeholder: "system prompt, leave blank for default" },
            { key: "isDefault", label: "Default", kind: "boolean" as const, value: target ? (target.isDefault ? "true" : "false") : "false" },
            { key: "defaultModel", label: "Default model", kind: "select" as const, value: modelValue, options: modelOptions },
        ];
    };

    const handleCreate = async (values: Record<string, string>) => {
        if (!client) {
            return;
        }
        try {
            const selectedModel = modelForValue(models, values.defaultModel);
            const defaultModel = selectedModel ? modelRefFromModel(selectedModel) : undefined;
            await client.createAgent({
                code: values.code.trim(),
                name: values.name.trim(),
                soul: values.soul.trim(),
                isDefault: values.isDefault === "true",
                defaultModel,
                defaultContextWindow: selectedModel?.contextWindow ?? 0,
            });
            setToast(`Agent created: ${values.name.trim()}`);
            await reload();
        } catch (e) {
            setToast(`create failed: ${(e as Error).message}`, true);
        }
        setMode({ kind: "list" });
    };

    const handleEdit = async (target: AgentDto, values: Record<string, string>) => {
        if (!client) {
            return;
        }
        try {
            const selectedModel = modelForValue(models, values.defaultModel);
            const defaultModel = selectedModel ? modelRefFromModel(selectedModel) : undefined;
            await client.updateAgent(target.code, {
                name: values.name.trim(),
                soul: values.soul.trim(),
                isDefault: values.isDefault === "true",
                defaultModel,
                defaultContextWindow: selectedModel?.contextWindow ?? target.defaultContextWindow,
            });
            setToast(`Agent updated: ${values.name.trim()}`);
            await reload();
        } catch (e) {
            setToast(`update failed: ${(e as Error).message}`, true);
        }
        setMode({ kind: "list" });
    };

    const handleDelete = async (target: AgentDto) => {
        if (!client) {
            return;
        }
        try {
            await client.deleteAgent(target.code);
            setToast(`Agent deleted: ${target.name}`);
            await reload();
        } catch (e) {
            setToast(`delete failed: ${(e as Error).message}`, true);
        }
        setMode({ kind: "list" });
    };

    const handleSwitch = async (target: AgentDto) => {
        if (!client) {
            return;
        }
        if (currentAgent?.code === target.code) {
            setToast("Already using this agent.");
            setOverlay(null);
            return;
        }
        await switchAgent(target);
    };

    if (mode.kind === "create") {
        return (
            <FormPanel
                title="Add Agent"
                fields={buildFields()}
                onAction={(action, values) => {
                    if (action === "save") {
                        void handleCreate(values);
                    } else {
                        setMode({ kind: "list" });
                    }
                }}
                onClose={() => setMode({ kind: "list" })}
            />
        );
    }

    if (mode.kind === "edit") {
        return (
            <FormPanel
                title={`Edit: ${mode.target.name}`}
                fields={buildFields(mode.target)}
                onAction={(action, values) => {
                    if (action === "save") {
                        void handleEdit(mode.target, values);
                    } else {
                        setMode({ kind: "list" });
                    }
                }}
                onClose={() => setMode({ kind: "list" })}
            />
        );
    }

    if (mode.kind === "confirm-delete") {
        return (
            <DeleteConfirm
                target={mode.target}
                onConfirm={() => void handleDelete(mode.target)}
                onCancel={() => setMode({ kind: "list" })}
            />
        );
    }

    return (
        <Panel title="Agents">
            {agents === null ? (
                <Spinner label="Loading..." />
            ) : agents.length === 0 ? (
                <Text dimColor>No agents. Press `a` to add one.</Text>
            ) : (
                <AgentList
                    agents={agents}
                    currentAgentCode={currentAgent?.code}
                    cursor={cursor}
                    onCursor={setCursor}
                    onSwitch={(a) => void handleSwitch(a)}
                    onAdd={() => setMode({ kind: "create" })}
                    onEdit={(a) => setMode({ kind: "edit", target: a })}
                    onDelete={(a) => setMode({ kind: "confirm-delete", target: a })}
                    onClose={close}
                />
            )}
        </Panel>
    );
}

function modelForValue(models: readonly ModelDto[], value: string): ModelDto | undefined {
    return models.find((model) => formatModelRef(modelRefFromModel(model)) === value);
}

// ─── Agent list table ───────────────────────────────────────────────

function AgentList({
    agents,
    currentAgentCode,
    cursor,
    onCursor,
    onSwitch,
    onAdd,
    onEdit,
    onDelete,
    onClose,
}: {
    agents: AgentDto[];
    currentAgentCode?: string;
    cursor: number;
    onCursor: (i: number) => void;
    onSwitch: (a: AgentDto) => void;
    onAdd: () => void;
    onEdit: (a: AgentDto) => void;
    onDelete: (a: AgentDto) => void;
    onClose: () => void;
}) {
    const dialogSize = useBottomDialogSize();
    const maxVisible = Math.max(dialogSize.height - 5, 1);
    const agentFlags = (agent: AgentDto): string =>
        `${agent.isDefault ? "[default] " : ""}${agent.code === currentAgentCode ? "← current" : ""}`.trim();
    const columns: Array<TableColumn<AgentDto>> = [
        {
            key: "name",
            header: "Name",
            value: (agent) => agent.name,
            render: (agent, selected) => (
                <Text color={selected ? "cyan" : "white"} bold={selected} wrap="truncate">
                    {agent.name}
                </Text>
            ),
        },
        {
            key: "flags",
            header: "Flags",
            value: agentFlags,
            render: (agent, selected) => (
                <Text color={selected ? "cyan" : "gray"} dimColor={!selected} wrap="truncate">
                    {agentFlags(agent)}
                </Text>
            ),
        },
    ];
    const tableLayout = createTableLayout(
        columns,
        agents,
        Math.max(dialogSize.width - 2, 0),
    );

    useListNavigation({
        items: agents,
        cursor,
        onCursor,
        onActivate: onSwitch,
        onClose,
        onInput: (input, _key, _event, agent) => {
            const lower = input.toLowerCase();
            if (lower === "s" && agent) {
                onSwitch(agent);
            } else if (lower === "a") {
                onAdd();
            } else if (lower === "e" && agent) {
                onEdit(agent);
            } else if (lower === "d" && agent) {
                onDelete(agent);
            }
        },
    });

    return (
        <Panel
            footer={(
                <ActionBar
                    actions={[
                        { key: "add", label: "Add" },
                        { key: "edit", label: "Edit" },
                        { key: "delete", label: "Delete", tone: "danger" },
                    ]}
                    onAction={(key) => {
                        const agent = agents[cursor];
                        if (key === "add") {
                            onAdd();
                        } else if (key === "edit" && agent) {
                            onEdit(agent);
                        } else if (key === "delete" && agent) {
                            onDelete(agent);
                        }
                    }}
                />
            )}
            hint="↑↓ navigate · Enter/s switch · e edit · d delete · Esc back"
        >
            <Box width="100%" height={1} marginBottom={1} overflow="hidden">
                <Box width={2} height={1}><Text> </Text></Box>
                <TableHeader columns={tableLayout} />
            </Box>
            <List
                items={agents}
                cursor={cursor}
                visibleCount={maxVisible}
                getKey={(agent) => agent.code}
                onCursor={onCursor}
                onActivate={onSwitch}
                renderItem={(agent, { selected }) => (
                    <TableRow
                        columns={tableLayout}
                        row={agent}
                        selected={selected}
                    />
                )}
            />
        </Panel>
    );
}

// ─── Delete confirm ─────────────────────────────────────────────────

function DeleteConfirm({
    target,
    onConfirm,
    onCancel,
}: {
    target: AgentDto;
    onConfirm: () => void;
    onCancel: () => void;
}) {
    return (
        <ConfirmDialog
            title={`Delete agent "${target.name}"?`}
            message="This also deletes all its sessions, messages and memories."
            onConfirm={onConfirm}
            onCancel={onCancel}
        />
    );
}
