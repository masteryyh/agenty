import { describe, expect, test } from "bun:test";

import { buildToolDisplay } from "./toolDisplay";

function toolCall(name: string, input: unknown, content?: string, isError = false) {
    return {
        id: `call-${name}`,
        name,
        arguments: JSON.stringify(input),
        result: content === undefined
            ? undefined
            : {
                callId: `call-${name}`,
                name,
                content,
                isError,
            },
    };
}

describe("tool display", () => {
    test("summarizes read_file without rendering its JSON envelope", () => {
        const display = buildToolDisplay(toolCall(
            "read_file",
            { path: "src/MessageItem.tsx", start_line: 10, end_line: 20 },
            JSON.stringify({
                path: "/repo/src/MessageItem.tsx",
                content: "10: const first = true;\n11: const second = false;",
                startLine: 10,
                endLine: 11,
                truncated: false,
            }),
        ));

        expect(display.label).toBe("Read file");
        expect(display.status).toBe("success");
        expect(display.summaryLines[0]).toContain("src/MessageItem.tsx");
        expect(display.summaryLines[0]).toContain("lines 10–11");
        expect(display.detailLines[0]).toBe("10: const first = true;");
        expect(display.detailLines.join("\n")).not.toContain(JSON.stringify("content"));
    });

    test("formats shell commands and shell_call_output results per command", () => {
        const display = buildToolDisplay(toolCall(
            "shell",
            { commands: ["printf hello", "false"] },
            JSON.stringify([{
                type: "shell_call_output",
                output: [
                    { stdout: "hello", stderr: "", outcome: { type: "exit", exitCode: 0 } },
                    { stdout: "", stderr: "failed", outcome: { type: "exit", exitCode: 1 } },
                ],
            }]),
        ));

        expect(display.label).toBe("Run shell");
        expect(display.status).toBe("error");
        expect(display.summaryLines).toEqual(["$ printf hello", "$ false"]);
        expect(display.detailLines).toHaveLength(0);
        expect(display.shellCommands?.map((command) => command.status)).toEqual([
            "success",
            "error",
        ]);
        expect(display.shellCommands?.[0]?.outputLines[0]).toEqual({
            text: "hello",
            stream: "stdout",
        });
        expect(display.shellCommands?.[0]?.endsWithNewline).toBe(false);
        expect(display.shellCommands?.[1]?.outputLines[0]).toEqual({
            text: "failed",
            stream: "stderr",
        });
    });

    test("marks a shell output that ends with a newline", () => {
        const display = buildToolDisplay(toolCall(
            "shell",
            { commands: ["printf version"] },
            JSON.stringify([{
                type: "shell_call_output",
                output: [{
                    stdout: "version\n",
                    stderr: "",
                    outcome: { type: "exit", exitCode: 0 },
                }],
            }]),
        ));

        expect(display.shellCommands?.[0]?.outputLines).toEqual([
            { text: "version", stream: "stdout" },
        ]);
        expect(display.shellCommands?.[0]?.endsWithNewline).toBe(true);
    });

    test("keeps the complete shell output for expanded details", () => {
        const lines = [
            "line 1",
            "line 2",
            "line 3",
            "line 4",
            "line 5",
            `long ${"x".repeat(180)}`,
        ];
        const display = buildToolDisplay(toolCall(
            "shell",
            { commands: ["cat output.txt"] },
            JSON.stringify([{
                type: "shell_call_output",
                output: [{
                    stdout: `${lines.join("\n")}\n`,
                    stderr: "",
                    outcome: { type: "exit", exitCode: 0 },
                }],
            }]),
        ));

        expect(display.shellCommands?.[0]?.outputLines.map(({ text }) => text)).toEqual(lines);
        expect(display.shellCommands?.[0]?.outputLines.at(-1)?.text).toHaveLength(185);
        expect(display.shellCommands?.[0]?.endsWithNewline).toBe(true);
    });

    test("uses the final displayed output stream for the newline marker", () => {
        const display = buildToolDisplay(toolCall(
            "shell",
            { commands: ["run command"] },
            JSON.stringify([{
                type: "shell_call_output",
                output: [{
                    stdout: "done\n",
                    stderr: "warning",
                    outcome: { type: "exit", exitCode: 1 },
                }],
            }]),
        ));

        expect(display.shellCommands?.[0]?.endsWithNewline).toBe(false);
    });

    test("keeps unknown tool arguments structured and full details available", () => {
        const display = buildToolDisplay(toolCall(
            "custom_lookup",
            { query: "needle", filters: { owner: "team" } },
            JSON.stringify({ rows: Array.from({ length: 30 }, (_, index) => index) }),
        ));

        expect(display.label).toBe("custom lookup");
        expect(display.summaryLines[0]).toBe("query: needle");
        expect(display.summaryLines[1]).toContain("filters:");
        expect(display.summaryLines.join("\n")).not.toContain("(");
        expect(display.detailLines.length).toBeGreaterThan(12);
        expect(display.detailLines.join("\n")).not.toContain("… more output");
        expect(display.detailLines.join("\n")).toContain("29");
    });

    test("marks an unfinished call as pending", () => {
        const display = buildToolDisplay(toolCall("read_file", { path: "README.md" }));

        expect(display.status).toBe("pending");
        expect(display.summaryLines[0]).toContain("README.md");
    });
});
