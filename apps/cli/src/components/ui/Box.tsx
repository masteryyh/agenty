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

import type { BoxRenderable, ColorInput, MouseEvent } from "@opentui/core";
import { forwardRef, useRef, type ReactNode } from "react";

export type BoxProps = {
	children?: ReactNode;
	id?: string;
	width?: number | "auto" | `${number}%`;
	height?: number | "auto" | `${number}%`;
	minWidth?: number;
	maxWidth?: number;
	minHeight?: number;
	maxHeight?: number;
	flexDirection?: "row" | "column" | "row-reverse" | "column-reverse";
	flexGrow?: number;
	flexShrink?: number;
	flexBasis?: number | "auto";
	flexWrap?: "nowrap" | "wrap" | "wrap-reverse";
	alignItems?: "auto" | "flex-start" | "center" | "flex-end" | "stretch" | "baseline" | "space-between" | "space-around" | "space-evenly";
	alignSelf?: "auto" | "flex-start" | "center" | "flex-end" | "stretch" | "baseline" | "space-between" | "space-around" | "space-evenly";
	justifyContent?: "flex-start" | "center" | "flex-end" | "space-between" | "space-around" | "space-evenly";
	padding?: number;
	paddingX?: number;
	paddingY?: number;
	paddingTop?: number;
	paddingRight?: number;
	paddingBottom?: number;
	paddingLeft?: number;
	margin?: number | "auto";
	marginX?: number | "auto";
	marginY?: number | "auto";
	marginTop?: number | "auto";
	marginRight?: number | "auto";
	marginBottom?: number | "auto";
	marginLeft?: number | "auto";
	gap?: number;
	overflow?: "visible" | "hidden" | "scroll";
	overflowY?: "visible" | "hidden";
	borderStyle?: "single" | "double" | "rounded" | "bold";
	borderColor?: string;
	borderTop?: boolean;
	borderRight?: boolean;
	borderBottom?: boolean;
	borderLeft?: boolean;
	backgroundColor?: ColorInput;
	position?: "relative" | "absolute";
	top?: number;
	right?: number;
	bottom?: number;
	left?: number;
	zIndex?: number;
	focusable?: boolean;
	focused?: boolean;
	onMouse?: (event: MouseEvent) => void;
	onMouseDown?: (event: MouseEvent) => void;
	onMouseClick?: () => void;
	onMouseUp?: (event: MouseEvent) => void;
	onMouseOver?: (event: MouseEvent) => void;
	onMouseOut?: (event: MouseEvent) => void;
	onMouseScroll?: (event: MouseEvent) => void;
};

export const Box = forwardRef<BoxRenderable, BoxProps>(function Box(
	{
		children,
		overflowY,
		borderTop,
		borderRight,
		borderBottom,
		borderLeft,
		borderStyle,
		flexWrap,
		flexDirection = "row",
		onMouseDown,
		onMouseClick,
		onMouseUp,
		...props
	},
	ref,
) {
	const clickStart = useRef<{ x: number; y: number } | null>(null);
	const hasBorderSides =
		borderTop !== undefined ||
		borderRight !== undefined ||
		borderBottom !== undefined ||
		borderLeft !== undefined;
	const border = hasBorderSides
		? ([
				...(borderTop === false ? [] : ["top"]),
				...(borderRight === false ? [] : ["right"]),
				...(borderBottom === false ? [] : ["bottom"]),
				...(borderLeft === false ? [] : ["left"]),
			] as Array<"top" | "right" | "bottom" | "left">)
		: undefined;
	return (
		<box
			ref={ref}
			{...props}
			border={border ?? (borderStyle ? true : undefined)}
			borderStyle={borderStyle === "bold" ? "heavy" : borderStyle}
			flexWrap={flexWrap === "nowrap" ? "no-wrap" : flexWrap}
			flexDirection={flexDirection}
			overflow={props.overflow ?? overflowY}
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
		>
			{children}
		</box>
	);
});
