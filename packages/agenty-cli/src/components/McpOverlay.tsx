import { useEffect, useMemo, useState } from "react";

import type { McpLogEntry, McpServerConfig, McpServerDto, McpTransport } from "../api/types";
import { useAppStore } from "../state/store";
import { ConfirmDialog } from "./ConfirmDialog";
import {
    type FormField,
    FormPanel,
    formString,
    formStringList,
    type FormValues,
} from "./FormPanel";
import { List, useListNavigation } from "./List";
import {
    ADD_MCP_SERVER_KEY,
    buildMcpRows,
    type McpListRow,
    mcpRowKey,
} from "./mcpRows";
import { Panel } from "./Panel";
import { Box, Text } from "./ui";

type Mode =
    | { kind: "list" }
    | { kind: "add" }
    | { kind: "edit"; target: McpServerDto }
    | { kind: "confirm-delete"; target: McpServerDto };

export const MCP_OVERLAY_HEIGHT = 20;

const transportOptions = [
    { label: "Streamable HTTP", value: "http" },
    { label: "SSE (deprecated)", value: "sse" },
    { label: "Stdio", value: "stdio" },
];

function statusLabel(server: McpServerDto): string {
    const suffix = server.deprecated ? " · deprecated" : "";
    if (server.status === "error" && server.error) {
        return `${server.status}${suffix}: ${server.error}`;
    }
    return `${server.status}${suffix}`;
}

function parseMap(value: string, field: string): Record<string, string> | undefined {
    if (!value.trim()) {
        return undefined;
    }
    const parsed: unknown = JSON.parse(value);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        throw new Error(`${field} must be a JSON object`);
    }
    const result: Record<string, string> = {};
    for (const [key, item] of Object.entries(parsed)) {
        if (typeof item !== "string") {
            throw new Error(`${field}.${key} must be a string`);
        }
        result[key] = item;
    }
    return result;
}

export function buildFields(
    target: McpServerDto | undefined,
    transport: McpTransport,
    includeEnabled = target === undefined,
): FormField[] {
    const config = target?.config;
    return [
        {
            key: "name",
            label: "Name",
            kind: "text",
            value: target?.name ?? "",
            readOnly: target !== undefined,
        },
        {
            key: "type",
            label: "Transport",
            kind: "select",
            value: transport,
            options: transportOptions,
        },
        {
            key: "enabled",
            label: "Enabled",
            kind: "boolean",
            value: String(config?.enabled ?? true),
            visible: includeEnabled,
        },
        {
            key: "command",
            label: "Command",
            kind: "text",
            value: config?.command ?? "",
            visible: transport === "stdio",
        },
        {
            key: "args",
            label: "Arguments",
            kind: "string-list",
            value: config?.args ?? [],
            visible: transport === "stdio",
        },
        {
            key: "env",
            label: "Environment JSON",
            kind: "text",
            value: config?.env ? JSON.stringify(config.env) : "",
            visible: transport === "stdio",
            placeholder: "{\"TOKEN\":\"${TOKEN}\"}",
        },
        {
            key: "url",
            label: "URL",
            kind: "text",
            value: config?.url ?? "",
            visible: transport !== "stdio",
        },
    ];
}

export function buildConfig(values: FormValues, target?: McpServerDto): McpServerConfig {
    const type = formString(values, "type") as McpTransport;
    const config: McpServerConfig = {
        type,
        enabled: target?.config.enabled ?? formString(values, "enabled") === "true",
    };
    if (type === "stdio") {
        config.command = formString(values, "command").trim();
        const args = formStringList(values, "args").filter((arg) => arg.length > 0);
        config.args = args.length > 0 ? args : undefined;
        config.env = parseMap(formString(values, "env"), "env");
    } else {
        config.url = formString(values, "url").trim();
        if (target && target.config.type !== "stdio") {
            config.headers = target.config.headers;
            config.bearerTokenEnvVar = target.config.bearerTokenEnvVar;
            if (type === "http") {
                config.oauth = target.config.oauth;
            }
        }
    }
    return config;
}

