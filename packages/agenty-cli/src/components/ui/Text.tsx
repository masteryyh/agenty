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

import { createTextAttributes, type MouseEvent } from "@opentui/core";
import { createContext, useContext, useRef, type ReactNode } from "react";

const TextNestingContext = createContext(false);

export type TextProps = {
	children?: ReactNode;
	color?: string;
	backgroundColor?: string;
	bold?: boolean;
	dimColor?: boolean;
	italic?: boolean;
	underline?: boolean;
	wrap?: "wrap" | "truncate" | "truncate-start" | "truncate-middle" | "truncate-end";
	selectable?: boolean;
	width?: number | "auto" | `${number}%`;
	height?: number | "auto" | `${number}%`;
	marginTop?: number;
	marginBottom?: number;
	onMouseDown?: (event: MouseEvent) => void;
	onMouseClick?: () => void;
	onMouseUp?: (event: MouseEvent) => void;
};

export function Text({
	children,
	color,
	backgroundColor,
	bold,
	dimColor,
	italic,
	underline,
	wrap,
	selectable,
	onMouseDown,
	onMouseClick,
	onMouseUp,
	...layout
}: TextProps) {
	const nested = useContext(TextNestingContext);
	const clickStart = useRef<{ x: number; y: number } | null>(null);
	const attributes = createTextAttributes({
		bold,
		dim: dimColor,
		italic,
		underline,
	});
	if (nested) {
		return (
			<span fg={color} bg={backgroundColor} attributes={attributes}>
				{children}
			</span>
		);
	}
	return (
		<TextNestingContext.Provider value>
			<text
				fg={color}
				bg={backgroundColor}
				attributes={attributes}
				selectable={selectable ?? true}
				wrapMode={wrap === "wrap" || !wrap ? "word" : "none"}
				truncate={wrap?.startsWith("truncate")}
				onMouseDown={
					onMouseDown || onMouseClick
						? (event) => {
							onMouseDown?.(event);
							if (event.button === 0) {
								clickStart.current = { x: event.x, y: event.y };
							}
						}
						: undefined
				}
				onMouseUp={
					onMouseUp || onMouseClick
						? (event) => {
							onMouseUp?.(event);
							const start = clickStart.current;
							clickStart.current = null;
							if (
								event.button === 0 &&
								start?.x === event.x &&
								start.y === event.y &&
								onMouseClick
							) {
								event.stopPropagation();
								onMouseClick();
							}
						}
						: undefined
				}
				{...layout}
			>
				{children}
			</text>
		</TextNestingContext.Provider>
	);
}
