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

import { render } from "ink";
import { App } from "./App";
import { runCLICommand } from "./cli/run";
import { useAppStore } from "./state/store";

const command = await runCLICommand(process.argv.slice(2));
if (command.handled) {
	process.exitCode = command.exitCode;
} else {
	const instance = render(<App />, {
		alternateScreen: true,
		interactive: true,
		exitOnCtrlC: true,
	});

	await instance.waitUntilExit().catch(() => {});
	const { abort, _localServerStop } = useAppStore.getState();
	abort();
	await _localServerStop?.();
	process.exit(0);
}
