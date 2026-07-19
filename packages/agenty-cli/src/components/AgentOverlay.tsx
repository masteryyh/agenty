import { useCallback, useEffect, useRef, useState } from "react";
import { useInput } from "../hooks/useInput";
import { Box, Spinner, Text } from "./ui";
import type { AgentDto } from "../api/types";
import { useAppStore } from "../state/store";
import { FormPanel } from "./FormPanel";
import type { FormField, FormOption } from "./FormPanel";
import { useBottomDialogSize } from "./BottomDialog";

function trunc(s: string, width: number): string {
	if (width <= 0) return "";
	if (s.length <= width) return s;
	if (width === 1) return "…";
	return s.slice(0, width - 1) + "…";
}

function pad(s: string, width: number): string {
	const clipped = trunc(s, width);
	return clipped + " ".repeat(Math.max(width - clipped.length, 0));
}

function parseModelIds(raw: string): string[] {
	try {
		const arr = JSON.parse(raw);
		if (Array.isArray(arr)) {
			return arr.filter((x): x is string => typeof x === "string");
		}
	} catch {
		// fall through
	}
	return [];
}

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
	const [cursor, setCursor] = useState(0);
	const [mode, setMode] = useState<Mode>({ kind: "list" });
	const modeRef = useRef(mode);
	modeRef.current = mode;

	const reload = useCallback(async () => {
		if (!client) return;
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
		if (!client) return;
		try {
			const models = await client.listModels();
			const chatModels = models.filter((m) => !m.embeddingModel);
			setModelOptions(
				chatModels.map((m) => ({
					label: `${m.provider?.name ?? "?"}/${m.name}`,
					value: m.id,
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
		if (modeRef.current.kind !== "list") return;
		if (agents !== null && agents.length > 0) return;
		if (key.escape) { close(); return; }
		if (input === "a") { setMode({ kind: "create" }); return; }
	});

	const buildFields = (target?: AgentDto): FormField[] => {
		const modelIdsValue = target?.models
			? JSON.stringify(target.models.map((m) => m.id))
			: "[]";
		return [
			{ key: "name", label: "Name", kind: "text" as const, value: target?.name ?? "", placeholder: "my-agent" },
			{ key: "soul", label: "Soul", kind: "text" as const, value: target?.soul ?? "", placeholder: "system prompt, leave blank for default" },
			{ key: "isDefault", label: "Default", kind: "boolean" as const, value: target ? (target.isDefault ? "true" : "false") : "false" },
			{ key: "modelIds", label: "Models", kind: "multiselect" as const, value: modelIdsValue, options: modelOptions },
		];
	};

	const handleCreate = async (values: Record<string, string>) => {
		if (!client) return;
		try {
			await client.createAgent({
				name: values.name.trim(),
				soul: values.soul.trim(),
				isDefault: values.isDefault === "true",
				modelIds: parseModelIds(values.modelIds),
			});
			setToast(`Agent created: ${values.name.trim()}`);
			await reload();
		} catch (e) {
			setToast(`create failed: ${(e as Error).message}`, true);
		}
		setMode({ kind: "list" });
	};

	const handleEdit = async (target: AgentDto, values: Record<string, string>) => {
		if (!client) return;
		try {
			await client.updateAgent(target.id, {
				name: values.name.trim(),
				soul: values.soul.trim(),
				isDefault: values.isDefault === "true",
				modelIds: parseModelIds(values.modelIds),
			});
			setToast(`Agent updated: ${values.name.trim()}`);
			await reload();
		} catch (e) {
			setToast(`update failed: ${(e as Error).message}`, true);
		}
		setMode({ kind: "list" });
	};

	const handleDelete = async (target: AgentDto) => {
		if (!client) return;
		try {
			await client.deleteAgent(target.id);
			setToast(`Agent deleted: ${target.name}`);
			await reload();
		} catch (e) {
			setToast(`delete failed: ${(e as Error).message}`, true);
		}
		setMode({ kind: "list" });
	};

	const handleSwitch = async (target: AgentDto) => {
		if (!client) return;
		if (currentAgent?.id === target.id) {
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
					if (action === "save") void handleCreate(values);
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
				fields={buildFields(mode.target)}
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
				<Text color="magenta" bold>Agents</Text>
			</Box>
			{agents === null ? (
				<Spinner label="Loading..." />
			) : agents.length === 0 ? (
				<Text dimColor>No agents. Press `a` to add one.</Text>
			) : (
				<AgentList
					agents={agents}
					currentAgentId={currentAgent?.id}
					cursor={cursor}
					onCursor={setCursor}
					onSwitch={(a) => void handleSwitch(a)}
					onAdd={() => setMode({ kind: "create" })}
					onEdit={(a) => setMode({ kind: "edit", target: a })}
					onDelete={(a) => setMode({ kind: "confirm-delete", target: a })}
					onClose={close}
				/>
			)}
		</Box>
	);
}

// ─── Agent list table ───────────────────────────────────────────────

function AgentList({
	agents,
	currentAgentId,
	cursor,
	onCursor,
	onSwitch,
	onAdd,
	onEdit,
	onDelete,
	onClose,
}: {
	agents: AgentDto[];
	currentAgentId?: string;
	cursor: number;
	onCursor: (i: number) => void;
	onSwitch: (a: AgentDto) => void;
	onAdd: () => void;
	onEdit: (a: AgentDto) => void;
	onDelete: (a: AgentDto) => void;
	onClose: () => void;
}) {
	const dialogSize = useBottomDialogSize();
	const n = agents.length;
	const compact = dialogSize.width < 44;
	const flagsWidth = compact ? 0 : 24;
	const nameWidth = compact
		? Math.max(dialogSize.width - 2, 8)
		: Math.max(Math.min(dialogSize.width - flagsWidth - 4, 32), 12);
	const maxVisible = Math.max(dialogSize.height - 6 - (compact ? 1 : 0), 1);
	const maxVis = Math.min(maxVisible, n);
	const half = Math.floor(maxVis / 2);
	let start = cursor - half;
	if (start < 0) start = 0;
	if (start + maxVis > n) start = Math.max(n - maxVis, 0);
	const visible = agents.slice(start, start + maxVis);

	useInput((input, key) => {
		if (key.escape) { onClose(); return; }
		if (key.upArrow) { onCursor(Math.max(cursor - 1, 0)); return; }
		if (key.downArrow) { onCursor(Math.min(cursor + 1, n - 1)); return; }
		const lower = input.toLowerCase();
		const a = agents[cursor];
		if (key.return || lower === "s") { onSwitch(a); return; }
		if (lower === "a") onAdd();
		else if (lower === "e") onEdit(a);
		else if (lower === "d") onDelete(a);
	});

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text dimColor>
					{compact
						? `  ${pad("Name", nameWidth)}`
						: `  ${pad("Name", nameWidth)}  ${pad("Flags", flagsWidth)}`}
				</Text>
			</Box>
			<Box flexDirection="column" flexGrow={1} overflow="hidden">
				{visible.map((a) => {
					const i = agents.indexOf(a);
					const selected = i === cursor;
					const flags =
						`${a.isDefault ? "[default] " : ""}${a.id === currentAgentId ? "← current" : ""}`.trim();
					const name = pad(a.name, nameWidth);
					return (
						<Box
							key={a.id}
							onMouseOver={() => onCursor(i)}
							onMouseClick={() => {
								onCursor(i);
								onSwitch(a);
							}}
						>
							<Text color={selected ? "cyan" : "gray"}>
								{selected ? "❯" : " "}
							</Text>
							<Text> </Text>
							<Text color={selected ? "cyan" : "white"} bold={selected}>
								{name}
							</Text>
							{compact ? null : <Text>  </Text>}
							{compact ? null : (
								<Text color={selected ? "cyan" : "gray"} dimColor={!selected}>
									{pad(flags, flagsWidth)}
								</Text>
							)}
						</Box>
					);
				})}
			</Box>
			{compact ? (
				<Box height={1} overflow="hidden">
					<Text dimColor wrap="truncate">
						{trunc(
							`${agents[cursor]?.isDefault ? "[default] " : ""}${agents[cursor]?.id === currentAgentId ? "← current" : ""}`.trim() || "No flags",
							dialogSize.width,
						)}
					</Text>
				</Box>
			) : null}
			<Box gap={2} height={1} overflow="hidden">
				<Text color="cyan" onMouseClick={onAdd}>[Add]</Text>
				<Text color="cyan" onMouseClick={() => onEdit(agents[cursor])}>[Edit]</Text>
				<Text color="cyan" onMouseClick={() => onDelete(agents[cursor])}>[Delete]</Text>
			</Box>
			<Box height={1} overflow="hidden">
				<Text dimColor wrap="truncate">
					{compact
						? "↑↓ move · Enter switch · e edit · d del · Esc"
						: "↑↓ navigate · Enter/s switch · e edit · d delete · Esc back"}
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
	target: AgentDto;
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
				Delete agent &quot;{target.name}&quot;?
			</Text>
			<Text dimColor>This also deletes all its sessions, messages and memories.</Text>
			<Box marginTop={1} gap={2}>
				<Text color="red" bold onMouseClick={onConfirm}>[Delete]</Text>
				<Text color="cyan" onMouseClick={onCancel}>[Cancel]</Text>
			</Box>
		</Box>
	);
}
