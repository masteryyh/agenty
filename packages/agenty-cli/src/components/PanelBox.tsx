import type { ReactNode } from "react";
import { Box } from "./ui";

interface PanelBoxProps {
	height: number;
	children: ReactNode;
}

export function PanelBox({ height, children }: PanelBoxProps) {
	return (
		<Box
			flexDirection="column"
			width="100%"
			height={height}
			borderStyle="single"
			borderColor="magenta"
			paddingX={1}
			paddingY={1}
		>
			<Box flexDirection="column" flexGrow={1} width="100%" overflow="hidden">
				{children}
			</Box>
		</Box>
	);
}
