import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act } from "react";

import { DropdownMenu } from "./DropdownMenu";

const OPTIONS = [
    { label: "Low", value: "low" },
    { label: "Medium", value: "medium" },
    { label: "High", value: "high" },
];

describe("DropdownMenu", () => {
    test("submits a single selected option with Enter", async () => {
        let submitted: string | string[] | undefined;
        const setup = await testRender(
            <DropdownMenu
                options={OPTIONS}
                mode="single"
                value="low"
                width={24}
                onSubmit={(value) => {
                    submitted = value;
                }}
                onClose={() => undefined}
            />,
            { width: 30, height: 8 },
        );

        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressArrow("down");
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressEnter();
                await setup.flush();
            });

            expect(submitted).toBe("medium");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("keeps multiple temporary selections until Enter", async () => {
        let submitted: string | string[] | undefined;
        const setup = await testRender(
            <DropdownMenu
                options={OPTIONS}
                mode="multiple"
                value={["low"]}
                width={24}
                onSubmit={(value) => {
                    submitted = value;
                }}
                onClose={() => undefined}
            />,
            { width: 30, height: 8 },
        );

        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressArrow("down");
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressKey(" ");
                await setup.flush();
            });
            await act(async () => {
                setup.mockInput.pressEnter();
                await setup.flush();
            });

            expect(submitted).toEqual(["low", "medium"]);
            expect(setup.captureCharFrame()).toContain("✓ Low");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("submits a single option when clicked", async () => {
        let submitted: string | string[] | undefined;
        const setup = await testRender(
            <DropdownMenu
                options={OPTIONS}
                mode="single"
                value="low"
                width={24}
                onSubmit={(value) => {
                    submitted = value;
                }}
                onClose={() => undefined}
            />,
            { width: 30, height: 8 },
        );

        try {
            await act(async () => {
                await setup.flush();
                await setup.mockMouse.click(5, 2);
                await setup.flush();
            });

            expect(submitted).toBe("medium");
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
