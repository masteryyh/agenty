import type { ReactNode } from "react";
import { useState } from "react";

import { theme } from "../consts/theme";
import { useInput } from "../hooks/useInput";
import { useBottomDialogSize } from "./BottomDialog";
import { ActionBar, Box, Text } from "./ui";

const CONFIRM_DIALOG_Z_INDEX = 120;

export interface ConfirmDialogProps {
    title: string;
    message: ReactNode;
    confirmLabel?: string;
    cancelLabel?: string;
    onConfirm: () => void;
    onCancel: () => void;
    active?: boolean;
    letterShortcuts?: boolean;
    tone?: "danger" | "default";
    dialogWidth?: number;
    dialogHeight?: number;
}

export function ConfirmDialog({
    title,
    message,
    confirmLabel = "Delete",
    cancelLabel = "Cancel",
    onConfirm,
    onCancel,
    active = true,
    letterShortcuts = true,
    tone = "danger",
    dialogWidth = 52,
    dialogHeight = 7,
}: ConfirmDialogProps) {
    const dialogSize = useBottomDialogSize();
    const [cursor, setCursor] = useState(1);
    const width = Math.max(Math.min(dialogWidth, dialogSize.width), 1);
    const height = Math.max(Math.min(dialogHeight, dialogSize.height), 1);
    const color = tone === "danger" ? theme.danger : theme.accent;

    const activate = (index: number) => {
        if (index === 0) {
            onConfirm();
        } else {
            onCancel();
        }
    };

    useInput((input, key) => {
        if (key.escape || (letterShortcuts && input.toLowerCase() === "n")) {
            onCancel();
            return;
        }
        if (letterShortcuts && input.toLowerCase() === "y") {
            onConfirm();
            return;
        }
        if (key.leftArrow || key.rightArrow) {
            setCursor((current) => current === 0 ? 1 : 0);
            return;
        }
        if (key.return) {
            activate(cursor);
        }
    }, { isActive: active });

    return (
        <Box
            position="absolute"
            left={0}
            top={0}
            width={dialogSize.width}
            height={dialogSize.height}
            zIndex={CONFIRM_DIALOG_Z_INDEX}
            alignItems="center"
            justifyContent="center"
        >
            <Box
                id="confirm-dialog"
                width={width}
                height={height}
                flexDirection="column"
                borderStyle="single"
                borderColor={color}
                backgroundColor={theme.surfaceRaised}
                paddingX={1}
                paddingY={1}
            >
                <Text color={color} bold>{title}</Text>
                <Box flexGrow={1} width="100%" overflow="hidden">
                    {typeof message === "string" || typeof message === "number" ? <Text wrap="wrap">{message}</Text> : message}
                </Box>
                <ActionBar
                    actions={[
                        { key: "confirm", label: confirmLabel, tone },
                        { key: "cancel", label: cancelLabel },
                    ]}
                    activeKey={cursor === 0 ? "confirm" : "cancel"}
                    onAction={(key) => activate(key === "confirm" ? 0 : 1)}
                />
            </Box>
        </Box>
    );
}
