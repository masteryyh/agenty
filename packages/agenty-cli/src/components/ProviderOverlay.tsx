import { useCallback, useEffect, useRef, useState } from "react";
import { useInput } from "../hooks/useInput";
import { Box, Spinner, Text } from "./ui";
import type { ModelProviderDto } from "../api/types";
import { useAppStore } from "../state/store";
import { providerTypes, providerDefaultBaseURLs } from "../consts/providerTypes";
import { FormPanel } from "./FormPanel";
import type { FormField } from "./FormPanel";
import { useBottomDialogSize } from "./BottomDialog";

const PROVIDER_TYPE_OPTIONS = providerTypes.map((t) => ({ label: t, value: t }));

function buildCreateFields(formType: string): FormField[] {
	const baseUrl = providerDefaultBaseURLs[formType] ?? "";
	return [
		{ key: "name", label: "Name", kind: "text" as const, value: "", placeholder: "my-provider" },
		{ key: "type", label: "Type", kind: "select" as const, value: formType, options: PROVIDER_TYPE_OPTIONS },
		{ key: "baseUrl", label: "Base URL", kind: "text" as const, value: baseUrl },
		{ key: "apiKey", label: "API Key", kind: "text" as const, value: "", placeholder: "sk-...", secret: true },
	];
}

