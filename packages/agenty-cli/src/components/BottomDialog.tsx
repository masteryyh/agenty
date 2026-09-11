import { useRenderer, useTerminalDimensions } from "@opentui/react";
import {
    createContext,
    type ReactNode,
    useCallback,
    useContext,
    useEffect,
    useLayoutEffect,
    useRef,
    useState,
} from "react";

import { theme } from "../consts/theme";
import { PanelBox } from "./PanelBox";
import { Box } from "./ui";

const DIALOG_Z_INDEX = 100;

interface BottomDialogSize {
    width: number;
    height: number;
    setContentHeight?: (height: number | null) => void;
}

const BottomDialogSizeContext = createContext<BottomDialogSize>({
    width: 1,
    height: 1,
});

export function useBottomDialogSize(): BottomDialogSize {
    return useContext(BottomDialogSizeContext);
}

export function useDialogContentHeight(height: number) {
    const { setContentHeight } = useBottomDialogSize();
    useLayoutEffect(() => {
        setContentHeight?.(height);
    }, [height, setContentHeight]);
    useLayoutEffect(() => () => setContentHeight?.(null), [setContentHeight]);
}

interface BottomDialogProps {
    width: number;
    height: number;
    children: ReactNode;
}

export function BottomDialog({ width, height, children }: BottomDialogProps) {
    const renderer = useRenderer();
    const terminal = useTerminalDimensions();
    const [contentHeight, setContentHeight] = useState<number | null>(null);
    const mounted = useRef(true);
    useLayoutEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);
    const updateContentHeight = useCallback((next: number | null) => {
        if (mounted.current) {
            setContentHeight(next);
        }
    }, []);
    const resolvedHeight = contentHeight === null
        ? height
        : Math.max(1, Math.min(contentHeight + 4, terminal.height - 2));
    // Capture during render, before the underlying input's focus prop is updated.
    const previousFocus = useRef(renderer.currentFocusedRenderable);

    useEffect(() => {
        previousFocus.current?.blur();
        return () => {
            const target = previousFocus.current;
            setTimeout(() => {
                if (target && !target.isDestroyed) {
                    target.focus();
                }
            }, 1);
        };
    }, []);

    const contentSize = {
        width: Math.max(width - 4, 1),
        height: Math.max(resolvedHeight - 4, 1),
        setContentHeight: updateContentHeight,
    };

    return (
        <Box
            flexDirection="column"
            position="absolute"
            left={1}
            bottom={0}
            width={width}
            height={resolvedHeight}
            zIndex={DIALOG_Z_INDEX}
            backgroundColor={theme.surface}
        >
            <BottomDialogSizeContext.Provider value={contentSize}>
                <PanelBox height={resolvedHeight}>{children}</PanelBox>
            </BottomDialogSizeContext.Provider>
        </Box>
    );
}
