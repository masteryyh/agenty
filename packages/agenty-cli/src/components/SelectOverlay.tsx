import { useEffect, useRef, useState } from "react";
import { useInput } from "../hooks/useInput";
import { Box, Select, Spinner, Text } from "./ui";

export interface SelectEntry<T> {
	label: string;
	data: T;
}

interface SelectOverlayProps<T> {
	title: string;
	load: () => Promise<SelectEntry<T>[]>;
	onSelect: (data: T) => void;
	onClose: () => void;
	emptyHint?: string;
	dialog?: boolean;
	visibleOptionCount?: number;
}

export function SelectOverlay<T>({
	title,
	load,
	onSelect,
	onClose,
	emptyHint,
	dialog = false,
	visibleOptionCount = 10,
}: SelectOverlayProps<T>) {
	const [entries, setEntries] = useState<SelectEntry<T>[] | null>(null);
	const [error, setError] = useState<string | null>(null);
	const loadRef = useRef(load);
	loadRef.current = load;

	useEffect(() => {
		let cancelled = false;
		loadRef
			.current()
			.then((r) => {
				if (!cancelled) setEntries(r);
			})
			.catch((e) => {
				if (!cancelled) setError((e as Error).message);
			});
		return () => {
			cancelled = true;
		};
	}, []);

	useInput((_input, key) => {
		if (key.escape) onClose();
	});

	const options =
		entries?.map((e, i) => ({ label: e.label, value: String(i) })) ?? [];

	return (
		<Box
			flexDirection="column"
			flexGrow={dialog ? 1 : undefined}
			paddingX={dialog ? 0 : 2}
			paddingY={dialog ? 0 : 1}
		>
			<Box marginBottom={1} gap={1}>
				<Text color="magenta" bold>
					{title}
				</Text>
				<Text dimColor>· Esc to cancel</Text>
			</Box>
			{error ? (
				<Text color="red">Failed: {error}</Text>
			) : entries === null ? (
				<Spinner label="Loading..." />
			) : entries.length === 0 ? (
				<Text dimColor>{emptyHint ?? "No items"}</Text>
			) : (
				<Select
					options={options}
					visibleOptionCount={Math.max(
						1,
						Math.min(options.length, visibleOptionCount),
					)}
					onChange={(v) => {
						const idx = Number(v);
						const entry = entries[idx];
						if (entry) onSelect(entry.data);
					}}
				/>
			)}
		</Box>
	);
}
