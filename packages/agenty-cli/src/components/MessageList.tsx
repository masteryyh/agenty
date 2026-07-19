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

import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import {
	CliRenderEvents,
	type ScrollBoxRenderable,
	type ThemeMode,
} from "@opentui/core";
import { useRenderer } from "@opentui/react";
import { useInput } from "../hooks/useInput";
import { Box, Text } from "./ui";
import type { UIMessage } from "../state/store";
import { MessageItem, type MessageRenderItem } from "./MessageItem";

const HINT_BACKGROUND = "#24383f";
const HINT_FOREGROUND = "#c7f5ff";

interface MessageListProps {
	history: UIMessage[];
	current: UIMessage | null;
	height: number;
	// Rendered as the first, scroll-away item at the top of the list (the logo).
	header?: ReactNode;
	// Whether keyboard scrolling (PageUp/PageDown, arrows, End, and wheel-via-
	// 1007 arrows) is active. Turned off while an overlay/palette owns input.
	interactive?: boolean;
}

export function MessageList({
	history,
	current,
	height,
	header,
	interactive = true,
}: MessageListProps) {
	const renderer = useRenderer();
	const listRef = useRef<ScrollBoxRenderable>(null);
	// follow = pinned to the bottom, auto-scrolling as new content streams in.
	// When the user scrolls up we detach; new messages then accrue into `unseen`.
	const [follow, setFollow] = useState(true);
	const [unseen, setUnseen] = useState(0);
	const [allReasoningExpanded, setAllReasoningExpanded] = useState(false);
	const [expandedReasoningIds, setExpandedReasoningIds] = useState<Set<string>>(
		() => new Set(),
	);
	const [now, setNow] = useState(() => Date.now());
	const [blinkOn, setBlinkOn] = useState(true);
	const [themeMode, setThemeMode] = useState<ThemeMode>(
		() => renderer.themeMode ?? "dark",
	);

	useEffect(() => {
		let cancelled = false;
		const handleThemeMode = (mode: ThemeMode) => {
			if (!cancelled) setThemeMode(mode);
		};

		renderer.on(CliRenderEvents.THEME_MODE, handleThemeMode);
		void renderer.waitForThemeMode().then((mode) => {
			if (mode) handleThemeMode(mode);
		});
		return () => {
			cancelled = true;
			renderer.off(CliRenderEvents.THEME_MODE, handleThemeMode);
		};
	}, [renderer]);

	const messages: UIMessage[] = useMemo(() => {
		const list: UIMessage[] = [...history];
		if (current) list.push(current);
		return list;
	}, [history, current]);

	const hasActiveReasoning = messages.some(
		(msg) => msg.reasoning && msg.reasoningStartedAt && !msg.reasoningEndedAt,
	);
	const hasPendingTool = messages.some((msg) =>
		msg.toolCalls?.some((tc) => !tc.result),
	);

	useEffect(() => {
		if (!hasActiveReasoning) return;
		const timer = setInterval(() => setNow(Date.now()), 100);
		return () => clearInterval(timer);
	}, [hasActiveReasoning]);

	useEffect(() => {
		if (!hasPendingTool) {
			setBlinkOn(true);
			return;
		}
		const timer = setInterval(() => setBlinkOn((on) => !on), 500);
		return () => clearInterval(timer);
	}, [hasPendingTool]);

	const renderItems: MessageRenderItem[] = useMemo(() => {
		const rendered: MessageRenderItem[] = [];
		for (const msg of messages) {
			if (msg.role === "user" || msg.role === "system") {
				rendered.push({
					id: `${msg.id}:message`,
					type: "message",
					role: msg.role,
					content: msg.content,
					error: msg.error,
				});
				continue;
			}

			if (msg.reasoning) {
				const id = `${msg.id}:reasoning`;
				const durationMillis =
					msg.reasoningDurationMillis ??
					(msg.reasoningStartedAt
						? (msg.reasoningEndedAt ?? now) - msg.reasoningStartedAt
						: 0);
				rendered.push({
					id,
					type: "reasoning",
					content: msg.reasoning,
					durationSeconds: Math.max(0, durationMillis / 1000),
					done: !!msg.reasoningEndedAt || msg.reasoningDurationMillis !== undefined,
					expanded: allReasoningExpanded || expandedReasoningIds.has(id),
				});
			}

			if (msg.content) {
				rendered.push({
					id: `${msg.id}:content`,
					type: "message",
					role: "assistant",
					content: msg.content,
				});
			}

			msg.toolCalls?.forEach((tc, index) => {
				rendered.push({
					id: `${msg.id}:tool:${tc.id || index}`,
					type: "tool",
					toolCall: tc,
					blinkOn,
				});
			});
		}
		return rendered;
	}, [
		messages,
		now,
		blinkOn,
		allReasoningExpanded,
		expandedReasoningIds,
	]);

	const empty = renderItems.length === 0;
	const itemIds = useMemo(
		() => renderItems.map((item) => item.id),
		[renderItems],
	);

	const isAtBottom = (offset?: number): boolean => {
		const ref = listRef.current;
		if (!ref) return follow;
		const contentHeight = ref.scrollHeight;
		const viewportHeight = ref.viewport.height;
		const maxScroll = Math.max(0, contentHeight - viewportHeight);
		const currentOffset = offset ?? ref.scrollTop;
		return maxScroll - currentOffset <= 0;
	};

	const attachToBottom = () => {
		setFollow(true);
		setUnseen(0);
	};

	const scrollToBottom = () => {
		const ref = listRef.current;
		if (!ref) return;
		ref.scrollTo(Math.max(0, ref.scrollHeight - ref.viewport.height));
	};

	const handleScrollPosition = (offset?: number) => {
		if (isAtBottom(offset)) attachToBottom();
		else setFollow(false);
	};

	// Track appended message ids while detached. Content deltas for the current
	// assistant message update the same id, so they do not inflate the unread count.
	const prevIdsRef = useRef(itemIds);
	useEffect(() => {
		const prevIds = prevIdsRef.current;
		const isAppend =
			itemIds.length >= prevIds.length &&
			prevIds.every((id, index) => itemIds[index] === id);
		const appended = isAppend ? itemIds.length - prevIds.length : 0;
		prevIdsRef.current = itemIds;

		if (!isAppend) {
			attachToBottom();
			scrollToBottom();
			return;
		}

		if (appended > 0) {
			if (follow || isAtBottom()) {
				attachToBottom();
				scrollToBottom();
			} else {
				setUnseen((n) => n + appended);
			}
		}
	}, [itemIds, follow]);

	useEffect(() => {
		if (follow) setUnseen(0);
	}, [follow]);

	const jumpToBottom = () => {
		attachToBottom();
		scrollToBottom();
	};

	// OpenTUI owns measurement and sticky-bottom behavior. This local state only
	// tracks whether new-message affordances should be shown while detached.
	const scrollByLines = (delta: number) => {
		const ref = listRef.current;
		if (!ref) return;
		if (delta < 0) {
			if (ref.scrollTop <= 0) return;
			if (follow) setFollow(false);
		}
		ref.scrollBy(delta);
		queueMicrotask(() => handleScrollPosition());
	};

	// Keyboard scrolling remains available while mouse wheel input is handled
	// natively by ScrollBox through OpenTUI's hit-test system.
	useInput(
		(input, key, event) => {
			if (key.ctrl && input === "r") {
				event.preventDefault();
				setAllReasoningExpanded((expanded) => {
					if (expanded) setExpandedReasoningIds(new Set());
					return !expanded;
				});
				return;
			}
			if (empty) return;
			const viewport = listRef.current?.viewport.height ?? 1;
			if (
				key.upArrow ||
				key.downArrow ||
				key.pageUp ||
				key.pageDown ||
				key.end
			) {
				event.preventDefault();
			}
			if (key.upArrow) scrollByLines(-1);
			else if (key.downArrow) scrollByLines(1);
			else if (key.pageUp) scrollByLines(-viewport);
			else if (key.pageDown) scrollByLines(viewport);
			else if (key.end) jumpToBottom();
		},
		{ isActive: interactive && !empty },
	);

	if (empty || height <= 0) {
		return (
			<Box height={Math.max(height, 0)} flexDirection="column" paddingX={1}>
				{header ? <Box>{header}</Box> : null}
				<Box flexGrow={1} justifyContent="center">
					<Text dimColor>
						Start chatting by typing a message below. Type /help for commands.
					</Text>
				</Box>
			</Box>
		);
	}

	const showHint = !follow && unseen > 0;
	const listHeight = Math.max(height, 1);
	const hintLabel = `↓ ${unseen} new message${unseen > 1 ? "s" : ""} · End to jump to latest`;

	return (
		<Box flexDirection="column" height={height} paddingX={1} overflow="hidden">
			<Box height={listHeight} overflowY="hidden">
				<scrollbox
					ref={listRef}
					height={listHeight}
					stickyScroll
					stickyStart="bottom"
					viewportCulling
					scrollY
					onMouseScroll={() => {
						queueMicrotask(() => handleScrollPosition());
					}}
					verticalScrollbarOptions={{ showArrows: false }}
				>
					{header ? (
						<Box key="__logo__" flexDirection="column">
							{header}
						</Box>
					) : null}
					{renderItems.map((item, i) => (
						<Box
							key={item.id}
							flexDirection="column"
							marginTop={i === 0 ? (header ? 1 : 0) : 1}
						>
							<MessageItem
								item={item}
								themeMode={themeMode}
								onToggleReasoning={(id) => {
									setExpandedReasoningIds((ids) => {
										const next = new Set(ids);
										if (next.has(id)) next.delete(id);
										else next.add(id);
										return next;
									});
								}}
							/>
						</Box>
					))}
				</scrollbox>
			</Box>
			{showHint ? (
				<Box height={1} marginTop={-2} justifyContent="center" overflow="hidden">
					<Text
						color={HINT_FOREGROUND}
						backgroundColor={HINT_BACKGROUND}
						onMouseClick={jumpToBottom}
					>
						{` ${hintLabel} `}
					</Text>
				</Box>
			) : null}
		</Box>
	);
}
