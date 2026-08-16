import { describe, expect, test } from "bun:test";

import { moveWizardListFocus, rowIndexForFocus, type WizardListFocus } from "./wizardNavigation";

describe("wizard list navigation", () => {
    test("moves from the last row to the first action", () => {
        const focus: WizardListFocus = { kind: "row", index: 2 };

        expect(moveWizardListFocus(focus, 3, "down")).toEqual({
            kind: "action",
            index: 0,
            rowIndex: 2,
        });
    });

    test("moves across actions with horizontal arrows", () => {
        const focus: WizardListFocus = { kind: "action", index: 0, rowIndex: 2 };

        expect(moveWizardListFocus(focus, 3, "right")).toEqual({
            kind: "action",
            index: 1,
            rowIndex: 2,
        });
        expect(moveWizardListFocus({ ...focus, index: 1 }, 3, "left")).toEqual(focus);
    });

    test("returns to the row used to enter the action bar", () => {
        const focus: WizardListFocus = { kind: "action", index: 1, rowIndex: 1 };

        expect(moveWizardListFocus(focus, 3, "up")).toEqual({ kind: "row", index: 1 });
        expect(rowIndexForFocus(focus)).toBe(1);
    });

    test("does not produce an invalid row for an empty list", () => {
        const focus: WizardListFocus = { kind: "row", index: 0 };

        expect(moveWizardListFocus(focus, 0, "down")).toEqual({
            kind: "action",
            index: 0,
            rowIndex: 0,
        });
    });
});
