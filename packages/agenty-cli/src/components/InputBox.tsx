import { forwardRef } from "react";

import type { PermissionMode, SkillDto, ToolDialect } from "../api/types";
import type { ComposerDocument } from "../composer/document";
import { effortColor, theme } from "../consts/theme";
import { useInput } from "../hooks/useInput";
import { useWindowSize } from "../hooks/useWindowSize";
import type { ToastMsg } from "../state/store";
import { StructuredTextInput, type StructuredTextInputHandle } from "./StructuredTextInput";
import { Box, Spinner, Text } from "./ui";

const PLACEHOLDER = "type a message, / for commands, or $ for skills";

function abbreviateCwd(wd: string, max = 40): string {
    const home = process.env.HOME ?? process.env.USERPROFILE ?? "";
    let display = home && wd.startsWith(home) ? `~${wd.slice(home.length)}` : wd;
    if (display.length <= max) {
        return display;
    }
    return `…${display.slice(display.length - max + 1)}`;
}

interface InputBoxProps {
    document: ComposerDocument;
    skills: SkillDto[];
    onChange: (document: ComposerDocument) => void;
    onSubmit: (document: ComposerDocument) => void;
    onCursorChange: (offset: number) => void;
    onHistoryMove: (direction: "up" | "down") => boolean;
    onTab: () => boolean;
    streaming: boolean;
    phrase: string | null;
    modelName: string;
    cwd: string;
    contextWindow: number;
    tokenConsumed: number;
    permissionMode: PermissionMode;
    pendingPermissionMode?: PermissionMode;
    toolDialect: ToolDialect;
    thinkingLevel: string;
    reasoningActive: boolean;
    abort: () => void;
    toast: ToastMsg | null;
    active?: boolean;
}

export const InputBox = forwardRef<StructuredTextInputHandle, InputBoxProps>(({
    document,
    skills,
    onChange,
    onSubmit,
    onCursorChange,
    onHistoryMove,
    onTab,
    streaming,
    phrase,
    modelName,
    cwd,
    contextWindow,
    tokenConsumed,
    permissionMode,
    pendingPermissionMode,
    toolDialect,
    thinkingLevel,
    reasoningActive,
    abort,
    toast,
    active = true,
}: InputBoxProps, ref) => {
    const { columns } = useWindowSize();
    const shortcuts = columns >= 100
        ? "? Help · Shift+Tab Permissions · ↑↓ History · Tab Complete · PgUp/PgDn Scroll"
        : columns >= 60
            ? "? Help · Shift+Tab Mode · ↑↓ History · PgUp/PgDn Scroll"
            : "? Help · Shift+Tab · ↑↓ History";

    useInput(
        (_input, key, event) => {
            if (streaming) {
                if (key.escape) {
                    abort();
                }
                return;
            }
            if (key.upArrow || key.downArrow) {
                event.preventDefault();
                event.stopPropagation();
                onHistoryMove(key.upArrow ? "up" : "down");
                return;
            }
            if (key.tab) {
                event.preventDefault();
                if (!key.shift) {
                    onTab();
                }
            }
        },
        { isActive: active },
    );

    return (
        <Box flexDirection="column" paddingX={1}>
            <Box flexDirection="row" height={1} overflow="hidden">
                {streaming && phrase ? (
                    <>
                        <Spinner label={phrase} />
                        {reasoningActive && thinkingLevel ? (
                            <Text dimColor>
                                {` (thinking with ${thinkingLevel} effort)`}
                            </Text>
                        ) : null}
                        <Text dimColor> Esc to cancel </Text>
                    </>
                ) : null}
                <Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
                    <Text color={theme.textFaint}>{"─".repeat(300)}</Text>
                </Box>
                {toast ? (
                    <Text color={toast.error ? theme.danger : theme.success}>{toast.text}</Text>
                ) : <>
                    <Text color={theme.accent}>{` ▸ ${modelName}`}</Text>
                    {thinkingLevel ? <Text color={effortColor(thinkingLevel)}>{` (${thinkingLevel})`}</Text> : null}
                </>}
            </Box>

            <Box flexDirection="row" height={1} overflow="hidden">
                <Box width={2} flexShrink={0} height={1}>
                    <Text color={theme.accent} bold>
                        {"❯ "}
                    </Text>
                </Box>
                {streaming ? (
                    <Text dimColor>{PLACEHOLDER}</Text>
                ) : (
                    <Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
                        <StructuredTextInput
                            ref={ref}
                            document={document}
                            skills={skills}
                            onChange={onChange}
                            onSubmit={onSubmit}
                            onCursorChange={onCursorChange}
                            placeholder={PLACEHOLDER}
                            focus={active}
                            keepFocus={active}
                        />
                    </Box>
                )}
            </Box>

            <Box height={1} overflow="hidden">
                <Text color={theme.textFaint}>{"─".repeat(300)}</Text>
            </Box>

            <Box flexDirection="row" height={1} overflow="hidden">
                <Box flexDirection="row" flexShrink={1} height={1} overflow="hidden">
                    <Text color={theme.textFaint} wrap="truncate-start">
                        {abbreviateCwd(cwd)}
                    </Text>
                    {permissionMode !== "ask" || pendingPermissionMode ? (
                        <>
                            <Text> </Text>
                            <Text color={permissionMode === "yolo" ? theme.danger : theme.accent}>
                                {`${permissionMode} mode${pendingPermissionMode ? ` → ${pendingPermissionMode} (pending)` : ""}`}
                            </Text>
                        </>
                    ) : null}
                    {toolDialect === "codex" ? (
                        <>
                            <Text> </Text>
                            <Text color={theme.accent}>codex mode</Text>
                        </>
                    ) : null}
                </Box>
                <Box flexGrow={1} flexBasis={0} height={1} overflow="hidden" />
                <Text color={theme.textFaint}>{`context: ${contextWindow}/${tokenConsumed}`}</Text>
            </Box>
            <Box height={1} overflow="hidden">
                <Text color={theme.textFaint} wrap="truncate">
                    {shortcuts}
                </Text>
            </Box>
        </Box>
    );
});

InputBox.displayName = "InputBox";
