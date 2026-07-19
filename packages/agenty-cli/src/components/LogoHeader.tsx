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

import { useMemo } from "react";
import { Box, GradientText, Text } from "./ui";
import { pickAsciiArt } from "../consts/asciiArts";
import { CLI_VERSION } from "../version";

interface LogoHeaderProps {
	runtimeVersion: string;
}

export function LogoHeader({ runtimeVersion }: LogoHeaderProps) {
	const art = useMemo(() => pickAsciiArt(), []);
	const artLines = art.split("\n");

	return (
		<Box
			borderStyle="rounded"
			borderColor="magenta"
			paddingX={1}
			flexDirection="row"
			gap={3}
		>
			<Box flexDirection="column">
				{artLines.map((line, i) => (
					<Text key={i} color="magenta" bold>
						{line}
					</Text>
				))}
			</Box>
			<Box flexDirection="column" justifyContent="center" gap={0}>
				<GradientText colors={["#00E5FF", "#FF00E5"]}>
					agenty-cli
				</GradientText>
				<Text color="gray">cli: v{CLI_VERSION}</Text>
				<Text color="gray">
					runtime: v{runtimeVersion || "—"}
				</Text>
			</Box>
		</Box>
	);
}
