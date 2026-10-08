import { type BaseRenderable, BoxRenderable } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, mock, test } from "bun:test";
import { act } from "react";

import { AGENTY_PARTICLE_LINES, AGENTY_TAGLINE } from "../consts/brand";
import { TuiRuntimeProvider } from "../tui/runtime";
import { WizardOverlay } from "./WizardOverlay";
import { WizardWelcome } from "./WizardWelcome";

function findControl(node: BaseRenderable, id: string): BoxRenderable | null {
    if (node instanceof BoxRenderable && node.id === id) {
        return node;
    }
    for (const child of node.getChildren()) {
        const found = findControl(child, id);
        if (found) {
            return found;
        }
    }
    return null;
}

describe("wizard welcome", () => {
    for (const [width, height] of [[80, 24], [120, 40], [40, 16], [40, 12]]) {
        test(`keeps the brand, copy and both options visible at ${width}x${height}`, async () => {
            const setup = await testRender(<WizardWelcome onBegin={() => undefined} onExit={() => undefined} />,
                { width, height });
            try {
                await act(async () => {
                    await setup.flush();
                });
                const frame = setup.captureCharFrame();
                const text = frame.replace(/\s+/g, " ");
                expect(text).toContain(AGENTY_TAGLINE);
                expect(text).toContain("Select or click \"Begin setup\" to complete your initialization wizard.");
                expect(frame).toContain("> Begin setup");
                expect(frame).toContain("Exit");
                expect(frame).not.toContain("Provider access");
                expect(frame).not.toContain("[Enter]");
                expect(frame).not.toContain("[Esc]");
                expect(frame).not.toContain("Config and sessions");
                if (height >= 14) {
                    for (const line of AGENTY_PARTICLE_LINES) {
                        expect(frame).toContain(line);
                    }
                } else {
                    expect(frame).toContain("agenty>");
                }
                for (const id of ["wizard-begin", "wizard-exit"]) {
                    const control = findControl(setup.renderer.root, id);
                    expect(control).not.toBeNull();
                    expect(control!.y).toBeGreaterThanOrEqual(0);
                    expect(control!.y + control!.height).toBeLessThanOrEqual(height);
                }
            } finally {
                act(() => setup.renderer.destroy());
            }
        });
    }

    test("ignores Esc and confirms only the selected option", async () => {
        const begin = mock(() => undefined);
        const exit = mock(() => undefined);
        const setup = await testRender(<WizardWelcome onBegin={begin} onExit={exit} />,
            { width: 80, height: 24 });
        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressEscape();
                await setup.mockInput.typeText("ny");
                await setup.flush();
            });
            expect(begin).not.toHaveBeenCalled();
            expect(exit).not.toHaveBeenCalled();
            await act(async () => {
                setup.mockInput.pressArrow("down");
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("> Exit");
            await act(async () => {
                setup.mockInput.pressEscape();
                setup.mockInput.pressEnter();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(exit).toHaveBeenCalledTimes(1);
            expect(begin).not.toHaveBeenCalled();
            await act(async () => {
                setup.mockInput.pressTab();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("> Begin setup");
            await act(async () => {
                setup.mockInput.pressEnter();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(begin).toHaveBeenCalledTimes(1);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("activates both options by mouse", async () => {
        const begin = mock(() => undefined);
        const exit = mock(() => undefined);
        const setup = await testRender(<WizardWelcome onBegin={begin} onExit={exit} />,
            { width: 80, height: 24 });
        try {
            await act(async () => {
                await setup.flush();
            });
            for (const id of ["wizard-begin", "wizard-exit"]) {
                const control = findControl(setup.renderer.root, id)!;
                await act(async () => {
                    await setup.mockMouse.click(control.x + 3, control.y);
                    await setup.flush();
                });
            }
            expect(begin).toHaveBeenCalledTimes(1);
            expect(exit).toHaveBeenCalledTimes(1);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("enters the centered provider page and returns to welcome with Esc", async () => {
        const exit = mock(() => undefined);
        const setup = await testRender(
            <TuiRuntimeProvider runtime={{ exit }}><WizardOverlay /></TuiRuntimeProvider>,
            { width: 80, height: 24 },
        );
        try {
            await act(async () => {
                await setup.flush();
            });
            expect(findControl(setup.renderer.root, "panel-box")).toBeNull();
            await act(async () => {
                setup.mockInput.pressEscape();
                await Bun.sleep(30);
                await setup.flush();
            });
            expect(exit).not.toHaveBeenCalled();
            await act(async () => {
                setup.mockInput.pressEnter();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Provider connections");
            expect(findControl(setup.renderer.root, "panel-box")).toBeNull();
            const page = findControl(setup.renderer.root, "wizard-page")!;
            expect(Math.abs(page.x - (80 - page.width) / 2)).toBeLessThanOrEqual(1);
            expect(Math.abs(page.y - (24 - page.height) / 2)).toBeLessThanOrEqual(1);
            await act(async () => {
                setup.mockInput.pressEscape();
                await Bun.sleep(30);
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Begin setup");
            expect(exit).not.toHaveBeenCalled();
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
