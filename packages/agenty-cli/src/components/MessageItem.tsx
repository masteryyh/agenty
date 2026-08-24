import type { ThemeMode } from "@opentui/core";
import type React from "react";
import { memo, useMemo } from "react";

import type { SystemMessageVariant, UIToolCall } from "../state/store";
import {
    buildToolDisplay,
    type ShellCommandDisplay,
    type ShellOutputStream,
    type ToolDisplay,
} from "./toolDisplay";
import { Box, Pressable, Text } from "./ui";

const USER_MESSAGE_BACKGROUNDS: Record<ThemeMode, string> = {
    dark: "#2a3f47",
    light: "#dbe8ec",
};

const COMPACTED_MESSAGE_BACKGROUNDS: Record<ThemeMode, string> = {
    dark: "#23332d",
    light: "#dfece4",
};

function statusGlyph(status: "pending" | "success" | "error", blinkOn: boolean): string {
    if (status === "success") {
        return "✓";
    }
    if (status === "error") {
        return "✗";
    }
    return blinkOn ? "…" : "·";
}

function statusColor(status: "pending" | "success" | "error"): string {
    if (status === "success") {
        return "green";
    }
    if (status === "error") {
        return "red";
    }
    return "magenta";
}

function outputColor(stream: ShellOutputStream): string | undefined {
    return stream === "stderr"
        ? "red"
        : stream === "empty" || stream === "pending" || stream === "newline"
            ? "gray"
            : undefined;
}

function ConnectedOutput({
    lines,
    marginLeft,
    trailingMarker,
}: {
    lines: Array<{ text: string; stream?: ShellOutputStream }>;
    marginLeft: number;
    trailingMarker?: string;
}) {
    return (
        <Box flexDirection="column" width="100%" marginLeft={marginLeft}>
            {lines.map((line, index) => {
                const stream = line.stream ?? "stdout";
                const muted = stream !== "stderr";
                const quiet = stream === "empty" || stream === "pending" || stream === "newline";
                return (
                    <Box
                        key={`${index}-${line.text}`}
                        flexDirection="row"
                        width="100%"
                        flexShrink={0}
                    >
                        <Box width={2} flexShrink={0}>
                            <Text width={2} color="gray" dimColor>
                                {index === 0 ? "⎿ " : "  "}
                            </Text>
                        </Box>
                        <Box flexGrow={1} flexShrink={1} flexBasis={0}>
                            <Text
                                width="100%"
                                wrap="wrap"
                                color={outputColor(stream)}
                                dimColor={muted}
                                italic={quiet}
                            >
                                {line.text}
                                {trailingMarker && index === lines.length - 1 ? (
                                    <Text color="gray" dimColor italic>
                                        {trailingMarker}
                                    </Text>
                                ) : null}
                            </Text>
                        </Box>
                    </Box>
                );
            })}
        </Box>
    );
}

function ShellCommandDetails({
    commands,
    blinkOn,
}: {
    commands: ShellCommandDisplay[];
    blinkOn: boolean;
}) {
    return (
        <Box flexDirection="column" width="100%" flexShrink={0}>
            {commands.map((command, index) => (
                <Box
                    key={`${index}-${command.command}`}
                    flexDirection="column"
                    width="100%"
                    flexShrink={0}
                    marginBottom={index === commands.length - 1 ? 0 : 1}
                >
                    <Box marginLeft={2}>
                        <Text width="100%" wrap="wrap">
                            <Text color={statusColor(command.status)}>
                                {statusGlyph(command.status, blinkOn)}
                            </Text>
                            <Text> $ {command.command}</Text>
                        </Text>
                    </Box>
                    <ConnectedOutput
                        lines={command.outputLines.length > 0
                            ? command.outputLines
                            : command.endsWithNewline
                                ? [{ text: "", stream: "newline" as const }]
                                : command.outputLines}
                        marginLeft={4}
                        trailingMarker={command.endsWithNewline && command.outputLines.length > 0 ? " ↵" : undefined}
                    />
                </Box>
            ))}
        </Box>
    );
}

function ToolCallLine({
    tc,
    display,
    expanded,
    onToggle,
    blinkOn,
}: {
    tc: UIToolCall;
    display: ToolDisplay;
    expanded: boolean;
    onToggle: () => void;
    blinkOn: boolean;
}) {
    const hasShellDetails = (display.shellCommands?.length ?? 0) > 0;
    const hasDetails = display.detailLines.length > 0 || hasShellDetails;
    return (
        <Pressable
            width="100%"
            flexDirection="column"
            flexShrink={0}
            disabled={!hasDetails}
            onPress={onToggle}
        >
            <Text width="100%" wrap="wrap">
                <Text color={statusColor(display.status)}>
                    {statusGlyph(display.status, blinkOn)}
                </Text>
                <Text> </Text>
                <Text bold>{display.label}</Text>
                {hasDetails ? <Text dimColor>{expanded ? " ▾" : " ▸"}</Text> : null}
            </Text>
            {!(expanded && hasShellDetails)
                ? display.summaryLines.map((line, index) => (
                    <Box key={`${tc.id}-summary-${index}`} marginLeft={2}>
                        <Text width="100%" dimColor wrap="wrap">
                            {line}
                        </Text>
                    </Box>
                ))
                : null}
            {expanded && hasShellDetails ? (
                <ShellCommandDetails commands={display.shellCommands ?? []} blinkOn={blinkOn} />
            ) : null}
            {expanded && !hasShellDetails && display.detailLines.length > 0 ? (
                <ConnectedOutput
                    lines={display.detailLines.map((text) => ({ text }))}
                    marginLeft={4}
                />
            ) : null}
        </Pressable>
    );
}

