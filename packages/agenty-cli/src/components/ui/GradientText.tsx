import { useMemo } from "react";
import { Text } from "./Text";

export function GradientText({
	children,
	colors,
}: {
	children: string;
	colors: [string, string];
}) {
	const segments = useMemo(() => {
		const chars = Array.from(children);
		const parse = (hex: string) => [
			Number.parseInt(hex.slice(1, 3), 16),
			Number.parseInt(hex.slice(3, 5), 16),
			Number.parseInt(hex.slice(5, 7), 16),
		];
		const start = parse(colors[0]);
		const end = parse(colors[1]);
		return chars.map((char, index) => {
			const ratio = chars.length <= 1 ? 0 : index / (chars.length - 1);
			const rgb = start.map((value, channel) =>
				Math.round(value + (end[channel] - value) * ratio),
			);
			return {
				char,
				color: `#${rgb.map((value) => value.toString(16).padStart(2, "0")).join("")}`,
			};
		});
	}, [children, colors]);
	return (
		<Text bold>
			{segments.map((segment, index) => (
				<Text key={`${segment.char}-${index}`} color={segment.color}>
					{segment.char}
				</Text>
			))}
		</Text>
	);
}
