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
	ToolResult,
} from "../api/types";
import { loadOptions, parseThinking } from "../config";
import type { CliOptions } from "../config";
import { pickStreamingPhrase } from "../consts/streamingPhrases";
import { startLocalServer } from "../localServer";

export type MessageStatus = "idle" | "streaming" | "error";

export type OverlayKind =
	| "model-select"
	| "provider"
	| "session-select"
	| "config"
	| "help"
	| "mcp"
	| "skill"
	| "agents"
	| "status"
	| null;

export interface ToastMsg {
	text: string;
	error: boolean;
}

export interface UIToolCall {
	id: string;
	name: string;
	arguments: string;
	result?: ToolResult;
}

export interface UIMessage {
	id: string;
	role: "user" | "assistant" | "system";
	content: string;
	reasoning?: string;
	reasoningStartedAt?: number;
	reasoningEndedAt?: number;
	reasoningDurationMillis?: number;
	toolCalls?: UIToolCall[];
	error?: boolean;
}

type Phase = "loading" | "error" | "wizard" | "ready";

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

	thinkingEnabled: boolean;
	thinkingLevel: string;

	history: UIMessage[];
	current: UIMessage | null;
	status: MessageStatus;
	chatError: string | null;
	tokenConsumed: number;
	phrase: string | null;

	abortController: AbortController | null;
	_localServerStop: (() => Promise<void>) | null;

	init: () => Promise<void>;
	finishWizard: () => Promise<void>;
	sendMessage: (text: string) => Promise<void>;
	abort: () => void;
	reset: () => void;
	newSession: () => Promise<void>;
	switchModel: (model: ModelDto) => Promise<void>;
	resumeSession: (session: ChatSessionDto) => Promise<void>;
	switchAgent: (agent: AgentDto) => Promise<void>;
	setOverlay: (overlay: OverlayKind) => void;
	setToast: (text: string, error?: boolean) => void;
	notify: (text: string, error?: boolean) => void;
	setThinking: (enabled: boolean, level: string) => void;
	compactSession: () => Promise<void>;
	setCwd: (path: string | null) => Promise<void>;
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

function finalizeReasoning(msg: UIMessage): UIMessage {
	return msg.reasoning && !msg.reasoningEndedAt
		? { ...msg, reasoningEndedAt: Date.now() }
		: msg;
}

function messageToUI(msg: ChatMessageDto): UIMessage {
	return {
		id: msg.id || nextId(),
		role: msg.role as "user" | "assistant" | "system",
		content: msg.content,
		reasoning: msg.reasoningContent,
		reasoningDurationMillis: msg.reasoningDurationMillis,
		toolCalls: msg.toolCalls?.map((tc) => ({ ...tc })),
		error: false,
	};
}

function buildHistory(messages: ChatMessageDto[]): UIMessage[] {
	const history: UIMessage[] = [];
	for (const msg of messages) {
		if (msg.role === "tool") {
			const tr = msg.toolResult;
			if (tr) {
				for (let i = history.length - 1; i >= 0; i--) {
					const h = history[i];
					if (h.role !== "assistant" || !h.toolCalls) continue;
					const idx = h.toolCalls.findIndex((tc) => tc.id === tr.callId);
					if (idx >= 0) {
						h.toolCalls[idx] = { ...h.toolCalls[idx], result: tr };
						break;
					}
				}
			}
			continue;
		}
		history.push(messageToUI(msg));
	}
	return history;
}

