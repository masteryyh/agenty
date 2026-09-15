import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act } from "react";

import type { PendingToolApproval } from "../state/store";
import { BottomDialog } from "./BottomDialog";
import { HitlOverlay } from "./HitlOverlay";

const approval: PendingToolApproval = {
    approvalId: "approval-1", sessionId: "session-1", roundId: "round-1",
    toolCall: { type: "tool_use", id: "call-1", name: "read_file", input: { path: "notes.txt" } },
    cwd: "/workspace", preview: { title: "Agenty wants to read this file: /workspace/notes.txt", detail: "Start line: 2\nEnd line: 5" },
    submitting: false, error: null,
};

describe("HitlOverlay", () => {
    for (const scenario of ["deny by default", "allow with arrow", "escape denies", "submitting"] as const) {
        test(scenario, async () => {
            const decisions: string[] = [];
            const setup = await testRender(
                <BottomDialog width={74} height={22}>
                    <HitlOverlay approval={{ ...approval, submitting: scenario === "submitting" }} onDecision={(value) => decisions.push(value)} />
                </BottomDialog>,
                { width: 76, height: 24 },
            );
            try {
                await act(async () => {
                    await setup.flush();
                });
                const frame = setup.captureCharFrame();
                expect(frame).toContain("Tool approval");
                expect(frame).toContain("Agenty wants to read this file:");
                expect(frame).toContain("[Deny]");
                if (scenario === "allow with arrow") {
                    await act(async () => {
                        setup.mockInput.pressArrow("left");
                        await setup.flush();
                    });
                }
                await act(async () => {
                    if (scenario === "escape denies") {
                        setup.mockInput.pressEscape();
                        await new Promise<void>((resolve) => setTimeout(resolve, 60));
                    } else {
                        setup.mockInput.pressEnter();
                        setup.mockInput.pressEnter();
                    }
                    await setup.flush();
                });
                expect(decisions).toEqual(scenario === "submitting" ? [] : [scenario === "allow with arrow" ? "allow" : "deny"]);
            } finally {
                act(() => setup.renderer.destroy());
            }
        });
    }

    test("keeps actions visible with long parameters in a small terminal and scrolls the preview", async () => {
        const setup = await testRender(
            <BottomDialog width={38} height={12}>
                <HitlOverlay
                    approval={{ ...approval, preview: { title: "Agenty wants to run these commands:", detail: Array.from({ length: 30 }, (_, i) => `command-${i}`).join("\n") } }}
                    onDecision={() => undefined}
                />
            </BottomDialog>,
            { width: 40, height: 14 },
        );
        try {
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Allow once");
            expect(setup.captureCharFrame()).toContain("[Deny]");
            await act(async () => {
                await setup.mockInput.pressKeys(Array.from({ length: 40 }, () => "ARROW_DOWN"), 5);
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("command-29");
            expect(setup.captureCharFrame()).toContain("[Deny]");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
