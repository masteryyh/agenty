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

import { useState, useRef, useCallback, useMemo } from "react";
import { Box, Text, useInput } from "ink";
import TextInput from "ink-text-input";

// ─── types ──────────────────────────────────────────────────────────

export interface FormOption {
	label: string;
	value: string;
}

export interface FormField {
	key: string;
	label: string;
	kind: "text" | "select" | "boolean" | "multiselect";
	value: string;
	options?: FormOption[];
	placeholder?: string;
	secret?: boolean;
	readOnly?: boolean;
	visible?: boolean;
}

export interface FormAction {
	key: string;
	label: string;
}

export interface FormPanelProps {
	title: string;
	fields: FormField[];
	actions?: FormAction[];
	onChange?: (key: string, allValues: Record<string, string>) => void;
	onAction: (key: string, values: Record<string, string>) => void;
	onClose: () => void;
}

// ─── display helpers ─────────────────────────────────────────────────

const KEY_WIDTH = 22;

function pad(s: string, w: number): string {
	if (w <= 0) return "";
	if (s.length <= w) return s + " ".repeat(w - s.length);
	if (w === 1) return "\u2026";
	return s.slice(0, w - 1) + "\u2026";
}

function maskValue(v: string): string {
	if (!v) return "\u2014";
	return "\u2022".repeat(Math.min(v.length, 20));
}

function selectLabel(options: FormOption[], value: string): string {
	const found = options.find((o) => o.value === value);
	return found ? found.label : value;
}

function parseMulti(value: string): Set<string> {
	try {
		const arr = JSON.parse(value);
		if (Array.isArray(arr)) {
			return new Set(arr.filter((x): x is string => typeof x === "string"));
		}
	} catch {
		// fall through
	}
	return new Set();
}

function serializeMulti(set: Set<string>): string {
	return JSON.stringify(Array.from(set));
}

// ─── component ───────────────────────────────────────────────────────

type FieldState =
	| { kind: "idle" }
	| { kind: "editing"; visibleIndex: number; text: string }
	| { kind: "selecting"; visibleIndex: number; selection: number }
	| {
			kind: "multi-selecting";
			visibleIndex: number;
			selection: number;
			chosen: Set<string>;
	  };

