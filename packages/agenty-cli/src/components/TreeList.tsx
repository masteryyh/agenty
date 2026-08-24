import type { ReactNode } from "react";

import { List, type ListRenderState } from "./List";

export interface TreeListItem<T> {
    key: string;
    depth: number;
    value: T;
}

export interface TreeListRenderState extends ListRenderState {
    prefix: string;
}

export interface TreeListProps<T> {
    items: Array<TreeListItem<T>>;
    cursor: number;
    visibleCount: number;
    renderItem: (item: T, state: TreeListRenderState) => ReactNode;
    onCursor: (index: number) => void;
    onActivate?: (item: T, index: number) => void;
    active?: boolean;
    emptyHint?: ReactNode;
}

export function TreeList<T>({
    items,
    cursor,
    visibleCount,
    renderItem,
    onCursor,
    onActivate,
    active = true,
    emptyHint,
}: TreeListProps<T>) {
    return (
        <List
            items={items}
            cursor={cursor}
            visibleCount={visibleCount}
            getKey={(item) => item.key}
            onCursor={onCursor}
            onActivate={(item, index) => onActivate?.(item.value, index)}
            active={active}
            emptyHint={emptyHint}
            renderItem={(item, state) => renderItem(item.value, {
                ...state,
                prefix: item.depth > 0
                    ? `${"  ".repeat(Math.max(item.depth - 1, 0))}└ `
                    : "",
            })}
        />
    );
}
