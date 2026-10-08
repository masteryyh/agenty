import { type ReactNode, useCallback, useLayoutEffect, useRef, useState } from "react";

import { theme } from "../consts/theme";
import { useWindowSize } from "../hooks/useWindowSize";
import { DialogSizeProvider } from "./BottomDialog";
import { Box } from "./ui";

export function WizardPage({ children, maxWidth = 88 }: { children: ReactNode; maxWidth?: number }) {
    const { columns, rows } = useWindowSize();
    const [contentHeight, setContentHeight] = useState<number | null>(null);
    const mounted = useRef(true);
    useLayoutEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);
    const updateContentHeight = useCallback((height: number | null) => {
        if (mounted.current) {
            setContentHeight(height);
        }
    }, []);
    const width = Math.max(1, Math.min(maxWidth, columns - (columns < 60 ? 4 : 8)));
    const availableHeight = Math.max(1, rows - (rows < 16 ? 2 : 4));
    const height = Math.min(contentHeight ?? availableHeight, availableHeight);

    return (
        <Box
            width="100%"
            height="100%"
            flexDirection="column"
            alignItems="center"
            justifyContent="center"
            backgroundColor={theme.base}
        >
            <DialogSizeProvider value={{ width, height, setContentHeight: updateContentHeight }}>
                <Box id="wizard-page" width={width} height={height} flexDirection="column">
                    {children}
                </Box>
            </DialogSizeProvider>
        </Box>
    );
}
