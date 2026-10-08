import { type BaseRenderable, BoxRenderable } from "@opentui/core";
import { testRender } from "@opentui/react/test-utils";
import { describe, expect, test } from "bun:test";
import { act } from "react";

import { AGENTY_PARTICLE_LINES } from "../consts/brand";
import { LOGO_HEADER_HEIGHT, LogoHeader } from "./LogoHeader";

function findHeader(renderable: BaseRenderable): BoxRenderable | null {
    if (renderable instanceof BoxRenderable && renderable.id === "agenty-header") {
        return renderable;
    }
    for (const child of renderable.getChildren()) {
        const match = findHeader(child);
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
            expect(findHeader(setup.renderer.root)?.height)
                .toBe(LOGO_HEADER_HEIGHT);
            expect(setup.captureCharFrame()).toContain("agenty>");

            await act(async () => {
                setup.resize(80, 20);
            });
            await act(async () => {
                await setup.flush();
            });
            expect(findHeader(setup.renderer.root)?.height)
                .toBe(LOGO_HEADER_HEIGHT);
            for (const line of AGENTY_PARTICLE_LINES) {
                expect(setup.captureCharFrame()).toContain(line);
            }
        } finally {
            act(() => {
                setup.renderer.destroy();
            });
        }
    });

    test("blinks the cursor without moving the wordmark or version", async () => {
        const setup = await testRender(<LogoHeader />, { width: 80, height: 5 });
        try {
            await act(async () => {
                await setup.flush();
            });
            const visible = setup.captureCharFrame();
            expect(visible).toContain("_");
            await act(async () => {
                await Bun.sleep(650);
            });
            await act(async () => {
                await setup.flush();
            });
            const hidden = setup.captureCharFrame();
            expect(hidden).not.toContain("_");
            expect(hidden).toBe(visible.replace("_", " "));
            expect(findHeader(setup.renderer.root)?.height).toBe(LOGO_HEADER_HEIGHT);
        } finally {
            act(() => setup.renderer.destroy());
        }
    });
});
