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

import { create } from "zustand";
import { AgentyClient } from "../api/client";
import type { PreparedSession } from "../api/client";
import { StreamEventType } from "../api/types";
import type {
	AgentDto,
	ChatDto,
	ChatMessageDto,
	ChatSessionDto,
	ModelDto,
	StreamEvent,
	ToolCall,
	ToolResult,
} from "../api/types";
import { loadOptions, parseThinking } from "../config";
import type { CliOptions } from "../config";
import { pickStreamingPhrase } from "../consts/streamingPhrases";

export type MessageStatus = "idle" | "streaming" | "error";

export type OverlayKind =
	| "model-select"
	| "provider"
	| "session-select"
	| "config"
	| "help"
	| null;

export interface ToastMsg {
	text: string;
	error: boolean;
}

export interface UIMessage {
	id: string;
	role: "user" | "assistant" | "tool" | "system";
	content: string;
	reasoning?: string;
	toolCalls?: ToolCall[];
	toolResult?: ToolResult;
	error?: boolean;
}

type Phase = "loading" | "error" | "ready";

interface AppState {
	phase: Phase;
	initError: string | null;
	opts: CliOptions;
	client: AgentyClient | null;
	agent: AgentDto | null;
	model: ModelDto | null;
	session: ChatSessionDto | null;
	runtimeVersion: string;
	overlay: OverlayKind;
	toast: ToastMsg | null;

	history: UIMessage[];
	current: UIMessage | null;
	status: MessageStatus;
	chatError: string | null;
	tokenConsumed: number;
	phrase: string | null;

	abortController: AbortController | null;

	init: () => Promise<void>;
	sendMessage: (text: string) => Promise<void>;
	abort: () => void;
	reset: () => void;
	newSession: () => Promise<void>;
	switchModel: (model: ModelDto) => Promise<void>;
	resumeSession: (session: ChatSessionDto) => Promise<void>;
	setOverlay: (overlay: OverlayKind) => void;
	setToast: (text: string, error?: boolean) => void;
	notify: (text: string, error?: boolean) => void;
}

let idCounter = 0;
function nextId(): string {
	idCounter += 1;
	return `msg-${idCounter}`;
}

function newAssistantMsg(): UIMessage {
	return {
		id: nextId(),
		role: "assistant",
		content: "",
		reasoning: "",
		toolCalls: [],
	};
}

function hasContent(msg: UIMessage): boolean {
	return !!(
		msg.content ||
		msg.reasoning ||
		(msg.toolCalls && msg.toolCalls.length > 0)
	);
}

function messageToUi(msg: ChatMessageDto): UIMessage {
	return {
		id: msg.id || nextId(),
		role: msg.role,
		content: msg.content,
		reasoning: msg.reasoningContent,
		toolCalls: msg.toolCalls,
		toolResult: msg.toolResult,
		error: false,
	};
}

