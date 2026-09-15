import { afterEach, describe, expect, test } from "bun:test";

import type { AgentyClient } from "../api/client";
import type {
    ChatSessionDto,
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
        toolCall: { type: "tool_use", id: `call-${id}`, name: "read_file", input: { path: "notes.txt" } },
        cwd: "/workspace",
        preview: { title: "Agenty wants to read this file: /workspace/notes.txt", detail: "Read the file contents." },
    };
}

afterEach(() => useAppStore.setState(useAppStore.getInitialState(), true));

function harness() {
    let listener: ((event: SessionEvent) => void) | undefined;
    let close: ((error: Error) => void) | undefined;
    let sequence = 0;
    const started = Promise.withResolvers<void>();
    const response = Promise.withResolvers<void>();
    const submissions: ToolApprovalResolution[] = [];
    const emit = (event: Partial<SessionEvent>) => listener?.({
        type: "round_started", sessionId: session.id, roundId: "round-1", sequence: ++sequence, ...event,
    });
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
    test("accepts approval before start response and preserves the next approval across a late decision response", async () => {
        const h = harness();
        const run = useAppStore.getState().sendMessage("read notes");
        await h.started.promise;
        expect(useAppStore.getState().pendingApproval?.approvalId).toBe("first");

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
});
