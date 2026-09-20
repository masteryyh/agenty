import type { ScrollBoxRenderable } from "@opentui/core";
import { type ReactNode, useRef } from "react";

import { commands } from "../commands/registry";
import { theme } from "../consts/theme";
import { useInput } from "../hooks/useInput";
import { useBottomDialogSize } from "./BottomDialog";
import { Panel } from "./Panel";
import { Box, Text } from "./ui";

function HelpSection({ title, children }: { title: string; children: ReactNode }) {
    return (
        <Box flexDirection="column" width="100%" marginBottom={1}>
            <Text color={theme.accent} bold>{title}</Text>
            {children}
        </Box>
    );
}

function HelpLine({
    label,
    labelWidth,
    description,
}: {
    label: string;
    labelWidth: number;
    description: string;
}) {
    return (
        <Box flexDirection="row" width="100%">
            <Box width={labelWidth} flexShrink={0}>
                <Text color={theme.accent} wrap="wrap">{label}</Text>
            </Box>
            <Box flexGrow={1} flexBasis={0}>
                <Text color={theme.textMuted} wrap="wrap">{description}</Text>
            </Box>
        </Box>
    );
}

export function HelpPanel({ onClose }: { onClose: () => void }) {
    const { width, height } = useBottomDialogSize();
    const scrollRef = useRef<ScrollBoxRenderable | null>(null);
    const wideLayout = width >= 64;
    const labelWidth = wideLayout ? 16 : width < 48 ? 12 : 18;

    const commandSection = (
        <HelpSection title="Commands">
            {commands.map((command) => (
                <HelpLine
                    key={command.name}
                    label={command.name}
                    labelWidth={labelWidth}
                    description={command.description}
                />
            ))}
        </HelpSection>
    );
    const syntaxSection = (
        <HelpSection title="Syntax">
            <HelpLine label="text" labelWidth={labelWidth} description="Send a normal prompt to the current session." />
            <HelpLine label="/command [args]" labelWidth={labelWidth} description="Run a CLI command; quoted arguments may contain spaces." />
            <HelpLine label="$skill-name" labelWidth={labelWidth} description="Search and insert a Skill reference with Tab." />
            <HelpLine label="Tab" labelWidth={labelWidth} description="Complete command, argument or Skill." />
        </HelpSection>
    );
    const shortcutsSection = (
        <HelpSection title="Shortcuts">
            <HelpLine label="?" labelWidth={labelWidth} description="Open or close this help panel when the input is empty." />
            <HelpLine label="↑ / ↓" labelWidth={labelWidth} description="Browse submitted input history." />
            <HelpLine label="Enter" labelWidth={labelWidth} description="Send input or confirm an overlay action." />
            <HelpLine label="Esc" labelWidth={labelWidth} description="Cancel, close an overlay or deny approval." />
            <HelpLine label="PageUp / PageDown" labelWidth={labelWidth} description="Scroll the message list or this panel." />
            <HelpLine label="End" labelWidth={labelWidth} description="Jump the message list to the latest output." />
            <HelpLine label="Ctrl+C" labelWidth={labelWidth} description="Exit agenty." />
            <HelpLine label="Ctrl+R / Ctrl+T" labelWidth={labelWidth} description="Toggle reasoning or tool details." />
            <HelpLine label="Shift+Tab" labelWidth={labelWidth} description="Cycle permission mode: ask, auto, yolo." />
            <HelpLine label="Y / N" labelWidth={labelWidth} description="Allow or deny the current tool approval." />
        </HelpSection>
    );

    useInput((input, key, event) => {
        if (key.escape || input === "?") {
            event.preventDefault();
            event.stopPropagation();
            onClose();
            return;
        }
        if (key.upArrow || key.downArrow || key.pageUp || key.pageDown) {
            event.preventDefault();
            event.stopPropagation();
            const viewport = scrollRef.current?.viewport.height ?? 1;
            const amount = key.pageUp || key.pageDown ? viewport : 1;
            scrollRef.current?.scrollBy(key.upArrow || key.pageUp ? -amount : amount);
        }
    });

    return (
        <Panel title="Help" description="Commands, input syntax and keyboard shortcuts" hint="? or Esc to close">
            <scrollbox
                id="help-scroll"
                ref={scrollRef}
                width="100%"
                height={Math.max(height - 4, 1)}
                scrollY
                viewportCulling={false}
                contentOptions={{ flexDirection: "column", flexGrow: 0, flexShrink: 0 }}
                verticalScrollbarOptions={{
                    showArrows: false,
                    trackOptions: {
                        backgroundColor: theme.surface,
                        foregroundColor: theme.borderStrong,
                    },
                }}
            >
                <Box flexDirection={wideLayout ? "row" : "column"} width="100%" gap={wideLayout ? 2 : 0}>
                    <Box
                        flexDirection="column"
                        width={wideLayout ? "48%" : "100%"}
                        flexGrow={wideLayout ? 1 : 0}
                        flexBasis={wideLayout ? 0 : "auto"}
                    >
                        {commandSection}
                    </Box>
                    <Box
                        flexDirection="column"
                        width={wideLayout ? "48%" : "100%"}
                        flexGrow={wideLayout ? 1 : 0}
                        flexBasis={wideLayout ? 0 : "auto"}
                    >
                        {syntaxSection}
                        {shortcutsSection}
                    </Box>
                </Box>
            </scrollbox>
        </Panel>
    );
}
