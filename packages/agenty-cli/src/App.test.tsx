import { BaseRenderable, InputRenderable } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { act } from "react";

import type { ChatSessionDto, ModelDto } from "./api/types";
import { App } from "./App";
import { useAppStore } from "./state/store";
import { TuiRuntimeProvider } from "./tui/runtime";

const model: ModelDto = {
    code: "model",
    providerCode: "provider",
    providerName: "Provider",
    name: "Model",
    contextWindow: 32_000,
    maxOutputTokens: 8_192,
    multiModal: false,
    light: false,
    isDefault: true,
};

const session: ChatSessionDto = {
    id: "session-1",
    currentModel: { providerCode: model.providerCode, modelCode: model.code },
    contextWindow: model.contextWindow,
    permissionMode: "ask",
    rounds: [],
    createdAt: "2026-09-20T00:00:00Z",
    updatedAt: "2026-09-20T00:00:00Z",
};

function findInput(renderable: BaseRenderable): InputRenderable | null {
    if (renderable instanceof InputRenderable) {
        return renderable;
    }
    for (const child of renderable.getChildren()) {
        const input = findInput(child);
        if (input) {
            return input;
        }
    }
    return null;
}

function TestApp() {
    return (
        <TuiRuntimeProvider runtime={{ exit: () => undefined }}>
            <App />
        </TuiRuntimeProvider>
    );
}

beforeEach(() => {
    useAppStore.setState({
        phase: "ready",
        initError: null,
        client: null,
        model,
        session,
        skills: [],
        overlay: null,
        toast: null,
        history: [],
        current: null,
        status: "idle",
        chatError: null,
        tokenConsumed: 0,
        phrase: null,
        promptHistory: [],
        activeSessionId: null,
        pendingApproval: null,
    });
});

afterEach(() => {
    useAppStore.setState({ phase: "loading", overlay: null, promptHistory: [] });
});

describe("chat input interactions", () => {
    test("distinguishes the effective permission mode from the pending selection", async () => {
        useAppStore.setState({ session: { ...session, permissionMode: "auto", pendingPermissionMode: "yolo" } });
        const setup = await testRender(<TestApp />, { width: 100, height: 24 });
        try {
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("auto mode → yolo (pending)");
            await act(async () => {
                useAppStore.setState({ session: { ...session, permissionMode: "yolo" } });
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("yolo mode");
            expect(setup.captureCharFrame()).not.toContain("(pending)");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("opens the shared help panel when question mark is typed into an empty input", async () => {
        const setup = await testRender(<TestApp />, { width: 80, height: 24 });

        try {
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("? Help · Shift+Tab Mode · ↑↓ History");

            await act(async () => {
                await setup.mockInput.typeText("?");
                await setup.flush();
            });

            expect(useAppStore.getState().overlay).toBe("help");
            expect(setup.captureCharFrame()).toContain("Commands, input syntax and keyboard shortcuts");
            expect(findInput(setup.renderer.root)?.value).toBe("");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("keeps the shortcut row and help panel usable in a narrow terminal", async () => {
        const setup = await testRender(<TestApp />, { width: 40, height: 12 });

        try {
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("? Help · Shift+Tab · ↑↓ History");

            await act(async () => {
                await setup.mockInput.typeText("?");
                await setup.flush();
            });
            const frame = setup.captureCharFrame();
            expect(frame).toContain("Help");
            expect(frame).toContain("Commands");
            expect(frame.split("\n")).toHaveLength(13);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("moves through persisted history and restores the draft", async () => {
        useAppStore.setState({ promptHistory: ["first", "/status"] });
        const setup = await testRender(<TestApp />, { width: 80, height: 24 });

        try {
            await act(async () => {
                await setup.flush();
                await setup.mockInput.typeText("draft");
                setup.mockInput.pressArrow("up");
                await setup.flush();
            });
            expect(findInput(setup.renderer.root)?.value).toBe("/status");

            await act(async () => {
                setup.mockInput.pressArrow("up");
                await setup.flush();
            });
            expect(findInput(setup.renderer.root)?.value).toBe("first");

            await act(async () => {
                setup.mockInput.pressArrow("down");
                setup.mockInput.pressArrow("down");
                await setup.flush();
            });
            expect(findInput(setup.renderer.root)?.value).toBe("draft");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
