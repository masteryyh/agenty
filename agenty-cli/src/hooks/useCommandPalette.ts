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

import { useEffect, useRef, useState } from "react";
import type { AgentyClient } from "../api/client";
import {
	findCommand,
	longestCommonPrefix,
	matchingCommands,
	parseCommandTokens,
	quoteArg,
} from "../commands/registry";
import type { Command } from "../commands/registry";

const MAX_ITEMS = 8;

export type Palette =
	| { mode: "none" }
	| { mode: "commands"; matches: Command[] }
	| {
			mode: "args";
			command: Command;
			candidates: string[] | null;
			loading: boolean;
			highlight: number;
	  };

export interface PaletteResult {
	palette: Palette;
	height: number;
	tab: () => string | null;
}

export function useCommandPalette(
	value: string,
	client: AgentyClient | null,
): PaletteResult {
	const [cache, setCache] = useState<Record<string, string[]>>({});
	const [loading, setLoading] = useState(false);
	const fetchedRef = useRef<Set<string>>(new Set());

	const tokens = parseCommandTokens(value);
	const startsSlash = value.startsWith("/");
	const cmdToken = startsSlash ? (tokens[0] ?? value) : "";
	const argPart = tokens.length > 1 ? tokens.slice(1).join(" ") : "";
	const hasSpace = value.includes(" ");
	const exactCmd = cmdToken ? findCommand(cmdToken) : undefined;

	let palette: Palette = { mode: "none" };

	if (startsSlash && exactCmd && exactCmd.completeArgs) {
		const cands = cache[exactCmd.name] ?? null;
		let highlight = 0;
		if (cands) {
			const exact = cands.findIndex((c) => c === argPart);
			if (exact >= 0) {
				highlight = exact;
			} else {
				const lower = argPart.toLowerCase();
				const prefix = cands.findIndex((c) =>
					c.toLowerCase().startsWith(lower),
				);
				highlight = prefix >= 0 ? prefix : 0;
			}
		}
		palette = {
			mode: "args",
			command: exactCmd,
			candidates: cands,
			loading,
			highlight,
		};
	} else if (startsSlash && !hasSpace) {
		const matches = matchingCommands(value);
		if (matches.length > 0) {
			palette = { mode: "commands", matches };
		}
	}

	const fetchKey = palette.mode === "args" ? palette.command.name : null;
	useEffect(() => {
		if (!fetchKey || !client || fetchedRef.current.has(fetchKey)) return;
		const cmd = findCommand(fetchKey);
		if (!cmd?.completeArgs) return;
		fetchedRef.current.add(fetchKey);
		setLoading(true);
		cmd.completeArgs(client)
			.then((c) => {
				setCache((prev) => ({ ...prev, [fetchKey]: c }));
			})
			.catch(() => {
				fetchedRef.current.delete(fetchKey);
			})
			.finally(() => {
				setLoading(false);
			});
	}, [fetchKey, client]);

	const tab = (): string | null => {
		if (palette.mode === "commands") {
			const names = palette.matches.map((m) => m.name);
			const prefix = longestCommonPrefix(names);
			if (prefix.length > value.length) return prefix;
			return null;
		}
		if (palette.mode === "args" && palette.candidates) {
			const cands = palette.candidates;
			if (cands.length === 0) return null;
			const selected = cands.findIndex((c) => c === argPart);
			let next: number;
			if (selected >= 0) {
				next = (selected + 1) % cands.length;
			} else {
				const lower = argPart.toLowerCase();
				const pi = cands.findIndex((c) => c.toLowerCase().startsWith(lower));
				next = pi >= 0 ? pi : 0;
			}
			return `${cmdToken} ${quoteArg(cands[next])}`;
		}
		return null;
	};

	let height = 0;
	if (palette.mode === "commands") {
		height = Math.min(palette.matches.length, MAX_ITEMS) + 1;
	} else if (palette.mode === "args") {
		const n = palette.candidates ? Math.min(palette.candidates.length, MAX_ITEMS) : 0;
		height = 1 + (palette.loading && !palette.candidates ? 1 : n);
	}

	return { palette, height, tab };
}
