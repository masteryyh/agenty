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
