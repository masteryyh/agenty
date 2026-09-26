import { afterEach, describe, expect, test } from "bun:test";

import type { AgentyClient } from "../api/client";
import type {
    ChatSessionDto,
    PermissionMode,
    SessionEvent,
    ToolApprovalRequest,
    ToolApprovalResolution,
} from "../api/types";
import { useAppStore } from "./store";

const session: ChatSessionDto = {
    id: "hitl-session", rounds: [], contextWindow: 32000,
    createdAt: "2026-09-14T00:00:00Z", updatedAt: "2026-09-14T00:00:00Z",
};

function approval(id: string): ToolApprovalRequest {
    return {
        approvalId: id,
        message: "Confirm discarding uncommitted changes.",
        toolCall: { type: "tool_use", id: `call-${id}`, name: "read_file", input: { path: "notes.txt" } },
        cwd: "/workspace",
        preview: { title: "Agenty wants to read this file: /workspace/notes.txt", detail: "Read the file contents." },
    };
}

afterEach(() => useAppStore.setState(useAppStore.getInitialState(), true));

function harness(options: { sessionPermissionBeforeRound?: boolean } = {}) {
    let listener: ((event: SessionEvent) => void) | undefined;
    let close: ((error: Error) => void) | undefined;
    let sequence = 0;
    const nilRoundId = "00000000-0000-0000-0000-000000000000";
    const started = Promise.withResolvers<void>();
    const response = Promise.withResolvers<void>();
    const submissions: ToolApprovalResolution[] = [];
    const emit = (event: Partial<SessionEvent>) => {
        const sessionLevel = event.type === "permission_mode_changed" && event.roundId === nilRoundId;
        listener?.({
            type: "round_started", sessionId: session.id, roundId: "round-1",
            sequence: sessionLevel ? 1 : ++sequence, ...event,
        });
    };
    const client = {
        onSessionEvent(callback: (event: SessionEvent) => void) {
            listener = callback;
            return () => {
                listener = undefined;
            };
        },
        onClose(callback: (error: Error) => void) {
            close = callback;
            return () => {
                close = undefined;
            };
        },
        async setSessionReasoningEffort() {},
        async startSession() {
            if (options.sessionPermissionBeforeRound) {
                emit({
                    type: "permission_mode_changed",
                    roundId: nilRoundId,
                    permissionMode: "auto",
                });
            }
            emit({ type: "round_started" });
            emit({ type: "tool_approval_requested", approval: approval("first") });
            started.resolve();
            return { sessionId: session.id, roundId: "round-1", status: "running" };
        },
        async getSession() {
            return session;
        },
        async resolveToolApproval(resolution: ToolApprovalResolution) {
            submissions.push(resolution);
            await response.promise;
        },
    } as unknown as AgentyClient;
    useAppStore.setState({ ...useAppStore.getInitialState(), client, session, phase: "ready" });
    return { emit, started, response, submissions, disconnect: () => close?.(new Error("core disconnected")) };
}

