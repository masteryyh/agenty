import { type BaseRenderable, ScrollBoxRenderable } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act } from "react";

import { MessageList } from "./MessageList";
import { Box } from "./ui";

function findScrollBox(renderable: BaseRenderable): ScrollBoxRenderable | null {
    if (renderable instanceof ScrollBoxRenderable) {
        return renderable;
    }
    for (const child of renderable.getChildren()) {
        const match = findScrollBox(child);
        if (match) {
            return match;
        }
    }
    return null;
}

function expectMeasuredChildren(scrollbox: ScrollBoxRenderable): void {
    const children = scrollbox.content.getChildren();
    for (let index = 1; index < children.length; index += 1) {
        const previous = children[index - 1];
        const current = children[index];
        expect(current.y).toBeGreaterThanOrEqual(previous.y + previous.height);
        expect(current.width).toBe(scrollbox.content.width);
    }
}

describe("MessageList layout", () => {
    test("removes boundary blank lines without collapsing paragraph spacing", async () => {
        const setup = await testRender(
            <MessageList
                history={[{
                    id: "assistant-1",
                    role: "assistant",
                    content: "\n\nFirst paragraph.\n\nSecond paragraph.\n\n",
                    toolCalls: [{ id: "call-1", name: "shell", arguments: "{}" }],
                }]}
                current={null}
                height={10}
            />,
            { width: 40, height: 12 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            const scrollbox = findScrollBox(setup.renderer.root);
            expect(scrollbox).not.toBeNull();
            const [message, tool] = scrollbox?.content.getChildren() ?? [];
            expect(message?.height).toBe(3);
            expect(tool?.y).toBe((message?.y ?? 0) + (message?.height ?? 0) + 1);
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });

    test("keeps separate messages spaced while grouping assistant content and tools", async () => {
        const setup = await testRender(
            <MessageList
                history={[
                    { id: "user-1", role: "user", content: "hello" },
                    {
                        id: "assistant-1",
                        role: "assistant",
                        content: "answer",
                        reasoning: "thinking",
                        toolCalls: [
                            { id: "call-1", name: "shell", arguments: "{}" },
                            { id: "call-2", name: "read", arguments: "{}" },
                        ],
                    },
                    { id: "user-2", role: "user", content: "next" },
                ]}
                current={null}
                height={20}
            />,
            { width: 40, height: 22 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            const scrollbox = findScrollBox(setup.renderer.root);
            expect(scrollbox).not.toBeNull();
            const children = scrollbox?.content.getChildren() ?? [];
            expect(children).toHaveLength(6);

            const [user, reasoning, content, firstTool, secondTool, nextUser] = children;
            expect(reasoning.y).toBe(user.y + user.height + 1);
            expect(content.y).toBe(reasoning.y + reasoning.height);
            expect(firstTool.y).toBe(content.y + content.height + 1);
            expect(secondTool.y).toBe(firstTool.y + firstTool.height + 1);
            expect(nextUser.y).toBe(secondTool.y + secondTool.height + 1);
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });

    test("renders a semantic tool summary instead of the raw result envelope", async () => {
        const setup = await testRender(
            <MessageList
                history={[{
                    id: "assistant-tool",
                    role: "assistant",
                    content: "",
                    toolCalls: [{
                        id: "call-read",
                        name: "read_file",
                        arguments: JSON.stringify({ path: "src/MessageItem.tsx" }),
                        result: {
                            callId: "call-read",
                            name: "read_file",
                            content: JSON.stringify({
                                path: "/repo/src/MessageItem.tsx",
                                content: "1: first line\n2: second line",
                                startLine: 1,
                                endLine: 2,
                                truncated: false,
                            }),
                            isError: false,
                        },
                    }],
                }]}
                current={null}
                height={10}
            />,
            { width: 60, height: 12 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            const frame = setup.captureCharFrame();
            expect(frame).toContain("Read file");
            expect(frame).toContain("src/MessageItem.tsx");
            expect(frame).not.toContain(JSON.stringify("content"));
            expect(frame).not.toContain("first line");

        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });

    test("places the first message directly after the scroll-away header", async () => {
        const setup = await testRender(
            <MessageList
                history={[{ id: "user-1", role: "user", content: "hello" }]}
                current={null}
                height={10}
                header={<Box height={5} />}
            />,
            { width: 40, height: 12 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            const scrollbox = findScrollBox(setup.renderer.root);
            expect(scrollbox).not.toBeNull();
            const [header, firstMessage] = scrollbox?.content.getChildren() ?? [];
            expect(firstMessage?.y).toBe((header?.y ?? 0) + (header?.height ?? 0));
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });

    test("keeps dynamic message blocks in one measured scroll container", async () => {
        const setup = await testRender(
            <MessageList
                history={[
                    { id: "user-1", role: "user", content: "user ".repeat(40) },
                    { id: "assistant-1", role: "assistant", content: "assistant ".repeat(60) },
                ]}
                current={{
                    id: "assistant-2",
                    role: "assistant",
                    content: "current ".repeat(60),
                    reasoning: "reasoning ".repeat(20),
                    toolCalls: [],
                }}
                height={10}
            />,
            { width: 40, height: 12 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            const scrollbox = findScrollBox(setup.renderer.root);
            expect(scrollbox).not.toBeNull();
            expect(scrollbox?.viewportCulling).toBe(false);
            expectMeasuredChildren(scrollbox!);

            await act(async () => {
                setup.resize(24, 12);
                await setup.flush();
            });
            expectMeasuredChildren(scrollbox!);

            const frame = setup.captureCharFrame();
            expect(frame).toContain("current");
            expect(frame.split("\n")).toHaveLength(13);
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });
});
