import { describe, expect, test } from "bun:test";

import { allocateColumnWidths, createTableLayout, truncateText } from "./Table";

describe("responsive table layout", () => {
    test("uses the screen and column count for equal baseline widths", () => {
        expect(allocateColumnWidths(90, [12, 12, 12])).toEqual([30, 30, 30]);
        expect(allocateColumnWidths(90, [80, 80, 80])).toEqual([30, 30, 30]);
        expect(allocateColumnWidths(90, [80, 8, 8])).toEqual([74, 8, 8]);
    });

    test("shares released width between overflowing columns without exceeding budget", () => {
        const widths = allocateColumnWidths(60, [50, 40, 5]);

        expect(widths).toEqual([28, 27, 5]);
        expect(widths.reduce((sum, width) => sum + width, 0)).toBeLessThanOrEqual(60);
        expect(Math.max(...widths)).toBeLessThanOrEqual(30);
    });

    test("only borrows space that other columns do not need for their content", () => {
        expect(allocateColumnWidths(60, [50, 18, 5])).toEqual([37, 18, 5]);
    });

    test("never exceeds the budget on terminals narrower than the column count", () => {
        const widths = allocateColumnWidths(2, [10, 10, 10]);

        expect(widths).toEqual([1, 1, 0]);
        expect(widths.reduce((sum, width) => sum + width, 0)).toBe(2);
    });

    test("includes headers, all rows and gaps in the table layout", () => {
        const rows = [
            { name: "Short", context: "1,000" },
            { name: "A much longer model name", context: "1,000,000" },
        ];
        const columns = createTableLayout([
            { key: "name", header: "Model", value: (row: typeof rows[number]) => row.name },
            { key: "context", header: "Context", value: (row: typeof rows[number]) => row.context },
        ], rows, 50, 2);

        expect(columns.map((column) => column.width)).toEqual([24, 24]);
        expect(columns.reduce((sum, column) => sum + column.width, 0) + 2)
            .toBe(50);
    });

    test("rejects columns without a visible header", () => {
        expect(() => createTableLayout([
            { key: "missing", header: " ", value: () => "value" },
        ], [{}], 20)).toThrow("requires a header");
    });

    test("truncates wide characters by terminal cells", () => {
        expect(truncateText("模型配置", 5)).toBe("模型…");
        expect(truncateText("model-name", 6)).toBe("model…");
    });
});
