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

import { Box, Text, useInput } from "ink";
import { Spinner } from "@inkjs/ui";

export interface CrudListItem {
	id: string;
	label: string;
	subtitle?: string;
	badge?: string;
}

interface CrudListProps {
	title: string;
	items: CrudListItem[] | null;
	cursor: number;
	onCursor: (i: number) => void;
	onSelect: (i: number) => void;
	onAdd: () => void;
	onEdit: (i: number) => void;
	onDelete: (i: number) => void;
	onClose: () => void;
	hints?: string;
}

const LABEL_W = 24;
const SUBTITLE_W = 40;

function pad(s: string, w: number): string {
	if (s.length >= w) return s.slice(0, w - 1) + "…";
	return s + " ".repeat(w - s.length);
}

export function CrudList({
	title,
	items,
	cursor,
	onCursor,
	onSelect,
	onAdd,
	onEdit,
	onDelete,
	onClose,
	hints,
}: CrudListProps) {
	useInput((input, key) => {
		if (key.escape) {
			onClose();
			return;
		}
		if (!items || items.length === 0) {
			if (input === "a") onAdd();
			return;
		}
		if (key.upArrow) {
			onCursor(Math.max(cursor - 1, 0));
			return;
		}
		if (key.downArrow) {
			onCursor(Math.min(cursor + 1, items.length - 1));
			return;
		}
		if (key.return) {
			onSelect(cursor);
			return;
		}
		const lower = input.toLowerCase();
		if (lower === "a") onAdd();
		else if (lower === "e") onEdit(cursor);
		else if (lower === "d") onDelete(cursor);
	});

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text color="magenta" bold>{title}</Text>
			</Box>
			{items === null ? (
				<Spinner label="Loading..." />
			) : items.length === 0 ? (
				<Text dimColor>No providers yet. Press `a` to add one.</Text>
			) : (
				<Box flexDirection="column">
					{/* header row */}
					<Box>
						<Text color="gray" dimColor>
							{"  "}{pad("Name", LABEL_W)}  {pad("Info", SUBTITLE_W)}
						</Text>
					</Box>
					{items.map((it, i) => {
						const selected = i === cursor;
						const label = pad(it.label, LABEL_W);
						const info = pad(it.subtitle ?? "", SUBTITLE_W);
						const badge = it.badge ? ` [${it.badge}]` : "";
						return (
							<Box key={it.id}>
								<Text color={selected ? "cyan" : "gray"}>
									{selected ? "❯" : " "}
								</Text>
								<Text> </Text>
								<Text
									color={selected ? "cyan" : "white"}
									bold={selected}
								>
									{label}
								</Text>
								<Text>  </Text>
								<Text
									color={selected ? "cyan" : "gray"}
									dimColor={!selected}
								>
									{info}
								</Text>
								{badge ? (
									<Text color="yellow" dimColor>{badge}</Text>
								) : null}
							</Box>
						);
					})}
				</Box>
			)}
			{hints ? (
				<Box marginTop={1}>
					<Text dimColor>{hints}</Text>
				</Box>
			) : null}
		</Box>
	);
}
