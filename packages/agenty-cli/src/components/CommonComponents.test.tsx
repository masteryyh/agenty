import { BaseRenderable, BoxRenderable, InputRenderable, RGBA } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act, useState } from "react";

import { BottomDialog } from "./BottomDialog";
import { ConfirmDialog } from "./ConfirmDialog";
import type { FormField } from "./FormPanel";
import {
    chooseDropdownPlacement,
    FormPanel,
    preferredDropdownRows,
    wrapFormLabel,
} from "./FormPanel";
import { List } from "./List";
import { Box, HOVER_BACKGROUND, Text } from "./ui";

function InteractiveList({ onActivate }: { onActivate: (value: string) => void }) {
    const [cursor, setCursor] = useState(0);
    return (
        <List
            items={["first", "second"]}
            cursor={cursor}
            visibleCount={2}
            getKey={(item) => item}
            onCursor={setCursor}
            onActivate={onActivate}
            renderItem={(item, { selected }) => (
                <Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
                    <Text color={selected ? "cyan" : "white"}>{item}</Text>
                </Box>
            )}
        />
    );
}

function findBox(
    renderable: BaseRenderable,
    predicate: (box: BoxRenderable) => boolean,
): BoxRenderable | null {
    if (renderable instanceof BoxRenderable && predicate(renderable)) {
        return renderable;
    }
    for (const child of renderable.getChildren()) {
        const match = findBox(child, predicate);
        if (match) {
            return match;
        }
    }
    return null;
}

function findInput(renderable: BaseRenderable): InputRenderable | null {
    if (renderable instanceof InputRenderable) {
        return renderable;
    }
    for (const child of renderable.getChildren()) {
        const match = findInput(child);
        if (match) {
            return match;
        }
    }
    return null;
}

