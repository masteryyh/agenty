import type { ReactNode } from "react";

import { theme } from "../consts/theme";
import { useDialogContentHeight } from "./BottomDialog";
import { Box, Text } from "./ui";

interface FormShellProps {
    titleLines: string[];
    errorLines: string[];
    preferredBodyHeight: number;
    bodyHeight: number;
    footerHeight: number;
    footer: ReactNode;
    hint: string;
    compact: boolean;
    children: ReactNode;
}

export function FormShell({
    titleLines,
    errorLines,
    preferredBodyHeight,
    bodyHeight,
    footerHeight,
    footer,
    hint,
    compact,
    children,
}: FormShellProps) {
    useDialogContentHeight(titleLines.length + 1 + errorLines.length + preferredBodyHeight + footerHeight + 2);

    return (
        <Box flexDirection="column" width="100%" flexGrow={1}>
            <Box flexDirection="column" width="100%" height={titleLines.length + (compact ? 0 : 1)} flexShrink={0}>
                <Text color={theme.accent} bold>{titleLines.join("\n")}</Text>
            </Box>
            {errorLines.length > 0 ? (
                <Box width="100%" height={errorLines.length} flexShrink={0}>
                    <Text color={theme.danger}>{errorLines.join("\n")}</Text>
                </Box>
            ) : null}
            <Box width="100%" height={bodyHeight} flexShrink={0} position="relative">
                {children}
            </Box>
            <Box width="100%" height={footerHeight + (compact ? 0 : 1)} paddingTop={compact ? 0 : 1} flexShrink={0} flexDirection="column">
                {footer}
            </Box>
            <Box width="100%" height={1} flexShrink={0} overflow="hidden">
                <Text dimColor wrap="truncate">{hint}</Text>
            </Box>
        </Box>
    );
}
