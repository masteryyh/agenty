import { type BaseRenderable, BoxRenderable, ScrollBoxRenderable } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { type ReactNode, useState } from "react";
import { act } from "react";

import { useWindowSize } from "../hooks/useWindowSize";
import { BottomDialog } from "./BottomDialog";
import { type FormField, FormPanel, formString, type FormValues } from "./FormPanel";
import { buildFields } from "./McpOverlay";
import { buildCreateModelFields } from "./ProviderOverlay";
import { Box, Text } from "./ui";

function Dialog({ children }: { children: ReactNode }) {
    const { columns } = useWindowSize();
    return <BottomDialog width={columns - 2} height={20}>{children}</BottomDialog>;
}

function findBox(node: BaseRenderable, predicate: (box: BoxRenderable) => boolean): BoxRenderable | undefined {
    if (node instanceof BoxRenderable && predicate(node)) {
        return node;
    }
    for (const child of node.getChildren()) {
        const match = findBox(child, predicate);
        if (match) {
            return match;
        }
    }
    return undefined;
}

function AdvancedModelForm() {
    const [advanced, setAdvanced] = useState(false);
    return (
        <Dialog>
            <FormPanel
                title="Add model"
                fields={buildCreateModelFields(advanced)}
                onChange={(key, values) => {
                    if (key === "advanced") {
                        setAdvanced(formString(values, key) === "true");
                    }
                }}
                onAction={() => undefined}
                onClose={() => undefined}
            />
        </Dialog>
    );
}

function AdvancedMcpForm() {
    const [advanced, setAdvanced] = useState(false);
    return (
        <Dialog>
            <FormPanel
                title="Add MCP Server"
                fields={buildFields(undefined, "http", true, advanced)}
                onChange={(key, values) => {
                    if (key === "advanced") {
                        setAdvanced(formString(values, key) === "true");
                    }
                }}
                onAction={() => undefined}
                onClose={() => undefined}
            />
        </Dialog>
    );
}

function StatefulActionsForm({ onAction }: { onAction: (action: string) => void }) {
    const [details, setDetails] = useState(false);
    return (
        <Dialog>
            <FormPanel
                title="MCP Server"
                fields={[{ key: "name", label: "Name", kind: "text", value: "local", readOnly: true }]}
                actions={[
                    { key: "save", label: "Save" },
                    ...(details ? [{ key: "login", label: "Login" }] : []),
                    { key: "disable", label: "Disable" },
                    { key: "delete", label: "Delete" },
                    { key: "cancel", label: "Cancel" },
                ]}
                afterFields={details ? <Box height={6}><Text>Connection logs</Text></Box> : undefined}
                onShortcut={(input) => {
                    if (input !== "l") {
                        return false;
                    }
                    setDetails((current) => !current);
                    return true;
                }}
                onAction={(action) => onAction(action)}
                onClose={() => undefined}
            />
        </Dialog>
    );
}

