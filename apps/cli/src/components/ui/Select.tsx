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