describe("tool approval lifecycle", () => {
    test("cancels unfinished tools in the ended round, including partial streamed calls", async () => {
        const h = harness();
        useAppStore.setState({ history: [{
            id: "older", roundId: "older-round", role: "assistant", content: "",
            toolCalls: [{ id: "older-call", name: "lookup", arguments: "{}" }],
        }] });
        const run = useAppStore.getState().sendMessage("run tools");
        await h.started.promise;
        h.emit({
            type: "message_appended", iteration: 1,
            message: {
                id: "tools", roundId: "round-1", role: "assistant", createdAt: "2026-09-24T00:00:00Z",
                content: ["done", "unfinished"].map((id) => ({ type: "tool_use", id, name: "lookup", input: {} })),
            },
        });
        h.emit({
            type: "message_appended", iteration: 1,
            message: {
                id: "result", roundId: "round-1", role: "user", createdAt: "2026-09-24T00:00:00Z",
                content: [{ type: "tool_result", toolUseId: "done", isError: false, content: [{ type: "text", text: "found" }] }],
            },
        });
        h.emit({
            type: "model_stream", iteration: 2,
            stream: { type: "tool_use_start", index: 0, toolUseId: "partial", toolName: "shell" },
        });
        h.emit({ type: "tool_review_started", review: { toolUseId: "partial" } });
        h.emit({ type: "round_ended", status: "cancelled" });
        const current = useAppStore.getState().current?.toolCalls?.[0];
        expect(current).toMatchObject({ id: "partial", cancelled: true });
        expect(current?.reviewing).toBeUndefined();
        await run;
        const calls = useAppStore.getState().history.flatMap((message) => message.toolCalls ?? []);
        expect(calls.find((call) => call.id === "unfinished")?.cancelled).toBe(true);
        expect(calls.find((call) => call.id === "done")?.result?.content).toBe("found");
        expect(calls.find((call) => call.id === "done")?.cancelled).toBeUndefined();
        expect(calls.find((call) => call.id === "older-call")?.cancelled).toBeUndefined();
    });

    test("rebuilds cancelled tools when reopening a persisted session", async () => {
        const persisted: ChatSessionDto = { ...session, rounds: [{
            id: "cancelled-round", sessionId: session.id, sequence: 1, status: "cancelled",
            model: { providerCode: "test", modelCode: "test" }, contextWindow: 32000,
            startedAt: "2026-09-24T00:00:00Z", usage: { input: 0, output: 0, total: 0 },
            messages: [{
                id: "assistant", roundId: "cancelled-round", role: "assistant", createdAt: "2026-09-24T00:00:00Z",
                content: ["complete", "missing"].map((id) => ({ type: "tool_use", id, name: "lookup", input: {} })),
            }, {
                id: "result", roundId: "cancelled-round", role: "user", createdAt: "2026-09-24T00:00:00Z",
                content: [{ type: "tool_result", toolUseId: "complete", isError: true, content: [{ type: "text", text: "denied" }] }],
            }],
        }] };
        const client = { async getSession() {
            return persisted;
        } } as unknown as AgentyClient;
        useAppStore.setState({ ...useAppStore.getInitialState(), client, session });
        await useAppStore.getState().resumeSession(session);
        const calls = useAppStore.getState().history[0].toolCalls!;
        expect(calls[0].result?.isError).toBe(true);
        expect(calls[0].cancelled).toBeUndefined();
        expect(calls[1].cancelled).toBe(true);
    });
    test("accepts approval before start response and preserves the next approval across a late decision response", async () => {
        const h = harness();
        const run = useAppStore.getState().sendMessage("read notes");
        await h.started.promise;
        expect(useAppStore.getState().pendingApproval?.approvalId).toBe("first");
        expect(useAppStore.getState().pendingApproval?.message).toBe("Confirm discarding uncommitted changes.");

        const decision = useAppStore.getState().resolveToolApproval("deny");
        await useAppStore.getState().resolveToolApproval("allow");
        expect(h.submissions).toEqual([{ sessionId: session.id, roundId: "round-1", approvalId: "first", decision: "deny" }]);
        h.emit({ type: "tool_approval_resolved", resolution: h.submissions[0] });
        h.emit({ type: "tool_approval_requested", approval: approval("second") });
        h.response.resolve();
        await decision;
        expect(useAppStore.getState().pendingApproval?.approvalId).toBe("second");

        h.emit({ type: "tool_approval_requested", sessionId: "other-session", approval: approval("wrong-session") });
        h.emit({ type: "tool_approval_requested", roundId: "old-round", approval: approval("wrong-round") });
        expect(useAppStore.getState().pendingApproval?.approvalId).toBe("second");
        h.emit({ type: "round_ended", status: "completed" });
        expect(useAppStore.getState().pendingApproval).toBeNull();
        await run;
    });

    test("does not bind a session-level permission event to the active round", async () => {
        const h = harness({ sessionPermissionBeforeRound: true });
        const run = useAppStore.getState().sendMessage("read notes");
        await h.started.promise;

        expect(useAppStore.getState().session?.permissionMode).toBe("auto");
        expect(useAppStore.getState().history.some((message) => message.content.includes("sequence gap"))).toBe(false);
        h.emit({ type: "round_ended", status: "completed" });
        await run;
    });

    test("retains the request and error after a failed decision RPC", async () => {
        const h = harness();
        const run = useAppStore.getState().sendMessage("read notes");
        await h.started.promise;
        const decision = useAppStore.getState().resolveToolApproval("allow");
        h.response.reject(new Error("submission failed"));
        await decision;
        expect(useAppStore.getState().pendingApproval).toMatchObject({ approvalId: "first", submitting: false, error: "submission failed" });
        h.emit({ type: "round_ended", status: "cancelled" });
        await run;
        expect(useAppStore.getState().pendingApproval).toBeNull();
    });

    test("clears pending approval on transport closure", async () => {
        const h = harness();
        const run = useAppStore.getState().sendMessage("read notes");
        await h.started.promise;
        h.disconnect();
        expect(useAppStore.getState().pendingApproval).toBeNull();
        await run;
        expect(useAppStore.getState().chatError).toBe("core disconnected");
    });

    test("marks a tool as reviewing only between review lifecycle events", async () => {
        let listener: ((event: SessionEvent) => void) | undefined;
        let sequence = 0;
        const reviewStarted = Promise.withResolvers<void>();
        const finishReview = Promise.withResolvers<void>();
        const emit = (event: Partial<SessionEvent>) => listener?.({
            type: "round_started",
            sessionId: session.id,
            roundId: "round-review",
            sequence: ++sequence,
            ...event,
        });
        const client = {
            onSessionEvent(callback: (event: SessionEvent) => void) {
                listener = callback;
                return () => {
                    listener = undefined;
                };
            },
            onClose() {
                return () => {};
            },
            async setSessionReasoningEffort() {},
            async startSession() {
                emit({ type: "round_started" });
                emit({
                    type: "model_stream",
                    iteration: 1,
                    stream: { type: "tool_use_start", index: 0, toolUseId: "call-review", toolName: "shell" },
                });
                emit({
                    type: "model_stream",
                    iteration: 1,
                    stream: {
                        type: "tool_use_done",
                        index: 0,
                        toolUseId: "call-review",
                        toolName: "shell",
                        toolInput: { commands: ["printf hello"] },
                    },
                });
                emit({
                    type: "message_appended",
                    iteration: 1,
                    message: {
                        id: "assistant-review",
                        roundId: "round-review",
                        role: "assistant",
                        content: [{ type: "tool_use", id: "call-review", name: "shell", input: { commands: ["printf hello"] } }],
                        createdAt: "2026-09-20T00:00:00Z",
                    },
                });
                emit({ type: "tool_review_started", review: { toolUseId: "call-review" } });
                reviewStarted.resolve();
                await finishReview.promise;
                emit({ type: "tool_review_resolved", review: { toolUseId: "call-review" } });
                emit({ type: "round_ended", status: "completed" });
                return { sessionId: session.id, roundId: "round-review", status: "running" };
            },
            async getSession() {
                return session;
            },
        } as unknown as AgentyClient;
        useAppStore.setState({ ...useAppStore.getInitialState(), client, session, phase: "ready" });

        const run = useAppStore.getState().sendMessage("review this tool");
        await reviewStarted.promise;
        expect(useAppStore.getState().current?.toolCalls?.[0]).toMatchObject({
            id: "call-review",
            reviewing: true,
        });

        finishReview.resolve();
        await run;
        const reviewed = useAppStore.getState().history
            .flatMap((message) => message.toolCalls ?? [])
            .find((call) => call.id === "call-review");
        expect(reviewed?.reviewing).toBeUndefined();
    });
});

