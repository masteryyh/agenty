import { useEffect, useRef, useState } from "react";

import { List, useListNavigation } from "./List";
import { Panel } from "./Panel";
import { Box, Spinner, Text } from "./ui";

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
    const [cursor, setCursor] = useState(0);
    const loadRef = useRef(load);
    loadRef.current = load;

    useEffect(() => {
        let cancelled = false;
        loadRef
            .current()
            .then((r) => {
                if (!cancelled) {
                    setEntries(r);
                }
            })
            .catch((error: unknown) => {
                if (!cancelled) {
                    setError(error instanceof Error ? error.message : String(error));
                }
            });
        return () => {
            cancelled = true;
        };
    }, []);

    useListNavigation({
        items: entries ?? [],
        cursor,
        onCursor: setCursor,
        onActivate: (entry) => onSelect(entry.data),
        onClose,
        active: entries !== null && entries.length > 0,
    });

    return (
        <Box
            flexDirection="column"
            flexGrow={dialog ? 1 : undefined}
            paddingX={dialog ? 0 : 2}
            paddingY={dialog ? 0 : 1}
        >
            <Panel title={title} error={error ? `Failed: ${error}` : null} hint="Enter select · Esc cancel">
                {entries === null ? (
                    <Spinner label="Loading..." />
                ) : entries.length === 0 ? (
                    <Text dimColor>{emptyHint ?? "No items"}</Text>
                ) : (
                    <List
                        items={entries}
                        cursor={cursor}
                        visibleCount={Math.max(1, Math.min(entries.length, visibleOptionCount))}
                        getKey={(_entry, index) => String(index)}
                        onCursor={setCursor}
                        onActivate={(entry) => onSelect(entry.data)}
                        renderItem={(entry, { selected }) => (
                            <Box flexGrow={1} flexBasis={0} height={1} overflow="hidden">
                                <Text color={selected ? "cyan" : "white"} bold={selected} wrap="truncate">
                                    {entry.label}
                                </Text>
                            </Box>
                        )}
                    />
                )}
            </Panel>
        </Box>
    );
}
