#!/usr/bin/env bun
/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