export function FormPanel({
	title,
	fields,
	actions,
	onChange,
	onAction,
	onClose,
}: FormPanelProps) {
	const [values, setValues] = useState<Record<string, string>>(() => {
		const init: Record<string, string> = {};
		for (const f of fields) init[f.key] = f.value;
		return init;
	});

	const actDefs: FormAction[] = actions ?? [
		{ key: "save", label: "Save" },
		{ key: "cancel", label: "Cancel" },
	];

	// visible subset
	const visibleFields = useMemo(
		() => fields.filter((f) => f.visible !== false),
		[fields],
	);
	const actionStart = visibleFields.length;
	const actionEnd = actionStart + actDefs.length - 1;

	const [cursor, setCursor] = useState(0);
	const [fstate, setFstate] = useState<FieldState>({ kind: "idle" });

	// refs for stale closure safety
	const valuesRef = useRef(values);
	valuesRef.current = values;
	const cursorRef = useRef(cursor);
	cursorRef.current = cursor;
	const fstateRef = useRef(fstate);
	fstateRef.current = fstate;
	const visibleRef = useRef(visibleFields);
	visibleRef.current = visibleFields;
	const actDefsRef = useRef(actDefs);
	actDefsRef.current = actDefs;

	const doSetValues = useCallback(
		(updater: (prev: Record<string, string>) => Record<string, string>) => {
			setValues((prev) => {
				const next = updater(prev);
				return next;
			});
		},
		[],
	);

	const notifyChange = useCallback(
		(key: string) => {
			if (!onChange) return;
			// read latest values via ref inside a timeout to avoid setState-in-setState
			queueMicrotask(() => {
				onChange(key, valuesRef.current);
			});
		},
		[onChange],
	);

	const updateValue = useCallback(
		(key: string, v: string) => {
			doSetValues((prev) => ({ ...prev, [key]: v }));
			notifyChange(key);
		},
		[doSetValues, notifyChange],
	);

	const commitEdit = useCallback(
		(text: string) => {
			const st = fstateRef.current;
			if (st.kind !== "editing") return;
			const f = visibleRef.current[st.visibleIndex];
			if (!f) return;
			doSetValues((prev) => ({ ...prev, [f.key]: text }));
			notifyChange(f.key);
			setFstate({ kind: "idle" });
		},
		[doSetValues, notifyChange],
	);

	const cancelEdit = useCallback(() => {
		setFstate({ kind: "idle" });
	}, []);

	const commitSelect = useCallback(
		(selection: number) => {
			const st = fstateRef.current;
			if (st.kind !== "selecting") return;
			const f = visibleRef.current[st.visibleIndex];
			if (!f) return;
			const opts = f.options ?? [];
			if (selection >= 0 && selection < opts.length) {
				doSetValues((prev) => ({ ...prev, [f.key]: opts[selection].value }));
				notifyChange(f.key);
			}
			setFstate({ kind: "idle" });
		},
		[doSetValues, notifyChange],
	);

	const cancelSelect = useCallback(() => {
		setFstate({ kind: "idle" });
	}, []);

	const commitMultiSelect = useCallback(() => {
		const st = fstateRef.current;
		if (st.kind !== "multi-selecting") {
			setFstate({ kind: "idle" });
			return;
		}
		const f = visibleRef.current[st.visibleIndex];
		if (!f) {
			setFstate({ kind: "idle" });
			return;
		}
		doSetValues((prev) => ({ ...prev, [f.key]: serializeMulti(st.chosen) }));
		notifyChange(f.key);
		setFstate({ kind: "idle" });
	}, [doSetValues, notifyChange]);

	const cancelMultiSelect = useCallback(() => {
		setFstate({ kind: "idle" });
	}, []);

	useInput((input, key) => {
		const st = fstateRef.current;

		// ── editing text field ──
		if (st.kind === "editing") {
			if (key.escape) { cancelEdit(); return; }
			// TextInput handles Enter to submit
			return;
		}

		// ── selecting option ──
		if (st.kind === "selecting") {
			const f = visibleRef.current[st.visibleIndex];
			const opts = f?.options ?? [];
			if (key.escape) { cancelSelect(); return; }
			if (key.upArrow) {
				setFstate((s) =>
					s.kind === "selecting"
						? { ...s, selection: s.selection > 0 ? s.selection - 1 : 0 }
						: s,
				);
				return;
			}
			if (key.downArrow) {
				setFstate((s) =>
					s.kind === "selecting"
						? {
								...s,
								selection:
									s.selection < opts.length - 1
										? s.selection + 1
										: s.selection,
							}
						: s,
				);
				return;
			}
			if (key.leftArrow || key.return) {
				commitSelect(st.selection);
				return;
			}
			return;
		}

		// ── multi-selecting options ──
		if (st.kind === "multi-selecting") {
			const f = visibleRef.current[st.visibleIndex];
			const opts = f?.options ?? [];
			if (key.escape) { cancelMultiSelect(); return; }
			if (key.upArrow) {
				setFstate((s) =>
					s.kind === "multi-selecting"
						? { ...s, selection: s.selection > 0 ? s.selection - 1 : 0 }
						: s,
				);
				return;
			}
			if (key.downArrow) {
				setFstate((s) =>
					s.kind === "multi-selecting"
						? {
								...s,
								selection:
									s.selection < opts.length - 1
										? s.selection + 1
										: s.selection,
							}
						: s,
				);
				return;
			}
			if (input === " ") {
				setFstate((s) => {
					if (s.kind !== "multi-selecting") return s;
					const opt = opts[s.selection];
					if (!opt) return s;
					const next = new Set(s.chosen);
					if (next.has(opt.value)) next.delete(opt.value);
					else next.add(opt.value);
					return { ...s, chosen: next };
				});
				return;
			}
			if (key.leftArrow || key.return) {
				commitMultiSelect();
				return;
			}
			return;
		}

		// ── idle navigation ──
		const c = cursorRef.current;

		if (key.escape) { onClose(); return; }

		// navigate among visible fields + actions
		if (key.upArrow) {
			setCursor((prev) => Math.max(prev - 1, 0));
			return;
		}
		if (key.downArrow) {
			setCursor((prev) => Math.min(prev + 1, actionEnd));
			return;
		}

		// cursor on action row
		if (c >= actionStart && c <= actionEnd) {
			if (key.leftArrow) {
				setCursor((prev) => (prev > actionStart ? prev - 1 : actionEnd));
				return;
			}
			if (key.rightArrow) {
				setCursor((prev) => (prev < actionEnd ? prev + 1 : actionStart));
				return;
			}
			if (key.return) {
				const act = actDefsRef.current[c - actionStart];
				if (act.key === "cancel") { onClose(); return; }
				onAction(act.key, valuesRef.current);
				return;
			}
			return;
		}

		// cursor on a visible field
		const vf = visibleRef.current[c];
		if (!vf) return;
		if (vf.readOnly) return;

		if (vf.kind === "boolean") {
			if (key.leftArrow || key.rightArrow) {
				const current = valuesRef.current[vf.key] ?? vf.value;
				updateValue(vf.key, current === "true" ? "false" : "true");
			}
			return;
		}

		if (vf.kind === "select") {
			if (key.rightArrow || key.return) {
				const opts = vf.options ?? [];
				if (opts.length === 0) return;
				const cur = valuesRef.current[vf.key] ?? vf.value;
				const idx = opts.findIndex((o) => o.value === cur);
				setFstate({
					kind: "selecting",
					visibleIndex: c,
					selection: idx >= 0 ? idx : 0,
				});
			}
			return;
		}

		if (vf.kind === "multiselect") {
			if (key.rightArrow || key.return) {
				const opts = vf.options ?? [];
				if (opts.length === 0) return;
				const chosen = parseMulti(valuesRef.current[vf.key] ?? vf.value);
				const firstChosen = opts.findIndex((o) => chosen.has(o.value));
				setFstate({
					kind: "multi-selecting",
					visibleIndex: c,
					selection: firstChosen >= 0 ? firstChosen : 0,
					chosen,
				});
			}
			return;
		}

		if (vf.kind === "text") {
			if (key.return) {
				setFstate({
					kind: "editing",
					visibleIndex: c,
					text: valuesRef.current[vf.key] ?? vf.value,
				});
			}
			return;
		}
	});

	// ── render ────────────────────────────────────────────────────────

	return (
		<Box flexDirection="column" flexGrow={1}>
			<Box marginBottom={1}>
				<Text color="magenta" bold>{title}</Text>
			</Box>

			<Box flexDirection="column" flexGrow={1} overflow="hidden">
				{visibleFields.map((f, vi) => {
					const isActive = cursor === vi;
					const st = fstate;
					const isEditing = st.kind === "editing" && st.visibleIndex === vi;
					const isSelecting =
						st.kind === "selecting" && st.visibleIndex === vi;
					const isMultiSelecting =
						st.kind === "multi-selecting" && st.visibleIndex === vi;
					const curVal = values[f.key] ?? f.value;

					return (
						<Box key={f.key} flexDirection="column">
							{/* label + value row */}
							<Box>
								<Box width={2}>
									<Text
										color={
											isActive && !isEditing ? "cyan" : "gray"
										}
									>
										{isActive && !isEditing ? "\u276f" : " "}
									</Text>
								</Box>
								<Box width={KEY_WIDTH}>
									<Text
										color={
											isActive && !isEditing
												? "cyan"
												: "gray"
										}
										bold={isActive && !isEditing}
									>
										{pad(f.label + ":", KEY_WIDTH)}
									</Text>
								</Box>
								<Text> </Text>
								{isEditing ? (
									<Box flexGrow={1}>
										<TextInput
											value={curVal}
											onChange={(v) =>
												updateValue(f.key, v)
											}
											onSubmit={(v) => commitEdit(v)}
											placeholder={f.placeholder ?? ""}
										/>
									</Box>
								) : (
									<Text
										color={
											isActive ? "cyan" : "white"
										}
									>
										{f.kind === "boolean"
											? renderBoolean(isActive, curVal)
											: f.kind === "select"
												? selectLabel(f.options ?? [], curVal)
												: f.kind === "multiselect"
													? renderMultiValue(curVal)
												: f.secret
												? maskValue(curVal)
												: curVal || (
														<Text dimColor>
															{"\u2014"}
														</Text>
													)}
									</Text>
								)}
							</Box>

							{/* expanded select options */}
							{isSelecting &&
							f.kind === "select" &&
							f.options ? (
								<Box
									flexDirection="column"
									marginLeft={KEY_WIDTH + 3}
								>
									{f.options.map((opt, oi) => {
										const sel =
											st.kind === "selecting" &&
											st.selection === oi;
										return (
											<Box key={opt.value}>
												<Text
													color={
														sel
															? "cyan"
															: "gray"
													}
												>
													{sel
														? "\u276f "
														: "  "}
												</Text>
												<Text
													color={
														sel
															? "cyan"
															: "white"
													}
													bold={sel}
												>
													{opt.label}
												</Text>
											</Box>
										);
									})}
								</Box>
							) : null}

							{/* expanded multiselect options */}
							{isMultiSelecting &&
							f.kind === "multiselect" &&
							f.options ? (
								<Box
									flexDirection="column"
									marginLeft={KEY_WIDTH + 3}
								>
									{f.options.map((opt, oi) => {
										const sel =
											st.kind === "multi-selecting" &&
											st.selection === oi;
										const checked =
											st.kind === "multi-selecting" &&
											st.chosen.has(opt.value);
										return (
											<Box key={opt.value}>
												<Text color={sel ? "cyan" : "gray"}>
													{sel ? "❯ " : "  "}
												</Text>
												<Text
													color={checked ? "cyan" : "white"}
													bold={checked}
												>
													{checked ? "✓ " : "☐ "}
													{opt.label}
												</Text>
											</Box>
										);
									})}
								</Box>
							) : null}
						</Box>
					);
				})}

				{/* spacer */}
				<Box flexGrow={1} />

				{/* action row */}
				<Box gap={3}>
					{actDefs.map((act, ai) => {
						const ac = actionStart + ai;
						const active = cursor === ac;
						return (
							<Box key={act.key}>
								<Text
									color={active ? "cyan" : "gray"}
									bold={active}
								>
									{active ? "\u276f " : "  "}
									{act.label}
								</Text>
							</Box>
						);
					})}
				</Box>
			</Box>

			{/* hints */}
			<Box>
				<Text dimColor>
					{"\u2191\u2193 navigate \u00b7 \u2190\u2192 toggle \u00b7 Enter edit/choose \u00b7 Space toggle · Esc back"}
				</Text>
			</Box>
		</Box>
	);
}

// ─── boolean render helper ───────────────────────────────────────────

function renderBoolean(active: boolean, v: string): React.ReactNode {
	const isTrue = v === "true";
	return (
		<>
			<Text
				color={active && isTrue ? "cyan" : "gray"}
				bold={active && isTrue}
			>
				{isTrue ? "\u25c9 true" : "\u25cb false"}
			</Text>
		</>
	);
}

function renderMultiValue(value: string): React.ReactNode {
	const chosen = parseMulti(value);
	if (chosen.size === 0) return <Text dimColor>{"\u2014"}</Text>;
	return <Text>{`${chosen.size} selected`}</Text>;
}
