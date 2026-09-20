import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act } from "react";

import { BottomDialog } from "./BottomDialog";
import { HelpPanel } from "./HelpPanel";

describe("HelpPanel", () => {
    test("renders commands, syntax and shortcut sections without /help", async () => {
        const setup = await testRender(
            <BottomDialog width={76} height={22}>
                <HelpPanel onClose={() => undefined} />
            </BottomDialog>,
            { width: 80, height: 24 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });
            const frame = setup.captureCharFrame();

            expect(frame).toContain("Commands");
            expect(frame.split("\n").some((line) => line.includes("Commands") && line.includes("Syntax"))).toBe(true);
            expect(frame).toContain("Syntax");
            expect(frame).not.toContain("Shortcuts");
            expect(frame).not.toContain("...");
            expect(frame).not.toContain("/help");
            expect(frame).not.toContain("/model [");
            expect(frame).not.toContain("/cwd [");

            await act(async () => {
                for (let index = 0; index < 50; index += 1) {
                    setup.mockInput.pressArrow("down");
                }
                await setup.flush();
            });
            const scrolledFrame = setup.captureCharFrame();
            expect(scrolledFrame).toContain("Shift+Tab");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("closes when the help shortcut is pressed", async () => {
        let closeCount = 0;
        const setup = await testRender(
            <BottomDialog width={76} height={22}>
                <HelpPanel onClose={() => {
                    closeCount += 1;
                }} />
            </BottomDialog>,
            { width: 80, height: 24 },
        );

        try {
            await act(async () => {
                await setup.flush();
                await setup.mockInput.typeText("?");
                await setup.flush();
            });
            expect(closeCount).toBe(1);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