describe("common TUI components", () => {
    test("places dropdowns above when there is not enough room below", () => {
        expect(chooseDropdownPlacement(8, 1, 12, 5, 5)).toEqual({
            side: "above",
            visibleRows: 5,
            height: 7,
        });
        expect(chooseDropdownPlacement(1, 1, 5, 5, 5).side).toBe("below");
    });

    test("grows dropdown rows from a four-row minimum to an eight-row maximum", () => {
        expect(preferredDropdownRows(5)).toBe(4);
        expect(preferredDropdownRows(16)).toBe(6);
        expect(preferredDropdownRows(40)).toBe(8);
    });

    test("wraps labels at the shared maximum width", () => {
        expect(wrapFormLabel("Supported reasoning efforts:", 12)).toEqual([
            "Supported",
            "reasoning",
            "efforts:",
        ]);
    });

    test("renders advanced options as a disclosure row", async () => {
        const setup = await testRender(
            <FormPanel
                title="Model"
                fields={[
                    { key: "name", label: "Model name", kind: "text", value: "model" },
                    { key: "advanced", label: "Advanced options", kind: "disclosure", value: "false" },
                    { key: "max", label: "Max output tokens", kind: "text", value: "8192", visible: false },
                ]}
                onAction={() => undefined}
                onClose={() => undefined}
            />,
            { width: 72, height: 12 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            expect(setup.captureCharFrame()).toContain("▸ Advanced options");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("highlights a hovered list row without selecting or activating it", async () => {
        const activated: string[] = [];
        const setup = await testRender(
            <InteractiveList onActivate={(value) => activated.push(value)} />,
            { width: 30, height: 4 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await setup.mockMouse.moveTo(5, 1, { delayMs: 10 });
            });
            await act(async () => {
                await setup.flush();
                await setup.waitForVisualIdle();
            });

            let frame = setup.captureCharFrame();
            expect(frame.split("\n")[0]).toContain("❯ first");
            expect(frame.split("\n")[1]).not.toContain("❯");
            expect(activated).toEqual([]);
            const hoverColor = RGBA.fromHex(HOVER_BACKGROUND);
            expect(setup.captureSpans().lines[1]?.spans.some((span) => span.bg.equals(hoverColor)))
                .toBe(true);
            await act(async () => {
                await setup.mockMouse.click(5, 1);
                await setup.flush();
            });

            frame = setup.captureCharFrame();
            expect(frame.split("\n")[1]).toContain("❯ second");
            expect(activated).toEqual(["second"]);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("edits a focused text row directly and confirms a select with a second Enter", async () => {
        const fields: FormField[] = [
            { key: "name", label: "Name", kind: "text", value: "" },
            {
                key: "type",
                label: "Type",
                kind: "select",
                value: "second",
                options: [
                    { label: "First", value: "first" },
                    { label: "Second", value: "second" },
                ],
            },
        ];
        let saved: Record<string, string> | undefined;
        const setup = await testRender(
            <FormPanel
                title="Form"
                fields={fields}
                onAction={(_action, values) => {
                    saved = values;
                }}
                onClose={() => undefined}
            />,
            { width: 50, height: 12 },
        );

        try {
            await act(async () => {
                await setup.flush();
                await setup.mockInput.typeText("Direct edit");
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressArrow("down");
                await setup.flush();
            });
            await act(async () => {
                await setup.mockInput.pressKeys(["RETURN"], 10);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });

            expect(saved).toBeUndefined();
            expect(setup.captureCharFrame()).toContain("❯ Second");

            await act(async () => {
                setup.mockInput.pressArrow("up");
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressEnter();
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressArrow("down");
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressEnter();
                await setup.flush();
            });

            expect(saved).toMatchObject({
                name: "Direct edit",
                type: "first",
            });
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("opens a multi-select dropdown and commits temporary choices", async () => {
        let saved: Record<string, string> | undefined;
        const setup = await testRender(
            <FormPanel
                title="Model"
                fields={[
                    {
                        key: "efforts",
                        label: "Supported reasoning efforts",
                        kind: "multiselect",
                        value: JSON.stringify(["low"]),
                        options: [
                            { label: "Low", value: "low" },
                            { label: "Medium", value: "medium" },
                            { label: "High", value: "high" },
                        ],
                    },
                ]}
                onAction={(_action, values) => {
                    saved = values;
                }}
                onClose={() => undefined}
            />,
            { width: 72, height: 14 },
        );

        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressEnter();
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Low");
            expect(setup.captureCharFrame()).toContain("Medium");

            await act(async () => {
                setup.mockInput.pressArrow("down");
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressKey(" ");
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressEnter();
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });

            expect(saved).toBeUndefined();
            expect(setup.captureCharFrame()).toContain("2 selected");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("uses form shortcuts outside text editing without intercepting typed text", async () => {
        const shortcuts: string[] = [];
        const setup = await testRender(
            <FormPanel
                title="Edit model"
                fields={[
                    { key: "code", label: "Model Code", kind: "text", value: "model", readOnly: true },
                    { key: "name", label: "Model name", kind: "text", value: "" },
                ]}
                onShortcut={(input) => {
                    if (input.toLowerCase() === "d") {
                        shortcuts.push("delete");
                        return true;
                    }
                    return false;
                }}
                onAction={() => undefined}
                onClose={() => undefined}
            />,
            { width: 50, height: 10 },
        );

        try {
            await act(async () => {
                await setup.flush();
                await setup.mockInput.typeText("d");
                setup.mockInput.pressArrow("down");
                await setup.flush();
            });
            await act(async () => {
                await setup.mockInput.typeText("model d");
                await setup.flush();
            });

            expect(shortcuts).toEqual(["delete"]);
            expect(setup.captureCharFrame()).toContain("model d");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("skips fields marked as non-focusable", async () => {
        const setup = await testRender(
            <FormPanel
                title="Configure provider"
                fields={[
                    {
                        key: "code",
                        label: "Provider Code",
                        kind: "text",
                        value: "openai",
                        readOnly: true,
                        focusable: false,
                    },
                    {
                        key: "apiKey",
                        label: "API Key",
                        kind: "text",
                        value: "",
                        secret: true,
                    },
                ]}
                onAction={() => undefined}
                onClose={() => undefined}
            />,
            { width: 50, height: 10 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            let frame = setup.captureCharFrame();
            const codeLine = frame.split("\n").find((line) => line.includes("openai")) ?? "";
            expect(codeLine).not.toContain("❯");
            expect(frame.split("\n").some((line) => line.includes("❯"))).toBe(true);

            await act(async () => {
                setup.mockInput.pressArrow("up");
                await setup.flush();
            });

            frame = setup.captureCharFrame();
            expect(frame.split("\n").some((line) => line.includes("❯"))).toBe(true);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("refocuses the active text input when clicked after blur", async () => {
        const setup = await testRender(
            <FormPanel
                title="Add model"
                fields={[
                    { key: "code", label: "Model Code", kind: "text", value: "" },
                ]}
                onAction={() => undefined}
                onClose={() => undefined}
            />,
            { width: 50, height: 10 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            const input = findInput(setup.renderer.root);
            expect(input).not.toBeNull();
            input?.blur();
            expect(input?.focused).toBe(false);

            await act(async () => {
                if (input) {
                    await setup.mockMouse.click(input.x + 1, input.y);
                }
                await setup.flush();
            });

            expect(input?.focused).toBe(true);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("centers confirmation dialogs inside the active panel", async () => {
        const setup = await testRender(
            <BottomDialog width={58} height={20}>
                <ConfirmDialog
                    title="Delete model?"
                    message="This cannot be undone."
                    onConfirm={() => undefined}
                    onCancel={() => undefined}
                />
            </BottomDialog>,
            { width: 60, height: 22 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            const dialog = findBox(
                setup.renderer.root,
                (box) => box.borderColor.equals(RGBA.fromHex("#ff0000")),
            );
            expect(dialog).not.toBeNull();
            const container = dialog?.parent;
            expect(container).toBeInstanceOf(BoxRenderable);
            if (!(container instanceof BoxRenderable) || !dialog) {
                return;
            }
            const left = dialog.x - container.x;
            const right = container.width - left - dialog.width;
            const top = dialog.y - container.y;
            const bottom = container.height - top - dialog.height;
            expect(Math.abs(left - right)).toBeLessThanOrEqual(1);
            expect(Math.abs(top - bottom)).toBeLessThanOrEqual(1);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
