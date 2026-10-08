import { type BaseRenderable, BoxRenderable, RGBA } from "@opentui/core";
import type { TestRendererSetup } from "@opentui/core/testing";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, mock, test } from "bun:test";
import { act } from "react";

import type { ModelProviderDto } from "../api/types";
import { theme } from "../consts/theme";
import { useAppStore } from "../state/store";
import { createStoreClient } from "../state/testClient";
import { TuiRuntimeProvider } from "../tui/runtime";
import { WizardOverlay } from "./WizardOverlay";

const provider: ModelProviderDto = {
    code: "local", name: "Local", type: "openai", baseUrl: "http://localhost:8080/v1",
    apiKey: "local-key", builtin: false, modelsCached: true,
    models: [{
        code: "local-model", name: "Local model", contextWindow: 32_000,
        maxOutputTokens: 8_192, multiModal: false, light: false, isDefault: true, cached: true,
    }],
    createdAt: "", updatedAt: "",
};

function findPage(node: BaseRenderable): BoxRenderable | null {
    if (node instanceof BoxRenderable && node.id === "wizard-page") {
        return node;
    }
    for (const child of node.getChildren()) {
        const page = findPage(child);
        if (page) {
            return page;
        }
    }
    return null;
}

async function settle(setup: TestRendererSetup, input?: () => void | Promise<void>, escape = false) {
    await act(async () => {
        await input?.();
        if (escape) {
            await Bun.sleep(30);
        }
        await setup.flush();
    });
    await act(async () => {
        await setup.flush();
    });
}

async function clickLabel(setup: TestRendererSetup, label: string) {
    const lines = setup.captureCharFrame().split("\n");
    const row = lines.findIndex((line) => line.includes(label));
    expect(row).toBeGreaterThanOrEqual(0);
    const column = lines[row]!.indexOf(label);
    await settle(setup, () => setup.mockMouse.click(column + 1, row));
    await settle(setup, () => setup.mockMouse.moveTo(0, 0));
}

function expectPage(setup: TestRendererSetup, title: string, width: number, height: number) {
    const frame = setup.captureCharFrame();
    expect(frame).toContain(title);
    expect(frame).not.toContain("┌");
    for (const hint of ["Esc back", "↑↓ move", "rows/actions", "Enter next", "d delete", " more · "]) {
        expect(frame).not.toContain(hint);
    }
    const page = findPage(setup.renderer.root)!;
    expect(page).not.toBeNull();
    expect(Math.abs(page.x - (width - page.width) / 2)).toBeLessThanOrEqual(1);
    expect(Math.abs(page.y - (height - page.height) / 2)).toBeLessThanOrEqual(1);
    expect(page.y).toBeGreaterThanOrEqual(0);
    expect(page.y + page.height).toBeLessThanOrEqual(height);
    const background = RGBA.fromHex(theme.base);
    expect(setup.captureSpans().lines.every((line) => line.spans.every((span) => span.bg.equals(background)))).toBe(true);
}

describe("wizard configuration pages", () => {
    for (const [width, height] of [[80, 24], [120, 40], [40, 16], [40, 12]]) {
        test(`centers every page on the same background and goes back with Esc at ${width}x${height}`, async () => {
            const previous = useAppStore.getState();
            const exit = mock(() => undefined);
            let resolveProviders!: (providers: ModelProviderDto[]) => void;
            useAppStore.setState({ client: createStoreClient({
                listProviders: () => new Promise((resolve) => {
                    resolveProviders = resolve;
                }),
            }) });
            const setup = await testRender(
                <TuiRuntimeProvider runtime={{ exit }}><WizardOverlay /></TuiRuntimeProvider>,
                { width, height, exitSignals: [], exitOnCtrlC: false },
            );
            try {
                await settle(setup, () => resolveProviders([provider]));
                await settle(setup, () => setup.mockInput.pressEnter());
                expectPage(setup, "Provider connections", width, height);
                expect(setup.captureCharFrame()).toContain("Back");
                await settle(setup, () => setup.mockInput.typeText("acdsyn"));
                expectPage(setup, "Provider connections", width, height);
                await settle(setup, () => setup.mockInput.pressEnter());
                expectPage(setup, "Add compatible provider", width, height);
                expect(setup.captureCharFrame()).toContain("Save provider");
                await settle(setup, () => setup.mockInput.pressEscape(), true);
                expectPage(setup, "Provider connections", width, height);
                await clickLabel(setup, "Continue to model");
                expectPage(setup, "Default session model", width, height);
                expect(setup.captureCharFrame()).toContain("Complete setup");
                expect(setup.captureCharFrame()).not.toContain("Delete model");
                await settle(setup, () => setup.mockInput.pressEnter());
                expectPage(setup, "Edit model: Local model", width, height);
                expect(setup.captureCharFrame()).toContain("Save model");
                expect(setup.captureCharFrame()).toContain("Delete model");
                await settle(setup, () => setup.mockInput.pressKey("d"));
                expectPage(setup, "Edit model: Local model", width, height);
                await clickLabel(setup, "Delete model");
                expect(setup.captureCharFrame()).toContain("Delete model \"Local model\"?");
                await settle(setup, () => setup.mockInput.typeText("yndc"));
                expect(setup.captureCharFrame()).toContain("Delete model \"Local model\"?");
                await clickLabel(setup, "Cancel");
                expectPage(setup, "Edit model: Local model", width, height);
                await settle(setup, () => setup.mockInput.pressEscape(), true);
                expectPage(setup, "Default session model", width, height);
                await settle(setup, () => setup.mockInput.pressArrow("up"));
                expect(setup.captureCharFrame()).toContain("Search:");
                await settle(setup, () => setup.mockInput.pressEscape(), true);
                expectPage(setup, "Provider connections", width, height);
                await settle(setup, () => setup.mockInput.pressEscape(), true);
                expect(setup.captureCharFrame()).toContain("Begin setup");
                await settle(setup, () => setup.mockInput.pressEscape(), true);
                expect(exit).not.toHaveBeenCalled();
                if (width === 80) {
                    await settle(setup, () => setup.mockInput.pressEnter());
                    await clickLabel(setup, "Continue to model");
                    await settle(setup, () => setup.mockInput.pressEnter());
                    await clickLabel(setup, "Delete model");
                    await settle(setup, () => setup.mockInput.pressArrow("left"));
                    await settle(setup, () => setup.mockInput.pressEnter());
                    expectPage(setup, "Default session model", width, height);
                    expect(setup.captureCharFrame()).not.toContain("local-model");
                }
            } finally {
                act(() => setup.renderer.destroy());
                useAppStore.setState(previous);
            }
        });
    }
});
