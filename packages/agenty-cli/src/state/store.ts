import { create } from "zustand";

import { AgentyClient } from "../api/client";
import type {
    AgentDto,
    ChatMessageDto,
    ChatSessionDto,
    CompactionEvent,
    ContentBlock,
    ModelDto,
    ReasoningEffort,
    SessionEvent,
    ToolResult,
} from "../api/types";
import type { CliOptions } from "../config";
import { loadOptions, parseThinking } from "../config";
import { pickStreamingPhrase } from "../consts/streamingPhrases";
import { startLocalCore } from "../localCore";

export type MessageStatus = "idle" | "streaming" | "compacting" | "error";
export type OverlayKind = "model-select" | "provider" | "session-select" | "help" | "agents" | "status" | null;
export type SystemMessageVariant = "compacted";
const TOAST_DURATION_MS = 3000;

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
    systemVariant?: SystemMessageVariant;
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
    activeSessionId: string | null;
    _localCoreStop: (() => Promise<void>) | null;
    init: () => Promise<void>;
    finishWizard: () => Promise<void>;
    sendMessage: (text: string) => Promise<void>;
    compactSession: () => Promise<void>;
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
    setCwd: (path: string | null) => Promise<void>;
}

let idCounter = 0;
function nextId(): string {
    idCounter += 1;
    return `msg-${idCounter}`;
}

function newAssistantMessage(): UIMessage {
    return { id: nextId(), role: "assistant", content: "", reasoning: "", toolCalls: [] };
}

function hasContent(message: UIMessage): boolean {
    return Boolean(message.content || message.reasoning || message.toolCalls?.length);
}

function finalizeReasoning(message: UIMessage): UIMessage {
    return message.reasoning && !message.reasoningEndedAt
        ? { ...message, reasoningEndedAt: Date.now() }
        : message;
}

function textFromBlocks(blocks: ContentBlock[], type: "text" | "reasoning"): string {
    return blocks
        .filter((block): block is Extract<ContentBlock, { type: typeof type }> => block.type === type)
        .map((block) => block.text)
        .join("");
}

function toolCallsFromBlocks(blocks: ContentBlock[]): UIToolCall[] {
    return blocks
        .filter((block): block is Extract<ContentBlock, { type: "tool_use" }> => block.type === "tool_use")
        .map((block) => ({
            id: block.id,
            name: block.name,
            arguments: typeof block.input === "string" ? block.input : JSON.stringify(block.input),
        }));
}

function messageToUI(message: ChatMessageDto): UIMessage {
    const content = message.content ?? [];
    return {
        id: message.id || nextId(),
        role: message.role,
        content: textFromBlocks(content, "text"),
        reasoning: textFromBlocks(content, "reasoning"),
        toolCalls: toolCallsFromBlocks(content),
    };
}

function toolResultText(blocks: ContentBlock[]): string {
    return textFromBlocks(blocks, "text") || JSON.stringify(blocks);
}

function attachToolResults(history: UIMessage[], message: ChatMessageDto): void {
    for (const block of message.content ?? []) {
        if (block.type !== "tool_result") {
            continue;
        }
        for (let index = history.length - 1; index >= 0; index--) {
            const calls = history[index].toolCalls;
            const callIndex = calls?.findIndex((call) => call.id === block.toolUseId) ?? -1;
            if (calls && callIndex >= 0) {
                calls[callIndex] = {
                    ...calls[callIndex],
                    result: {
                        callId: block.toolUseId,
                        name: calls[callIndex].name,
                        content: toolResultText(block.content),
                        isError: block.isError,
                    },
                };
                break;
            }
        }
    }
}

function buildHistory(session: ChatSessionDto): UIMessage[] {
    const history: UIMessage[] = [];
    for (const round of session.rounds ?? []) {
        for (const message of round.messages ?? []) {
            if ((message.content ?? []).some((block) => block.type === "tool_result")) {
                attachToolResults(history, message);
                continue;
            }
            history.push(messageToUI(message));
        }
    }
    return history;
}

function actualContextSize(session: ChatSessionDto): number {
    for (let roundIndex = (session.rounds ?? []).length - 1; roundIndex >= 0; roundIndex -= 1) {
        const messages = session.rounds[roundIndex]?.messages ?? [];
        for (let messageIndex = messages.length - 1; messageIndex >= 0; messageIndex -= 1) {
            const message = messages[messageIndex];
            if (message.role === "assistant" && message.usage) {
                return message.usage.input + message.usage.output;
            }
        }
    }
    return 0;
}

