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
