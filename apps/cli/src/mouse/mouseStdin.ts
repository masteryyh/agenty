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

import { useEffect, useState } from "react";

// Alternate screen scroll mode (DECSET 1007): while in the alternate screen,
// the terminal translates mouse wheel events into Up/Down arrow-key presses.
// We scroll by handling those arrows in useInput - one line per notch - which
// avoids enabling button-event mouse tracking (1000/1002) that would hijack the
// terminal's native text selection. Native selection then works across the
// whole window, including empty space, just like a plain terminal app.
export const MOUSE_ON = "\x1b[?1007h";
export const MOUSE_OFF = "\x1b[?1007l";

function detectMouseCapability(): boolean {
	const term = process.env.TERM ?? "";
	// Dumb terminals and the Linux virtual console do not honor 1007; they get
	// keyboard-only scrolling (PageUp/Down, arrows, End).
	return term !== "" && term !== "dumb" && term !== "linux";
}

let mouseCapable = detectMouseCapability();

/**
 * Whether the terminal supports alternate-screen mouse-wheel scrolling (1007).
 * Components can use this to decide whether to advertise wheel scrolling or
 * fall back to keyboard scrolling.
 */
export function useMouseCapability(): boolean {
	const [capable, setCapable] = useState(mouseCapable);
	useEffect(() => {
		// No runtime downgrade path today; reserved for future terminals that
		// silently ignore 1007.
		setCapable(mouseCapable);
		return () => {};
	}, []);
	return capable;
}
