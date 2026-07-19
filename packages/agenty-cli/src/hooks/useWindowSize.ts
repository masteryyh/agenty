import { useTerminalDimensions } from "@opentui/react";

export function useWindowSize(): { rows: number; columns: number } {
	const { width, height } = useTerminalDimensions();
	return { rows: height, columns: width };
}