function buildEditFields(target: ModelProviderDto): FormField[] {
	if (target.isPreset) {
		return [
			{ key: "name", label: "Name", kind: "text" as const, value: target.name, readOnly: true },
			{ key: "type", label: "Type", kind: "text" as const, value: target.type, readOnly: true },
			{ key: "baseUrl", label: "Base URL", kind: "text" as const, value: target.baseUrl, readOnly: true },
			{ key: "apiKey", label: "API Key", kind: "text" as const, value: "", placeholder: "leave blank to keep", secret: true },
		];
	}
	return [
		{ key: "name", label: "Name", kind: "text" as const, value: target.name, placeholder: target.name },
		{ key: "type", label: "Type", kind: "select" as const, value: target.type, options: PROVIDER_TYPE_OPTIONS },
		{ key: "baseUrl", label: "Base URL", kind: "text" as const, value: target.baseUrl },
		{ key: "apiKey", label: "API Key", kind: "text" as const, value: "", placeholder: "leave blank to keep", secret: true },
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
	| { kind: "edit"; target: ModelProviderDto }
	| { kind: "confirm-delete"; target: ModelProviderDto };

export function ProviderOverlay() {
	const client = useAppStore((s) => s.client);
	const setToast = useAppStore((s) => s.setToast);
	const setOverlay = useAppStore((s) => s.setOverlay);

	const [providers, setProviders] = useState<ModelProviderDto[] | null>(null);
	const [cursor, setCursor] = useState(0);
	const [mode, setMode] = useState<Mode>({ kind: "list" });
	const modeRef = useRef(mode);
	modeRef.current = mode;

	const reload = useCallback(async () => {
		if (!client) return;
		try {
			const list = await client.listProviders();
			setProviders(list);
			setCursor((c) => Math.min(c, Math.max(list.length - 1, 0)));
		} catch (e) {
			setToast(`failed to load providers: ${(e as Error).message}`, true);
			setProviders([]);
		}
	}, [client, setToast]);

	useEffect(() => {
		void reload();
	}, [reload]);

	// track provider type for auto baseUrl
	const [formType, setFormType] = useState<string>(providerTypes[0]);

	const close = () => setOverlay(null);

	// Top-level input handler — ensures empty / loading list states respond to keyboard.
	useInput((input, key) => {
		if (modeRef.current.kind !== "list") return;
		if (providers !== null && providers.length > 0) return;
		if (key.escape) { close(); return; }
		if (input === "a") { setMode({ kind: "create" }); return; }
	});

	const handleCreate = async (values: Record<string, string>) => {
		if (!client) return;
		try {
			await client.createProvider({
				name: values.name.trim(),
				type: values.type,
				baseUrl: values.baseUrl.trim(),
				apiKey: values.apiKey,
			});
			setToast(`Provider created: ${values.name.trim()}`);
			await reload();
		} catch (e) {
			setToast(`create failed: ${(e as Error).message}`, true);
		}
		setMode({ kind: "list" });
	};

	const handleEdit = async (target: ModelProviderDto, values: Record<string, string>) => {
		if (!client) return;
		try {
			const dto: Record<string, string> = {};
			if (!target.isPreset) {
				dto.name = values.name.trim();
				dto.type = values.type;
				dto.baseUrl = values.baseUrl.trim();
			}
			if (values.apiKey && values.apiKey.trim() !== "") dto.apiKey = values.apiKey;
			await client.updateProvider(target.id, dto);
			setToast(`Provider updated: ${values.name.trim()}`);
			await reload();
		} catch (e) {
			setToast(`update failed: ${(e as Error).message}`, true);
		}
		setMode({ kind: "list" });
	};

	const handleDelete = async (target: ModelProviderDto) => {
		if (!client) return;
		try {
			await client.deleteProvider(target.id);
			setToast(`Provider deleted: ${target.name}`);
			await reload();
		} catch (e) {
			setToast(`delete failed: ${(e as Error).message}`, true);
		}
		setMode({ kind: "list" });
	};

	if (mode.kind === "create") {
		return (
			<FormPanel
				title="Add Provider"
				fields={buildCreateFields(formType)}
				onChange={(key, allValues) => {
					if (key === "type") setFormType(allValues.type);
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
				fields={buildEditFields(mode.target)}
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
				<Text color="magenta" bold>Providers</Text>
			</Box>
			{providers === null ? (
				<Spinner label="Loading..." />
			) : providers.length === 0 ? (
				<Text dimColor>No providers. Press `a` to add one.</Text>
			) : (
				<ProviderList
					providers={providers}
					cursor={cursor}
					onCursor={setCursor}
					onSelect={(t) => setMode({ kind: "edit", target: t })}
					onAdd={() => setMode({ kind: "create" })}
					onDelete={(t) => {
						if (t.isPreset) {
							setToast(`cannot delete preset provider: ${t.name}`, true);
							return;
						}
						setMode({ kind: "confirm-delete", target: t });
					}}
					onClose={close}
				/>
			)}
		</Box>
	);
}

// ─── Provider list table ───────────────────────────────────────────

function ProviderList({
	providers,
	cursor,
	onCursor,
	onSelect,
	onAdd,
	onDelete,
	onClose,
}: {
	providers: ModelProviderDto[];
	cursor: number;
	onCursor: (i: number) => void;
	onSelect: (p: ModelProviderDto) => void;
	onAdd: () => void;
	onDelete: (p: ModelProviderDto) => void;
	onClose: () => void;
}) {
	const dialogSize = useBottomDialogSize();
	const n = providers.length;
	const compact = dialogSize.width < 46;
	const nameWidth = compact
		? Math.max(dialogSize.width - 2, 8)
		: Math.max(Math.min(Math.floor(dialogSize.width * 0.28), 30), 14);
	const urlWidth = compact
		? 0
		: Math.max(dialogSize.width - nameWidth - 4, 8);
	const maxVisible = Math.max(dialogSize.height - 7 - (compact ? 1 : 0), 1);
	const maxVis = Math.min(maxVisible, n);
	const half = Math.floor(maxVis / 2);
	let start = cursor - half;
	if (start < 0) start = 0;
	if (start + maxVis > n) start = Math.max(n - maxVis, 0);
	const visible = providers.slice(start, start + maxVis);

	useInput((input, key) => {
		if (key.escape) { onClose(); return; }
		if (key.upArrow) { onCursor(Math.max(cursor - 1, 0)); return; }
		if (key.downArrow) { onCursor(Math.min(cursor + 1, n - 1)); return; }
		if (key.return) { onSelect(providers[cursor]); return; }
		const lower = input.toLowerCase();
		if (lower === "a") onAdd();
		else if (lower === "d") onDelete(providers[cursor]);
	});

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text dimColor>
					{compact
						? `  ${pad("Name", nameWidth)}`
						: `  ${pad("Name", nameWidth)}  ${pad("Base URL", urlWidth)}`}
				</Text>
			</Box>
			<Box flexDirection="column" flexGrow={1} overflow="hidden">
				{visible.map((p) => {
					const i = providers.indexOf(p);
					const selected = i === cursor;
					const customBadge = p.isPreset ? "" : " [custom]";
					const nameW = Math.max(nameWidth - customBadge.length, 1);
					const name = pad(p.name, nameW);
					const url = pad(p.baseUrl, urlWidth);
					return (
						<Box
							key={p.id}
							onMouseOver={() => onCursor(i)}
							onMouseClick={() => {
								onCursor(i);
								onSelect(p);
							}}
						>
							<Text color={selected ? "cyan" : "gray"}>
								{selected ? "\u276f" : " "}
							</Text>
							<Text> </Text>
							<Text color={selected ? "cyan" : "white"} bold={selected}>
								{name}
							</Text>
							{customBadge ? <Text dimColor>{customBadge}</Text> : null}
							{compact ? null : <Text>  </Text>}
							{compact ? null : (
								<Text color={selected ? "cyan" : "gray"} dimColor={!selected}>
									{url}
								</Text>
							)}
						</Box>
					);
				})}
			</Box>
			{compact ? (
				<Box height={1} overflow="hidden">
					<Text dimColor wrap="truncate">
						{trunc(`Base URL: ${providers[cursor]?.baseUrl ?? "—"}`, dialogSize.width)}
					</Text>
				</Box>
			) : null}
			<Box gap={3} height={1} marginTop={1} overflow="hidden">
				<Text color="cyan" onMouseClick={onAdd}>Add</Text>
				<Text color="cyan" onMouseClick={() => onDelete(providers[cursor])}>Delete</Text>
			</Box>
			<Box height={1} overflow="hidden">
				<Text dimColor wrap="truncate">
					{compact
						? "\u2191\u2193 move · Enter edit · d del · Esc close"
						: "\u2191\u2193 navigate · Enter edit · d delete · Esc back"}
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
	target: ModelProviderDto;
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
				Delete provider "{target.name}"?
			</Text>
			<Text dimColor>This also deletes all its models.</Text>
			<Box marginTop={1} gap={2}>
				<Text color="red" bold onMouseClick={onConfirm}>[Delete]</Text>
				<Text color="cyan" onMouseClick={onCancel}>[Cancel]</Text>
			</Box>
		</Box>
	);
}
