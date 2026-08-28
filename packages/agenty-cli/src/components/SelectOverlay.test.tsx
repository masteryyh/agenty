import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act } from "react";

import { SelectOverlay } from "./SelectOverlay";

async function settleEffects(): Promise<void> {
    await new Promise<void>((resolve) => {
        setTimeout(resolve, 50);
    });
}

describe("SelectOverlay", () => {
    test("closes on Escape while entries are unavailable", async () => {
        let closeCount = 0;
        let resolveLoad: ((entries: Array<{ label: string; data: string }>) => void) | undefined;
        const load = () => new Promise<Array<{ label: string; data: string }>>((resolve) => {
            resolveLoad = resolve;
        });
        const setup = await testRender(
            <SelectOverlay
                title="Models"
                load={load}
                onSelect={() => undefined}
                onClose={() => {
                    closeCount++;
                }}
            />,
            { width: 48, height: 8 },
        );

        try {
            await act(async () => {
                await settleEffects();
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressEscape();
                await settleEffects();
                await setup.flush();
            });

            expect(closeCount).toBe(1);
        } finally {
            resolveLoad?.([]);
            act(() => setup.renderer.destroy());
        }
    });

    test("closes on Escape after loading an empty result", async () => {
        let closeCount = 0;
        let resolveLoad: ((entries: Array<{ label: string; data: string }>) => void) | undefined;
        const load = () => new Promise<Array<{ label: string; data: string }>>((resolve) => {
            resolveLoad = resolve;
        });
        const setup = await testRender(
            <SelectOverlay
                title="Models"
                load={load}
                onSelect={() => undefined}
                onClose={() => {
                    closeCount++;
                }}
            />,
            { width: 48, height: 8 },
        );

        try {
            await act(async () => {
                await settleEffects();
            });
            await act(async () => {
                resolveLoad?.([]);
                await settleEffects();
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("No items");
            await act(async () => {
                setup.mockInput.pressEscape();
                await settleEffects();
                await setup.flush();
            });

            expect(closeCount).toBe(1);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("closes on Escape after a load failure", async () => {
        let closeCount = 0;
        let rejectLoad: ((reason?: unknown) => void) | undefined;
        const load = () => new Promise<Array<{ label: string; data: string }>>((_resolve, reject) => {
            rejectLoad = reject;
        });
        const setup = await testRender(
            <SelectOverlay
                title="Models"
                load={load}
                onSelect={() => undefined}
                onClose={() => {
                    closeCount++;
                }}
            />,
            { width: 48, height: 8 },
        );

        try {
            await act(async () => {
                await settleEffects();
            });
            await act(async () => {
                rejectLoad?.(new Error("load failed"));
                await settleEffects();
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Failed: load failed");
            await act(async () => {
                setup.mockInput.pressEscape();
                await settleEffects();
                await setup.flush();
            });

            expect(closeCount).toBe(1);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
