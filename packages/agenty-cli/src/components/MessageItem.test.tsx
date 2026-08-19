import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act } from "react";

import { MessageItem, type MessageRenderItem } from "./MessageItem";

const shellItem: MessageRenderItem = {
    id: "message-1:tool:call-1",
    groupId: "message-1",
    type: "tool",
    expanded: true,
    blinkOn: true,
    toolCall: {
        id: "call-1",
        name: "shell",
        arguments: JSON.stringify({ commands: ["printf hello", "false"] }),
        result: {
            callId: "call-1",
            name: "shell",
            content: JSON.stringify([{
                type: "shell_call_output",
                output: [
                    { stdout: "hello", stderr: "", outcome: { type: "exit", exitCode: 0 } },
                    { stdout: "", stderr: "failed", outcome: { type: "exit", exitCode: 1 } },
                ],
            }]),
            isError: false,
        },
    },
};

describe("MessageItem tool output", () => {
    test("connects each shell output to its command with status symbols", async () => {
        const setup = await testRender(<MessageItem item={shellItem} />, {
            width: 60,
            height: 20,
        });

        try {
            await act(async () => {
                await setup.flush();
            });

            const frame = setup.captureCharFrame();
            const lines = frame.split("\n");
            const title = lines.findIndex((line) => line.includes("Run shell"));
            const firstCommand = lines.findIndex((line) => line.includes("$ printf hello"));
            const firstOutput = lines.findIndex((line) => line.includes("⎿ hello"));
            const secondCommand = lines.findIndex((line) => line.includes("$ false"));
            const secondOutput = lines.findIndex((line) => line.includes("⎿ failed"));

            expect(firstCommand).toBe(title + 1);
            expect(firstCommand).toBeGreaterThanOrEqual(0);
            expect(firstOutput).toBe(firstCommand + 1);
            expect(secondCommand).toBe(firstOutput + 2);
            expect(secondOutput).toBe(secondCommand + 1);
            expect(lines[firstOutput + 1]?.replace("│", "").trim()).toBe("");
            expect(lines[firstCommand]).toContain("✓");
            expect(lines[secondCommand]).toContain("✗");
            expect(frame).not.toContain("exit ");
            expect(frame).not.toContain("Command ");
            expect((lines[firstOutput]?.indexOf("⎿") ?? 0))
                .toBeGreaterThan(lines[firstCommand]?.indexOf("✓") ?? 0);
            expect((lines[secondOutput]?.indexOf("⎿") ?? 0))
                .toBeGreaterThan(lines[secondCommand]?.indexOf("✗") ?? 0);
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });

    test("keeps wrapped output aligned under the output column", async () => {
        const longOutputItem: MessageRenderItem = {
            ...shellItem,
            toolCall: {
                ...shellItem.toolCall,
                result: {
                    ...shellItem.toolCall.result!,
                    content: JSON.stringify([{
                        type: "shell_call_output",
                        output: [{
                            stdout: `OUTPUT_${"z".repeat(100)}`,
                            stderr: "",
                            outcome: { type: "exit", exitCode: 0 },
                        }],
                    }]),
                },
            },
        };
        const setup = await testRender(<MessageItem item={longOutputItem} />, {
            width: 40,
            height: 12,
        });

        try {
            await act(async () => {
                await setup.flush();
            });

            const lines = setup.captureCharFrame().split("\n");
            const firstOutput = lines.findIndex((line) => line.includes("OUTPUT_"));
            const outputColumn = lines[firstOutput]?.indexOf("OUTPUT_") ?? -1;
            const wrappedLines = lines.slice(firstOutput + 1).filter((line) => line.includes("z"));

            expect(firstOutput).toBeGreaterThanOrEqual(0);
            expect(wrappedLines.length).toBeGreaterThan(1);
            expect(wrappedLines.every((line) => line.indexOf("z") === outputColumn)).toBe(true);
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });

    test("renders every expanded shell output line", async () => {
        const outputLines = ["line 1", "line 2", "line 3", "line 4", "line 5"];
        const completeItem: MessageRenderItem = {
            ...shellItem,
            toolCall: {
                ...shellItem.toolCall,
                arguments: JSON.stringify({ commands: ["cat output.txt"] }),
                result: {
                    ...shellItem.toolCall.result!,
                    content: JSON.stringify([{
                        type: "shell_call_output",
                        output: [{
                            stdout: `${outputLines.join("\n")}\n`,
                            stderr: "",
                            outcome: { type: "exit", exitCode: 0 },
                        }],
                    }]),
                },
            },
        };
        const setup = await testRender(<MessageItem item={completeItem} />, {
            width: 60,
            height: 12,
        });

        try {
            await act(async () => {
                await setup.flush();
            });

            const frame = setup.captureCharFrame();
            expect(frame).toContain("line 1");
            expect(frame).toContain("line 5");
            expect(frame).not.toContain("more lines");
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });

    test("renders a marker for a shell output that ends with a newline", async () => {
        const newlineItem: MessageRenderItem = {
            ...shellItem,
            toolCall: {
                ...shellItem.toolCall,
                arguments: JSON.stringify({ commands: ["printf version"] }),
                result: {
                    ...shellItem.toolCall.result!,
                    content: JSON.stringify([{
                        type: "shell_call_output",
                        output: [{
                            stdout: "version\n",
                            stderr: "",
                            outcome: { type: "exit", exitCode: 0 },
                        }],
                    }]),
                },
            },
        };
        const setup = await testRender(<MessageItem item={newlineItem} />, {
            width: 60,
            height: 10,
        });

        try {
            await act(async () => {
                await setup.flush();
            });

            const frame = setup.captureCharFrame();
            expect(frame).toContain("⎿ version ↵");
            expect(frame).not.toContain("⎿ ↵");
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });
});
