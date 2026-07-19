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
