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
