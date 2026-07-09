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

import { EventEmitter } from "node:events";
import { useEffect, useRef } from "react";

export interface WheelEvent {
	direction: "up" | "down";
}

export interface ClickEvent {
	x: number;
	y: number;
}

// ink@7 does not parse mouse events; we intercept SGR mouse tracking sequences
// out of the raw stdin stream before they can reach ink's keyboard pipeline
// (and, e.g., pollute the TextInput). Mouse events are re-published here so the
// message list can subscribe without touching the keyboard path.
const emitter = new EventEmitter();
emitter.setMaxListeners(Infinity);
export const mouseEmitter = emitter;

// SGR mouse report: ESC [ < Cb ; Cx ; Cy (M|m). Wheel reports set bit 6 of Cb
// (64 = wheel up, 65 = wheel down).
const SGR_MOUSE = /\x1b\[<(\d+);(\d+);(\d+)[Mm]/g;
// An incomplete SGR mouse report at the tail of a chunk, to hold across reads.
const SGR_MOUSE_PARTIAL = /\x1b\[<[\d;]*$/;

function filterMouse(input: string): string {
	let clean = "";
	let lastIndex = 0;
	SGR_MOUSE.lastIndex = 0;
	let match: RegExpExecArray | null;
	while ((match = SGR_MOUSE.exec(input)) !== null) {
		clean += input.slice(lastIndex, match.index);
		lastIndex = match.index + match[0].length;
		const cb = Number(match[1]);
		const x = Number(match[2]);
		const y = Number(match[3]);
		const suffix = match[0].endsWith("M") ? "press" : "release";
		// Bit 6 marks a wheel event, bit 0 its direction.
		if (cb & 64) {
			emitter.emit("wheel", {
				direction: cb & 1 ? "down" : "up",
			} satisfies WheelEvent);
		} else if (suffix === "press" && (cb & 3) === 0 && !(cb & 32)) {
			emitter.emit("click", { x, y } satisfies ClickEvent);
		}
	}
	clean += input.slice(lastIndex);
	return clean;
}

/**
 * Wrap a stdin stream so that SGR mouse sequences are stripped from the bytes
 * ink reads. Only `read()` is intercepted; every other stream/EventEmitter
 * member is forwarded to the underlying stdin unchanged.
 */
export function wrapStdin(stdin: NodeJS.ReadStream): NodeJS.ReadStream {
	let buffer = "";
	return new Proxy(stdin, {
		get(target, prop, receiver) {
			if (prop === "read") {
				return (...args: unknown[]) => {
					const read = target.read as (...a: unknown[]) => unknown;
					const chunk = read.apply(target, args);
					if (chunk === null || chunk === undefined) {
						return chunk;
					}
					const str = buffer + String(chunk);
					// Hold back an incomplete trailing mouse report so a report
					// split across two reads is still recognized as a whole.
					const partial = str.match(SGR_MOUSE_PARTIAL);
					if (partial) {
						buffer = partial[0];
						return filterMouse(str.slice(0, str.length - partial[0].length));
					}
					buffer = "";
					return filterMouse(str);
				};
			}
			const value = Reflect.get(target, prop, receiver);
			return typeof value === "function" ? value.bind(target) : value;
		},
	}) as NodeJS.ReadStream;
}

/**
 * Subscribe to wheel events published by {@link wrapStdin}. The latest callback
 * is stored in a ref so inline handlers do not thrash the subscription.
 */
export function useMouseWheel(
	onWheel: (event: WheelEvent) => void,
	active = true,
): void {
	const handlerRef = useRef(onWheel);
	handlerRef.current = onWheel;
	useEffect(() => {
		if (!active) return;
		const listener = (event: WheelEvent) => handlerRef.current(event);
		emitter.on("wheel", listener);
		return () => {
			emitter.off("wheel", listener);
		};
	}, [active]);
}

export function useMouseClick(
	onClick: (event: ClickEvent) => void,
	active = true,
): void {
	const handlerRef = useRef(onClick);
	handlerRef.current = onClick;
	useEffect(() => {
		if (!active) return;
		const listener = (event: ClickEvent) => handlerRef.current(event);
		emitter.on("click", listener);
		return () => {
			emitter.off("click", listener);
		};
	}, [active]);
}
