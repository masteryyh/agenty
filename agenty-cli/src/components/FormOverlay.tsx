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

import { useState } from "react";
import { Box, Text, useInput } from "ink";
import TextInput from "ink-text-input";
import { Select } from "@inkjs/ui";

export interface FormField {
	key: string;
	label: string;
	kind: "text" | "select";
	options?: string[];
	placeholder?: string;
	secret?: boolean;
	required?: boolean;
	defaultValue?: string;
}

interface FormOverlayProps {
	title: string;
	fields: FormField[];
	submitLabel?: string;
	onSubmit: (values: Record<string, string>) => void;
	onClose: () => void;
}

export function FormOverlay({
	title,
	fields,
	submitLabel = "Submit",
	onSubmit,
	onClose,
}: FormOverlayProps) {
	const [values, setValues] = useState<Record<string, string>>(() => {
		const init: Record<string, string> = {};
		for (const f of fields) init[f.key] = f.defaultValue ?? "";
		return init;
	});
	const [focus, setFocus] = useState(0);
	const [error, setError] = useState<string | null>(null);

	useInput((_input, key) => {
		if (key.escape) {
			onSubmit(values);
			onClose();
			return;
		}
		if (key.upArrow) {
			setFocus((f) => Math.max(f - 1, 0));
			setError(null);
			return;
		}
		if (key.downArrow) {
			setFocus((f) => Math.min(f + 1, fields.length - 1));
			setError(null);
			return;
		}
	});

	const isLast = focus === fields.length - 1;

	return (
		<Box flexDirection="column">
			<Box marginBottom={1} gap={1}>
				<Text color="magenta" bold>
					{title}
				</Text>
			</Box>

			{fields.map((f, i) => {
				const active = i === focus;
				const val = values[f.key];
				const display =
					f.kind === "select"
						? val
						: f.secret && val
							? "•".repeat(Math.min(val.length, 16))
							: val;
				return (
					<Box key={f.key} flexDirection="column">
						<Box gap={1}>
							<Text color={active ? "cyan" : "gray"}>
								{active ? "❯" : " "}
							</Text>
							<Text color={active ? "cyan" : "gray"} bold={active}>
								{f.label}:
							</Text>
							{!active ? (
								<Text color="white">{display || <Text dimColor>—</Text>}</Text>
							) : null}
						</Box>
						{active && f.kind === "text" ? (
							<Box paddingLeft={2}>
								<TextInput
									value={val}
									onChange={(v) => {
										setValues((prev) => ({ ...prev, [f.key]: v }));
										setError(null);
									}}
									placeholder={f.placeholder ?? ""}
								/>
							</Box>
						) : null}
						{active && f.kind === "select" && f.options ? (
							<Box paddingLeft={2}>
								<Select
									options={f.options.map((o) => ({ label: o, value: o }))}
									defaultValue={val || f.options[0]}
									onChange={(v) => {
										setValues((prev) => ({ ...prev, [f.key]: v }));
										setError(null);
										if (isLast) {
											onSubmit({ ...values, [f.key]: v });
											onClose();
										} else {
											setFocus((f) => f + 1);
										}
									}}
								/>
							</Box>
						) : null}
					</Box>
				);
			})}

			{error ? (
				<Text color="red">{error}</Text>
			) : null}
		</Box>
	);
}
