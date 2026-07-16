/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import { useCallback, useEffect, useRef, useState } from "react";
import { useInput } from "../hooks/useInput";
import { Box, Spinner, Text } from "./ui";
import type { MCPServerDto } from "../api/types";
import { useAppStore } from "../state/store";
import { FormPanel } from "./FormPanel";
import type { FormField } from "./FormPanel";
import { useBottomDialogSize } from "./BottomDialog";

const MCP_TRANSPORTS = ["stdio", "sse", "streamable-http"] as const;

const MCP_TRANSPORT_OPTIONS = MCP_TRANSPORTS.map((t) => ({ label: t, value: t }));

function buildMcpFields(transport: string, target?: MCPServerDto): FormField[] {
	const isStdio = transport === "stdio";
	return [
		{ key: "name", label: "Name", kind: "text" as const, value: target?.name ?? "", placeholder: "my-mcp-server" },
		{ key: "transport", label: "Transport", kind: "select" as const, value: transport, options: MCP_TRANSPORT_OPTIONS },
		{ key: "command", label: "Command", kind: "text" as const, value: target?.command ?? "", placeholder: "npx", visible: isStdio },
		{ key: "args", label: "Args", kind: "text" as const, value: target?.args ? JSON.stringify(target.args) : "", placeholder: '["-y","..."]', visible: isStdio },
		{ key: "env", label: "Env", kind: "text" as const, value: target?.env ? JSON.stringify(target.env) : "", placeholder: '{"HOME":"/tmp"}', visible: isStdio },
		{ key: "url", label: "URL", kind: "text" as const, value: target?.url ?? "", placeholder: "http://localhost:8080/sse", visible: !isStdio },
		{ key: "headers", label: "Headers", kind: "text" as const, value: target?.headers ? JSON.stringify(target.headers) : "", placeholder: '{"Auth":"..."}', visible: !isStdio },
		{ key: "enabled", label: "Enabled", kind: "boolean" as const, value: target ? (target.enabled ? "true" : "false") : "true" },
	];
}

function trunc(s: string, width: number): string {
	if (width <= 0) return "";
	if (s.length <= width) return s;
	if (width === 1) return "\u2026";
	return s.slice(0, width - 1) + "\u2026";
}

function pad(s: string, width: number): string {
	const clipped = trunc(s, width);
	return clipped + " ".repeat(Math.max(width - clipped.length, 0));
}

type Mode =
	| { kind: "list" }
	| { kind: "create" }
	| { kind: "edit"; target: MCPServerDto }
	| { kind: "confirm-delete"; target: MCPServerDto };

