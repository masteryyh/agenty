import { useMemo } from "react";

import { pickAsciiArt } from "../consts/asciiArts";
import { AGENTY_VERSION } from "../version";
import { Box, GradientText, Text } from "./ui";

export const LOGO_HEADER_HEIGHT = 5;

export function LogoHeader() {
    const art = useMemo(() => pickAsciiArt(), []);
    const artLines = art.split("\n");

    return (
        <Box
            borderStyle="rounded"
            borderColor="magenta"
            paddingX={1}
            flexDirection="row"
            flexShrink={0}
            gap={3}
            height={LOGO_HEADER_HEIGHT}
            overflow="hidden"
        >
            <Box flexDirection="column" flexShrink={1} overflow="hidden">
                {artLines.map((line, i) => (
                    <Text key={i} color="magenta" bold wrap="truncate">
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
                <GradientText colors={["#00E5FF", "#FF00E5"]}>
                    agenty
                </GradientText>
                <Text color="gray" wrap="truncate">v{AGENTY_VERSION}</Text>
            </Box>
        </Box>
    );
}
