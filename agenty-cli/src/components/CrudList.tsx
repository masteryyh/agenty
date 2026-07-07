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
		<Box flexDirection="column" paddingX={2} paddingY={1}>
			<Box marginBottom={1} gap={1}>
				<Text color="magenta" bold>
					{title}
				</Text>
				<Text dimColor>· {hints ?? "↑↓ navigate · Enter edit · a add · d delete · Esc back"}</Text>
			</Box>
			{items === null ? (
				<Spinner label="Loading..." />
			) : items.length === 0 ? (
				<Text dimColor>No providers yet. Press `a` to add one.</Text>
			) : (
				items.map((it, i) => (
					<Box key={it.id} gap={1}>
						<Text color={i === cursor ? "cyan" : "gray"}>
							{i === cursor ? "❯" : " "}
						</Text>
						<Text color="white" bold={i === cursor}>
							{it.label}
						</Text>
						{it.badge ? (
							<Text color="yellow" dimColor>
								[{it.badge}]
							</Text>
						) : null}
						{it.subtitle ? (
							<Text color="gray" dimColor>
								{it.subtitle}
							</Text>
						) : null}
					</Box>
				))
			)}
		</Box>
	);
}
