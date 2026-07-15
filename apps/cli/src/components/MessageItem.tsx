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

import { memo } from "react";
import type React from "react";
import { Box, Text } from "ink";
import type { ToolResult } from "../api/types";
import type { UIToolCall } from "../state/store";

const GUTTER = "▏";
const RAIL_BORDER = {
	topLeft: "",
	topRight: "",
	bottomLeft: "",
	bottomRight: "",
	left: GUTTER,
	right: "",
	top: "",
	bottom: "",
} as const;

const ARG_KEYS = [
	"filePath",
	"file_path",
	"path",
	"command",
	"cmd",
	"query",
	"pattern",
	"url",
	"name",
	"filename",
	"id",
];

function collapse(text: string): string {
	return text.replace(/\s+/g, " ").trim();
}

function truncate(text: string, max: number): string {
	if (text.length <= max) return text;
	return `${text.slice(0, max)}…`;
}

function summarizeArgs(argsJson: string): string {
	if (!argsJson) return "";
	try {
		const parsed = JSON.parse(argsJson) as Record<string, unknown>;
		for (const key of ARG_KEYS) {
			const v = parsed[key];
			if (typeof v === "string" && v) {
				return truncate(collapse(v), 48);
			}
		}
		const first = Object.values(parsed).find((v) => typeof v === "string" && v);
		if (typeof first === "string") return truncate(collapse(first), 48);
		return truncate(collapse(argsJson), 48);
	} catch {
		return truncate(collapse(argsJson), 48);
	}
}

interface ResultSummary {
	glyph: string;
	color: string;
	summary: string;
	detailLines: string[];
}

function summarizeResult(result: ToolResult | undefined): ResultSummary {
	if (!result) {
		return { glyph: "…", color: "gray", summary: "", detailLines: [] };
	}
	const lines = result.content
		.split("\n")
		.map((l) => l.trim())
		.filter(Boolean);

	if (result.isError) {
		return {
			glyph: "✗",
			color: "red",
			summary: lines.length > 0 ? truncate(lines[0], 60) : "error",
			detailLines: lines.slice(1, 3).map((l) => truncate(l, 80)),
		};
	}

	const firstLine = lines.length > 0 ? truncate(lines[0], 60) : "";
	const extra = lines.length > 1 ? ` +${lines.length - 1} lines` : "";
	return {
		glyph: "✓",
		color: "green",
		summary: firstLine ? `${firstLine}${extra}` : "done",
		detailLines: [],
	};
}

function ToolCallLine({ tc }: { tc: UIToolCall }) {
	const { glyph, color, summary, detailLines } = summarizeResult(tc.result);
	const args = summarizeArgs(tc.arguments);
	return (
		<Box flexDirection="column">
			<Text wrap="wrap">
				<Text bold>{tc.name}</Text>
				<Text dimColor>({args})</Text>
				<Text> </Text>
				<Text color={color}>{glyph}</Text>
				{summary ? <Text color="gray"> {summary}</Text> : null}
			</Text>
			{detailLines.map((line, i) => (
				<Box key={`${tc.id}-detail-${i}`} marginLeft={2}>
					<Text dimColor color="red">
						● {line}
					</Text>
				</Box>
			))}
		</Box>
	);
}

export type MessageRenderItem =
	| {
			id: string;
			type: "message";
			role: "user" | "assistant" | "system";
			content: string;
			error?: boolean;
	  }
	| {
			id: string;
			type: "reasoning";
			content: string;
			durationSeconds: number;
			done: boolean;
			expanded: boolean;
	  }
	| {
			id: string;
			type: "tool";
			toolCall: UIToolCall;
			blinkOn: boolean;
	  };

function Rail({
	color,
	children,
}: {
	color: string;
	children: React.ReactNode;
}) {
	return (
		<Box
			flexDirection="column"
			borderStyle={RAIL_BORDER}
			borderColor={color}
			borderTop={false}
			borderRight={false}
			borderBottom={false}
			paddingLeft={1}
		>
			{children}
		</Box>
	);
}

export const MessageItem = memo(function MessageItem({
	item,
}: {
	item: MessageRenderItem;
}) {
	if (item.type === "reasoning") {
		return (
			<Rail color="magenta">
				<Text dimColor={!item.expanded} italic={!item.expanded} wrap="wrap">
					{item.done
						? `Thought for ${item.durationSeconds.toFixed(1)}s.`
						: `Thinking for ${item.durationSeconds.toFixed(1)}s...`}
				</Text>
				{item.expanded ? (
					<Box marginTop={1}>
						<Text dimColor italic wrap="wrap">
							{item.content}
						</Text>
					</Box>
				) : null}
			</Rail>
		);
	}

	if (item.type === "tool") {
		const done = !!item.toolCall.result;
		return (
			<Rail color={done || item.blinkOn ? "magenta" : "gray"}>
				<ToolCallLine tc={item.toolCall} />
			</Rail>
		);
	}

	if (item.role === "user") {
		return (
			<Rail color="cyan">
				<Text wrap="wrap">
					<Text dimColor>you</Text>
					<Text color="cyan"> › </Text>
					{item.content}
				</Text>
			</Rail>
		);
	}

	if (item.role === "system") {
		return (
			<Rail color={item.error ? "red" : "yellow"}>
				<Text color={item.error ? "red" : "yellow"}>
					{item.error ? "✗" : "●"} {item.content}
				</Text>
			</Rail>
		);
	}

	return (
		<Rail color="magenta">
			<Text wrap="wrap">
				<Text color="magenta">⏺ </Text>
				{item.content}
			</Text>
		</Rail>
	);
});
