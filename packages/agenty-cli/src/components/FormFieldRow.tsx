import type { ReactNode } from "react";

import { theme } from "../consts/theme";
import { FORM_MARKER_WIDTH, type FormLayout } from "./formLayout";
import { Box, Pressable, Text } from "./ui";

interface FormFieldRowProps {
    layout: FormLayout;
    labelLines: string[];
    height: number;
    selected: boolean;
    disabled: boolean;
    disclosure?: string;
    onPress: () => void;
    children?: ReactNode;
}

export function FormFieldRow({
    layout,
    labelLines,
    height,
    selected,
    disabled,
    disclosure,
    onPress,
    children,
}: FormFieldRowProps) {
    const stacked = layout.mode === "stacked";
    return (
        <Pressable
            width="100%"
            height={height}
            flexShrink={0}
            alignItems="flex-start"
            disabled={disabled}
            onPress={onPress}
        >
            <Box width={FORM_MARKER_WIDTH} height={1} flexShrink={0}>
                <Text color={selected ? theme.selection : theme.textMuted}>{selected ? "❯" : " "}</Text>
            </Box>
            {disclosure ? (
                <Text color={selected ? theme.selection : theme.textMuted} bold={selected} wrap="truncate">
                    {disclosure}
                </Text>
            ) : (
                <Box
                    width={Math.max(layout.contentWidth - FORM_MARKER_WIDTH, 1)}
                    height={height}
                    flexDirection={stacked ? "column" : "row"}
                    gap={stacked ? 0 : 2}
                >
                    <Box width={layout.labelWidth} height={labelLines.length} flexDirection="column" flexShrink={0}>
                        {labelLines.map((line, index) => (
                            <Box key={index} width="100%" height={1} justifyContent={stacked ? "flex-start" : "flex-end"}>
                                <Text color={selected ? theme.selection : theme.textMuted} bold={selected}>{line}</Text>
                            </Box>
                        ))}
                    </Box>
                    <Box width={layout.valueWidth} height={1} flexShrink={0} overflow="hidden">
                        {children}
                    </Box>
                </Box>
            )}
        </Pressable>
    );
}
