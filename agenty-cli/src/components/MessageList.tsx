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

import { useEffect, useMemo, useRef } from "react";
import { Box, Text } from "ink";
import { ScrollList, type ScrollListRef } from "ink-scroll-list";
import type { UIMessage } from "../state/store";
import { MessageItem } from "./MessageItem";

interface MessageListProps {
	history: UIMessage[];
	current: UIMessage | null;
	height: number;
}

export function MessageList({ history, current, height }: MessageListProps) {
	const listRef = useRef<ScrollListRef>(null);

	const items: UIMessage[] = useMemo(() => {
		const list: UIMessage[] = [...history];
		if (current) list.push(current);
		return list;
	}, [history, current]);

	const empty = items.length === 0;

	useEffect(() => {
		listRef.current?.remeasure();
	}, [height, items.length]);

	useEffect(() => {
		const onResize = () => listRef.current?.remeasure();
		process.stdout.on("resize", onResize);
		return () => {
			process.stdout.off("resize", onResize);
		};
	}, []);

	if (empty || height <= 0) {
		return (
			<Box height={Math.max(height, 0)} overflowY="hidden" paddingX={1} justifyContent="center">
				<Text dimColor>
					Start chatting by typing a message below. Type /help for commands.
				</Text>
			</Box>
		);
	}

	return (
		<Box height={height} overflowY="hidden" paddingX={1}>
			<ScrollList
				ref={listRef}
				height={height}
				selectedIndex={items.length - 1}
				scrollAlignment="auto"
			>
				{items.map((msg) => (
					<Box key={msg.id} flexDirection="column">
						<MessageItem msg={msg} />
					</Box>
				))}
			</ScrollList>
		</Box>
	);
}
