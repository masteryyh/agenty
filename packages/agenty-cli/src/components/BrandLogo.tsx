import { useEffect, useState } from "react";

import { AGENTY_PARTICLE_LINES, AGENTY_PARTICLE_WIDTH } from "../consts/brand";
import { theme } from "../consts/theme";
import { Box, Text } from "./ui";

export const BRAND_LOGO_WIDTH = AGENTY_PARTICLE_WIDTH + 2;

export function BrandLogo({ compact = false }: { compact?: boolean }) {
    const [cursorVisible, setCursorVisible] = useState(true);

    useEffect(() => {
        const timer = setInterval(() => {
            setCursorVisible((visible) => !visible);
        }, 600);
        return () => clearInterval(timer);
    }, []);

    return (
        <Box id="agenty-brand" width={compact ? 8 : BRAND_LOGO_WIDTH} height={compact ? 1 : 3}
            flexShrink={0} alignItems="flex-end" gap={compact ? 0 : 1}>
            {compact ? <Text color={theme.accentBright} bold>agenty&gt;</Text> : (
                <Box width={AGENTY_PARTICLE_WIDTH} height={3} flexDirection="column" flexShrink={0}>
                    {AGENTY_PARTICLE_LINES.map((line, index) => (
                        <Text key={index} color={theme.accentBright} height={1} wrap="truncate">{line}</Text>
                    ))}
                </Box>
            )}
            <Text color={theme.accentBright} width={1} height={1}>{cursorVisible ? "_" : " "}</Text>
        </Box>
    );
}
