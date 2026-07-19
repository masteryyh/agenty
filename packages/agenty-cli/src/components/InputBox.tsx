import { useState } from "react";
import { useInput } from "../hooks/useInput";
import { Box, Spinner, Text, TextInput } from "./ui";
import type { ToastMsg } from "../state/store";

const PLACEHOLDER = "type a message, or / for commands";

function abbreviateCwd(wd: string, max = 40): string {
	const home = process.env.HOME ?? process.env.USERPROFILE ?? "";
	let display = home && wd.startsWith(home) ? `~${wd.slice(home.length)}` : wd;
	if (display.length <= max) return display;
	return `…${display.slice(display.length - max + 1)}`;
}

function effortColor(level: string): string {
	switch (level) {
		case "low":
			return "gray";
		case "medium":
			return "yellow";
		case "high":
			return "cyan";
		case "xhigh":
		case "max":
			return "magenta";
		default:
			return "green";
	}
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
	thinkingLevel: string;
	reasoningActive: boolean;
	abort: () => void;
	toast: ToastMsg | null;
	active?: boolean;
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
	thinkingLevel,
	reasoningActive,
	abort,
	toast,
	active = true,
}: InputBoxProps) {
	const [tabNonce, setTabNonce] = useState(0);

	useInput(
		(_input, key, event) => {
			if (streaming) {
				if (key.escape) abort();
				return;
			}
			if (key.tab) {
				event.preventDefault();
				if (onTab()) setTabNonce((n) => n + 1);
			}
		},
		{ isActive: active },
	);

	return (
		<Box flexDirection="column" paddingX={1}>
			<Box flexDirection="row" height={1} overflow="hidden">
				{streaming && phrase ? (
					<>
						<Spinner label={phrase} />
						{reasoningActive && thinkingLevel ? (
							<Text dimColor>
								{` (thinking with ${thinkingLevel} effort)`}
							</Text>
						) : null}
						<Text dimColor> Esc to cancel </Text>
					</>
				) : null}
				<Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
					<Text color="gray">{"─".repeat(300)}</Text>
				</Box>
				{toast ? (
					<Text color={toast.error ? "red" : "green"}>{toast.text}</Text>
				) : (
					<Text color="cyan">{` ▸ ${modelName}`}</Text>
				)}
			</Box>

			<Box flexDirection="row" height={1} overflow="hidden">
				<Box width={2} flexShrink={0} height={1}>
					<Text color="cyan" bold>
						{"❯ "}
					</Text>
				</Box>
				{streaming ? (
					<Text dimColor>{PLACEHOLDER}</Text>
				) : (
					<Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
						<TextInput
							key={tabNonce}
							value={value}
							onChange={onChange}
							onSubmit={onSubmit}
							placeholder={PLACEHOLDER}
							focus={active}
							keepFocus={active}
						/>
					</Box>
				)}
			</Box>

			<Box height={1} overflow="hidden">
				<Text color="gray">{"─".repeat(300)}</Text>
			</Box>

			<Box flexDirection="row" height={1} overflow="hidden">
				<Box flexDirection="row" flexShrink={1} height={1} overflow="hidden">
					<Text color="gray" dimColor wrap="truncate-start">
						{abbreviateCwd(cwd)}
					</Text>
					{thinkingLevel ? <Text> </Text> : null}
					{thinkingLevel ? (
						<Text color={effortColor(thinkingLevel)}>{`thinking: ${thinkingLevel}`}</Text>
					) : null}
				</Box>
				<Box flexGrow={1} flexBasis={0} height={1} overflow="hidden" />
				<Text color="gray" dimColor>{`tokens: ${tokenConsumed}`}</Text>
			</Box>
		</Box>
	);
}
