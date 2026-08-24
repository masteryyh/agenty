import type { KeyEvent } from "@opentui/core";
import type { ReactNode } from "react";

import type { InputKey } from "../hooks/useInput";
import { useInput } from "../hooks/useInput";
import {
    createTableLayout,
    type TableColumn,
    TableHeader,
    TableRow,
} from "./Table";
import { Box, Pressable, Text } from "./ui";

export interface ListRenderState {
    index: number;
    selected: boolean;
}

export interface ListProps<T> {
    items: T[];
    cursor: number;
    visibleCount: number;
    getKey: (item: T, index: number) => string;
    renderItem: (item: T, state: ListRenderState) => ReactNode;
    onCursor: (index: number) => void;
    onActivate?: (item: T, index: number) => void;
    active?: boolean;
    emptyHint?: ReactNode;
}

export function listWindow(
    itemCount: number,
    cursor: number,
    visibleCount: number,
): { start: number; end: number } {
    const count = Math.max(Math.min(visibleCount, itemCount), 0);
    const boundedCursor = Math.min(Math.max(cursor, 0), Math.max(itemCount - 1, 0));
    const half = Math.floor(count / 2);
    let start = Math.max(boundedCursor - half, 0);
    if (start + count > itemCount) {
        start = Math.max(itemCount - count, 0);
    }
    return { start, end: start + count };
}

export function List<T>({
    items,
    cursor,
    visibleCount,
    getKey,
    renderItem,
    onCursor,
    onActivate,
    active = true,
    emptyHint,
}: ListProps<T>) {
    if (items.length === 0) {
        return (
            <Box flexGrow={1} width="100%">
                {emptyHint ?? <Text dimColor>No items.</Text>}
            </Box>
        );
    }

    const { start, end } = listWindow(items.length, cursor, visibleCount);
    return (
        <Box flexDirection="column" flexGrow={1} width="100%" overflow="hidden">
            {items.slice(start, end).map((item, localIndex) => {
                const index = start + localIndex;
                const selected = active && index === cursor;
                return (
                    <Pressable
                        key={getKey(item, index)}
                        width="100%"
                        height={1}
                        overflow="hidden"
                        onPress={() => {
                            onCursor(index);
                            onActivate?.(item, index);
                        }}
                    >
                        <Box width={2} height={1}>
                            <Text color={selected ? "cyan" : "gray"}>
                                {selected ? "❯" : " "}
                            </Text>
                        </Box>
                        {renderItem(item, { index, selected })}
                    </Pressable>
                );
            })}
        </Box>
    );
}

export interface ListNavigationOptions<T> {
    items: T[];
    cursor: number;
    onCursor: (index: number) => void;
    onActivate?: (item: T, index: number) => void;
    onClose?: () => void;
    onInput?: (
        input: string,
        key: InputKey,
        event: KeyEvent,
        item: T | undefined,
    ) => void;
    active?: boolean;
}

export function useListNavigation<T>({
    items,
    cursor,
    onCursor,
    onActivate,
    onClose,
    onInput,
    active = true,
}: ListNavigationOptions<T>) {
    useInput((input, key, event) => {
        if (key.escape && onClose) {
            onClose();
            return;
        }
        if (key.upArrow) {
            event.preventDefault();
            onCursor(Math.max(cursor - 1, 0));
            return;
        }
        if (key.downArrow) {
            event.preventDefault();
            onCursor(Math.min(cursor + 1, Math.max(items.length - 1, 0)));
            return;
        }

        const item = items[cursor];
        if (key.return && item && onActivate) {
            onActivate(item, cursor);
            return;
        }
        onInput?.(input, key, event, item);
    }, { isActive: active });
}

export interface KeyValueRow {
    key: string;
    value: string;
}

export function KeyValueList({
    rows,
    availableWidth,
}: {
    rows: KeyValueRow[];
    availableWidth: number;
}) {
    const columns: Array<TableColumn<KeyValueRow>> = [
        {
            key: "key",
            header: "Key",
            value: (row) => row.key,
            render: (row) => <Text color="gray" wrap="truncate">{row.key}</Text>,
        },
        {
            key: "value",
            header: "Value",
            value: (row) => row.value,
            render: (row) => <Text color="white" wrap="truncate">{row.value}</Text>,
        },
    ];
    const tableLayout = createTableLayout(columns, rows, availableWidth, 1);
    return (
        <Box flexDirection="column" flexGrow={1} width="100%">
            <TableHeader columns={tableLayout} gap={1} />
            {rows.map((row) => (
                <TableRow
                    key={row.key}
                    columns={tableLayout}
                    row={row}
                    selected={false}
                    gap={1}
                />
            ))}
        </Box>
    );
}
