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

import type { KeyEvent } from "@opentui/core";
import { useKeyboard } from "@opentui/react";

export interface InputKey {
	ctrl: boolean;
	shift: boolean;
	meta: boolean;
	return: boolean;
	escape: boolean;
	tab: boolean;
	upArrow: boolean;
	downArrow: boolean;
	leftArrow: boolean;
	rightArrow: boolean;
	pageUp: boolean;
	pageDown: boolean;
	home: boolean;
	end: boolean;
	backspace: boolean;
	delete: boolean;
}

function toInputKey(event: KeyEvent): InputKey {
	return {
		ctrl: event.ctrl,
		shift: event.shift,
		meta: event.meta,
		return: event.name === "return" || event.name === "linefeed",
		escape: event.name === "escape",
		tab: event.name === "tab",
		upArrow: event.name === "up",
		downArrow: event.name === "down",
		leftArrow: event.name === "left",
		rightArrow: event.name === "right",
		pageUp: event.name === "pageup",
		pageDown: event.name === "pagedown",
		home: event.name === "home",
		end: event.name === "end",
		backspace: event.name === "backspace",
		delete: event.name === "delete",
	};
}

export function useInput(
	handler: (input: string, key: InputKey, event: KeyEvent) => void,
	options: { isActive?: boolean } = {},
) {
	useKeyboard((event) => {
		if (options.isActive === false || event.eventType === "release") return;
		handler(event.sequence ?? "", toInputKey(event), event);
	});
}
