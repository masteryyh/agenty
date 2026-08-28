import { TextAttributes } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act } from "react";

import { useWindowSize } from "../hooks/useWindowSize";
import { BottomDialog } from "./BottomDialog";
import { FormPanel } from "./FormPanel";
import { KeyValueList } from "./List";
import { Table, type TableColumn } from "./Table";

interface ModelRow {
    model: string;
    context: string;
    state: string;
}

const MODEL_ROWS: ModelRow[] = [
    {
        model: "DeepSeek V4 Flash · deepseek-v4-flash",
        context: "1,000,000",
        state: "current",
    },
    {
        model: "DeepSeek V4 Pro · deepseek-v4-pro",
        context: "1,000,000",
        state: "",
    },
];

const MODEL_COLUMNS: Array<TableColumn<ModelRow>> = [
    { key: "model", header: "Model", value: (row) => row.model },
    { key: "context", header: "Context", value: (row) => row.context },
    { key: "state", header: "State", value: (row) => row.state },
];

interface ProviderRow {
    name: string;
    state: string;
}

const PROVIDER_ROWS: ProviderRow[] = [
    { name: "OpenAI", state: "configured" },
    { name: "OpenAI (Legacy API)", state: "unconfigured" },
    { name: "Anthropic", state: "custom" },
];

const PROVIDER_COLUMNS: Array<TableColumn<ProviderRow>> = [
    { key: "name", header: "Provider / model", value: (row) => row.name },
    { key: "state", header: "State", value: (row) => row.state },
];

function ResponsiveTable() {
    const { columns } = useWindowSize();
    return (
        <Table
            columns={MODEL_COLUMNS}
            rows={MODEL_ROWS}
            availableWidth={columns}
        />
    );
}

function ResponsiveProviderTable() {
    const { columns } = useWindowSize();
    return (
        <Table
            columns={PROVIDER_COLUMNS}
            rows={PROVIDER_ROWS}
            availableWidth={columns}
        />
    );
}

function ResponsiveForm() {
    const { columns, rows } = useWindowSize();
    return (
        <BottomDialog
            width={Math.max(columns - 2, 1)}
            height={Math.max(Math.min(rows - 2, 12), 1)}
        >
            <FormPanel
                title="Provider"
                fields={[
                    {
                        key: "code",
                        label: "Provider Code",
                        kind: "text",
                        value: "deepseek",
                        readOnly: true,
                    },
                    {
                        key: "baseUrl",
                        label: "Base URL",
                        kind: "text",
                        value: "https://api.deepseek.com/v1",
                    },
                ]}
                onAction={() => undefined}
                onClose={() => undefined}
            />
        </BottomDialog>
    );
}

describe("responsive table and form layout", () => {
    test("distributes three columns across the full width and reflows when resized", async () => {
        const setup = await testRender(<ResponsiveTable />, { width: 180, height: 6 });

        try {
            await act(async () => {
                await setup.flush();
            });

            let lines = setup.captureCharFrame().split("\n");
            const header = lines[0];
            expect(header).toContain("MODEL");
            expect(header.indexOf("CONTEXT")).toBeGreaterThanOrEqual(55);
            expect(header.indexOf("STATE")).toBeGreaterThanOrEqual(115);
            expect(lines[1].indexOf("DeepSeek")).toBe(0);
            expect(lines[1].indexOf("1,000,000")).toBeGreaterThanOrEqual(55);
            expect(lines[1].indexOf("current")).toBeGreaterThanOrEqual(115);

            const headerSpans = setup.captureSpans().lines[0]?.spans ?? [];
            for (const label of ["MODEL", "CONTEXT", "STATE"]) {
                const span = headerSpans.find(({ text }) => text.includes(label));
                expect(span).toBeDefined();
                expect((span?.attributes ?? 0) & TextAttributes.BOLD).toBeTruthy();
            }

            await act(async () => {
                setup.resize(54, 6);
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });

            lines = setup.captureCharFrame().split("\n");
            expect(lines[1].indexOf("DeepSeek")).toBe(0);
            expect(lines[1]).toContain("1,000,000");
            expect(lines[1].length).toBeLessThanOrEqual(54);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("distributes two provider columns across the full width", async () => {
        const setup = await testRender(<ResponsiveProviderTable />, {
            width: 180,
            height: 6,
        });

        try {
            await act(async () => {
                await setup.flush();
            });

            const lines = setup.captureCharFrame().split("\n");
            expect(lines[0]).toContain("PROVIDER / MODEL");
            expect(lines[0].indexOf("STATE")).toBeGreaterThanOrEqual(85);
            expect(lines[1].indexOf("OpenAI")).toBe(0);
            expect(lines[1].indexOf("configured")).toBeGreaterThanOrEqual(85);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("shows uppercase bold headers for key-value tables", async () => {
        const setup = await testRender(
            <KeyValueList
                rows={[{ key: "Session", value: "session-1" }]}
                availableWidth={60}
            />,
            { width: 60, height: 4 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });

            expect(setup.captureCharFrame().split("\n")[0]).toContain("KEY");
            expect(setup.captureCharFrame().split("\n")[0]).toContain("VALUE");
            const spans = setup.captureSpans().lines[0]?.spans ?? [];
            const headerSpans = spans.filter(({ text }) => /KEY|VALUE/.test(text));
            expect(headerSpans.length).toBeGreaterThan(0);
            for (const span of headerSpans) {
                expect(span.attributes & TextAttributes.BOLD).toBeTruthy();
            }
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("uses a stable label/value layout on wide and resized screens", async () => {
        const setup = await testRender(<ResponsiveForm />, { width: 180, height: 16 });

        try {
            await act(async () => {
                await setup.flush();
            });

            let line = setup.captureCharFrame().split("\n")
                .find((candidate) => candidate.includes("Provider Code:")) ?? "";
            expect(line.indexOf("Provider Code:")).toBeGreaterThanOrEqual(0);
            expect(setup.captureCharFrame()).toContain("Provider Code:");

            await act(async () => {
                setup.resize(72, 16);
                await setup.flush();
            });

            line = setup.captureCharFrame().split("\n")
                .find((candidate) => candidate.includes("Provider Code:")) ?? "";
            expect(line.indexOf("Provider Code:")).toBeGreaterThanOrEqual(0);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