describe("overlay forms", () => {
    test("keeps columns adjacent on wide screens and preserves drafts through stacked layout", async () => {
        let saved: FormValues | undefined;
        const setup = await testRender(
            <Dialog>
                <FormPanel
                    title="Add MCP Server"
                    fields={buildFields(undefined, "http")}
                    onAction={(_action, values) => {
                        saved = values;
                    }}
                    onClose={() => undefined}
                />
            </Dialog>,
            { width: 180, height: 26 },
        );
        try {
            await act(async () => {
                await setup.flush();
                await setup.mockInput.typeText("local-server");
                await setup.flush();
            });
            let frame = setup.captureCharFrame();
            const nameLine = frame.split("\n").find((line) => line.includes("Name:"))!;
            const valueStart = nameLine.indexOf("local-server");
            expect(valueStart - nameLine.indexOf("Name:")).toBe(7);
            expect(valueStart).toBeLessThan(35);
            expect(frame.split("\n").find((line) => line.includes("Transport:"))?.indexOf("Streamable HTTP")).toBe(valueStart);

            await act(async () => {
                setup.resize(72, 26);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            frame = setup.captureCharFrame();
            expect(frame.split("\n").find((line) => line.includes("Name:"))?.indexOf("local-server")).toBe(valueStart);

            await act(async () => {
                setup.resize(38, 26);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            const lines = setup.captureCharFrame().split("\n");
            const nameRow = lines.findIndex((line) => line.includes("Name:"));
            expect(lines[nameRow]).not.toContain("local-server");
            expect(lines[nameRow + 1]).toContain("local-server");
            expect(lines[nameRow + 1].indexOf("local-server")).toBe(lines[nameRow].indexOf("Name:"));
            expect(setup.captureCharFrame()).toContain("Cancel");

            await act(async () => {
                setup.resize(180, 26);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            for (let i = 0; i < 5; i++) {
                await act(async () => {
                    await setup.mockInput.pressKeys(["ARROW_DOWN"], 15);
                    await setup.flush();
                });
            }
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await setup.mockInput.pressKeys(["RETURN"], 15);
                await setup.flush();
            });
            expect(saved?.name).toBe("local-server");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("grows and shrinks advanced model fields without moving the value column", async () => {
        const setup = await testRender(<AdvancedModelForm />, { width: 90, height: 30 });
        try {
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await new Promise((resolve) => setTimeout(resolve, 20));
                await setup.flush();
            });
            const dialog = findBox(setup.renderer.root, (box) => box.id === "panel-box")!;
            const collapsedHeight = dialog.height;
            const originalValueStart = setup.captureCharFrame().split("\n").find((line) => line.includes("128000"))!.indexOf("128000");
            for (let i = 0; i < 6; i++) {
                await act(async () => {
                    await setup.mockInput.pressKeys(["ARROW_DOWN"], 15);
                    await setup.flush();
                });
            }
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await setup.mockInput.pressKeys(["ARROW_RIGHT"], 15);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Max output tokens:");
            expect(dialog.height).toBeGreaterThan(collapsedHeight);
            expect(setup.captureCharFrame().split("\n").find((line) => line.includes("128000"))!.indexOf("128000")).toBe(originalValueStart);
            await act(async () => {
                await setup.mockInput.pressKeys(["ARROW_LEFT"], 15);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).not.toContain("Max output tokens:");
            expect(dialog.height).toBe(collapsedHeight);
            expect(setup.captureCharFrame()).toContain("❯ ▸ Advanced options");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("keeps MCP headers collapsed until Advanced Options is opened", async () => {
        const setup = await testRender(<AdvancedMcpForm />, { width: 90, height: 24 });
        try {
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("▸ Advanced Options");
            expect(setup.captureCharFrame()).not.toContain("Headers JSON:");

            for (let i = 0; i < 4; i++) {
                await act(async () => {
                    await setup.mockInput.pressKeys(["ARROW_DOWN"], 15);
                    await setup.flush();
                });
            }
            await act(async () => {
                await setup.mockInput.pressKeys(["ARROW_RIGHT"], 15);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Headers JSON:");

            await act(async () => {
                await setup.mockInput.pressKeys(["ARROW_LEFT"], 15);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).not.toContain("Headers JSON:");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("scrolls long forms while keeping the title, actions and dropdown visible", async () => {
        const fields: FormField[] = Array.from({ length: 12 }, (_, index) => ({
            key: `option-${index}`, label: `Option ${index}`, kind: "boolean", value: "false",
        }));
        fields.push({
            key: "type", label: "Transport", kind: "select", value: "http",
            options: [{ label: "Streamable HTTP", value: "http" }, { label: "SSE (deprecated)", value: "sse" }, { label: "Stdio", value: "stdio" }],
        });
        const setup = await testRender(
            <Dialog><FormPanel title="Long form" fields={fields} onAction={() => undefined} onClose={() => undefined} /></Dialog>,
            { width: 72, height: 20 },
        );
        try {
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await new Promise((resolve) => setTimeout(resolve, 20));
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("↓ more");
            for (let i = 0; i < 12; i++) {
                await act(async () => {
                    await setup.mockInput.pressKeys(["ARROW_DOWN"], 15);
                    await setup.flush();
                });
            }
            await act(async () => {
                await setup.flush();
            });
            let frame = setup.captureCharFrame();
            expect(frame).toContain("Long form");
            expect(frame).toContain("Save");
            expect(frame).toContain("Transport:");
            expect(frame).not.toContain("Option 0:");
            const valueStart = frame.split("\n").find((line) => line.includes("Transport:"))!.indexOf("Streamable HTTP");
            await act(async () => {
                await setup.mockInput.pressKeys(["RETURN"], 15);
                await setup.flush();
            });
            const menu = findBox(setup.renderer.root, (box) => box.id === "dropdown-menu")!;
            const viewport = findBox(setup.renderer.root, (box) => box instanceof ScrollBoxRenderable)!;
            expect(menu.x).toBe(valueStart);
            expect(menu.y).toBeGreaterThanOrEqual(viewport.y);
            expect(menu.y + menu.height).toBeLessThanOrEqual(viewport.y + viewport.height);
            frame = setup.captureCharFrame();
            expect(frame).toContain("SSE (deprecated)");
            expect(frame).toContain("Stdio");
            expect(frame).toContain("Save");
            await act(async () => {
                await setup.mockInput.pressKeys(["ESCAPE"], 50);
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("❯ Transport:");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("wraps MCP actions, measures logs, and keeps focus when Login appears", async () => {
        const called: string[] = [];
        const setup = await testRender(<StatefulActionsForm onAction={(action) => called.push(action)} />, { width: 38, height: 26 });
        try {
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await new Promise((resolve) => setTimeout(resolve, 20));
                await setup.flush();
            });
            const dialog = findBox(setup.renderer.root, (box) => box.id === "panel-box")!;
            const initialHeight = dialog.height;
            for (let i = 0; i < 2; i++) {
                await act(async () => {
                    await setup.mockInput.pressKeys(["ARROW_DOWN"], 15);
                    await setup.flush();
                });
            }
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("[Disable]");
            await act(async () => {
                await setup.mockInput.typeText("l", 15);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("[Disable]");
            expect(setup.captureCharFrame()).toContain("Login");
            expect(setup.captureCharFrame()).toContain("Connection logs");
            expect(setup.captureCharFrame()).toContain("Cancel");
            expect(dialog.height).toBeGreaterThan(initialHeight);
            await act(async () => {
                await setup.mockInput.pressKeys(["RETURN"], 15);
                await setup.flush();
            });
            expect(called).toEqual(["disable"]);
            await act(async () => {
                await setup.mockInput.typeText("l", 15);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(dialog.height).toBe(initialHeight);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("keeps choices and wrapped actions usable in a short terminal", async () => {
        let closed = false;
        const setup = await testRender(
            <Dialog>
                <FormPanel
                    title="MCP Server"
                    fields={buildFields(undefined, "http").filter((field) => field.key === "type")}
                    actions={["Save", "Login", "Disable", "Delete", "Cancel"].map((label) => ({ key: label.toLowerCase(), label }))}
                    onAction={() => undefined}
                    onClose={() => {
                        closed = true;
                    }}
                />
            </Dialog>,
            { width: 38, height: 12 },
        );
        try {
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await setup.mockInput.pressKeys(["RETURN"], 15);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("❯ Streamable HTTP");
            expect(setup.captureCharFrame()).toContain("Cancel");
            await act(async () => {
                await setup.mockInput.pressKeys(["ARROW_DOWN"], 15);
                await setup.flush();
            });
            await act(async () => {
                await setup.mockInput.pressKeys(["ARROW_DOWN"], 15);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("❯ Stdio");
            expect(setup.captureCharFrame()).toContain("Cancel");
            await act(async () => {
                await setup.mockInput.pressKeys(["ESCAPE"], 50);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(closed).toBe(false);
            expect(setup.captureCharFrame()).toContain("Streamable HTTP");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("restores the default dialog height on leaving a form", async () => {
        function SwitchView() {
            const [open, setOpen] = useState(true);
            return (
                <Dialog>
                    {open ? <FormPanel title="Short form" fields={[]} onAction={() => undefined} onClose={() => setOpen(false)} /> : <Text>List</Text>}
                </Dialog>
            );
        }
        const setup = await testRender(<SwitchView />, { width: 80, height: 26 });
        try {
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await new Promise((resolve) => setTimeout(resolve, 20));
                await setup.flush();
            });
            const dialog = findBox(setup.renderer.root, (box) => box.id === "panel-box")!;
            expect(dialog.height).toBeLessThan(20);
            await act(async () => {
                await setup.mockInput.pressKeys(["ESCAPE"], 50);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("List");
            expect(dialog.height).toBe(20);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
