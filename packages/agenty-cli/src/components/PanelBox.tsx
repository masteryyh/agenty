import type { ReactNode } from "react";

import { theme } from "../consts/theme";
import { Box } from "./ui";

interface PanelBoxProps {
    height: number;
    children: ReactNode;
}

export function PanelBox({ height, children }: PanelBoxProps) {
    return (
        <Box
            id="panel-box"
            flexDirection="column"
            width="100%"
            height={height}
            borderStyle="single"
            borderColor={theme.borderStrong}
            paddingX={1}
            paddingY={1}
        >
            <Box flexDirection="column" flexGrow={1} width="100%" overflow="hidden">
                {children}
            </Box>
        </Box>
    );
}