function ToolMessageItem({
    item,
    onToggleTool,
}: {
    item: Extract<MessageRenderItem, { type: "tool" }>;
    onToggleTool?: (id: string) => void;
}) {
    const display = useMemo(
        () => buildToolDisplay(item.toolCall, item.expanded),
        [item.toolCall, item.expanded],
    );
    const done = !!item.toolCall.result;
    return (
        <Rail
            color={display.status === "error" ? "red" : done || item.blinkOn ? "magenta" : "gray"}
        >
            <ToolCallLine
                tc={item.toolCall}
                display={display}
                expanded={item.expanded}
                blinkOn={item.blinkOn}
                onToggle={() => onToggleTool?.(item.id)}
            />
        </Rail>
    );
}

export type MessageRenderItem =
    | {
        id: string;
        groupId: string;
        type: "message";
        role: "user" | "assistant" | "system";
        content: string;
        error?: boolean;
        systemVariant?: SystemMessageVariant;
    }
    | {
        id: string;
        groupId: string;
        type: "reasoning";
        content: string;
        durationSeconds: number;
        done: boolean;
        expanded: boolean;
    }
    | {
        id: string;
        groupId: string;
        type: "tool";
        toolCall: UIToolCall;
        blinkOn: boolean;
        expanded: boolean;
    };

function Rail({
    color,
    children,
    onMouseClick,
}: {
    color: string;
    children: React.ReactNode;
    onMouseClick?: () => void;
}) {
    return (
        <Pressable
            flexDirection="column"
            width="100%"
            flexShrink={0}
            borderStyle="single"
            borderColor={color}
            borderTop={false}
            borderRight={false}
            borderBottom={false}
            paddingLeft={1}
            disabled={!onMouseClick}
            onPress={onMouseClick}
        >
            {children}
        </Pressable>
    );
}

export const MessageItem = memo(({
    item,
    onToggleReasoning,
    onToggleTool,
    themeMode = "dark",
}: {
    item: MessageRenderItem;
    onToggleReasoning?: (id: string) => void;
    onToggleTool?: (id: string) => void;
    themeMode?: ThemeMode;
}) => {
    if (item.type === "reasoning") {
        return (
            <Pressable
                flexDirection="column"
                width="100%"
                paddingX={1}
                disabled={!onToggleReasoning}
                onPress={() => onToggleReasoning?.(item.id)}
            >
                <Text width="100%" dimColor={!item.expanded} italic={!item.expanded} wrap="wrap">
                    {item.done
                        ? `Thought for ${item.durationSeconds.toFixed(1)}s.`
                        : `Thinking for ${item.durationSeconds.toFixed(1)}s...`}
                </Text>
                {item.expanded ? (
                    <Box marginTop={1}>
                        <Text width="100%" dimColor italic wrap="wrap">
                            {item.content}
                        </Text>
                    </Box>
                ) : null}
            </Pressable>
        );
    }

    if (item.type === "tool") {
        return <ToolMessageItem item={item} onToggleTool={onToggleTool} />;
    }

    if (item.role === "user") {
        return (
            <Box
                width="100%"
                paddingX={1}
                backgroundColor={USER_MESSAGE_BACKGROUNDS[themeMode]}
            >
                <Text width="100%" wrap="wrap">
                    <Text dimColor>you</Text>
                    <Text color="cyan"> › </Text>
                    {item.content}
                </Text>
            </Box>
        );
    }

    if (item.role === "system") {
        if (item.systemVariant === "compacted") {
            return (
                <Box
                    width="100%"
                    paddingX={1}
                    backgroundColor={COMPACTED_MESSAGE_BACKGROUNDS[themeMode]}
                >
                    <Text width="100%" italic dimColor wrap="wrap">
                        {item.content}
                    </Text>
                </Box>
            );
        }

        return (
            <Rail color={item.error ? "red" : "yellow"}>
                <Text width="100%" color={item.error ? "red" : "yellow"} wrap="wrap">
                    {item.error ? "✗" : "●"} {item.content}
                </Text>
            </Rail>
        );
    }

    return (
        <Box width="100%" flexShrink={0} paddingX={1}>
            <Text width="100%" wrap="wrap">
                {item.content}
            </Text>
        </Box>
    );
});
