import { KeyEvent } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, mock, test } from "bun:test";
import { act, useState } from "react";

import { Box, TextInput } from "../components/ui";
import { INTERRUPT_CONFIRM_MS } from "./interrupt";
import { TuiRuntimeProvider } from "./runtime";

function Input() {
    const [value, setValue] = useState("");
    return <Box width="100%" height="100%"><TextInput value={value} onChange={setValue} onSubmit={() => undefined} /></Box>;
}

describe("TUI interrupt handling", () => {
    test("shares confirmation between Ctrl+C and SIGINT and ignores held-key repeats", async () => {
        const exit = mock((_code?: number) => undefined);
        const signalListeners = process.listenerCount("SIGINT");
        const setup = await testRender(
            <TuiRuntimeProvider runtime={{ exit }}><Input /></TuiRuntimeProvider>,
            { width: 80, height: 24, exitSignals: [], exitOnCtrlC: false },
        );
        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressCtrlC();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(exit).not.toHaveBeenCalled();
            expect(setup.captureCharFrame()).toContain("Press Ctrl+C again to exit.");
            await act(async () => {
                setup.renderer.keyInput.emit("keypress", new KeyEvent({
                    name: "c", sequence: "\u0003", raw: "\u0003", ctrl: true,
                    shift: false, meta: false, option: false, number: false,
                    eventType: "repeat", source: "kitty",
                }));
            });
            expect(exit).not.toHaveBeenCalled();
            await act(async () => {
                process.emit("SIGINT");
                await setup.flush();
            });
            expect(exit).toHaveBeenCalledTimes(1);
            expect(exit).toHaveBeenCalledWith(130);
        } finally {
            act(() => setup.renderer.destroy());
        }
        expect(process.listenerCount("SIGINT")).toBe(signalListeners);
    });

    test("ordinary input and timeout cancel the pending exit", async () => {
        const exit = mock(() => undefined);
        const setup = await testRender(
            <TuiRuntimeProvider runtime={{ exit }}><Input /></TuiRuntimeProvider>,
            { width: 80, height: 24, exitSignals: [], exitOnCtrlC: false },
        );
        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressCtrlC();
                await setup.mockInput.typeText("a");
                await setup.flush();
            });
            expect(setup.captureCharFrame()).not.toContain("again to exit");
            await act(async () => {
                setup.mockInput.pressCtrlC();
                await setup.flush();
            });
            expect(exit).not.toHaveBeenCalled();
            await act(async () => {
                await Bun.sleep(INTERRUPT_CONFIRM_MS + 20);
                await setup.flush();
            });
            expect(setup.captureCharFrame()).not.toContain("again to exit");
            await act(async () => {
                setup.mockInput.pressCtrlC();
                await setup.flush();
            });
            expect(exit).not.toHaveBeenCalled();
            await act(async () => {
                process.emit("SIGTERM");
            });
            expect(exit).toHaveBeenCalledWith(0);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
