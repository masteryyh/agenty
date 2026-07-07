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

import { Box, Text, useWindowSize } from "ink";
import type { Palette } from "../hooks/useCommandPalette";

const MAX_ITEMS = 8;
const HIGHLIGHT = "#4FA8FF";

interface CommandPaletteProps {
	palette: Palette;
	marginTop: number;
}

export function CommandPalette({ palette, marginTop }: CommandPaletteProps) {
	const { columns } = useWindowSize();
	if (palette.mode === "none") return null;

	const width = Math.max(columns, 1);
	const padSpaces = (contentLen: number): string => {
		const w = width - contentLen;
		return w > 0 ? " ".repeat(w) : "";
	};

	if (palette.mode === "commands") {
		const matchPrefix = palette.matchPrefix;
		const isExactSlash = matchPrefix === "/";
		const items = palette.matches.slice(0, MAX_ITEMS);
		return (
			<Box flexDirection="column" marginTop={marginTop}>
				{items.map((c, i) => {
					const isFull = c.name === matchPrefix;
					const selected = i === palette.highlight;
					const cursor = selected ? "❯ " : "  ";
					if (isExactSlash) {
						const contentLen =
							cursor.length + c.name.length + 3 + c.description.length;
						return (
							<Text key={c.name}>
								<Text color={selected ? HIGHLIGHT : undefined}>
									{cursor}
								</Text>
								<Text color={selected ? HIGHLIGHT : undefined}>
									{c.name}
								</Text>
								<Text color="gray">
									{" — "}
									{c.description}
								</Text>
								<Text>{padSpaces(contentLen)}</Text>
							</Text>
						);
					}
					const matchedPart = c.name.slice(0, matchPrefix.length);
					const unmatchedPart = c.name.slice(matchPrefix.length);
					const contentLen =
						cursor.length + c.name.length + 3 + c.description.length;
					return (
						<Text key={c.name}>
							<Text color={selected ? HIGHLIGHT : undefined}>
								{cursor}
							</Text>
							<Text
								color={HIGHLIGHT}
								bold={isFull}
							>
								{matchedPart}
							</Text>
							{unmatchedPart ? (
								<Text color={selected ? HIGHLIGHT : undefined}>
									{unmatchedPart}
								</Text>
							) : null}
							<Text color="gray">
								{" — "}
								{c.description}
							</Text>
							<Text>{padSpaces(contentLen)}</Text>
						</Text>
					);
				})}
				<Text dimColor>
					{"  Tab to select · Enter to run"}
					{padSpaces(2 + "Tab to select · Enter to run".length)}
				</Text>
			</Box>
		);
	}

	const { command, candidates, loading, highlight } = palette;
	const windowStart = Math.floor(highlight / MAX_ITEMS) * MAX_ITEMS;
	const items = candidates
		? candidates.slice(windowStart, windowStart + MAX_ITEMS)
		: [];
	const headerRest = ` ${command.argHint ?? ""} · Tab to cycle`;

	return (
		<Box flexDirection="column" marginTop={marginTop}>
			<Text>
				{" "}
				<Text color={HIGHLIGHT} bold>
					{command.name}
				</Text>
				<Text dimColor>{headerRest}</Text>
				<Text>{padSpaces(1 + command.name.length + headerRest.length)}</Text>
			</Text>
			{loading && !candidates ? (
				<Text dimColor>
					{"   loading candidates…"}
					{padSpaces(3 + "loading candidates…".length)}
				</Text>
			) : items.length === 0 ? (
				<Text dimColor>
					{"   no candidates"}
					{padSpaces(3 + "no candidates".length)}
				</Text>
			) : (
				items.map((c, i) => {
					const absIdx = windowStart + i;
					const selected = absIdx === highlight;
					const prefix = ` ${selected ? "❯" : " "} `;
					const contentLen = prefix.length + c.length;
					return (
						<Text
							key={`${absIdx}-${c}`}
							color={selected ? HIGHLIGHT : "white"}
							dimColor={!selected}
							bold={selected}
						>
							{prefix}
							{c}
							{padSpaces(contentLen)}
						</Text>
					);
				})
			)}
		</Box>
	);
}
