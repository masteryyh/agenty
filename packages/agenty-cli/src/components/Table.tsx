import type { ReactNode } from "react";

import { Box, Text } from "./ui";

export interface TableColumn<T> {
    key: string;
    header: string;
    value: (row: T) => string;
    render?: (row: T, selected: boolean) => ReactNode;
}

export interface TableColumnLayout<T> extends TableColumn<T> {
    width: number;
}

export interface TableProps<T> {
    columns: Array<TableColumn<T>>;
    rows: T[];
    availableWidth: number;
    getRowKey?: (row: T, index: number) => string;
    isRowSelected?: (row: T, index: number) => boolean;
    gap?: number;
}

export interface TableRowProps<T> {
    columns: Array<TableColumnLayout<T>>;
    row: T;
    selected: boolean;
    gap?: number;
}

export function textWidth(value: string): number {
    return Bun.stringWidth(value);
}

export function truncateText(value: string, width: number): string {
    if (width <= 0) {
        return "";
    }
    if (textWidth(value) <= width) {
        return value;
    }
    if (width === 1) {
        return "…";
    }

    let result = "";
    let used = 0;
    for (const character of value) {
        const characterWidth = textWidth(character);
        if (used + characterWidth > width - 1) {
            break;
        }
        result += character;
        used += characterWidth;
    }
    return `${result}…`;
}

export function allocateColumnWidths(
    availableWidth: number,
    contentWidths: number[],
): number[] {
    const columnCount = contentWidths.length;
    if (columnCount === 0) {
        return [];
    }

    const budget = Math.max(Math.floor(availableWidth), 0);
    const defaultWidth = Math.floor(budget / columnCount);
    const remainder = budget % columnCount;
    const widths = contentWidths.map((_contentWidth, index) =>
        defaultWidth + (index < remainder ? 1 : 0));

    while (true) {
        const receivers = widths
            .map((width, index) => ({ index, need: contentWidths[index] - width }))
            .filter(({ need }) => need > 0);
        const donors = widths
            .map((width, index) => ({ index, spare: width - contentWidths[index] }))
            .filter(({ spare }) => spare > 0);
        if (receivers.length === 0 || donors.length === 0) {
            break;
        }

        let remainingSpare = donors.reduce((sum, donor) => sum + donor.spare, 0);
        const share = Math.max(Math.floor(remainingSpare / receivers.length), 1);
        let transferred = 0;
        for (const receiver of receivers) {
            let receiverBudget = Math.min(receiver.need, share, remainingSpare);
            for (const donor of donors) {
                if (receiverBudget <= 0) {
                    break;
                }
                if (donor.spare <= 0) {
                    continue;
                }
                const amount = Math.min(receiverBudget, donor.spare);
                widths[receiver.index] += amount;
                widths[donor.index] -= amount;
                receiverBudget -= amount;
                donor.spare -= amount;
                remainingSpare -= amount;
                transferred += amount;
            }
            if (remainingSpare === 0) {
                break;
            }
        }
        if (transferred === 0) {
            break;
        }
    }

    return widths;
}

export function createTableLayout<T>(
    columns: Array<TableColumn<T>>,
    rows: T[],
    availableWidth: number,
    gap = 2,
): Array<TableColumnLayout<T>> {
    const gapWidth = Math.max(columns.length - 1, 0) * gap;
    const contentBudget = Math.max(availableWidth - gapWidth, 0);
    const contentWidths = columns.map((column) => {
        const header = column.header.trim();
        if (!header) {
            throw new Error(`Table column "${column.key}" requires a header.`);
        }
        return Math.max(
            textWidth(header.toUpperCase()),
            ...rows.map((row) => textWidth(column.value(row))),
        );
    });
    const widths = allocateColumnWidths(
        contentBudget,
        contentWidths,
    );
    return columns.map((column, index) => ({ ...column, width: widths[index] }));
}

export function Table<T>({
    columns,
    rows,
    availableWidth,
    getRowKey,
    isRowSelected,
    gap = 2,
}: TableProps<T>) {
    const layout = createTableLayout(columns, rows, availableWidth, gap);
    return (
        <Box flexDirection="column" width="100%">
            <TableHeader columns={layout} gap={gap} />
            {rows.map((row, index) => (
                <TableRow
                    key={getRowKey?.(row, index) ?? index}
                    columns={layout}
                    row={row}
                    selected={isRowSelected?.(row, index) ?? false}
                    gap={gap}
                />
            ))}
        </Box>
    );
}

export function TableHeader<T>({
    columns,
    gap = 2,
}: {
    columns: Array<TableColumnLayout<T>>;
    gap?: number;
}) {
    return (
        <Box width="100%" height={1} gap={gap} overflow="hidden">
            {columns.map((column) => (
                <TableCell
                    key={column.key}
                    width={column.width}
                >
                    <Text dimColor bold wrap="truncate">
                        {truncateText(column.header.toUpperCase(), column.width)}
                    </Text>
                </TableCell>
            ))}
        </Box>
    );
}

export function TableRow<T>({
    columns,
    row,
    selected,
    gap = 2,
}: TableRowProps<T>) {
    return (
        <Box width="100%" height={1} gap={gap} overflow="hidden">
            {columns.map((column) => (
                <TableCell
                    key={column.key}
                    width={column.width}
                >
                    {column.render ? (
                        column.render(row, selected)
                    ) : (
                        <Text
                            color={selected ? "cyan" : "white"}
                            bold={selected}
                            wrap="truncate"
                        >
                            {truncateText(column.value(row), column.width)}
                        </Text>
                    )}
                </TableCell>
            ))}
        </Box>
    );
}

export function TableCell({
    children,
    width,
}: {
    children: ReactNode;
    width: number;
}) {
    return (
        <Box
            width={width}
            height={1}
            justifyContent="flex-start"
            overflow="hidden"
        >
            {children}
        </Box>
    );
}
