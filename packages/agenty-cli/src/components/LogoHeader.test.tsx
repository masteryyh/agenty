import { type BaseRenderable, BoxRenderable } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act } from "react";

import { LOGO_HEADER_HEIGHT, LogoHeader } from "./LogoHeader";

function findBorderedBox(renderable: BaseRenderable): BoxRenderable | null {
    if (renderable instanceof BoxRenderable && renderable.border) {
        return renderable;
    }
    for (const child of renderable.getChildren()) {
        const match = findBorderedBox(child);
        if (match) {
            return match;
        }
    }
    return null;
}

describe("LogoHeader", () => {
    test("keeps a stable height in narrow and resized terminals", async () => {
        const setup = await testRender(<LogoHeader />, {
            width: 12,
            height: 20,
        });

        try {
            await act(async () => {
                await setup.flush();
            });
            expect(findBorderedBox(setup.renderer.root)?.height)
                .toBe(LOGO_HEADER_HEIGHT);

            await act(async () => {
                setup.resize(80, 20);
                await setup.flush();
            });
            expect(findBorderedBox(setup.renderer.root)?.height)
                .toBe(LOGO_HEADER_HEIGHT);
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });
});
