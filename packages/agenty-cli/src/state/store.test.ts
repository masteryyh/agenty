import { describe, expect, test } from "bun:test";

import type { AgentyClient } from "../api/client";
import type { ChatSessionDto, SessionEvent } from "../api/types";
import { useAppStore } from "./store";

const session: ChatSessionDto = {
    id: "session-1",
    agentSlug: "default",
    currentModel: { providerSlug: "provider", modelSlug: "model" },
    contextWindow: 32_000,
    rounds: [],
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
};

function makeEvent(sequence: number, event: Omit<SessionEvent, "sessionId" | "roundId" | "sequence">): SessionEvent {
    return {
        sessionId: session.id,
        roundId: "round-1",
        sequence,
        ...event,
    };
}

describe("chat tool event projection", () => {
    test("upserts duplicate starts and upgrades placeholder ids before attaching results", async () => {
        const listeners = new Set<(event: SessionEvent) => void>();
        const client = {
            onSessionEvent(listener: (event: SessionEvent) => void) {
                listeners.add(listener);
                return () => listeners.delete(listener);
            },
            onClose() {
                return () => {};
            },
            async setSessionReasoningEffort() {
                return session;
            },
            async startSession() {
                const events = [
                    makeEvent(1, { type: "round_started" }),
                    makeEvent(2, {
                        type: "model_stream",
                        iteration: 1,
                        stream: { type: "tool_use_start", index: 0, toolName: "read_file" },
                    }),
                    makeEvent(3, {
                        type: "model_stream",
                        iteration: 1,
                        stream: { type: "tool_use_start", index: 0, toolUseId: "call-1", toolName: "read_file" },
                    }),
                    makeEvent(4, {
                        type: "model_stream",
                        iteration: 1,
                        stream: { type: "tool_use_start", index: 0, toolUseId: "call-1", toolName: "read_file" },
                    }),
                    makeEvent(5, {
                        type: "model_stream",
                        iteration: 1,
                        stream: { type: "tool_input_delta", index: 0, toolUseId: "call-1", delta: "{\"path\":\"README.md\"}" },
                    }),
                    makeEvent(6, {
                        type: "model_stream",
                        iteration: 1,
                        stream: {
                            type: "tool_use_done",
                            index: 0,
                            toolUseId: "call-1",
                            toolName: "read_file",
                            toolInput: { path: "README.md" },
                        },
                    }),
                    makeEvent(7, {
                        type: "message_appended",
                        iteration: 1,
                        message: {
                            id: "assistant-1",
                            roundId: "round-1",
                            role: "assistant",
                            content: [{ type: "tool_use", id: "call-1", name: "read_file", input: { path: "README.md" } }],
                            createdAt: "2026-01-01T00:00:01Z",
                        },
                    }),
                    makeEvent(8, {
                        type: "message_appended",
                        iteration: 1,
                        message: {
                            id: "tool-result-1",
                            roundId: "round-1",
                            role: "user",
                            content: [{
                                type: "tool_result",
                                toolUseId: "call-1",
                                content: [{ type: "text", text: "ok" }],
                                isError: false,
                            }],
                            createdAt: "2026-01-01T00:00:02Z",
                        },
                    }),
                    makeEvent(9, {
                        type: "model_stream",
                        iteration: 2,
                        stream: { type: "tool_use_start", index: 0, toolUseId: "call-2", toolName: "shell" },
                    }),
                    makeEvent(10, {
                        type: "model_stream",
                        iteration: 2,
                        stream: { type: "tool_use_start", index: 0, toolUseId: "call-2", toolName: "shell" },
                    }),
                    makeEvent(11, {
                        type: "model_stream",
                        iteration: 2,
                        stream: {
                            type: "tool_use_done",
                            index: 0,
                            toolUseId: "call-2",
                            toolName: "shell",
                            toolInput: { commands: ["pwd"] },
                        },
                    }),
                    makeEvent(12, {
                        type: "message_appended",
                        iteration: 2,
                        message: {
                            id: "assistant-2",
                            roundId: "round-1",
                            role: "assistant",
                            content: [{ type: "tool_use", id: "call-2", name: "shell", input: { commands: ["pwd"] } }],
                            createdAt: "2026-01-01T00:00:03Z",
                        },
                    }),
                    makeEvent(13, {
                        type: "message_appended",
                        iteration: 2,
                        message: {
                            id: "tool-result-2",
                            roundId: "round-1",
                            role: "user",
                            content: [{
                                type: "tool_result",
                                toolUseId: "call-2",
                                content: [{ type: "text", text: "/tmp" }],
                                isError: false,
                            }],
                            createdAt: "2026-01-01T00:00:04Z",
                        },
                    }),
                    makeEvent(14, { type: "round_ended", status: "completed" }),
                ];
                for (const event of events) {
                    for (const listener of listeners) {
                        listener(event);
                    }
                }
                return { sessionId: session.id, roundId: "round-1", status: "running" as const };
            },
            async getSession() {
                return session;
            },
        } as unknown as AgentyClient;

        useAppStore.setState({
            client,
            session,
            history: [],
            current: null,
            status: "idle",
            activeSessionId: null,
            thinkingEnabled: false,
            thinkingLevel: "",
            chatError: null,
            phrase: null,
        });

        await useAppStore.getState().sendMessage("inspect the file");

        const messages = useAppStore.getState().history;
        const toolCalls = messages
            .filter((message) => message.role === "assistant")
            .flatMap((message) => message.toolCalls ?? []);
        expect(toolCalls).toHaveLength(2);
        expect(toolCalls.find((call) => call.id === "call-1")).toMatchObject({
            name: "read_file",
            result: {
                callId: "call-1",
                content: "ok",
                isError: false,
            },
        });
        expect(toolCalls.find((call) => call.id === "call-2")).toMatchObject({
            name: "shell",
            result: {
                callId: "call-2",
                content: "/tmp",
                isError: false,
            },
        });
    });
});
