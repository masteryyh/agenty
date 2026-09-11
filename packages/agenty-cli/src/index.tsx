#!/usr/bin/env bun
import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";

import { App } from "./App";
import { runCLICommand } from "./cli/run";
import { Box } from "./components/ui";
import { theme } from "./consts/theme";
import { useAppStore } from "./state/store";
import { TuiRuntimeProvider } from "./tui/runtime";

const command = await runCLICommand(process.argv.slice(2));
if (command.handled) {
    process.exitCode = command.exitCode;
} else {
    await useAppStore.getState().init();

    let resolveDestroyed!: () => void;
    const destroyed = new Promise<void>((resolve) => {
        resolveDestroyed = resolve;
    });
    const renderer = await createCliRenderer({
        screenMode: "alternate-screen",
        consoleMode: "disabled",
        exitOnCtrlC: false,
        exitSignals: [],
        useMouse: true,
        autoFocus: true,
        onDestroy: resolveDestroyed,
    });
    const root = createRoot(renderer);
    let shuttingDown = false;

    const shutdown = async (exitCode = 0) => {
        if (shuttingDown) {
            return;
        }
        shuttingDown = true;
        process.exitCode = exitCode;
        const { abort, _localCoreStop } = useAppStore.getState();
        try {
            abort();
            root.unmount();
            await _localCoreStop?.();
        } finally {
            renderer.destroy();
        }
    };

    const onSignal = () => {
        void shutdown(0);
    };
    process.once("SIGINT", onSignal);
    process.once("SIGTERM", onSignal);

    root.render(
        <TuiRuntimeProvider runtime={{ exit: (code) => void shutdown(code) }}>
            {/* Absolute dark base: every token is an absolute hex value, so the
                app paints its own background instead of inheriting the
                terminal's (which may be light). */}
            <Box width="100%" height="100%" flexDirection="column" backgroundColor={theme.base}>
                <App />
            </Box>
        </TuiRuntimeProvider>,
    );

    await destroyed;
    process.off("SIGINT", onSignal);
    process.off("SIGTERM", onSignal);
}
