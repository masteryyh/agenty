import type { SelectRenderable } from "@opentui/core";
import { useRef } from "react";

export type SelectProps = {
	options: Array<{ label: string; value: string }>;
	visibleOptionCount: number;
	onChange: (value: string) => void;
};

export function Select({
	options,
	visibleOptionCount,
	onChange,
}: SelectProps) {
	const ref = useRef<SelectRenderable>(null);
	return (
		<select
			ref={ref}
			focused
			height={Math.max(1, visibleOptionCount)}
			options={options.map((option) => ({
				name: option.label,
				description: "",
				value: option.value,
			}))}
			showDescription={false}
			showScrollIndicator={options.length > visibleOptionCount}
			selectedBackgroundColor="#24383f"
			selectedTextColor="#c7f5ff"
			onSelect={(_index, option) => {
				if (option) onChange(String(option.value));
			}}
		/>
	);
}
