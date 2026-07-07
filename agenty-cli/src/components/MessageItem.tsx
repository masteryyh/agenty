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

import { Box, Text } from "ink";
import type { UIMessage } from "../state/store";

function truncate(text: string, max: number): string {
	const single = text.replace(/\s+/g, " ").trim();
	if (single.length <= max) return single;
	return `${single.slice(0, max)}…`;
}

export function MessageItem({ msg }: { msg: UIMessage }) {
	if (msg.role === "user") {
		return (
			<Box flexDirection="column">
				<Text color="cyan" bold>
					You
				</Text>
				<Text>{msg.content}</Text>
			</Box>
		);
	}

	if (msg.role === "tool" && msg.toolResult) {
		const tr = msg.toolResult;
		return (
			<Box flexDirection="column" marginLeft={2}>
				<Text color={tr.isError ? "red" : "gray"}>
					↳ {tr.name || "tool"} {tr.isError ? "(error)" : "(ok)"}
				</Text>
				{tr.content ? (
					<Text color="gray" dimColor>
						{truncate(tr.content, 500)}
					</Text>
				) : null}
			</Box>
		);
	}

	if (msg.role === "system") {
		return (
			<Box>
				<Text color={msg.error ? "red" : "yellow"}>
					{msg.error ? "✗ " : ""}
					{msg.content}
				</Text>
			</Box>
		);
	}

	return (
		<Box flexDirection="column">
			<Text color="green" bold>
				Assistant
			</Text>
			{msg.reasoning ? (
				<Text dimColor italic>
					Thinking: {msg.reasoning}
				</Text>
			) : null}
			{msg.toolCalls && msg.toolCalls.length > 0
				? msg.toolCalls.map((tc, i) => (
						<Box key={tc.id || `${tc.name}-${i}`} flexDirection="column">
							<Text color="yellow">🔧 {tc.name}</Text>
							{tc.arguments ? (
								<Text color="gray" dimColor>
									{truncate(tc.arguments, 300)}
								</Text>
							) : null}
						</Box>
					))
				: null}
			{msg.content ? <Text>{msg.content}</Text> : null}
		</Box>
	);
}
