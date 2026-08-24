import { RGBA } from "@opentui/core";
import type { ReactNode } from "react";
import { useState } from "react";

import { useInput } from "../hooks/useInput";
import { useBottomDialogSize } from "./BottomDialog";
import { ActionBar, Box, Text } from "./ui";

const CONFIRM_DIALOG_Z_INDEX = 120;
const TERMINAL_BACKGROUND = RGBA.defaultBackground();

export interface ConfirmDialogProps {
    title: string;
    message: ReactNode;
    confirmLabel?: string;
    cancelLabel?: string;
    onConfirm: () => void;
    onCancel: () => void;
    active?: boolean;
}

export function ConfirmDialog({
    title,
    message,
    confirmLabel = "Delete",
    cancelLabel = "Cancel",
    onConfirm,
    onCancel,
    active = true,
}: ConfirmDialogProps) {
    const dialogSize = useBottomDialogSize();
    const [cursor, setCursor] = useState(1);
    const width = Math.max(Math.min(52, dialogSize.width), 1);
    const height = Math.max(Math.min(7, dialogSize.height), 1);

    const activate = (index: number) => {
        if (index === 0) {
            onConfirm();
        } else {
            onCancel();
        }
    };

    useInput((input, key) => {
        if (key.escape || input.toLowerCase() === "n") {
            onCancel();
            return;
        }
        if (input.toLowerCase() === "y") {
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
                width={width}
                height={height}
                flexDirection="column"
                borderStyle="single"
                borderColor="red"
                backgroundColor={TERMINAL_BACKGROUND}
                paddingX={1}
                paddingY={1}
            >
                <Text color="red" bold>{title}</Text>
                <Box flexGrow={1} width="100%" overflow="hidden">
                    <Text wrap="wrap">{message}</Text>
                </Box>
                <ActionBar
                    actions={[
                        { key: "confirm", label: confirmLabel, tone: "danger" },
                        { key: "cancel", label: cancelLabel },
                    ]}
                    activeKey={cursor === 0 ? "confirm" : "cancel"}
                    onAction={(key) => activate(key === "confirm" ? 0 : 1)}
                />
            </Box>
        </Box>
    );
}
