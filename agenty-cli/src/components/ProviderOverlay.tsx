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

import { useCallback, useEffect, useState } from "react";
import { Box, Text, useInput } from "ink";
import TextInput from "ink-text-input";
import { Select, Spinner } from "@inkjs/ui";
import type { ModelProviderDto } from "../api/types";
import { useAppStore } from "../state/store";
import { providerTypes, providerDefaultBaseURLs } from "../consts/providerTypes";

const MAX_VISIBLE = 9;
const NAME_W = 34;
const URL_W = 62;

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

	const close = () => setOverlay(null);

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
		return <CreateForm onSave={handleCreate} onCancel={() => setMode({ kind: "list" })} />;
	}

	if (mode.kind === "edit") {
		return (
			<EditForm
				target={mode.target}
				onSave={(v) => void handleEdit(mode.target, v)}
				onCancel={() => setMode({ kind: "list" })}
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

	// --- list view ---
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
	const n = providers.length;
	const maxVis = Math.min(MAX_VISIBLE, n);
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
					{"  "}{pad("Name", NAME_W)}  {pad("Base URL", URL_W)}  Status
				</Text>
			</Box>
			<Box flexDirection="column" flexGrow={1} overflow="hidden">
				{visible.map((p) => {
					const i = providers.indexOf(p);
					const selected = i === cursor;
					const configured = !!p.apiKeyCensored;
					const status = configured ? "[configured]" : p.isPreset ? "" : "[not configured]";
					const statusColor = configured ? "green" : "gray";
					const customBadge = p.isPreset ? "" : " [custom]";
					const nameW = NAME_W - customBadge.length;
					const name = pad(p.name, nameW);
					const url = pad(p.baseUrl, URL_W);
					return (
						<Box key={p.id}>
							<Text color={selected ? "cyan" : "gray"}>
								{selected ? "❯" : " "}
							</Text>
							<Text> </Text>
							<Text color={selected ? "cyan" : "white"} bold={selected}>
								{name}
							</Text>
							{customBadge ? <Text dimColor>{customBadge}</Text> : null}
							<Text>  </Text>
							<Text color={selected ? "cyan" : "gray"} dimColor={!selected}>
								{url}
							</Text>
							<Text>  </Text>
							{status ? (
								<Text color={statusColor} dimColor={!selected}>
									{status}
								</Text>
							) : null}
						</Box>
					);
				})}
			</Box>
			<Box>
				<Text dimColor>↑↓ navigate · Enter edit · a add · d delete · Esc back</Text>
			</Box>
		</Box>
	);
}

// ─── Create form ────────────────────────────────────────────────────

function CreateForm({
	onSave,
	onCancel,
}: {
	onSave: (values: Record<string, string>) => void;
	onCancel: () => void;
}) {
	const [name, setName] = useState("");
	const [type, setType] = useState<string>(providerTypes[0]);
	const [baseUrl, setBaseUrl] = useState(providerDefaultBaseURLs[providerTypes[0]]);
	const [apiKey, setApiKey] = useState("");
	const [focus, setFocus] = useState(0);
	const fields = ["name", "type", "baseUrl", "apiKey"] as const;
	const actionOffset = fields.length;

	const save = () => onSave({ name, type, baseUrl, apiKey });
	const cancel = () => onCancel();

	useInput((_input, key) => {
		if (key.escape) {
			cancel();
			return;
		}
		if (key.leftArrow && focus >= actionOffset) { setFocus(actionOffset); return; }
		if (key.rightArrow && focus >= actionOffset) { setFocus(actionOffset + 1); return; }
		if (key.upArrow) {
			setFocus((f) => f >= actionOffset ? fields.length - 1 : Math.max(f - 1, 0));
			return;
		}
		if (key.downArrow) {
			setFocus((f) => f >= actionOffset ? f : Math.min(f + 1, actionOffset));
			return;
		}
		if (key.return && focus >= actionOffset) {
			if (focus === actionOffset) save();
			else cancel();
		}
	});

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text color="magenta" bold>Add Provider</Text>
			</Box>
				<FieldRow label="Name" focus={focus === 0}>
					{focus === 0 ? (
						<TextInput
							value={name}
							onChange={setName}
							onSubmit={() => setFocus(1)}
							placeholder="my-provider"
						/>
					) : (
						<Text>{name || <Text dimColor>—</Text>}</Text>
					)}
				</FieldRow>
				<FieldRow label="Type" focus={focus === 1}>
					{focus === 1 ? (
						<Select
							options={providerTypes.map((o) => ({ label: o, value: o }))}
							defaultValue={type}
							onChange={(v) => { setType(v); setBaseUrl(providerDefaultBaseURLs[v] ?? ""); }}
						/>
					) : (
						<Text>{type}</Text>
					)}
				</FieldRow>
				<FieldRow label="Base URL" focus={focus === 2}>
					{focus === 2 ? (
						<TextInput value={baseUrl} onChange={setBaseUrl} onSubmit={() => setFocus(3)} />
					) : (
						<Text>{baseUrl || <Text dimColor>—</Text>}</Text>
					)}
				</FieldRow>
				<FieldRow label="API Key" focus={focus === 3}>
					{focus === 3 ? (
						<TextInput value={apiKey} onChange={setApiKey} onSubmit={() => setFocus(4)} placeholder="sk-..." />
					) : (
						<Text>{apiKey ? "•".repeat(Math.min(apiKey.length, 16)) : <Text dimColor>—</Text>}</Text>
					)}
				</FieldRow>
				<ActionRow
					saveFocus={focus === actionOffset}
					cancelFocus={focus === actionOffset + 1}
				/>
				<Box flexGrow={1} />
				<Box>
					<Text dimColor>↑↓ navigate · ←→ action · Enter choose · Esc cancel</Text>
				</Box>
			</Box>
		);
	}

	// ─── Edit form ──────────────────────────────────────────────────────

