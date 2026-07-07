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
import { Spinner } from "@inkjs/ui";

const PLACEHOLDER = "type a message, or / for commands";

function abbreviateCwd(wd: string, max = 40): string {
	const home = process.env.HOME ?? process.env.USERPROFILE ?? "";
	let display = home && wd.startsWith(home) ? `~${wd.slice(home.length)}` : wd;
	if (display.length <= max) return display;
	return `…${display.slice(display.length - max + 1)}`;
}

interface InputBoxProps {
	value: string;
	onChange: (v: string) => void;
	onSubmit: (text: string) => void;
	onTab: () => boolean;
	streaming: boolean;
	phrase: string | null;
	modelName: string;
	cwd: string;
	tokenConsumed: number;
	abort: () => void;
}

export function InputBox({
	value,
	onChange,
	onSubmit,
	onTab,
	streaming,
	phrase,
	modelName,
	cwd,
	tokenConsumed,
	abort,
}: InputBoxProps) {
	const [tabNonce, setTabNonce] = useState(0);

	useInput((_input, key) => {
		if (streaming) {
			if (key.escape) abort();
			return;
		}
		if (key.tab) {
			if (onTab()) setTabNonce((n) => n + 1);
		}
	});

	return (
		<Box flexDirection="column" paddingX={1}>
			<Box flexDirection="row" height={1} overflow="hidden">
				{streaming && phrase ? (
					<>
						<Spinner label={phrase} />
						<Text dimColor> Esc to cancel </Text>
					</>
				) : null}
				<Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
					<Text color="gray">{"─".repeat(300)}</Text>
				</Box>
				<Text color="cyan">{` ▸ ${modelName}`}</Text>
			</Box>

			<Box flexDirection="row">
				<Text color="cyan" bold>
					{"> "}
				</Text>
				{streaming ? (
					<Text dimColor>{PLACEHOLDER}</Text>
				) : (
					<TextInput
						key={tabNonce}
						value={value}
						onChange={onChange}
						onSubmit={onSubmit}
						placeholder={PLACEHOLDER}
					/>
				)}
			</Box>

			<Box height={1} overflow="hidden">
				<Text color="gray">{"─".repeat(300)}</Text>
			</Box>

			<Box flexDirection="row" height={1} overflow="hidden">
				<Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
					<Text color="gray" dimColor wrap="truncate-start">
						{abbreviateCwd(cwd)}
					</Text>
				</Box>
				<Text color="gray" dimColor>{`tokens: ${tokenConsumed}`}</Text>
			</Box>
		</Box>
	);
}
