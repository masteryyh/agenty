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
import { Text } from "ink";
import { Spinner } from "@inkjs/ui";
import type { SystemConfigDto, UpdateSystemConfigDto } from "../api/types";
import { useAppStore } from "../state/store";
import { FormPanel } from "./FormPanel";
import type { FormField } from "./FormPanel";

const SEARCH_PROVIDER_OPTIONS = [
	{ label: "disabled", value: "disabled" },
	{ label: "Tavily", value: "tavily" },
	{ label: "Brave", value: "brave" },
	{ label: "Firecrawl", value: "firecrawl" },
];

const EDITABLE_KEYS = [
	"webSearchProvider",
	"braveApiKey",
	"tavilyApiKey",
	"firecrawlApiKey",
	"firecrawlBaseUrl",
] as const;

function buildConfigFields(s: SystemConfigDto): FormField[] {
	return [
		{ key: "webSearchProvider", label: "Web Search", kind: "select", value: s.webSearchProvider ?? "disabled", options: SEARCH_PROVIDER_OPTIONS },
		{ key: "braveApiKey", label: "Brave API Key", kind: "text", value: s.braveApiKey ?? "", secret: true },
		{ key: "tavilyApiKey", label: "Tavily API Key", kind: "text", value: s.tavilyApiKey ?? "", secret: true },
		{ key: "firecrawlApiKey", label: "Firecrawl API Key", kind: "text", value: s.firecrawlApiKey ?? "", secret: true },
		{ key: "firecrawlBaseUrl", label: "Firecrawl Base URL", kind: "text", value: s.firecrawlBaseUrl ?? "" },
	];
}

export function ConfigOverlay() {
	const client = useAppStore((s) => s.client);
	const setToast = useAppStore((s) => s.setToast);
	const setOverlay = useAppStore((s) => s.setOverlay);

	const [settings, setSettings] = useState<SystemConfigDto | null>(null);
	const [error, setError] = useState<string | null>(null);

	const reload = useCallback(async () => {
		if (!client) return;
		try {
			const s = await client.getConfig();
			setSettings(s);
			setError(null);
		} catch (e) {
			setError((e as Error).message);
		}
	}, [client]);

	useEffect(() => {
		void reload();
	}, [reload]);

	const handleAction = async (action: string, values: Record<string, string>) => {
		if (action !== "save") {
			setOverlay(null);
			return;
		}
		if (!client || !settings) return;

		const dto: UpdateSystemConfigDto = {};
		for (const key of EDITABLE_KEYS) {
			if ((values[key] ?? "") !== (settings[key] ?? "")) {
				(dto as Record<string, string>)[key] = values[key] ?? "";
			}
		}

		try {
			if (Object.keys(dto).length > 0) {
				const updated = await client.updateConfig(dto);
				setSettings(updated);
				setToast("Settings saved.");
			} else {
				setToast("No settings changed.");
			}
			setOverlay(null);
		} catch (e) {
			setToast(`save failed: ${(e as Error).message}`, true);
		}
	};

	if (error) {
		return <Text color="red">Failed to load settings: {error}</Text>;
	}

	if (!settings) {
		return <Spinner label="Loading settings..." />;
	}

	return (
		<FormPanel
			title="Settings"
			fields={buildConfigFields(settings)}
			onAction={handleAction}
			onClose={() => setOverlay(null)}
		/>
	);
}
