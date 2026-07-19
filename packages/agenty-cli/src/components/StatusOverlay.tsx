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

import { useInput } from "../hooks/useInput";
import { Box, Text } from "./ui";
import { useAppStore } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";

function pad(s: string, w: number): string {
	if (s.length >= w) return s;
	return s + " ".repeat(w - s.length);
}

function trunc(s: string, width: number): string {
	if (width <= 0) return "";
	if (s.length <= width) return s;
	if (width === 1) return "…";
	return `${s.slice(0, width - 1)}…`;
}

export function StatusOverlay() {
	const dialogSize = useBottomDialogSize();
	const agent = useAppStore((s) => s.agent);
	const model = useAppStore((s) => s.model);
	const session = useAppStore((s) => s.session);
	const thinkingEnabled = useAppStore((s) => s.thinkingEnabled);
	const thinkingLevel = useAppStore((s) => s.thinkingLevel);
	const history = useAppStore((s) => s.history);
	const tokenConsumed = useAppStore((s) => s.tokenConsumed);
	const setOverlay = useAppStore((s) => s.setOverlay);

	useInput((_input, key) => {
		if (key.escape) setOverlay(null);
	});

	const thinking = thinkingEnabled
		? `on${thinkingLevel ? ` (${thinkingLevel} effort)` : ""}`
		: "off";
	const rows: [string, string][] = [
		["Session", session?.id ?? "?"],
		["Agent", agent?.name ?? "?"],
		["Model", `${model?.provider?.name ?? "?"}/${model?.name ?? "?"}`],
		["Thinking", thinking],
		["Messages", String(history.length)],
		["Tokens", String(tokenConsumed)],
		["CWD", session?.cwd ?? process.cwd()],
	];
	const keyWidth = Math.min(10, Math.max(Math.floor(dialogSize.width / 4), 7));
	const valueWidth = Math.max(dialogSize.width - keyWidth - 1, 1);

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text color="magenta" bold>Status</Text>
			</Box>
			<Box flexDirection="column" flexGrow={1}>
				{rows.map(([k, v]) => (
					<Box key={k} gap={1}>
						<Text color="gray">{pad(k, keyWidth)}</Text>
						<Text color="white">{trunc(v, valueWidth)}</Text>
					</Box>
				))}
			</Box>
			<Box>
				<Text dimColor>Esc to close</Text>
			</Box>
		</Box>
	);
}
