import type { KeyEvent } from "@opentui/core";
import { useRenderer } from "@opentui/react";
import { createContext, type ReactNode, useContext, useEffect, useMemo, useState } from "react";

import { Box, Text } from "../components/ui";
import { theme } from "../consts/theme";
import { createInterruptController } from "./interrupt";

export interface TuiRuntime {
    exit: (code?: number) => void;
}

const TuiRuntimeContext = createContext<TuiRuntime | null>(null);

export function TuiRuntimeProvider({
    runtime,
    children,
}: {
    runtime: TuiRuntime;
    children: ReactNode;
}) {
    const renderer = useRenderer();
    const [interruptPending, setInterruptPending] = useState(false);
    const interrupts = useMemo(
        () => createInterruptController(runtime.exit, setInterruptPending),
        [runtime.exit],
    );

    useEffect(() => {
        const onKey = (event: KeyEvent) => {
            if (event.eventType === "release") {
                return;
            }
            if (event.ctrl && event.name === "c") {
                event.preventDefault();
                event.stopPropagation();
                if (event.eventType !== "repeat") {
                    interrupts.request();
                }
            } else {
                interrupts.cancel();
            }
        };
        const terminate = () => runtime.exit(0);
        // Handle interrupts before focused inputs and page shortcuts consume them.
        renderer.keyInput.prependListener("keypress", onKey);
        process.on("SIGINT", interrupts.request);
        process.on("SIGTERM", terminate);
        return () => {
            renderer.keyInput.off("keypress", onKey);
            process.off("SIGINT", interrupts.request);
            process.off("SIGTERM", terminate);
            interrupts.dispose();
        };
    }, [interrupts, renderer, runtime.exit]);

    return (
        <TuiRuntimeContext.Provider value={runtime}>
            {children}
            {interruptPending ? (
                <Box
                    id="interrupt-hint"
                    position="absolute"
                    left={0}
                    bottom={0}
                    width="100%"
                    height={1}
                    zIndex={200}
                    justifyContent="center"
                    backgroundColor={theme.base}
                >
                    <Text color={theme.warning}>Press Ctrl+C again to exit.</Text>
                </Box>
            ) : null}
        </TuiRuntimeContext.Provider>
    );
}

export function useTuiRuntime(): TuiRuntime {
    const runtime = useContext(TuiRuntimeContext);
    if (!runtime) {
        throw new Error("useTuiRuntime must be used inside TuiRuntimeProvider");
    }
    return runtime;
}