function EditForm({
	target,
	onSave,
	onCancel,
}: {
	target: ModelProviderDto;
	onSave: (values: Record<string, string>) => void;
	onCancel: () => void;
}) {
	const [name, setName] = useState(target.name);
	const [type, setType] = useState(target.type);
	const [baseUrl, setBaseUrl] = useState(target.baseUrl);
	const [apiKey, setApiKey] = useState("");
	const [focus, setFocus] = useState(0);

	const editableFields = target.isPreset
		? (["apiKey"] as const)
		: (["name", "type", "baseUrl", "apiKey"] as const);
	const actionOffset = editableFields.length;

	const save = () => onSave({ name, type, baseUrl, apiKey });
	const cancel = () => onCancel();

	useInput((_input, key) => {
		if (key.escape) {
			cancel();
			return;
		}
		if (key.leftArrow && focus >= actionOffset) { setFocus(actionOffset); return; }
		if (key.rightArrow && focus >= actionOffset) { setFocus(actionOffset + 1); return; }
		if (key.upArrow) {
			setFocus((f) => f >= actionOffset ? editableFields.length - 1 : Math.max(f - 1, 0));
			return;
		}
		if (key.downArrow) {
			setFocus((f) => f >= actionOffset ? f : Math.min(f + 1, actionOffset));
			return;
		}
		if (key.return && focus >= actionOffset) {
			if (focus === actionOffset) save();
			else cancel();
		}
	});

	const isPreset = target.isPreset;

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text color="magenta" bold>Edit: {target.name}</Text>
			</Box>

				{isPreset ? (
					<>
						<FieldRow label="Name" focus={false}>
							<Text dimColor>{target.name}</Text>
						</FieldRow>
						<FieldRow label="Type" focus={false}>
							<Text dimColor>{target.type}</Text>
						</FieldRow>
						<FieldRow label="Base URL" focus={false}>
							<Text dimColor>{target.baseUrl}</Text>
						</FieldRow>
					</>
				) : (
						<>
							<FieldRow label="Name" focus={focus === 0}>
								{focus === 0 ? (
									<TextInput value={name} onChange={setName} onSubmit={() => setFocus(1)} />
								) : (
									<Text>{name || <Text dimColor>—</Text>}</Text>
								)}
							</FieldRow>
							<FieldRow label="Type" focus={focus === 1}>
								{focus === 1 ? (
									<Select
										options={providerTypes.map((o) => ({ label: o, value: o }))}
										defaultValue={type}
										onChange={setType}
									/>
								) : (
									<Text>{type}</Text>
								)}
							</FieldRow>
							<FieldRow label="Base URL" focus={focus === 2}>
								{focus === 2 ? (
									<TextInput value={baseUrl} onChange={setBaseUrl} onSubmit={() => setFocus(3)} />
								) : (
									<Text>{baseUrl || <Text dimColor>—</Text>}</Text>
								)}
							</FieldRow>
						</>
					)}

				<FieldRow label="API Key" focus={isPreset ? focus === 0 : focus === 3}>
					{(isPreset ? focus === 0 : focus === 3) ? (
						<TextInput
							value={apiKey}
							onChange={setApiKey}
							onSubmit={() => setFocus(actionOffset)}
							placeholder="leave blank to keep"
						/>
					) : (
						<Text>{apiKey ? "•".repeat(Math.min(apiKey.length, 16)) : <Text dimColor>leave blank to keep</Text>}</Text>
					)}
				</FieldRow>

				<ActionRow
					saveFocus={focus === actionOffset}
					cancelFocus={focus === actionOffset + 1}
				/>
				<Box flexGrow={1} />
				<Box>
					<Text dimColor>↑↓ navigate · ←→ action · Enter choose · Esc cancel</Text>
				</Box>
			</Box>
		);
	}

	// ─── Field row ──────────────────────────────────────────────────────

function FieldRow({
	label,
	focus,
	children,
}: {
	label: string;
	focus: boolean;
	children: React.ReactNode;
}) {
	return (
		<Box>
			<Text color={focus ? "cyan" : "gray"}>
				{focus ? "❯" : " "}
			</Text>
			<Text> </Text>
			<Box width={12}>
				<Text color={focus ? "cyan" : "gray"} bold={focus}>{label}:</Text>
			</Box>
			<Box flexGrow={1}>
				{children}
			</Box>
		</Box>
	);
}

function ActionRow({
	saveFocus,
	cancelFocus,
}: {
	saveFocus: boolean;
	cancelFocus: boolean;
}) {
	return (
		<Box marginTop={1} gap={2}>
			<Text color={saveFocus ? "cyan" : "gray"} bold={saveFocus}>
				{saveFocus ? "❯ " : "  "}Save
			</Text>
			<Text color={cancelFocus ? "cyan" : "gray"} bold={cancelFocus}>
				{cancelFocus ? "❯ " : "  "}Cancel
			</Text>
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
			<Box marginTop={1}>
				<Text dimColor>y to confirm · n or Esc to cancel</Text>
			</Box>
		</Box>
	);
}