export function McpOverlay() {
    const client = useAppStore((state) => state.client);
    const setOverlay = useAppStore((state) => state.setOverlay);
    const setToast = useAppStore((state) => state.setToast);
    const [servers, setServers] = useState<McpServerDto[]>([]);
    const [selectedKey, setSelectedKey] = useState<string | null>(null);
    const [mode, setMode] = useState<Mode>({ kind: "list" });
    const [transport, setTransport] = useState<McpTransport>("http");
    const [logs, setLogs] = useState<McpLogEntry[]>([]);
    const [logsLoading, setLogsLoading] = useState(false);
    const [logsError, setLogsError] = useState<string | null>(null);

    const reload = async () => {
        if (!client) {
            return;
        }
        try {
            const next = await client.listMcpServers();
            setServers(next);
            setSelectedKey((current) => {
                if (current === ADD_MCP_SERVER_KEY || next.some((server) => server.name === current)) {
                    return current;
                }
                return next[0]?.name ?? ADD_MCP_SERVER_KEY;
            });
            setMode((current) => {
                if (current.kind !== "edit") {
                    return current;
                }
                const target = next.find((server) => server.name === current.target.name);
                return target ? { kind: "edit", target } : { kind: "list" };
            });
        } catch (error) {
            setToast(`failed to load MCP servers: ${error instanceof Error ? error.message : String(error)}`, true);
        }
    };

    useEffect(() => {
        void reload();
        if (!client) {
            return;
        }
        return client.onMcpEvent((event) => {
            void reload();
            if (event.authUrl) {
                setToast(`Open this URL to authorize ${event.name}: ${event.authUrl}`);
                openBrowser(event.authUrl);
            }
        });
    }, [client]);

    useEffect(() => {
        if (!client || mode.kind !== "edit" || !hasConnectionIssue(mode.target)) {
            setLogs([]);
            setLogsLoading(false);
            setLogsError(null);
            return;
        }

        let cancelled = false;
        setLogsLoading(true);
        setLogsError(null);
        void client.listMcpServerLogs(mode.target.name)
            .then((next) => {
                if (!cancelled) {
                    setLogs(next);
                }
            })
            .catch((error: unknown) => {
                if (!cancelled) {
                    setLogs([]);
                    setLogsError(error instanceof Error ? error.message : String(error));
                }
            })
            .finally(() => {
                if (!cancelled) {
                    setLogsLoading(false);
                }
            });
        return () => {
            cancelled = true;
        };
    }, [client, mode]);

    const openRow = (row: McpListRow) => {
        if (row.kind === "add-server") {
            setTransport("http");
            setMode({ kind: "add" });
            return;
        }
        setTransport(row.server.config.type);
        setMode({ kind: "edit", target: row.server });
    };

    const applyEditOperation = (operation: Promise<McpServerDto>, message: string) => {
        void operation
            .then((server) => {
                setToast(message);
                setSelectedKey(server.name);
                setMode({ kind: "edit", target: server });
                return reload();
            })
            .catch((error: unknown) => setToast(error instanceof Error ? error.message : String(error), true));
    };

    if (mode.kind === "add" || mode.kind === "edit") {
        const target = mode.kind === "edit" ? mode.target : undefined;
        const editing = target !== undefined;
        return (
            <FormPanel
                title={editing ? `Update MCP Server · ${target.name}` : "Add MCP Server"}
                fields={buildFields(target, transport, !editing)}
                actions={editing ? editActions(target) : undefined}
                afterFields={editing && hasConnectionIssue(target) ? (
                    <McpLogs logs={logs} loading={logsLoading} error={logsError} />
                ) : undefined}
                hint={transport === "sse" ? "SSE transport is deprecated; use Streamable HTTP when available." : undefined}
                onChange={(key, values) => {
                    if (key === "type") {
                        setTransport(formString(values, "type") as McpTransport);
                    }
                }}
                onAction={(action, values) => {
                    if (!client) {
                        return;
                    }
                    if (!editing) {
                        if (action !== "save") {
                            setMode({ kind: "list" });
                            return;
                        }
                        try {
                            const config = buildConfig(values);
                            void client.createMcpServer(formString(values, "name").trim(), config).then((server) => {
                                setToast("MCP server added.");
                                setSelectedKey(server.name);
                                setMode({ kind: "list" });
                                return reload();
                            }).catch((error: unknown) => setToast(error instanceof Error ? error.message : String(error), true));
                        } catch (error) {
                            setToast(error instanceof Error ? error.message : String(error), true);
                        }
                        return;
                    }

                    if (action === "delete") {
                        setMode({ kind: "confirm-delete", target });
                        return;
                    }
                    if (action === "login") {
                        applyEditOperation(client.loginMcpServer(target.name), "MCP login started.");
                        return;
                    }
                    if (action === "enable" || action === "disable") {
                        applyEditOperation(
                            client.setMcpEnabled(target.name, action === "enable"),
                            action === "enable" ? "MCP server enabled." : "MCP server disabled.",
                        );
                        return;
                    }
                    if (action !== "save") {
                        return;
                    }
                    try {
                        const config = buildConfig(values, target);
                        applyEditOperation(client.updateMcpServer(target.name, config), "MCP server updated.");
                    } catch (error) {
                        setToast(error instanceof Error ? error.message : String(error), true);
                    }
                }}
                onClose={() => setMode({ kind: "list" })}
            />
        );
    }

    if (mode.kind === "confirm-delete") {
        return (
            <ConfirmDialog
                title={`Delete MCP server "${mode.target.name}"?`}
                message="This disconnects the server, removes its configuration, and deletes saved OAuth credentials."
                onConfirm={() => {
                    if (!client) {
                        setMode({ kind: "list" });
                        return;
                    }
                    void client.removeMcpServer(mode.target.name)
                        .then(() => {
                            setToast("MCP server deleted.");
                            setSelectedKey(ADD_MCP_SERVER_KEY);
                            setMode({ kind: "list" });
                            return reload();
                        })
                        .catch((error: unknown) => setToast(error instanceof Error ? error.message : String(error), true));
                }}
                onCancel={() => setMode({ kind: "list" })}
            />
        );
    }

    return (
        <Panel title="MCP Servers" hint="↑↓ servers · Enter edit/add · Esc close">
            <McpServerList
                servers={servers}
                selectedKey={selectedKey}
                onSelectedKey={setSelectedKey}
                onActivate={openRow}
                onClose={() => setOverlay(null)}
            />
        </Panel>
    );
}

