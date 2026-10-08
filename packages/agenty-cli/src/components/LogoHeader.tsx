import { theme } from "../consts/theme";
import { useWindowSize } from "../hooks/useWindowSize";
import { AGENTY_VERSION } from "../version";
import { BRAND_LOGO_WIDTH, BrandLogo } from "./BrandLogo";
import { Box, Text } from "./ui";

export const LOGO_HEADER_HEIGHT = 5;

export function LogoHeader() {
    const { columns } = useWindowSize();
    const versionLabel = `v${AGENTY_VERSION}`;
    const compact = columns < BRAND_LOGO_WIDTH + 5 + versionLabel.length;

    return (
        <Box
            id="agenty-header"
            paddingX={1}
            paddingY={1}
            flexDirection={compact ? "column" : "row"}
            flexShrink={0}
            gap={compact ? 0 : 3}
            height={LOGO_HEADER_HEIGHT}
            overflow="hidden"
        >
            <BrandLogo compact={compact} />
            <Box
                flexDirection="column"
                flexShrink={1}
                minWidth={0}
                justifyContent="center"
                gap={0}
            >
                <Text color={theme.textMuted} wrap="truncate">{versionLabel}</Text>
            </Box>
        </Box>
    );
}
