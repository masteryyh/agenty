import { useEffect, useState } from "react";
import { Text } from "./Text";

const SPINNER_FRAMES = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];

export function Spinner({ label }: { label: string }) {
	const [frame, setFrame] = useState(0);
	useEffect(() => {
		const timer = setInterval(() => {
			setFrame((value) => (value + 1) % SPINNER_FRAMES.length);
		}, 80);
		return () => clearInterval(timer);
	}, []);
	return (
		<Text color="cyan">
			{SPINNER_FRAMES[frame]} {label}
		</Text>
	);
}
