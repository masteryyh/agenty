import { describe, expect, mock, setSystemTime, test } from "bun:test";

import { createInterruptController, INTERRUPT_CONFIRM_MS } from "./interrupt";

describe("interrupt confirmation", () => {
    test("requires two interrupts within the confirmation window", () => {
        const exit = mock(() => undefined);
        const pending = mock((_value: boolean) => undefined);
        const controller = createInterruptController(exit, pending);
        try {
            setSystemTime(1_000);
            controller.request();
            expect(exit).not.toHaveBeenCalled();
            expect(pending).toHaveBeenLastCalledWith(true);
            setSystemTime(1_000 + INTERRUPT_CONFIRM_MS);
            controller.request();
            expect(exit).not.toHaveBeenCalled();
            setSystemTime(1_001 + INTERRUPT_CONFIRM_MS);
            controller.request();
            expect(exit).toHaveBeenCalledTimes(1);
            expect(exit).toHaveBeenCalledWith(130);
            expect(pending).toHaveBeenLastCalledWith(false);
        } finally {
            controller.dispose();
            setSystemTime();
        }
    });

    test("cancels confirmation and clears its timer on disposal", async () => {
        const exit = mock(() => undefined);
        const pending = mock((_value: boolean) => undefined);
        const controller = createInterruptController(exit, pending);
        controller.request();
        controller.cancel();
        expect(pending).toHaveBeenLastCalledWith(false);
        controller.request();
        expect(exit).not.toHaveBeenCalled();
        controller.dispose();
        pending.mockClear();
        await Bun.sleep(INTERRUPT_CONFIRM_MS + 20);
        expect(pending).not.toHaveBeenCalled();
        expect(exit).not.toHaveBeenCalled();
    });
});
