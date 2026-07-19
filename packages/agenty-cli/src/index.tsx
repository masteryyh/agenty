#!/usr/bin/env bun
import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { App } from "./App";
import { runCLICommand } from "./cli/run";
import { useAppStore } from "./state/store";
import { TuiRuntimeProvider } from "./tui/runtime";

const command = await runCLICommand(process.argv.slice(2));
if (command.handled) {
	process.exitCode = command.exitCode;
} else {
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
		if (shuttingDown) return;
		shuttingDown = true;
		process.exitCode = exitCode;
		const { abort, _localServerStop } = useAppStore.getState();
		try {
			abort();
			root.unmount();
			await _localServerStop?.();
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
			<App />
		</TuiRuntimeProvider>,
	);

	await destroyed;
	process.off("SIGINT", onSignal);
	process.off("SIGTERM", onSignal);
}