describe("pending permission selection", () => {
    test("cycles immediately and serializes RPCs without accepting stale responses", async () => {
        const firstStarted = Promise.withResolvers<void>();
        const releaseFirst = Promise.withResolvers<void>();
        const requests: PermissionMode[] = [];
        const client = {
            async setSessionPermissionMode(_id: string, mode: PermissionMode) {
                requests.push(mode);
                if (requests.length === 1) {
                    firstStarted.resolve();
                    await releaseFirst.promise;
                }
                return { ...session, permissionMode: "ask", pendingPermissionMode: mode === "ask" ? undefined : mode };
            },
        } as unknown as AgentyClient;
        useAppStore.setState({ ...useAppStore.getInitialState(), client, session: { ...session, permissionMode: "ask" } });
        const first = useAppStore.getState().togglePermissionMode();
        await firstStarted.promise;
        expect(useAppStore.getState().session?.pendingPermissionMode).toBe("auto");
        const second = useAppStore.getState().togglePermissionMode();
        expect(useAppStore.getState().session?.pendingPermissionMode).toBe("yolo");
        const third = useAppStore.getState().togglePermissionMode();
        expect(useAppStore.getState().session?.pendingPermissionMode).toBeUndefined();
        releaseFirst.resolve();
        await Promise.all([first, second, third]);
        expect(requests).toEqual(["auto", "ask"]);
        expect(useAppStore.getState().session?.permissionMode).toBe("ask");
        expect(useAppStore.getState().session?.pendingPermissionMode).toBeUndefined();
    });

    test("does not restore pending mode when the effective event precedes its RPC response", async () => {
        const h = harness();
        const run = useAppStore.getState().sendMessage("read");
        await h.started.promise;
        const client = useAppStore.getState().client!;
        client.setSessionPermissionMode = async () => {
            h.emit({ type: "permission_mode_changed", permissionMode: "yolo" });
            return { ...session, permissionMode: "ask", pendingPermissionMode: "yolo" };
        };
        await useAppStore.getState().setPermissionMode("yolo");
        expect(useAppStore.getState().session?.permissionMode).toBe("yolo");
        expect(useAppStore.getState().session?.pendingPermissionMode).toBeUndefined();
        expect(useAppStore.getState().toast?.text).toBe("permissions: yolo");
        h.emit({ type: "round_ended", status: "completed" });
        await run;
    });
});
