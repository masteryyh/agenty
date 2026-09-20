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

    test("cycles permission mode with Shift+Tab", async () => {
        let toggles = 0;
        const setup = await testRender(
            <BottomDialog width={74} height={22}>
                <HitlOverlay
                    approval={approval}
                    onDecision={() => undefined}
                    onTogglePermission={() => {
                        toggles++;
                    }}
                />
            </BottomDialog>,
            { width: 76, height: 24 },
        );
        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressTab({ shift: true });
                await setup.flush();
            });
            expect(toggles).toBe(1);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});

for (const message of [
    "Confirm discarding uncommitted changes.",
    "Auto review failed: invalid output; please review this action.",
]) {
    test(`shows review message: ${message}`, async () => {
        const setup = await testRender(
            <BottomDialog width={74} height={22}>
                <HitlOverlay approval={{ ...approval, message }} onDecision={() => undefined} />
            </BottomDialog>,
            { width: 76, height: 24 },
        );
        try {
            await act(async () => {
                await setup.flush();
            });
            const frame = setup.captureCharFrame();
            expect(frame).toContain(message);
            expect(frame).toContain("[Deny]");
            expect(frame).toContain("Allow once");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
}

test("wraps long review reasons in a small terminal while keeping actions visible", async () => {
    const setup = await testRender(
        <BottomDialog width={38} height={12}>
            <HitlOverlay
                approval={{ ...approval, message: "Confirm deleting local files because this operation discards uncommitted work and cannot be undone automatically." }}
                onDecision={() => undefined}
            />
        </BottomDialog>,
        { width: 40, height: 14 },
    );
    try {
        await act(async () => {
            await setup.flush();
        });
        expect(setup.captureCharFrame()).toContain("Confirm deleting local files");
        expect(setup.captureCharFrame()).toContain("[Deny]");
        await act(async () => {
            await setup.mockInput.pressKeys(Array.from({ length: 15 }, () => "ARROW_DOWN"), 5);
            await setup.flush();
        });
        expect(setup.captureCharFrame()).toContain("End line: 5");
        expect(setup.captureCharFrame()).toContain("Allow once");
    } finally {
        act(() => setup.renderer.destroy());
    }
});
