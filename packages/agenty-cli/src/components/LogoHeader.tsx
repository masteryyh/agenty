import { useMemo } from "react";

import { pickAsciiArt } from "../consts/asciiArts";
import { theme } from "../consts/theme";
import { AGENTY_VERSION } from "../version";
import { Box, Text } from "./ui";

export const LOGO_HEADER_HEIGHT = 5;

export function LogoHeader() {
    const art = useMemo(() => pickAsciiArt(), []);
    const artLines = art.split("\n");

    return (
        <Box
            borderStyle="rounded"
            borderColor={theme.border}
            paddingX={1}
            flexDirection="row"
            flexShrink={0}
            gap={3}
            height={LOGO_HEADER_HEIGHT}
            overflow="hidden"
        >
            <Box flexDirection="column" flexShrink={1} overflow="hidden">
                {artLines.map((line, i) => (
                    <Text key={i} color={theme.accent} bold wrap="truncate">
                        {line}
                    </Text>
                ))}
            </Box>
            <Box
                flexDirection="column"
                flexShrink={0}
                justifyContent="center"
                gap={0}
            >
                <Text color={theme.accentBright} bold>agenty</Text>
                <Text color={theme.textMuted} wrap="truncate">v{AGENTY_VERSION}</Text>
            </Box>
        </Box>
    );
}
