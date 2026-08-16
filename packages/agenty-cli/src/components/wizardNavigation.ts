export type WizardListFocus =
    | { kind: "row"; index: number }
    | { kind: "action"; index: number; rowIndex: number };

export type WizardListDirection = "up" | "down" | "left" | "right";

export function rowIndexForFocus(focus: WizardListFocus): number {
    return focus.kind === "row" ? focus.index : focus.rowIndex;
}

function clamp(value: number, minimum: number, maximum: number): number {
    return Math.min(Math.max(value, minimum), maximum);
}

export function moveWizardListFocus(
    focus: WizardListFocus,
    rowCount: number,
    direction: WizardListDirection,
    actionCount = 2,
): WizardListFocus {
    const rows = Math.max(rowCount, 0);
    const actions = Math.max(actionCount, 0);

    if (direction === "left" || direction === "right") {
        if (focus.kind !== "action" || actions === 0) {
            return focus;
        }
        const delta = direction === "left" ? -1 : 1;
        return {
            ...focus,
            index: clamp(focus.index + delta, 0, actions - 1),
        };
    }

    if (focus.kind === "action") {
        if (direction === "up" && rows > 0) {
            return { kind: "row", index: clamp(focus.rowIndex, 0, rows - 1) };
        }
        return focus;
    }

    if (rows === 0) {
        return actions > 0
            ? { kind: "action", index: 0, rowIndex: 0 }
            : focus;
    }

    if (direction === "up") {
        return { kind: "row", index: Math.max(focus.index - 1, 0) };
    }
    if (focus.index < rows - 1) {
        return { kind: "row", index: focus.index + 1 };
    }
    return actions > 0
        ? { kind: "action", index: 0, rowIndex: rows - 1 }
        : focus;
}
