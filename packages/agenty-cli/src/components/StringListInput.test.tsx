import { BaseRenderable, InputRenderable } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act, useState } from "react";

import { StringListInput } from "./StringListInput";

function findInput(renderable: BaseRenderable): InputRenderable | null {
    if (renderable instanceof InputRenderable) {
        return renderable;
    }
    for (const child of renderable.getChildren()) {
        const match = findInput(child);
        if (match) {
            return match;
        }
    }
    return null;
}

function ControlledStringList({
    initial,
    onChange,
}: {
    initial: string[];
    onChange: (value: string[]) => void;
}) {
    const [value, setValue] = useState(initial);
    return (
        <StringListInput
            value={value}
            width={30}
            onChange={(next) => {
                setValue(next);
                onChange(next);
            }}
        />
    );
}

describe("StringListInput", () => {
    test("commits typed text as an immutable tag", async () => {
        let changed: string[] | undefined;
        const setup = await testRender(
            <ControlledStringList
                initial={["node", "--stdio"]}
                onChange={(value) => {
                    changed = value;
                }}
            />,
            { width: 36, height: 4 },
        );

        try {
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("[node]");
            expect(setup.captureCharFrame()).toContain("[--stdio]");
            expect(findInput(setup.renderer.root)?.focused).toBe(true);

            await act(async () => {
                await setup.mockInput.typeText("--flag");
                await setup.flush();
            });
            expect(changed).toBeUndefined();

            await act(async () => {
                setup.mockInput.pressEnter();
                await setup.flush();
            });
            expect(changed).toEqual(["node", "--stdio", "--flag"]);
            await setup.waitForFrame((frame) => frame.includes("[--flag]"));
            expect(findInput(setup.renderer.root)?.focused).toBe(true);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("backspace removes the selected tag as one value", async () => {
        let changed: string[] | undefined;
        const setup = await testRender(
            <ControlledStringList
                initial={["one", "two", "three"]}
                onChange={(value) => {
                    changed = value;
                }}
            />,
            { width: 36, height: 4 },
        );

        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressArrow("left");
                setup.mockInput.pressArrow("left");
                setup.mockInput.pressBackspace();
                await setup.flush();
            });
            expect(changed).toEqual(["one", "three"]);
            await setup.waitForFrame((frame) => !frame.includes("[two]"));
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("backspace with an empty draft removes the last tag", async () => {
        let changed: string[] | undefined;
        const setup = await testRender(
            <ControlledStringList
                initial={["node"]}
                onChange={(value) => {
                    changed = value;
                }}
            />,
            { width: 36, height: 4 },
        );

        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressBackspace();
                await setup.flush();
            });
            expect(changed).toEqual([]);
            await setup.waitForFrame((frame) => !frame.includes("[node]"));
        } finally {
            act(() => setup.renderer.destroy());
        }
    });

    test("keeps spaces inside each committed tag", async () => {
        let changed: string[] | undefined;
        const setup = await testRender(
            <ControlledStringList
                initial={[]}
                onChange={(value) => {
                    changed = value;
                }}
            />,
            { width: 60, height: 4 },
        );

        try {
            await act(async () => {
                await setup.flush();
                await setup.mockInput.typeText("--label=hello world");
                setup.mockInput.pressEnter();
                await setup.flush();
            });

            expect(changed).toEqual(["--label=hello world"]);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