function McpServerList({
    servers,
    selectedKey,
    onSelectedKey,
    onActivate,
    onClose,
}: {
    servers: McpServerDto[];
    selectedKey: string | null;
    onSelectedKey: (key: string) => void;
    onActivate: (row: McpListRow) => void;
    onClose: () => void;
}) {
    const rows = useMemo(() => buildMcpRows(servers), [servers]);
    const cursor = Math.max(rows.findIndex((row) => mcpRowKey(row) === selectedKey), 0);

    const selectCursor = (index: number) => {
        const row = rows[index];
        if (!row) {
            return;
        }
        onSelectedKey(mcpRowKey(row));
    };

    useListNavigation({
        items: rows,
        cursor,
        onCursor: selectCursor,
        onActivate: (row) => onActivate(row),
        onClose,
        onInput: (_input, _key, event) => {
            event.preventDefault();
            event.stopPropagation();
        },
    });

    return (
        <Box flexDirection="column" flexGrow={1} width="100%">
            <List
                items={rows}
                cursor={cursor}
                visibleCount={8}
                gap={1}
                getKey={(row) => row.key}
                onCursor={selectCursor}
                onActivate={onActivate}
                renderItem={(row, state) => (
                    <Box flexDirection="column" flexGrow={1} height={1} overflow="hidden">
                        {row.kind === "server" ? (
                            <Text color={state.selected ? "cyan" : "white"} bold={state.selected} wrap="truncate">
                                {row.server.name} · {row.server.config.type}{row.server.deprecated ? " (deprecated)" : ""} · {row.server.toolCount} tools · {statusLabel(row.server)}
                            </Text>
                        ) : (
                            <Text color={state.selected ? "cyan" : "gray"} dimColor={!state.selected}>
                                {row.label}
                            </Text>
                        )}
                    </Box>
                )}
            />
        </Box>
    );
}

function editActions(target: McpServerDto): Array<{ key: string; label: string }> {
    const actions = [{ key: "save", label: "Save" }];
    if (needsLogin(target)) {
        actions.push({ key: "login", label: "Login" });
    }
    actions.push(
        { key: target.config.enabled === false ? "enable" : "disable", label: target.config.enabled === false ? "Enable" : "Disable" },
        { key: "delete", label: "Delete" },
        { key: "cancel", label: "Cancel" },
    );
    return actions;
}

function needsLogin(server: McpServerDto): boolean {
    return server.config.type === "http" && (
        server.status === "auth-required" ||
        server.errorStage === "authorize" ||
        Boolean(server.authUrl)
    );
}

function hasConnectionIssue(server: McpServerDto): boolean {
    return server.status === "error" || server.status === "auth-required" || Boolean(server.error);
}

function McpLogs({
    logs,
    loading,
    error,
}: {
    logs: McpLogEntry[];
    loading: boolean;
    error: string | null;
}) {
    const recent = logs.slice(-5);
    return (
        <Box flexDirection="column" width="100%" height={Math.max(recent.length + 1, 2)} overflow="hidden">
            <Text color="magenta" bold>Connection logs</Text>
            {loading ? <Text dimColor>Loading logs…</Text> : null}
            {error ? <Text color="red" wrap="truncate">Failed to load logs: {error}</Text> : null}
            {!loading && !error && recent.length === 0 ? <Text dimColor>No connection logs captured.</Text> : null}
            {!loading && !error ? recent.map((log, index) => {
                const time = log.time.length >= 19 ? log.time.slice(11, 19) : log.time;
                const stage = log.stage ? `/${log.stage}` : "";
                return (
                    <Text key={`${log.time}:${index}`} color={log.level === "error" ? "red" : "gray"} wrap="truncate">
                        {time} {log.level}/{log.source}{stage}: {log.message}
                    </Text>
                );
            }) : null}
        </Box>
    );
}

function openBrowser(url: string) {
    try {
        const parsed = new URL(url);
        if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
            return;
        }
    } catch {
        return;
    }
    const command = process.platform === "darwin" ? "open" : process.platform === "win32" ? "cmd" : "xdg-open";
    const args = process.platform === "win32" ? ["/c", "start", "", url] : [url];
    void Bun.spawn([command, ...args], { stdout: "ignore", stderr: "ignore" });
}
