import { useState } from "react";

import { AGENTY_TAGLINE } from "../consts/brand";
import { theme } from "../consts/theme";
import { useInput } from "../hooks/useInput";
import { useWindowSize } from "../hooks/useWindowSize";
import { AGENTY_VERSION } from "../version";
import { BRAND_LOGO_WIDTH, BrandLogo } from "./BrandLogo";
import { Box, Pressable, Text } from "./ui";

export function WizardWelcome({ onBegin, onExit }: {
    onBegin: () => void;
    onExit: () => void;
}) {
    const { columns, rows } = useWindowSize();
    const [selection, setSelection] = useState<"begin" | "exit">("begin");
    const compact = rows < 20;
    const minimal = rows < 14;
    const width = Math.max(1, Math.min(54, columns - (columns < 60 ? 4 : 8)));

    useInput((_input, key) => {
        if (key.upArrow || key.downArrow || key.tab) {
            setSelection((current) => current === "begin" ? "exit" : "begin");
        } else if (key.return) {
            if (selection === "begin") {
                onBegin();
            } else {
                onExit();
            }
        }
    });

    return (
        <Box id="wizard-welcome" width="100%" height="100%" flexDirection="column"
            alignItems="center" justifyContent="center" backgroundColor={theme.base}>
            <Box width={width} flexDirection="column" flexShrink={0}>
                <Box height={1} flexShrink={0} justifyContent="space-between">
                    <Text color={theme.textFaint}>FIRST RUN</Text>
                    <Text color={theme.textFaint} wrap="truncate">v{AGENTY_VERSION}</Text>
                </Box>
                <Box marginTop={minimal ? 0 : compact ? 1 : 2} flexShrink={0}>
                    <BrandLogo compact={minimal || width < BRAND_LOGO_WIDTH} />
                </Box>
                <Box marginTop={minimal ? 0 : 1} flexShrink={0}>
                    <Text color={theme.textMuted}>{AGENTY_TAGLINE}</Text>
                </Box>
                <Box marginTop={compact ? 1 : 2} flexDirection="column" flexShrink={0}>
                    <Text bold>Welcome to agenty.</Text>
                    <Text color={theme.textMuted}>
                        Select or click "Begin setup" to complete your initialization wizard.
                    </Text>
                </Box>
                <Box marginTop={compact ? 1 : 2} flexDirection="column" gap={compact ? 0 : 1}
                    flexShrink={0}>
                    <Pressable id="wizard-begin" height={1} onPress={onBegin}>
                        <Text color={selection === "begin" ? theme.selection : theme.textMuted}
                            bold={selection === "begin"}>{selection === "begin" ? "> " : "  "}Begin setup</Text>
                    </Pressable>
                    <Pressable id="wizard-exit" height={1} onPress={onExit}>
                        <Text color={selection === "exit" ? theme.selection : theme.textMuted}
                            bold={selection === "exit"}>{selection === "exit" ? "> " : "  "}Exit</Text>
                    </Pressable>
                </Box>
            </Box>
        </Box>
    );
}
