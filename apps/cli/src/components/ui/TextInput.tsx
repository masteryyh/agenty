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

import { RenderableEvents, type InputRenderable } from "@opentui/core";
import { forwardRef, useCallback, useEffect, useRef } from "react";

export type TextInputProps = {
	value: string;
	onChange: (value: string) => void;
	onSubmit: (value: string) => void;
	placeholder?: string;
	focus?: boolean;
	keepFocus?: boolean;
};

export const TextInput = forwardRef<InputRenderable, TextInputProps>(
	function TextInput(
		{
			value,
			onChange,
			onSubmit,
			placeholder,
			focus = true,
			keepFocus = false,
		},
		ref,
	) {
		const inputRef = useRef<InputRenderable | null>(null);
		const keepFocusRef = useRef(focus && keepFocus);
		keepFocusRef.current = focus && keepFocus;
		const assignRef = useCallback(
			(node: InputRenderable | null) => {
				inputRef.current = node;
				if (typeof ref === "function") ref(node);
				else if (ref) ref.current = node;
			},
			[ref],
		);

		useEffect(() => {
			if (focus) inputRef.current?.focus();
			else inputRef.current?.blur();
		}, [focus]);

		useEffect(() => {
			const input = inputRef.current;
			if (!input) return;

			let refocusTimer: ReturnType<typeof setTimeout> | null = null;
			const handleBlur = () => {
				if (!keepFocusRef.current) return;
				refocusTimer = setTimeout(() => {
					if (keepFocusRef.current && !input.isDestroyed) input.focus();
				}, 1);
			};

			input.on(RenderableEvents.BLURRED, handleBlur);
			return () => {
				if (refocusTimer) clearTimeout(refocusTimer);
				input.off(RenderableEvents.BLURRED, handleBlur);
			};
		}, []);

		const handleSubmit = (submitted: unknown) => {
			onSubmit(typeof submitted === "string" ? submitted : value);
		};
		return (
			<input
				ref={assignRef}
				width="100%"
				value={value}
				placeholder={placeholder}
				focused={focus}
				textColor="#ffffff"
				focusedTextColor="#ffffff"
				cursorColor="#00e5ff"
				onInput={onChange}
				onSubmit={handleSubmit}
			/>
		);
	},
);
