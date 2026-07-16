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

import { RGBA } from "@opentui/core";
import { useRenderer } from "@opentui/react";
import {
	createContext,
	useContext,
	useEffect,
	useRef,
	type ReactNode,
} from "react";
import { PanelBox } from "./PanelBox";
import { Box } from "./ui";

const DIALOG_Z_INDEX = 100;
const TERMINAL_BACKGROUND = RGBA.defaultBackground();

interface BottomDialogSize {
	width: number;
	height: number;
}

const BottomDialogSizeContext = createContext<BottomDialogSize>({
	width: 1,
	height: 1,
});

export function useBottomDialogSize(): BottomDialogSize {
	return useContext(BottomDialogSizeContext);
}

interface BottomDialogProps {
	width: number;
	height: number;
	children: ReactNode;
}

export function BottomDialog({ width, height, children }: BottomDialogProps) {
	const renderer = useRenderer();
	// Capture during render, before the underlying input's focus prop is updated.
	const previousFocus = useRef(renderer.currentFocusedRenderable);

	useEffect(() => {
		previousFocus.current?.blur();
		return () => {
			const target = previousFocus.current;
			setTimeout(() => {
				if (target && !target.isDestroyed) target.focus();
			}, 1);
		};
	}, []);

	const contentSize = {
		width: Math.max(width - 4, 1),
		height: Math.max(height - 4, 1),
	};

	return (
		<Box
			flexDirection="column"
			position="absolute"
			left={1}
			bottom={0}
			width={width}
			height={height}
			zIndex={DIALOG_Z_INDEX}
			backgroundColor={TERMINAL_BACKGROUND}
		>
			<BottomDialogSizeContext.Provider value={contentSize}>
				<PanelBox height={height}>{children}</PanelBox>
			</BottomDialogSizeContext.Provider>
		</Box>
	);
}
