import type { ReactNode } from "react";

import { Box, Text } from "./ui";

export interface PanelProps {
    title?: ReactNode;
    description?: ReactNode;
    error?: string | null;
    children: ReactNode;
    footer?: ReactNode;
    hint?: ReactNode;
    gap?: number;
}

export function Panel({
    title,
    description,
    error,
    children,
    footer,
    hint,
    gap = 0,
}: PanelProps) {
    return (
        <Box flexDirection="column" flexGrow={1} width="100%" gap={gap}>
            {title ? (
                <Box flexDirection="column" width="100%" marginBottom={description ? 0 : 1}>
                    <Text color="magenta" bold>{title}</Text>
                    {description ? <Text dimColor>{description}</Text> : null}
                </Box>
            ) : null}
            {error ? <Text color="red">{error}</Text> : null}
            <Box flexDirection="column" flexGrow={1} width="100%" overflow="hidden">
                {children}
            </Box>
            {footer ? <Box width="100%">{footer}</Box> : null}
            {hint ? (
                <Box width="100%" height={1} overflow="hidden">
                    <Text dimColor wrap="truncate">{hint}</Text>
                </Box>
            ) : null}
        </Box>
    );
}