export const useAppStore = create<AppState>((set, get) => {
	const flushCurrent = () => {
		const cur = get().current;
		if (cur && hasContent(cur)) {
			set((s) => ({ history: [...s.history, cur], current: null }));
		} else {
			set({ current: null });
		}
	};

	const pushSystem = (text: string, error = false) => {
		set((s) => ({
			history: [
				...s.history,
				{ id: nextId(), role: "system", content: text, error },
			],
		}));
	};

	const setToast = (text: string, error = false) => {
		set({ toast: { text, error } });
		setTimeout(() => {
			set((s) => (s.toast?.text === text ? { toast: null } : {}));
		}, 5000);
	};

	const handleToolCall = (evt: StreamEvent) => {
		const tc = evt.toolCall;
		if (!tc) return;
		set((s) => {
			const cur = s.current ?? newAssistantMsg();
			const calls = cur.toolCalls ? [...cur.toolCalls] : [];
			if (evt.type === StreamEventType.ToolCallStart) {
				calls.push({ id: tc.id, name: tc.name, arguments: tc.arguments });
			} else if (evt.type === StreamEventType.ToolCallDelta) {
				const idx = tc.id
					? calls.findIndex((c) => c.id === tc.id)
					: calls.length - 1;
				if (idx >= 0) {
					calls[idx] = {
						...calls[idx],
						arguments: calls[idx].arguments + (tc.arguments || ""),
					};
				}
			} else {
				const idx = tc.id
					? calls.findIndex((c) => c.id === tc.id)
					: calls.length - 1;
				if (idx >= 0) {
					calls[idx] = {
						...calls[idx],
						name: tc.name || calls[idx].name,
						arguments: tc.arguments || calls[idx].arguments,
					};
				} else {
					calls.push({ id: tc.id, name: tc.name, arguments: tc.arguments });
				}
			}
			return { current: { ...cur, toolCalls: calls } };
		});
	};

	const handleEvent = (evt: StreamEvent) => {
		switch (evt.type) {
			case StreamEventType.ContentDelta:
				if (evt.content) {
					set((s) => {
						const cur = s.current ?? newAssistantMsg();
						return { current: { ...cur, content: cur.content + evt.content } };
					});
				}
				break;
			case StreamEventType.ReasoningDelta:
				if (evt.reasoning) {
					set((s) => {
						const cur = s.current ?? newAssistantMsg();
						return {
							current: {
								...cur,
								reasoning: (cur.reasoning ?? "") + evt.reasoning,
							},
						};
					});
				}
				break;
			case StreamEventType.ToolCallStart:
			case StreamEventType.ToolCallDelta:
			case StreamEventType.ToolCallDone:
				handleToolCall(evt);
				break;
			case StreamEventType.ToolResult:
				flushCurrent();
				if (evt.toolResult) {
					set((s) => ({
						history: [
							...s.history,
							{
								id: nextId(),
								role: "tool",
								content: "",
								toolResult: evt.toolResult,
							},
						],
					}));
				}
				break;
			case StreamEventType.MessageDone:
				flushCurrent();
				break;
			case StreamEventType.Usage:
				if (evt.usage) set({ tokenConsumed: evt.usage.totalTokens });
				break;
			case StreamEventType.Error: {
				flushCurrent();
				const msg = evt.error || "unknown error";
				set({ chatError: msg });
				pushSystem(msg, true);
				break;
			}
			case StreamEventType.ModelSwitch:
			case StreamEventType.CompactionStart:
			case StreamEventType.CompactionDone:
				break;
		}
	};

	return {
		phase: "loading",
		initError: null,
		opts: loadOptions(),
		client: null,
		agent: null,
		model: null,
		session: null,
		runtimeVersion: "",
		overlay: null,
		toast: null,
		history: [],
		current: null,
		status: "idle",
		chatError: null,
		tokenConsumed: 0,
		phrase: null,
		abortController: null,

		init: async () => {
			const opts = get().opts;
			const client = new AgentyClient(
				opts.serverURL,
				opts.username,
				opts.password,
			);
			try {
				const version = await client.checkVersion();
				const prepared: PreparedSession = await client.prepareSession({
					agentRef: opts.agentRef,
					modelRef: opts.modelRef,
					newSession: opts.newSession,
				});
				const history = (prepared.session.messages ?? []).map(messageToUi);
				set({
					phase: "ready",
					client,
					agent: prepared.agent,
					model: prepared.model,
					session: prepared.session,
					runtimeVersion: version.version ?? "",
					history,
					tokenConsumed: prepared.session.tokenConsumed,
					initError: null,
				});
			} catch (err) {
				set({ phase: "error", initError: (err as Error).message });
			}
		},

		sendMessage: async (text) => {
			const trimmed = text.trim();
			if (!trimmed) return;
			const state = get();
			if (state.abortController) return;
			const { client, session, model, opts } = state;
			if (!client || !session || !model) return;

			const userMsg: UIMessage = {
				id: nextId(),
				role: "user",
				content: trimmed,
			};
			const controller = new AbortController();
			const thinking = parseThinking(opts.thinking);

			set((s) => ({
				history: [...s.history, userMsg],
				current: newAssistantMsg(),
				status: "streaming",
				chatError: null,
				phrase: pickStreamingPhrase(),
				abortController: controller,
			}));

			const dto: ChatDto = {
				modelId: model.id,
				message: trimmed,
				thinking: thinking.thinking,
				thinkingLevel: thinking.thinkingLevel,
			};

			try {
				await client.streamChat(
					session.id,
					dto,
					handleEvent,
					controller.signal,
				);
			} catch (err) {
				if (!controller.signal.aborted) {
					const msg = (err as Error).message;
					set({ chatError: msg });
					pushSystem(msg, true);
				}
			} finally {
				flushCurrent();
				set({ status: "idle", abortController: null, phrase: null });
			}
		},

		abort: () => {
			const { abortController } = get();
			abortController?.abort();
			flushCurrent();
			set({ status: "idle", abortController: null, phrase: null });
		},

		reset: () => {
			get().abortController?.abort();
			set({
				phase: "loading",
				initError: null,
				history: [],
				current: null,
				status: "idle",
				chatError: null,
				tokenConsumed: 0,
				phrase: null,
				abortController: null,
				overlay: null,
			});
		},

		switchModel: async (model) => {
			const { client, session } = get();
			if (!client || !session) return;
			try {
				await client.compactSessionForModel(session.id, model.id);
			} catch (err) {
				pushSystem(`compact failed: ${(err as Error).message}`, true);
			}
			set({ model, overlay: null });
			setToast(`Switched to ${model.provider?.name ?? "?"}/${model.name}`);
		},

		newSession: async () => {
			const { client, agent } = get();
			if (!client || !agent) return;
			try {
				const session = await client.createSession(agent.id);
				set({
					session,
					history: [],
					current: null,
					status: "idle",
					chatError: null,
					tokenConsumed: 0,
					phrase: null,
					overlay: null,
				});
				setToast("New session created.");
			} catch (err) {
				pushSystem(`new session failed: ${(err as Error).message}`, true);
			}
		},

		resumeSession: async (session) => {
			const { client } = get();
			if (!client) return;
			try {
				const full = await client.getSession(session.id);
				const history = (full.messages ?? []).map(messageToUi);
				set({
					session: full,
					history,
					current: null,
					status: "idle",
					chatError: null,
					tokenConsumed: full.tokenConsumed,
					phrase: null,
					overlay: null,
				});
			} catch (err) {
				pushSystem(`resume failed: ${(err as Error).message}`, true);
			}
		},

		setOverlay: (overlay) => set({ overlay }),

		setToast,

		notify: (text, error = false) => pushSystem(text, error),
	};
});