export function McpOverlay() {
	const client = useAppStore((s) => s.client);
	const setToast = useAppStore((s) => s.setToast);
	const setOverlay = useAppStore((s) => s.setOverlay);

	const [servers, setServers] = useState<MCPServerDto[] | null>(null);
	const [cursor, setCursor] = useState(0);
	const [mode, setMode] = useState<Mode>({ kind: "list" });
	const modeRef = useRef(mode);
	modeRef.current = mode;

	const reload = useCallback(async () => {
		if (!client) return;
		try {
			const list = await client.listMcpServers();
			setServers(list);
			setCursor((c) => Math.min(c, Math.max(list.length - 1, 0)));
		} catch (e) {
			setToast(`failed to load MCP servers: ${(e as Error).message}`, true);
			setServers([]);
		}
	}, [client, setToast]);

	useEffect(() => {
		void reload();
	}, [reload]);

	// track transport for dynamic field visibility in FormPanel
	const [formTransport, setFormTransport] = useState<string>("stdio");

	const close = () => setOverlay(null);

	// Top-level input handler — ensures empty / loading list states respond to keyboard.
	// Only fires in list mode; sub-forms handle their own input.
	useInput((input, key) => {
		if (modeRef.current.kind !== "list") return;
		if (servers !== null && servers.length > 0) return;
		if (key.escape) { close(); return; }
		if (input === "a") { setMode({ kind: "create" }); return; }
	});

	const handleCreate = async (values: Record<string, string>) => {
		if (!client) return;
		try {
			const dto: Record<string, unknown> = {
				name: values.name.trim(),
				transport: values.transport,
				enabled: values.enabled === "true",
			};
			if (values.transport === "stdio") {
				dto.command = values.command.trim();
				try { dto.args = JSON.parse(values.args || "[]"); } catch { dto.args = []; }
				try { dto.env = JSON.parse(values.env || "{}"); } catch { dto.env = {}; }
			} else {
				dto.url = values.url.trim();
				try { dto.headers = JSON.parse(values.headers || "{}"); } catch { dto.headers = {}; }
			}
			await client.createMcpServer(dto as any);
			setToast(`MCP server created: ${values.name.trim()}`);
			await reload();
		} catch (e) {
			setToast(`create failed: ${(e as Error).message}`, true);
		}
		setMode({ kind: "list" });
	};

	const handleEdit = async (target: MCPServerDto, values: Record<string, string>) => {
		if (!client) return;
		try {
			const dto: Record<string, unknown> = { name: values.name.trim() };
			if (values.transport === "stdio") {
				dto.transport = "stdio";
				dto.command = values.command.trim();
				try { dto.args = JSON.parse(values.args || "[]"); } catch { dto.args = []; }
				try { dto.env = JSON.parse(values.env || "{}"); } catch { dto.env = {}; }
			} else {
				dto.transport = values.transport;
				dto.url = values.url.trim();
				try { dto.headers = JSON.parse(values.headers || "{}"); } catch { dto.headers = {}; }
			}
			if (values.enabled !== "") {
				dto.enabled = values.enabled === "true";
			}
			await client.updateMcpServer(target.id, dto as any);
			setToast(`MCP server updated: ${values.name.trim()}`);
			await reload();
		} catch (e) {
			setToast(`update failed: ${(e as Error).message}`, true);
		}
		setMode({ kind: "list" });
	};

	const handleDelete = async (target: MCPServerDto) => {
		if (!client) return;
		try {
			await client.deleteMcpServer(target.id);
			setToast(`MCP server deleted: ${target.name}`);
			await reload();
		} catch (e) {
			setToast(`delete failed: ${(e as Error).message}`, true);
		}
		setMode({ kind: "list" });
	};

	const handleConnect = async (target: MCPServerDto) => {
		if (!client) return;
		try {
			await client.connectMcpServer(target.id);
			setToast(`Connected: ${target.name}`);
			await reload();
		} catch (e) {
			setToast(`connect failed: ${(e as Error).message}`, true);
		}
	};

	const handleDisconnect = async (target: MCPServerDto) => {
		if (!client) return;
		try {
			await client.disconnectMcpServer(target.id);
			setToast(`Disconnected: ${target.name}`);
			await reload();
		} catch (e) {
			setToast(`disconnect failed: ${(e as Error).message}`, true);
		}
	};

	if (mode.kind === "create") {
		return (
			<FormPanel
				title="Add MCP Server"
				fields={buildMcpFields(formTransport)}
				onChange={(key, allValues) => {
					if (key === "transport") setFormTransport(allValues.transport);
				}}
				onAction={(action, values) => {
					if (action === "save") handleCreate(values);
					else setMode({ kind: "list" });
				}}
				onClose={() => setMode({ kind: "list" })}
			/>
		);
	}

	if (mode.kind === "edit") {
		return (
			<FormPanel
				title={`Edit: ${mode.target.name}`}
				fields={buildMcpFields(formTransport, mode.target)}
				onChange={(key, allValues) => {
					if (key === "transport") setFormTransport(allValues.transport);
				}}
				onAction={(action, values) => {
					if (action === "save") void handleEdit(mode.target, values);
					else setMode({ kind: "list" });
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
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text color="magenta" bold>MCP Servers</Text>
			</Box>
			{servers === null ? (
				<Spinner label="Loading..." />
			) : servers.length === 0 ? (
				<Text dimColor>No MCP servers. Press `a` to add one.</Text>
			) : (
				<ServerList
					servers={servers}
					cursor={cursor}
					onCursor={setCursor}
					onEdit={(t) => setMode({ kind: "edit", target: t })}
					onAdd={() => setMode({ kind: "create" })}
					onDelete={(t) => setMode({ kind: "confirm-delete", target: t })}
					onConnect={(t) => void handleConnect(t)}
					onDisconnect={(t) => void handleDisconnect(t)}
					onClose={close}
				/>
			)}
		</Box>
	);
}

// ─── Server list table ─────────────────────────────────────────────

function ServerList({
	servers,
	cursor,
	onCursor,
	onEdit,
	onAdd,
	onDelete,
	onConnect,
	onDisconnect,
	onClose,
}: {
	servers: MCPServerDto[];
	cursor: number;
	onCursor: (i: number) => void;
	onEdit: (s: MCPServerDto) => void;
	onAdd: () => void;
	onDelete: (s: MCPServerDto) => void;
	onConnect: (s: MCPServerDto) => void;
	onDisconnect: (s: MCPServerDto) => void;
	onClose: () => void;
}) {
	const dialogSize = useBottomDialogSize();
	const n = servers.length;
	const transportWidth = 16;
	const statusWidth = 16;
	const compact = dialogSize.width < 58;
	const nameWidth = compact
		? Math.max(dialogSize.width - statusWidth - 4, 8)
		: Math.max(
				Math.min(dialogSize.width - transportWidth - statusWidth - 6, 32),
				12,
			);
	const maxVisible = Math.max(dialogSize.height - 6 - (compact ? 1 : 0), 1);
	const maxVis = Math.min(maxVisible, n);
	const half = Math.floor(maxVis / 2);
	let start = cursor - half;
	if (start < 0) start = 0;
	if (start + maxVis > n) start = Math.max(n - maxVis, 0);
	const visible = servers.slice(start, start + maxVis);

	useInput((input, key) => {
		if (key.escape) { onClose(); return; }
		if (key.upArrow) { onCursor(Math.max(cursor - 1, 0)); return; }
		if (key.downArrow) { onCursor(Math.min(cursor + 1, n - 1)); return; }
		const lower = input.toLowerCase();
		const s = servers[cursor];
		if (key.return) { onEdit(s); return; }
		if (lower === "a") onAdd();
		else if (lower === "e") onEdit(s);
		else if (lower === "d") onDelete(s);
		else if (lower === "c") onConnect(s);
		else if (lower === "x") onDisconnect(s);
	});

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text dimColor>
					{compact
						? `  ${pad("Name", nameWidth)}  ${pad("Status", statusWidth)}`
						: `  ${pad("Name", nameWidth)}  ${pad("Transport", transportWidth)}  ${pad("Status", statusWidth)}`}
				</Text>
			</Box>
			<Box flexDirection="column" flexGrow={1} overflow="hidden">
				{visible.map((s) => {
					const i = servers.indexOf(s);
					const selected = i === cursor;
					const statusText = s.status ?? (s.enabled ? "enabled" : "disabled");
					const isConnected = s.status === "connected";
					const statusColor = isConnected ? "green" : "gray";
					const name = pad(s.name, nameWidth);
					const transport = pad(s.transport, transportWidth);
					const st = pad(statusText, statusWidth);
					return (
						<Box
							key={s.id}
							onMouseOver={() => onCursor(i)}
							onMouseClick={() => {
								onCursor(i);
								onEdit(s);
							}}
						>
							<Text color={selected ? "cyan" : "gray"}>
								{selected ? "\u276f" : " "}
							</Text>
							<Text> </Text>
							<Text color={selected ? "cyan" : "white"} bold={selected}>
								{name}
							</Text>
							{compact ? null : <Text>  </Text>}
							{compact ? null : (
								<Text color={selected ? "cyan" : "gray"} dimColor={!selected}>
									{transport}
								</Text>
							)}
							<Text>  </Text>
							<Text color={statusColor} dimColor={!selected}>
								{st}
							</Text>
						</Box>
					);
				})}
			</Box>
			{compact ? (
				<Box height={1} overflow="hidden">
					<Text dimColor wrap="truncate">
						{trunc(
							`${servers[cursor]?.transport ?? "?"} · ${servers[cursor]?.status ?? "disabled"} · ${servers[cursor]?.tools?.length ?? 0} tools`,
							dialogSize.width,
						)}
					</Text>
				</Box>
			) : null}
			<Box gap={1} height={1} overflow="hidden">
				<Text color="cyan" onMouseClick={onAdd}>[Add]</Text>
				<Text color="cyan" onMouseClick={() => onConnect(servers[cursor])}>
					{compact ? "[On]" : "[Connect]"}
				</Text>
				<Text color="cyan" onMouseClick={() => onDisconnect(servers[cursor])}>
					{compact ? "[Off]" : "[Disconnect]"}
				</Text>
				<Text color="cyan" onMouseClick={() => onDelete(servers[cursor])}>
					{compact ? "[Del]" : "[Delete]"}
				</Text>
			</Box>
			<Box height={1} overflow="hidden">
				<Text dimColor wrap="truncate">
					{compact
						? "\u2191\u2193 move · Enter edit · c on · x off · Esc"
						: "\u2191\u2193 navigate · Enter edit · c connect · x disconnect · Esc back"}
				</Text>
			</Box>
		</Box>
	);
}

// ─── Delete confirm ─────────────────────────────────────────────────

function DeleteConfirm({
	target,
	onConfirm,
	onCancel,
}: {
	target: MCPServerDto;
	onConfirm: () => void;
	onCancel: () => void;
}) {
	useInput((input, key) => {
		if (key.escape) { onCancel(); return; }
		const lower = input.toLowerCase();
		if (lower === "y") onConfirm();
		else if (lower === "n") onCancel();
	});

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Text color="red" bold>
				Delete MCP server "{target.name}"?
			</Text>
			<Box marginTop={1} gap={2}>
				<Text color="red" bold onMouseClick={onConfirm}>[Delete]</Text>
				<Text color="cyan" onMouseClick={onCancel}>[Cancel]</Text>
			</Box>
		</Box>
	);
}
