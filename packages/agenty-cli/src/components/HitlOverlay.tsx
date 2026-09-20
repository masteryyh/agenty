import type { ScrollBoxRenderable } from "@opentui/core";
import { useRef, useState } from "react";

import type { ToolApprovalResolution } from "../api/types";
import { theme } from "../consts/theme";
import { useInput } from "../hooks/useInput";
import type { PendingToolApproval } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";
import { ActionBar, Box, Text } from "./ui";

export const HITL_OVERLAY_HEIGHT = 22;

interface HitlOverlayProps {
    approval: PendingToolApproval;
    onDecision: (decision: ToolApprovalResolution["decision"]) => void;
    onTogglePermission?: () => void;
}

// Control characters must never become terminal instructions in an approval.
function displayText(value: string): string {
    return value.replace(/\p{Cc}/gu, (character) => character === "\n" || character === "\t"
        ? character : `\\u${character.charCodeAt(0).toString(16).padStart(4, "0")}`);
}

export function HitlOverlay({ approval, onDecision, onTogglePermission = () => undefined }: HitlOverlayProps) {
    const { width, height } = useBottomDialogSize();
    const [choice, setChoice] = useState<ToolApprovalResolution["decision"]>("deny");
    const scroll = useRef<ScrollBoxRenderable | null>(null);
    const submitted = useRef(false);
    // A failed RPC makes the same request actionable again.
    if (!approval.submitting && approval.error) {
        submitted.current = false;
    }
    const choose = (decision: ToolApprovalResolution["decision"]) => {
        if (approval.submitting || submitted.current) {
            return;
        }
        submitted.current = true;
        onDecision(decision);
    };
    const showHints = height >= 6;
    const previewHeight = Math.max(height - (showHints ? 4 : 2), 1);

    useInput((input, key, event) => {
        if (key.shift && key.tab) {
            event.preventDefault();
            event.stopPropagation();
            onTogglePermission();
            return;
        }
        if (key.ctrl || key.meta) {
            return;
        }
        event.preventDefault();
        event.stopPropagation();
        if (key.upArrow || key.downArrow || key.pageUp || key.pageDown) {
            const amount = key.pageUp || key.pageDown ? previewHeight : 1;
            scroll.current?.scrollBy(key.upArrow || key.pageUp ? -amount : amount);
        } else if (key.leftArrow || key.rightArrow || key.tab) {
            setChoice((current) => current === "allow" ? "deny" : "allow");
        } else if (key.escape || input.toLowerCase() === "n") {
            choose("deny");
        } else if (input.toLowerCase() === "y") {
            choose("allow");
        } else if (key.return) {
            choose(choice);
        }
    });

    return (
        <Box id="hitl-overlay" flexDirection="column" width={width} height={height} overflow="hidden">
            <Box height={1} flexShrink={0} gap={1}>
                <Text bold color={theme.accent}>Tool approval</Text>
                {width >= 40 ? <Text color={theme.textMuted}>· {displayText(approval.toolCall.name)}</Text> : null}
            </Box>
            <scrollbox
                id="hitl-preview"
                ref={scroll}
                width="100%"
                height={previewHeight}
                flexShrink={0}
                scrollY
                scrollX={false}
                viewportCulling={false}
                contentOptions={{ flexDirection: "column", flexGrow: 0, flexShrink: 0 }}
                verticalScrollbarOptions={{
                    showArrows: false,
                    trackOptions: { backgroundColor: theme.surface, foregroundColor: theme.borderStrong },
                }}
            >
                <Box flexDirection="column" width="100%" flexShrink={0} paddingY={showHints ? 1 : 0} gap={1}>
                    {approval.message ? <Text width="100%" color={theme.warning} wrap="wrap">{displayText(approval.message)}</Text> : null}
                    <Text width="100%" bold wrap="wrap">{displayText(approval.preview.title)}</Text>
                    <Text width="100%" color={theme.textMuted} wrap="wrap">{`Working directory: ${displayText(approval.cwd)}`}</Text>
                    <Box flexDirection="column" width="100%" flexShrink={0} backgroundColor={theme.surfaceRaised} paddingX={1} paddingY={showHints ? 1 : 0}>
                        <Text width="100%" wrap="wrap">{displayText(approval.preview.detail)}</Text>
                    </Box>
                    {approval.error ? <Text color={theme.danger} wrap="wrap">{displayText(approval.error)}</Text> : null}
                </Box>
            </scrollbox>
            {showHints ? (
                <Box height={1} flexShrink={0}>
                    <Text color={theme.textMuted}>{approval.submitting ? "Sending decision…" : width >= 40 ? "This decision applies to this call only." : "This call only."}</Text>
                </Box>
            ) : null}
            <ActionBar
                actions={[
                    { key: "allow", label: "Allow once", disabled: approval.submitting },
                    { key: "deny", label: "Deny", disabled: approval.submitting },
                ]}
                activeKey={choice}
                onAction={(key) => choose(key === "allow" ? "allow" : "deny")}
            />
            {showHints ? (
                <Text dimColor>{width >= 56 ? "Shift+Tab Cycle mode · ← → Choose · Enter Confirm · Esc Deny · ↑ ↓ Scroll" : "Enter · Esc Deny · ↑ ↓ Scroll"}</Text>
            ) : null}
        </Box>
    );
}