export const useAppStore = create<AppState>((set, get) => {
	const flushCurrent = () => {
		const cur = get().current;
		if (cur && hasContent(cur)) {
			set((s) => ({
				history: [...s.history, finalizeReasoning(cur)],
				current: null,
			}));
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
		if (!tc) {
			return;
		}

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
			return {
				current: {
					...finalizeReasoning(cur),
					toolCalls: calls,
				},
			};
		});
	};

	const handleEvent = (evt: StreamEvent) => {
		switch (evt.type) {
			case StreamEventType.ContentDelta:
				if (evt.content) {
					set((s) => {
						let cur = s.current;
						let history = s.history;
						if (cur && cur.toolCalls?.some((c) => c.result)) {
							if (hasContent(cur)) {
								history = [...history, finalizeReasoning(cur)];
							}
							cur = newAssistantMsg();
						}
						cur = cur ?? newAssistantMsg();
						return {
							history,
							current: {
								...finalizeReasoning(cur),
								content: cur.content + evt.content,
							},
						};
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
								reasoningStartedAt: cur.reasoningStartedAt ?? Date.now(),
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
				if (evt.toolResult) {
					const tr = evt.toolResult;
					set((s) => {
						const cur = s.current ?? newAssistantMsg();
						const calls = cur.toolCalls ? [...cur.toolCalls] : [];
						const idx = calls.findIndex((c) => c.id === tr.callId);
						if (idx >= 0) {
							calls[idx] = { ...calls[idx], result: tr };
						} else {
							calls.push({ id: tr.callId, name: tr.name, arguments: "", result: tr });
						}
						return {
							current: {
								...finalizeReasoning(cur),
								toolCalls: calls,
							},
						};
					});
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

	const prepareAndReady = async (client: AgentyClient, opts: CliOptions) => {
		const prepared: PreparedSession = await client.prepareSession({
			agentRef: opts.agentRef,
			modelRef: opts.modelRef,
			newSession: opts.newSession,
		});
		const history = buildHistory(prepared.session.messages ?? []);
		set({
			phase: "ready",
			client,
			agent: prepared.agent,
			model: prepared.model,
			session: prepared.session,
			runtimeVersion: get().runtimeVersion,
			history,
			tokenConsumed: prepared.session.tokenConsumed,
			thinkingEnabled: parseThinking(opts.thinking).thinking,
			thinkingLevel: parseThinking(opts.thinking).thinkingLevel,
			initError: null,
		});
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
		thinkingEnabled: false,
		thinkingLevel: "",
		history: [],
		current: null,
		status: "idle",
		chatError: null,
		tokenConsumed: 0,
		phrase: null,
		abortController: null,
		_localServerStop: null,

		init: async () => {
			const opts = get().opts;
			try {
				let serverURL = opts.serverURL;
				if (opts.localMode) {
					const local = await startLocalServer({
						databasePath: opts.databasePath,
						debug: opts.backendDebug,
					});
					serverURL = local.url;
					set({ _localServerStop: local.stop });
				}
				const client = new AgentyClient(
					serverURL,
					opts.username,
					opts.password,
				);
				const version = await client.checkVersion();
				set({ runtimeVersion: version.version ?? "" });

				const initialized = await client.isInitialized();
				if (!initialized) {
					// First run: hand off to the setup wizard before preparing a session.
					set({ phase: "wizard", client, initError: null });
					return;
				}

				await prepareAndReady(client, opts);
			} catch (err) {
				set({ phase: "error", initError: (err as Error).message });
			}
		},

		finishWizard: async () => {
			const { client, opts } = get();
			if (!client) return;
			try {
				await client.setInitialized();
				await prepareAndReady(client, opts);
			} catch (err) {
				set({ phase: "error", initError: (err as Error).message });
			}
		},

		sendMessage: async (text) => {
			const trimmed = text.trim();
			if (!trimmed) return;
			const state = get();
			if (state.abortController) return;
			const { client, session, model } = state;
			if (!client || !session || !model) return;

			const userMsg: UIMessage = {
				id: nextId(),
				role: "user",
				content: trimmed,
			};
			const controller = new AbortController();
			const thinking = {
				thinking: state.thinkingEnabled,
				thinkingLevel: state.thinkingLevel,
			};

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
				const history = buildHistory(full.messages ?? []);
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

		switchAgent: async (agent) => {
			const { client } = get();
			if (!client) return;
			try {
				const session =
					(await client.getLastSessionByAgent(agent.id)) ??
					(await client.createSession(agent.id));
				const history = buildHistory(session.messages ?? []);
				let model = get().model;
				const agentModelIds = (agent.models ?? []).map((m) => m.id);
				if (
					!model ||
					(agentModelIds.length > 0 && !agentModelIds.includes(model.id))
				) {
					model = agent.models?.[0] ?? (await client.getDefaultModel());
				}
				set({
					agent,
					session,
					history,
					current: null,
					status: "idle",
					chatError: null,
					tokenConsumed: session.tokenConsumed,
					phrase: null,
					overlay: null,
					model,
				});
				setToast(`Switched to agent: ${agent.name}`);
			} catch (err) {
				pushSystem(`switch agent failed: ${(err as Error).message}`, true);
			}
		},

		setOverlay: (overlay) => set({ overlay }),

		setToast,

		notify: (text, error = false) => pushSystem(text, error),

		setThinking: (enabled, level) => {
			set({ thinkingEnabled: enabled, thinkingLevel: level });
			const label = enabled
				? level
					? `thinking enabled (${level} effort)`
					: "thinking enabled"
				: "thinking disabled";
			setToast(label);
		},

		compactSession: async () => {
			const { client, session, model, status } = get();
			if (!client || !session || !model) return;
			if (status === "streaming") {
				setToast("cannot compact while streaming", true);
				return;
			}
			try {
				const ok = await client.compactSessionForModel(session.id, model.id);
				setToast(ok ? "Context compacted." : "Compaction skipped (nothing to compact).");
			} catch (err) {
				setToast(`compact failed: ${(err as Error).message}`, true);
			}
		},

		setCwd: async (path) => {
			const { client, session } = get();
			if (!client || !session) return;
			try {
				await client.setSessionCwd(session.id, path);
				set((s) => ({
					session: s.session ? { ...s.session, cwd: path ?? undefined } : null,
				}));
				setToast(path ? `CWD set to ${path}` : "CWD cleared.");
			} catch (err) {
				setToast(`cwd failed: ${(err as Error).message}`, true);
			}
		},
	};
});
