import { textWidth } from "./Table";

export interface FormLayout {
    mode: "wide" | "compact" | "stacked";
    contentWidth: number;
    labelWidth: number;
    valueStart: number;
    valueWidth: number;
}

export const FORM_MARKER_WIDTH = 2;
const FORM_COLUMN_GAP = 2;

export function resolveFormLayout(width: number, labels: string[]): FormLayout {
    const contentWidth = Math.max(width, 1);
    const rowWidth = Math.max(contentWidth - FORM_MARKER_WIDTH, 1);
    const mode = contentWidth < 44 ? "stacked" : contentWidth < 64 ? "compact" : "wide";
    const longestLabel = Math.max(1, ...labels.map((label) => textWidth(`${label}:`)));
    const labelWidth = mode === "stacked"
        ? rowWidth
        : Math.min(longestLabel, mode === "compact" ? 18 : 24);
    const valueStart = FORM_MARKER_WIDTH + (mode === "stacked" ? 0 : labelWidth + FORM_COLUMN_GAP);

    return {
        mode,
        contentWidth,
        labelWidth,
        valueStart,
        valueWidth: Math.max(contentWidth - valueStart, 1),
    };
}

export function wrapFormLabel(value: string, width: number): string[] {
    if (width <= 0) {
        return [""];
    }
    const lines: string[] = [];
    for (const paragraph of value.split("\n")) {
        let current = "";
        for (const word of paragraph.trim().split(/\s+/)) {
            if (current && textWidth(`${current} ${word}`) <= width) {
                current += ` ${word}`;
                continue;
            }
            if (current) {
                lines.push(current);
            }
            current = "";
            for (const character of word) {
                if (current && textWidth(current + character) > width) {
                    lines.push(current);
                    current = "";
                }
                current += character;
            }
        }
        lines.push(current);
    }
    return lines;
}

export function layoutFormActions<T extends { label: string }>(actions: T[], width: number): T[][] {
    const rows: T[][] = [];
    let row: T[] = [];
    let used = 0;
    for (const action of actions) {
        const actionWidth = Math.min(textWidth(action.label) + 2, width);
        if (row.length > 0 && used + 3 + actionWidth > width) {
            rows.push(row);
            row = [];
            used = 0;
        }
        used += (row.length > 0 ? 3 : 0) + actionWidth;
        row.push(action);
    }
    if (row.length > 0) {
        rows.push(row);
    }
    return rows;
}
