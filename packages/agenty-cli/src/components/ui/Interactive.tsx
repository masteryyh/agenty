import type { ReactNode } from "react";
import { useState } from "react";

import { Box, type BoxProps } from "./Box";
import { Text, TextBackgroundContext } from "./Text";

export const HOVER_BACKGROUND = "#243238";

export interface PressableProps extends Omit<
    BoxProps,
    "backgroundColor" | "children" | "onMouse" | "onMouseClick" | "onMouseOut" | "onMouseOver"
> {
    children: ReactNode;
    backgroundColor?: BoxProps["backgroundColor"];
    disabled?: boolean;
    hoverBackgroundColor?: BoxProps["backgroundColor"];
    onPress?: () => void;
}

export function Pressable({
    children,
    backgroundColor,
    disabled = false,
    hoverBackgroundColor = HOVER_BACKGROUND,
    onPress,
    ...layout
}: PressableProps) {
    const [hovered, setHovered] = useState(false);
    const interactive = !disabled && onPress !== undefined;
    const resolvedBackground = hovered && interactive ? hoverBackgroundColor : backgroundColor;
    const textBackground = typeof resolvedBackground === "string"
        ? resolvedBackground
        : undefined;

    return (
        <TextBackgroundContext.Provider value={textBackground}>
            <Box
                {...layout}
                backgroundColor={resolvedBackground}
                onMouse={interactive ? (event) => {
                    if (event.type === "over" || event.type === "move") {
                        setHovered(true);
                    } else if (event.type === "out") {
                        setHovered(false);
                    }
                } : undefined}
                onMouseOver={interactive ? () => setHovered(true) : undefined}
                onMouseOut={interactive ? () => setHovered(false) : undefined}
                onMouseClick={interactive ? onPress : undefined}
            >
                {children}
            </Box>
        </TextBackgroundContext.Provider>
    );
}

export type ActionTone = "default" | "danger";

export interface ActionBarItem {
    key: string;
    label: string;
    disabled?: boolean;
    tone?: ActionTone;
}

export interface ActionBarProps {
    actions: ActionBarItem[];
    activeKey?: string;
    gap?: number;
    onAction: (key: string) => void;
}

export function ActionBar({
    actions,
    activeKey,
    gap = 2,
    onAction,
}: ActionBarProps) {
    return (
        <Box width="100%" height={1} gap={gap} overflow="hidden">
            {actions.map((action) => {
                const active = activeKey === action.key;
                const color = action.disabled
                    ? "gray"
                    : action.tone === "danger"
                        ? "red"
                        : active
                            ? "cyan"
                            : "gray";
                return (
                    <Pressable
                        key={action.key}
                        height={1}
                        disabled={action.disabled}
                        onPress={() => onAction(action.key)}
                    >
                        <Text color={color} bold={active || action.tone === "danger"}>
                            {active ? `[${action.label}]` : ` ${action.label} `}
                        </Text>
                    </Pressable>
                );
            })}
        </Box>
    );
}
