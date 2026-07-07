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
import type { SystemSettingsDto, UpdateSystemSettingsDto } from "../api/types";
import { useAppStore } from "../state/store";

const SEARCH_PROVIDERS = [
	{ label: "disabled", value: "disabled" },
	{ label: "Tavily", value: "tavily" },
	{ label: "Brave", value: "brave" },
	{ label: "Firecrawl", value: "firecrawl" },
];

interface SettingItem {
	key: EditableSettingKey;
	label: string;
	value: string;
	secret?: boolean;
	select?: { label: string; value: string }[];
}

type EditableSettingKey =
	| "webSearchProvider"
	| "braveApiKey"
	| "tavilyApiKey"
	| "firecrawlApiKey"
	| "firecrawlBaseUrl";

const EDITABLE_KEYS: EditableSettingKey[] = [
	"webSearchProvider",
	"braveApiKey",
	"tavilyApiKey",
	"firecrawlApiKey",
	"firecrawlBaseUrl",
];

function settingsToList(s: SystemSettingsDto): SettingItem[] {
	const searchLabel =
		SEARCH_PROVIDERS.find((p) => p.value === s.webSearchProvider)?.label ??
		s.webSearchProvider;
	return [
		{ key: "webSearchProvider", label: "Web Search", value: searchLabel, select: SEARCH_PROVIDERS },
		{ key: "braveApiKey", label: "Brave API Key", value: s.braveApiKey ?? "", secret: true },
		{ key: "tavilyApiKey", label: "Tavily API Key", value: s.tavilyApiKey ?? "", secret: true },
		{ key: "firecrawlApiKey", label: "Firecrawl API Key", value: s.firecrawlApiKey ?? "", secret: true },
		{ key: "firecrawlBaseUrl", label: "Firecrawl Base URL", value: s.firecrawlBaseUrl ?? "" },
	];
}

export function ConfigOverlay() {
	const client = useAppStore((s) => s.client);
	const setToast = useAppStore((s) => s.setToast);
	const setOverlay = useAppStore((s) => s.setOverlay);

	const [settings, setSettings] = useState<SystemSettingsDto | null>(null);
	const [draft, setDraft] = useState<SystemSettingsDto | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [cursor, setCursor] = useState(0);
	const [editing, setEditing] = useState<EditableSettingKey | null>(null);
	const [editValue, setEditValue] = useState("");

	const items = draft ? settingsToList(draft) : [];
	const saveIndex = items.length;
	const cancelIndex = items.length + 1;

	const reload = useCallback(async () => {
		if (!client) return;
		try {
			const s = await client.getSettings();
			setSettings(s);
			setDraft(s);
			setError(null);
		} catch (e) {
			setError((e as Error).message);
		}
	}, [client]);

	useEffect(() => {
		void reload();
	}, [reload]);

	const updateDraft = (key: EditableSettingKey, value: string) => {
		setDraft((prev) => prev ? { ...prev, [key]: value } : prev);
	};

	const saveDraft = async () => {
		if (!client || !settings || !draft) return;
		const dto: UpdateSystemSettingsDto = {};
		for (const key of EDITABLE_KEYS) {
			if ((draft[key] ?? "") !== (settings[key] ?? "")) {
				dto[key] = draft[key] ?? "";
			}
		}
		try {
			if (Object.keys(dto).length > 0) {
				const updated = await client.updateSettings(dto);
				setSettings(updated);
				setDraft(updated);
				setToast("Settings saved.");
			} else {
				setToast("No settings changed.");
			}
			setOverlay(null);
		} catch (e) {
			setToast(`save failed: ${(e as Error).message}`, true);
		}
	};

	const cancelDraft = () => {
		setDraft(settings);
		setEditing(null);
		setOverlay(null);
	};

	useInput((_input, key) => {
		if (key.escape) {
			cancelDraft();
			return;
		}
		if (editing) return;
		if (key.leftArrow && cursor >= saveIndex) {
			setCursor(saveIndex);
			return;
		}
		if (key.rightArrow && cursor >= saveIndex) {
			setCursor(cancelIndex);
			return;
		}
		if (key.upArrow) {
			setCursor((c) => c >= saveIndex ? items.length - 1 : Math.max(c - 1, 0));
			return;
		}
		if (key.downArrow) {
			setCursor((c) => c >= saveIndex ? c : Math.min(c + 1, saveIndex));
			return;
		}
		if (key.return && cursor === saveIndex) {
			void saveDraft();
			return;
		}
		if (key.return && cursor === cancelIndex) {
			cancelDraft();
			return;
		}
		if (key.return && items[cursor]) {
			const item = items[cursor];
			if (item.select) return;
			setEditValue(item.value);
			setEditing(item.key);
			return;
		}
	});

	if (error) {
		return <Text color="red">Failed to load settings: {error}</Text>;
	}

	if (!settings) {
		return <Spinner label="Loading settings..." />;
	}

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text color="magenta" bold>Settings</Text>
			</Box>

			<Box flexDirection="column" flexGrow={1} overflow="hidden">
				{editing ? (
					<Box flexDirection="column">
						<Box marginBottom={1}>
							<Text color="cyan">
								Editing {items.find((i) => i.key === editing)?.label ?? editing}:
							</Text>
						</Box>
						<TextInput
							value={editValue}
							onChange={setEditValue}
							onSubmit={(v) => {
								updateDraft(editing, v);
								setEditing(null);
							}}
						/>
					</Box>
				) : (
					<>
						{items.map((item, i) => {
							const selected = i === cursor;
							const displayVal =
								item.secret && item.value
									? "•".repeat(Math.min(item.value.length, 12))
									: item.value || "—";
							return (
								<Box key={item.key} gap={1}>
									<Text color={selected ? "cyan" : "gray"}>
										{selected ? "❯" : " "}
									</Text>
									<Text color="white" bold={selected}>
										{item.label}:
									</Text>
									<Text color={selected ? "cyan" : "gray"} dimColor={!selected}>
										{displayVal}
									</Text>
									{item.key === "webSearchProvider" && selected ? (
										<Box marginLeft={1}>
											<Select
												options={SEARCH_PROVIDERS}
												defaultValue={draft?.webSearchProvider}
												onChange={(v) => updateDraft("webSearchProvider", v)}
											/>
										</Box>
									) : null}
								</Box>
							);
						})}
						<Box flexGrow={1} />
						<Box gap={2}>
							<Text color={cursor === saveIndex ? "cyan" : "gray"} bold={cursor === saveIndex}>
								{cursor === saveIndex ? "❯ " : "  "}Save
							</Text>
							<Text color={cursor === cancelIndex ? "cyan" : "gray"} bold={cursor === cancelIndex}>
								{cursor === cancelIndex ? "❯ " : "  "}Cancel
							</Text>
						</Box>
					</>
				)}
			</Box>
			<Box>
				<Text dimColor>↑↓ select · ←→ action · Enter edit/choose · Esc cancel</Text>
			</Box>
		</Box>
	);
}
