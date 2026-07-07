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
import type { ModelProviderDto } from "../api/types";
import { useAppStore } from "../state/store";
import { CrudList, type CrudListItem } from "./CrudList";
import { FormOverlay, type FormField } from "./FormOverlay";
import { providerTypes, providerDefaultBaseURLs } from "../consts/providerTypes";

type Mode =
	| { kind: "list" }
	| { kind: "create" }
	| { kind: "edit"; target: ModelProviderDto }
	| { kind: "confirm-delete"; target: ModelProviderDto };

function toListItem(p: ModelProviderDto): CrudListItem {
	return {
		id: p.id,
		label: p.name,
		subtitle: `${p.type} · ${p.baseUrl}`,
		badge: p.isPreset ? "preset" : undefined,
	};
}

export function ProviderOverlay() {
	const client = useAppStore((s) => s.client);
	const notify = useAppStore((s) => s.notify);
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
			notify(`failed to load providers: ${(e as Error).message}`, true);
			setProviders([]);
		}
	}, [client, notify]);

	useEffect(() => {
		void reload();
	}, [reload]);

	const items = providers?.map(toListItem) ?? null;

	const buildCreateFields = (): FormField[] => [
		{ key: "name", label: "Name", kind: "text", required: true, placeholder: "my-provider" },
		{
			key: "type",
			label: "Type",
			kind: "select",
			options: [...providerTypes],
			defaultValue: providerTypes[0],
		},
		{
			key: "baseUrl",
			label: "Base URL",
			kind: "text",
			required: true,
			defaultValue: providerDefaultBaseURLs[providerTypes[0]],
		},
		{ key: "apiKey", label: "API Key", kind: "text", required: true, secret: true },
	];

	const buildEditFields = (target: ModelProviderDto): FormField[] => {
		const fields: FormField[] = [
			{ key: "name", label: "Name", kind: "text", required: true, defaultValue: target.name },
		];
		if (!target.isPreset) {
			fields.push({
				key: "type",
				label: "Type",
				kind: "select",
				options: [...providerTypes],
				defaultValue: target.type,
			});
		}
		fields.push({
			key: "baseUrl",
			label: "Base URL",
			kind: "text",
			required: true,
			defaultValue: target.baseUrl,
		});
		fields.push({
			key: "apiKey",
			label: "API Key",
			kind: "text",
			secret: true,
			placeholder: "leave blank to keep",
		});
		return fields;
	};

	const handleCreate = async (values: Record<string, string>) => {
		if (!client) return;
		try {
			await client.createProvider({
				name: values.name.trim(),
				type: values.type,
				baseUrl: values.baseUrl.trim(),
				apiKey: values.apiKey,
			});
			notify(`Provider created: ${values.name.trim()}`);
			await reload();
			setMode({ kind: "list" });
		} catch (e) {
			notify(`create failed: ${(e as Error).message}`, true);
			setMode({ kind: "list" });
		}
	};

	const handleEdit = async (
		target: ModelProviderDto,
		values: Record<string, string>,
	) => {
		if (!client) return;
		try {
			const dto: Record<string, string> = {
				name: values.name.trim(),
				baseUrl: values.baseUrl.trim(),
			};
			if (!target.isPreset) dto.type = values.type;
			if (values.apiKey && values.apiKey.trim() !== "") dto.apiKey = values.apiKey;
			await client.updateProvider(target.id, dto);
			notify(`Provider updated: ${values.name.trim()}`);
			await reload();
			setMode({ kind: "list" });
		} catch (e) {
			notify(`update failed: ${(e as Error).message}`, true);
			setMode({ kind: "list" });
		}
	};

	const handleDelete = async (target: ModelProviderDto) => {
		if (!client) return;
		try {
			await client.deleteProvider(target.id);
			notify(`Provider deleted: ${target.name}`);
			await reload();
			setMode({ kind: "list" });
		} catch (e) {
			notify(`delete failed: ${(e as Error).message}`, true);
			setMode({ kind: "list" });
		}
	};

	if (mode.kind === "create") {
		return (
			<FormOverlay
				title="Add Provider"
				fields={buildCreateFields()}
				submitLabel="Create"
				onSubmit={(v) => void handleCreate(v)}
				onClose={() => setMode({ kind: "list" })}
			/>
		);
	}

	if (mode.kind === "edit") {
		const target = mode.target;
		return (
			<FormOverlay
				title={`Edit Provider — ${target.name}`}
				fields={buildEditFields(target)}
				submitLabel="Save"
				onSubmit={(v) => void handleEdit(target, v)}
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
		<CrudList
			title="Providers"
			items={items}
			cursor={cursor}
			onCursor={setCursor}
			onSelect={(i) => {
				const t = providers?.[i];
				if (t) setMode({ kind: "edit", target: t });
			}}
			onAdd={() => setMode({ kind: "create" })}
			onEdit={(i) => {
				const t = providers?.[i];
				if (t) setMode({ kind: "edit", target: t });
			}}
			onDelete={(i) => {
				const t = providers?.[i];
				if (!t) return;
				if (t.isPreset) {
					notify(`cannot delete preset provider: ${t.name}`, true);
					return;
				}
				setMode({ kind: "confirm-delete", target: t });
			}}
			onClose={() => setOverlay(null)}
		/>
	);
}

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
		if (key.escape) {
			onCancel();
			return;
		}
		const lower = input.toLowerCase();
		if (lower === "y") onConfirm();
		else if (lower === "n") onCancel();
	});

	return (
		<Box flexDirection="column" paddingX={2} paddingY={1}>
			<Text color="red" bold>
				Delete provider "{target.name}"?
			</Text>
			<Text dimColor>
				This also deletes all its models. y to confirm · n/Esc to cancel
			</Text>
		</Box>
	);
}
