import type { ToolResult } from "../api/types";
import type { UIToolCall } from "../state/store";

const MAX_SUMMARY_LINES = 4;
const MAX_VALUE_LENGTH = 96;

type JsonRecord = Record<string, unknown>;

export type ToolDisplayStatus = "pending" | "success" | "error";

export type ShellOutputStream = "stdout" | "stderr" | "empty" | "pending" | "newline";

export interface ShellCommandDisplay {
    command: string;
    status: ToolDisplayStatus;
    endsWithNewline: boolean;
    outputLines: Array<{
        text: string;
        stream: ShellOutputStream;
    }>;
}

export interface ToolDisplay {
    label: string;
    status: ToolDisplayStatus;
    summaryLines: string[];
    detailLines: string[];
    shellCommands?: ShellCommandDisplay[];
}

const TOOL_LABELS: Record<string, string> = {
    read_file: "Read file",
    write_file: "Write file",
    patch_file: "Edit file",
    delete_file: "Delete file",
    grep: "Search text",
    glob: "Find files",
    ls: "List directory",
    shell: "Run shell",
};

function isRecord(value: unknown): value is JsonRecord {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

function parseJson(value: string): unknown {
    if (!value.trim()) {
        return undefined;
    }
    try {
        return JSON.parse(value) as unknown;
    } catch {
        return undefined;
    }
}

function parseArguments(value: string): JsonRecord | undefined {
    const parsed = parseJson(value);
    return isRecord(parsed) ? parsed : undefined;
}

function parseResult(result: ToolResult | undefined): unknown {
    if (!result) {
        return undefined;
    }
    return parseJson(result.content);
}

function resultRecord(result: ToolResult | undefined): JsonRecord | undefined {
    const parsed = parseResult(result);
    if (isRecord(parsed)) {
        return parsed;
    }
    if (Array.isArray(parsed)) {
        const block = parsed.find(
            (value): value is JsonRecord =>
                isRecord(value) && value.type === "shell_call_output",
        );
        return block;
    }
    return undefined;
}

function collapse(value: string): string {
    return value.replace(/\s+/g, " ").trim();
}

function truncate(value: string, limit: number): string {
    const collapsed = collapse(value);
    if (collapsed.length <= limit) {
        return collapsed;
    }
    return `${collapsed.slice(0, Math.max(0, limit - 1))}…`;
}

function stringValue(value: unknown, fallback = ""): string {
    return typeof value === "string" ? value : fallback;
}

function numberValue(value: unknown): number | undefined {
    return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function booleanValue(value: unknown): boolean | undefined {
    return typeof value === "boolean" ? value : undefined;
}

function formatPath(value: unknown): string {
    const path = stringValue(value);
    return path ? truncate(path, MAX_VALUE_LENGTH) : "working directory";
}

function formatRange(input: JsonRecord | undefined, result: JsonRecord | undefined): string {
    const start = numberValue(result?.startLine) ?? numberValue(input?.start_line);
    const end = numberValue(result?.endLine) ?? numberValue(input?.end_line);
    if (start === undefined && end === undefined) {
        return "";
    }
    if (start !== undefined && end !== undefined) {
        return `lines ${start}–${end}`;
    }
    return start !== undefined ? `from line ${start}` : `through line ${end}`;
}

function formatBytes(value: unknown): string {
    const bytes = numberValue(value);
    return bytes === undefined ? "" : `${bytes.toLocaleString()} bytes`;
}

function formatCount(value: unknown, singular: string, plural = `${singular}s`): string {
    const count = numberValue(value);
    if (count === undefined) {
        return "";
    }
    return `${count.toLocaleString()} ${count === 1 ? singular : plural}`;
}

function splitLines(value: string): string[] {
    return value.split(/\r?\n/);
}

function previewShellOutput(
    value: string,
    stream: "stdout" | "stderr",
): {
    lines: ShellCommandDisplay["outputLines"];
    endsWithNewline: boolean;
} {
    const endsWithNewline = value.endsWith("\n");
    const content = endsWithNewline ? value.replace(/\r?\n$/, "") : value;
    return {
        lines: content
            ? splitLines(content).map((text) => ({ text, stream }))
            : [],
        endsWithNewline,
    };
}

function formatUnknownValue(value: unknown): string {
    if (typeof value === "string") {
        return truncate(value, MAX_VALUE_LENGTH);
    }
    if (typeof value === "number" || typeof value === "boolean") {
        return String(value);
    }
    if (value === null) {
        return "null";
    }
    try {
        return truncate(JSON.stringify(value), MAX_VALUE_LENGTH);
    } catch {
        return "value";
    }
}

function formatUnknownArguments(input: JsonRecord | undefined, raw: string): string[] {
    if (!input) {
        return raw.trim() ? [`arguments: ${truncate(raw, MAX_VALUE_LENGTH)}`] : [];
    }
    const entries = Object.entries(input).slice(0, MAX_SUMMARY_LINES);
    const lines = entries.map(([key, value]) => `${key}: ${formatUnknownValue(value)}`);
    if (Object.keys(input).length > entries.length) {
        lines.push(`… ${Object.keys(input).length - entries.length} more arguments`);
    }
    return lines;
}

function formatResultPreview(result: ToolResult | undefined): string[] {
    if (!result || !result.content.trim()) {
        return [];
    }
    const parsed = parseResult(result);
    if (parsed !== undefined) {
        try {
            return splitLines(JSON.stringify(parsed, null, 2));
        } catch {
            // Fall through to the raw text when a provider returned an unusual value.
        }
    }
    return splitLines(result.content);
}

function toolStatus(result: ToolResult | undefined): ToolDisplayStatus {
    if (!result) {
        return "pending";
    }
    return result.isError ? "error" : "success";
}

function shellStatus(result: ToolResult | undefined, outputList: unknown[]): ToolDisplayStatus {
    if (!result) {
        return "pending";
    }
    if (result.isError) {
        return "error";
    }
    if (outputList.length === 0) {
        return "success";
    }
    const hasFailedCommand = outputList.some((entry) => {
        if (!isRecord(entry) || !isRecord(entry.outcome)) {
            return false;
        }
        const exitCode = numberValue(entry.outcome.exitCode);
        return entry.outcome.type === "timeout" || (exitCode !== undefined && exitCode !== 0);
    });
    return hasFailedCommand ? "error" : "success";
}

function readFileDisplay(input: JsonRecord | undefined, result: ToolResult | undefined): ToolDisplay {
    const output = resultRecord(result);
    const path = formatPath(input?.path ?? output?.path);
    const range = formatRange(input, output);
    const truncated = booleanValue(output?.truncated);
    const summary = [range, truncated ? "truncated" : ""].filter(Boolean).join(" · ");
    const details = typeof output?.content === "string" ? splitLines(output.content) : [];
    return {
        label: TOOL_LABELS.read_file,
        status: toolStatus(result),
        summaryLines: [summary ? `${path} · ${summary}` : path],
        detailLines: details,
    };
}

function writeFileDisplay(input: JsonRecord | undefined, result: ToolResult | undefined): ToolDisplay {
    const output = resultRecord(result);
    const path = formatPath(input?.path ?? output?.path);
    const created = booleanValue(output?.created);
    const bytes = formatBytes(output?.bytesWritten);
    const action = created === undefined ? "file" : created ? "created" : "updated";
    const summary = [path, action, bytes].filter(Boolean).join(" · ");
    return {
        label: TOOL_LABELS.write_file,
        status: toolStatus(result),
        summaryLines: [summary],
        detailLines: [],
    };
}

function patchFileDisplay(input: JsonRecord | undefined, result: ToolResult | undefined): ToolDisplay {
    const output = resultRecord(result);
    const path = formatPath(input?.path ?? output?.path);
    const replacements = formatCount(output?.replacements, "replacement");
    const bytes = formatBytes(output?.bytesWritten);
    return {
        label: TOOL_LABELS.patch_file,
        status: toolStatus(result),
        summaryLines: [[path, replacements, bytes].filter(Boolean).join(" · ")],
        detailLines: [],
    };
}

function deleteFileDisplay(input: JsonRecord | undefined, result: ToolResult | undefined): ToolDisplay {
    const output = resultRecord(result);
    const path = formatPath(input?.path ?? output?.path);
    const deleted = booleanValue(output?.deleted);
    const action = deleted === undefined ? "file" : deleted ? "deleted" : "not deleted";
    return {
        label: TOOL_LABELS.delete_file,
        status: toolStatus(result),
        summaryLines: [`${path} · ${action}`],
        detailLines: [],
    };
}

function grepDisplay(input: JsonRecord | undefined, result: ToolResult | undefined): ToolDisplay {
    const output = resultRecord(result);
    const pattern = truncate(stringValue(input?.pattern, "pattern"), 72);
    const path = formatPath(input?.path ?? output?.root);
    const glob = stringValue(input?.glob);
    const matchCount = formatCount(output?.matches && Array.isArray(output.matches) ? output.matches.length : undefined, "match");
    const truncated = booleanValue(output?.truncated) ? "truncated" : "";
    const details = Array.isArray(output?.matches)
        ? output.matches.flatMap((match) => {
            if (!isRecord(match)) {
                return [];
            }
            const matchPath = stringValue(match.path, path);
            const line = numberValue(match.line);
            const column = numberValue(match.column);
            const location = [matchPath, line, column].filter((value) => value !== undefined).join(":");
            const text = stringValue(match.text);
            return [text ? `${location} ${text}` : location];
        })
        : [];
    const resultState = result ? matchCount || "completed" : "waiting";
    const summary = [
        `/${pattern}/`,
        path,
        glob ? `glob ${truncate(glob, 48)}` : "",
        resultState,
        truncated,
    ].filter(Boolean).join(" · ");
    return {
        label: TOOL_LABELS.grep,
        status: toolStatus(result),
        summaryLines: [summary],
        detailLines: details,
    };
}

function globDisplay(input: JsonRecord | undefined, result: ToolResult | undefined): ToolDisplay {
    const output = resultRecord(result);
    const pattern = truncate(stringValue(input?.pattern, "pattern"), 72);
    const path = formatPath(input?.path ?? output?.root);
    const paths = Array.isArray(output?.paths) ? output.paths : [];
    const count = formatCount(paths.length, "file");
    const truncated = booleanValue(output?.truncated) ? "truncated" : "";
    return {
        label: TOOL_LABELS.glob,
        status: toolStatus(result),
        summaryLines: [`${pattern} · ${path} · ${result ? count || "completed" : "waiting"}${truncated ? ` · ${truncated}` : ""}`],
        detailLines: paths.length > 0 ? paths.map((value) => String(value)) : [],
    };
}

function listDisplay(input: JsonRecord | undefined, result: ToolResult | undefined): ToolDisplay {
    const output = resultRecord(result);
    const path = formatPath(input?.path ?? output?.path);
    const entries = Array.isArray(output?.entries) ? output.entries : [];
    const count = formatCount(entries.length, "entry");
    const truncated = booleanValue(output?.truncated) ? "truncated" : "";
    const details = entries.flatMap((entry) => {
        if (!isRecord(entry)) {
            return [];
        }
        const name = stringValue(entry.name);
        const type = stringValue(entry.type, "entry");
        const size = numberValue(entry.size);
        return [`${type} ${name}${size === undefined ? "" : ` · ${size.toLocaleString()} bytes`}`];
    });
    return {
        label: TOOL_LABELS.ls,
        status: toolStatus(result),
        summaryLines: [`${path} · ${result ? count || "completed" : "waiting"}${truncated ? ` · ${truncated}` : ""}`],
        detailLines: details,
    };
}

function shellDisplay(
    input: JsonRecord | undefined,
    result: ToolResult | undefined,
    expanded: boolean,
): ToolDisplay {
    const commands = Array.isArray(input?.commands)
        ? input.commands.filter((value): value is string => typeof value === "string")
        : [];
    const outputs = resultRecord(result)?.output;
    const outputList: unknown[] = Array.isArray(outputs) ? outputs : [];
    const summaryLines = commands.slice(0, MAX_SUMMARY_LINES).map((command) => `$ ${truncate(command, 120)}`);
    if (commands.length > summaryLines.length) {
        summaryLines.push(`… ${commands.length - summaryLines.length} more commands`);
    }
    if (summaryLines.length === 0) {
        summaryLines.push("commands");
    }

    const shellCommands = commands.map((command, index): ShellCommandDisplay => {
        const output = outputList[index];
        if (!isRecord(output)) {
            return {
                command,
                status: result ? "error" : "pending",
                endsWithNewline: false,
                outputLines: [{
                    text: result ? "missing command result" : "waiting…",
                    stream: result ? "empty" : "pending",
                }],
            };
        }

        const outcome = isRecord(output.outcome) ? output.outcome : undefined;
        const outcomeType = stringValue(outcome?.type);
        const exitCode = numberValue(outcome?.exitCode);
        const status: ToolDisplayStatus = !result?.isError && outcomeType === "exit" && exitCode === 0
            ? "success"
            : "error";
        if (!expanded) {
            return {
                command,
                status,
                endsWithNewline: false,
                outputLines: [],
            };
        }

        const stdoutValue = stringValue(output.stdout);
        const stderrValue = stringValue(output.stderr);
        const selectedOutput = stdoutValue !== ""
            ? { value: stdoutValue, stream: "stdout" as const }
            : stderrValue !== ""
                ? { value: stderrValue, stream: "stderr" as const }
                : undefined;
        if (selectedOutput) {
            const preview = previewShellOutput(selectedOutput.value, selectedOutput.stream);
            return {
                command,
                status,
                endsWithNewline: preview.endsWithNewline,
                outputLines: preview.lines,
            };
        }

        const emptyOutput = outcomeType === "exit" && exitCode !== undefined
            ? `(process exited with code ${exitCode})`
            : outcomeType === "timeout"
                ? "(process timed out)"
                : "no output";
        return {
            command,
            status,
            endsWithNewline: false,
            outputLines: [{ text: emptyOutput, stream: "empty" }],
        };
    });
    return {
        label: TOOL_LABELS.shell,
        status: shellStatus(result, outputList),
        summaryLines,
        detailLines: [],
        shellCommands,
    };
}

function unknownDisplay(name: string, input: JsonRecord | undefined, rawArguments: string, result: ToolResult | undefined): ToolDisplay {
    const details = formatResultPreview(result);
    const resultSummary = result
        ? `${result.isError ? "failed" : "completed"} · ${result.content.length.toLocaleString()} chars`
        : "waiting for result";
    return {
        label: name.replace(/_/g, " "),
        status: toolStatus(result),
        summaryLines: [...formatUnknownArguments(input, rawArguments), resultSummary],
        detailLines: details,
    };
}

export function buildToolDisplay(toolCall: UIToolCall, expanded = true): ToolDisplay {
    const input = parseArguments(toolCall.arguments);
    const display = (() => {
        switch (toolCall.name) {
            case "read_file":
                return readFileDisplay(input, toolCall.result);
            case "write_file":
                return writeFileDisplay(input, toolCall.result);
            case "patch_file":
                return patchFileDisplay(input, toolCall.result);
            case "delete_file":
                return deleteFileDisplay(input, toolCall.result);
            case "grep":
                return grepDisplay(input, toolCall.result);
            case "glob":
                return globDisplay(input, toolCall.result);
            case "ls":
                return listDisplay(input, toolCall.result);
            case "shell":
                return shellDisplay(input, toolCall.result, expanded);
            default:
                return unknownDisplay(toolCall.name, input, toolCall.arguments, toolCall.result);
        }
    })();
    const errorLine = toolCall.result?.isError
        ? [`Error: ${truncate(toolCall.result.content, MAX_VALUE_LENGTH)}`]
        : [];
    return {
        ...display,
        summaryLines: [...display.summaryLines, ...errorLine],
        detailLines: display.detailLines,
    };
}