function reasoningEffort(enabled: boolean, level: string): ReasoningEffort {
    if (!enabled) {
        return "off";
    }
    if (level === "low" || level === "medium" || level === "high" || level === "xhigh" || level === "max") {
        return level;
    }
    return "medium";
}

export const useAppStore = create<AppState>((set, get) => {
    const flushCurrent = () => {
        const current = get().current;
        if (current && hasContent(current)) {
            set((state) => ({ history: [...state.history, finalizeReasoning(current)], current: null }));
            return;
        }
        set({ current: null });
    };

    const pushSystem = (
        text: string,
        error = false,
        systemVariant?: SystemMessageVariant,
    ) => {
        set((state) => ({
            history: [...state.history, {
                id: nextId(),
                role: "system",
                content: text,
                error,
                systemVariant,
            }],
        }));
    };

    const setToast = (text: string, error = false) => {
        set({ toast: { text, error } });
        setTimeout(() => {
            set((state) => state.toast?.text === text ? { toast: null } : {});
        }, TOAST_DURATION_MS);
    };

    const handleEvent = (event: SessionEvent) => {
        if (event.type === "model_stream" && event.stream) {
            const stream = event.stream;
            if (stream.type === "text_delta" && stream.delta) {
                set((state) => {
                    let current = state.current ?? newAssistantMessage();
                    let history = state.history;
                    if (current.toolCalls?.some((call) => call.result)) {
                        history = [...history, finalizeReasoning(current)];
                        current = newAssistantMessage();
                    }
                    return {
                        history,
                        current: { ...finalizeReasoning(current), content: current.content + stream.delta },
                    };
                });
            } else if (stream.type === "reasoning_delta" && stream.delta) {
                set((state) => {
                    let current = state.current ?? newAssistantMessage();
                    let history = state.history;
                    if (current.toolCalls?.some((call) => call.result)) {
                        history = [...history, finalizeReasoning(current)];
                        current = newAssistantMessage();
                    }
                    return { history, current: {
                        ...current,
                        reasoningStartedAt: current.reasoningStartedAt ?? Date.now(),
                        reasoning: `${current.reasoning ?? ""}${stream.delta}`,
                    } };
                });
            } else if (stream.type === "tool_use_start") {
                set((state) => {
                    const current = finalizeReasoning(state.current ?? newAssistantMessage());
                    return { current: { ...current, toolCalls: [...(current.toolCalls ?? []), {
                        id: stream.toolUseId ?? `tool-${event.iteration ?? 0}-${stream.index ?? 0}`,
                        name: stream.toolName ?? "tool",
                        arguments: "",
                    }] } };
                });
            } else if (stream.type === "tool_input_delta" && stream.delta) {
                set((state) => {
                    const current = state.current ?? newAssistantMessage();
                    const calls = [...(current.toolCalls ?? [])];
                    const index = stream.toolUseId ? calls.findIndex((call) => call.id === stream.toolUseId) : calls.length - 1;
                    if (index >= 0) {
                        calls[index] = { ...calls[index], arguments: calls[index].arguments + stream.delta };
                    }
                    return { current: { ...current, toolCalls: calls } };
                });
            } else if (stream.type === "tool_use_done") {
                set((state) => {
                    const current = state.current ?? newAssistantMessage();
                    const calls = [...(current.toolCalls ?? [])];
                    const index = stream.toolUseId ? calls.findIndex((call) => call.id === stream.toolUseId) : calls.length - 1;
                    if (index >= 0) {
                        calls[index] = {
                            ...calls[index],
                            name: stream.toolName ?? calls[index].name,
                            arguments: stream.toolInput === undefined ? calls[index].arguments : JSON.stringify(stream.toolInput),
                        };
                    }
                    return { current: { ...current, toolCalls: calls } };
                });
            }
            return;
        }

        if (event.type === "message_appended" && event.message) {
            if ((event.message.content ?? []).some((block) => block.type === "tool_result")) {
                set((state) => {
                    const current = state.current ?? newAssistantMessage();
                    const holder = [current];
                    attachToolResults(holder, event.message!);
                    return { current: holder[0] };
                });
            } else if (event.message.role === "assistant") {
                set((state) => {
                    let current = state.current ?? newAssistantMessage();
                    let history = state.history;
                    if (current.toolCalls?.some((call) => call.result)) {
                        history = [...history, finalizeReasoning(current)];
                        current = newAssistantMessage();
                    }
                    const contextSize = event.message?.usage
                        ? event.message.usage.input + event.message.usage.output
                        : state.tokenConsumed;
                    if (hasContent(current)) {
                        return { history, tokenConsumed: contextSize };
                    }
                    return { history, current: messageToUI(event.message!), tokenConsumed: contextSize };
                });
            }
        }
    };

    const handleCompactionEvent = (event: CompactionEvent) => {
        if (event.sessionId !== get().session?.id) {
            return;
        }
        if (event.type === "started") {
            flushCurrent();
            set({
                status: "compacting",
                phrase: "Session compacting...",
                activeSessionId: event.sessionId,
            });
        } else if (event.type === "failed" && event.error) {
            pushSystem(`Session compaction failed: ${event.error}`, true);
        } else if (event.type === "completed") {
            if (get().status === "compacting") {
                set({ status: "streaming", phrase: pickStreamingPhrase() });
            }
            pushSystem("Session compacted.", false, "compacted");
        }
    };

    const prepareAndReady = async (client: AgentyClient, options: CliOptions) => {
        const parsed = parseThinking(options.thinking);
        const prepared = await client.prepareSession({
            agentRef: options.agentRef,
            modelRef: options.modelRef,
            newSession: options.newSession,
            reasoningEffort: reasoningEffort(parsed.thinking, parsed.thinkingLevel),
        });
        set({
            phase: "ready",
            client,
            agent: prepared.agent,
            model: prepared.model,
            session: prepared.session,
            history: buildHistory(prepared.session),
            tokenConsumed: actualContextSize(prepared.session),
            thinkingEnabled: parsed.thinking,
            thinkingLevel: parsed.thinkingLevel,
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
        activeSessionId: null,
        _localCoreStop: null,

        init: async () => {
            try {
                const options = get().opts;
                const local = await startLocalCore({ dataDir: options.dataDir });
                const client = new AgentyClient(local.rpc);
                set({ client, _localCoreStop: local.stop });
                client.onCompactionEvent(handleCompactionEvent);
                if (!(await client.isInitialized())) {
                    set({ phase: "wizard", initError: null });
                    return;
                }
                await prepareAndReady(client, options);
            } catch (error) {
                set({ phase: "error", initError: (error as Error).message });
            }
        },

        finishWizard: async () => {
            const { client, opts } = get();
            if (!client) {
                return;
            }
            try {
                await prepareAndReady(client, opts);
            } catch (error) {
                set({ phase: "error", initError: (error as Error).message });
            }
        },

        sendMessage: async (text) => {
            const trimmed = text.trim();
            const state = get();
            if (!trimmed || (state.status !== "idle" && state.status !== "error") || !state.client || !state.session) {
                return;
            }
            const { client, session } = state;
            set((currentState) => ({
                history: [...currentState.history, { id: nextId(), role: "user", content: trimmed }],
                current: newAssistantMessage(),
                status: "streaming",
                chatError: null,
                phrase: pickStreamingPhrase(),
                activeSessionId: session.id,
            }));

            let resolveTerminal!: (event: SessionEvent) => void;
            let rejectTerminal!: (reason: Error) => void;
            const terminal = new Promise<SessionEvent>((resolve, reject) => {
                resolveTerminal = resolve;
                rejectTerminal = reject;
            });
            const unsubscribeClose = client.onClose(rejectTerminal);
            let lastSequence = 0;
            const unsubscribe = client.onSessionEvent((event) => {
                if (event.sessionId !== session.id) {
                    return;
                }
                if (event.sequence !== lastSequence + 1) {
                    pushSystem(`session event sequence gap: received ${event.sequence} after ${lastSequence}`, true);
                }
                lastSequence = event.sequence;
                handleEvent(event);
                if (event.type === "round_ended") {
                    resolveTerminal(event);
                }
            });

            try {
                await client.setSessionReasoningEffort(
                    session.id,
                    reasoningEffort(state.thinkingEnabled, state.thinkingLevel),
                );
                await client.startSession(session.id, trimmed);
                const ended = await terminal;
                if (ended.status === "failed" || ended.error) {
                    const message = ended.error ?? "agent round failed";
                    set({ chatError: message });
                    pushSystem(message, true);
                }
                const updated = await client.getSession(session.id);
                set({ session: updated, tokenConsumed: actualContextSize(updated) });
            } catch (error) {
                const message = (error as Error).message;
                set({ chatError: message });
                pushSystem(message, true);
            } finally {
                unsubscribe();
                unsubscribeClose();
                flushCurrent();
                set({ status: "idle", phrase: null, activeSessionId: null });
            }
        },

        compactSession: async () => {
            const { client, session, status } = get();
            if (!client || !session || (status !== "idle" && status !== "error")) {
                return;
            }
            set({ status: "compacting", phrase: "Session compacting...", activeSessionId: session.id, chatError: null });
            try {
                await client.compactSession(session.id);
                const updated = await client.getSession(session.id);
                set({ session: updated, tokenConsumed: actualContextSize(updated) });
            } catch (error) {
                const message = (error as Error).message;
                set({ chatError: message });
                pushSystem(`Session compaction failed: ${message}`, true);
            } finally {
                set({ status: "idle", phrase: null, activeSessionId: null });
            }
        },

        abort: () => {
            const { client, activeSessionId } = get();
            if (client && activeSessionId) {
                void client.stopSession(activeSessionId).catch((error: unknown) =>
                    pushSystem(error instanceof Error ? error.message : String(error), true));
            }
        },

        reset: () => {
            get().abort();
            set({
                phase: "loading",
                initError: null,
                history: [],
                current: null,
                status: "idle",
                chatError: null,
                tokenConsumed: 0,
                phrase: null,
                activeSessionId: null,
                overlay: null,
            });
        },

        newSession: async () => {
            const { client, agent, model, thinkingEnabled, thinkingLevel } = get();
            if (!client || !agent || !model) {
                return;
            }
            try {
                const session = await client.createSession(agent.slug, model, reasoningEffort(thinkingEnabled, thinkingLevel));
                set({ session, history: [], current: null, tokenConsumed: 0, overlay: null });
                setToast("New session created.");
            } catch (error) {
                pushSystem(`new session failed: ${(error as Error).message}`, true);
            }
        },

        switchModel: async (model) => {
            const { client, session } = get();
            if (!client || !session) {
                return;
            }
            try {
                const updated = await client.setSessionModel(session.id, model);
                set({
                    model,
                    session: updated,
                    tokenConsumed: actualContextSize(updated),
                    overlay: null,
                    status: "idle",
                    phrase: null,
                    activeSessionId: null,
                });
                setToast(`Switched to ${model.providerName}/${model.name}`);
            } catch (error) {
                set({ status: "idle", phrase: null, activeSessionId: null });
                pushSystem(`switch model failed: ${(error as Error).message}`, true);
            }
        },

        resumeSession: async (session) => {
            const { client } = get();
            if (!client) {
                return;
            }
            try {
                const full = await client.getSession(session.id);
                const model = full.currentModel
                    ? await client.resolveModel(`${full.currentModel.providerSlug}/${full.currentModel.modelSlug}`)
                    : get().model;
                set({ session: full, model, history: buildHistory(full), current: null, tokenConsumed: actualContextSize(full), overlay: null });
            } catch (error) {
                pushSystem(`resume failed: ${(error as Error).message}`, true);
            }
        },

        switchAgent: async (agent) => {
            const { client } = get();
            if (!client) {
                return;
            }
            try {
                const model = agent.defaultModel
                    ? await client.resolveModel(`${agent.defaultModel.providerSlug}/${agent.defaultModel.modelSlug}`)
                    : await client.getDefaultModel();
                const session = await client.getLastSessionByAgent(agent.slug) ?? await client.createSession(agent.slug, model);
                set({ agent, model, session, history: buildHistory(session), current: null, tokenConsumed: actualContextSize(session), overlay: null });
                setToast(`Switched to agent: ${agent.name}`);
            } catch (error) {
                pushSystem(`switch agent failed: ${(error as Error).message}`, true);
            }
        },

        setOverlay: (overlay) => set({ overlay }),
        setToast,
        notify: (text, error = false) => pushSystem(text, error),
        setThinking: (enabled, level) => {
            set({ thinkingEnabled: enabled, thinkingLevel: level });
            setToast(enabled ? `thinking enabled (${level || "medium"} effort)` : "thinking disabled");
        },
        setCwd: async (path) => {
            const { client, session } = get();
            if (!client || !session) {
                return;
            }
            try {
                const updated = await client.setSessionCwd(session.id, path);
                set({ session: updated });
                setToast(path ? `CWD set to ${path}` : "CWD cleared.");
            } catch (error) {
                setToast(`cwd failed: ${(error as Error).message}`, true);
            }
        },
    };
});
