import { testRender } from "@opentui/react/test-utils";
import { describe, expect, mock, test } from "bun:test";
import { act } from "react";

import type { HubModelDetails, ModelDownload, ModelDownloadPlan } from "../api/types";
import { useAppStore } from "../state/store";
import { createStoreClient } from "../state/testClient";
import { BottomDialog } from "./BottomDialog";
import { ModelDownloadDialog } from "./ModelDownloadDialog";

const model: HubModelDetails = { id: "Qwen/test", name: "Qwen test", architecture: "Dense", source: "huggingface", downloads: 1, revision: "rev", parameters: 800_000_000,
    variants: [{ id: "q4.gguf", format: "gguf", size: 500 }, { id: "q8.gguf", format: "gguf", size: 900 }] };
const plan: ModelDownloadPlan = { id: "q4", source: model.source, repository: model.id, variant: "q4.gguf", files: [], directory: "/models/qwen", totalBytes: 500, requiredBytes: 500, supported: true, enoughSpace: true,
    storage: { directory: "/models", volume: "/", totalBytes: 1000, usedBytes: 100, availableBytes: 900 } };
const download: ModelDownload = { id: "download", source: model.source, repository: model.id, variant: "q4.gguf", directory: plan.directory, status: "downloading", totalBytes: 500, downloadedBytes: 0, files: [] };

async function renderDialog(onClose: () => void, onDownload: (value: ModelDownload) => void, width = 80, height = 24) {
    const setup = await testRender(<BottomDialog width={width - 2} height={height - 2}><ModelDownloadDialog model={model} source="huggingface" onClose={onClose} onDownload={onDownload} /></BottomDialog>, { width, height });
    const flush = async () => {
        for (let i = 0; i < 3; i++) {
            await act(async () => {
                await setup.flush();
            });
        }
    };
    const press = async (...keys: string[]) => {
        for (const key of keys) {
            await act(async () => {
                await setup.mockInput.pressKeys([key], key === "ESCAPE" ? 50 : 10);
                await setup.flush();
            });
            await flush();
        }
    };
    const click = async (label: string) => {
        const lines = setup.captureCharFrame().split("\n");
        const y = lines.findIndex((line) => line.includes(label) && line.includes("Cancel"));
        expect(y).toBeGreaterThanOrEqual(0);
        await act(async () => {
            await setup.mockMouse.click(lines[y]!.indexOf(label), y);
            await setup.flush();
        });
        await flush();
    };
    await flush();
    return { ...setup, press, click, flush };
}

describe("model download dialog", () => {
    test("requires a checked variant and ignores results for a previous selection", async () => {
        const previous = useAppStore.getState();
        const first = Promise.withResolvers<ModelDownloadPlan>();
        const second = Promise.withResolvers<ModelDownloadPlan>();
        const prepare = mock(async (input: { variant: string }) => input.variant === "q4.gguf" ? first.promise : second.promise);
        const start = mock(async () => download);
        const accepted = mock(() => {});
        useAppStore.setState({ client: createStoreClient({ hubModel: async () => model, prepareModelDownload: prepare, startModelDownload: start }) });
        const setup = await renderDialog(() => {}, accepted);
        try {
            expect(prepare).not.toHaveBeenCalled();
            await setup.click("Download");
            expect(start).not.toHaveBeenCalled();
            await setup.press("RETURN", "RETURN");
            expect(prepare).toHaveBeenLastCalledWith({ source: "huggingface", repository: model.id, variant: "q4.gguf" });
            await setup.click("Download");
            expect(start).not.toHaveBeenCalled();
            await setup.press("RETURN", "ARROW_DOWN", "RETURN");
            await act(async () => {
                first.resolve(plan);
            });
            await setup.flush();
            await setup.click("Download");
            expect(start).not.toHaveBeenCalled();
            await act(async () => {
                second.resolve({ ...plan, id: "q8", variant: "q8.gguf", supported: false, reason: "Unsupported architecture" });
            });
            await setup.flush();
            expect(setup.captureCharFrame()).toContain("Unsupported model");
            await setup.click("Download");
            expect(start).not.toHaveBeenCalled();
            await setup.press("RETURN", "ARROW_UP", "RETURN");
            expect(setup.captureCharFrame()).toContain("Compatible with local inference");
            // Re-selecting the current variant must keep its valid check result.
            await setup.press("RETURN", "RETURN");
            await setup.click("Download");
            expect(start).toHaveBeenCalledWith("q4", false);
            expect(accepted).toHaveBeenCalledWith(download);
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });

    test("keeps admission errors visible and requires another check before retry", async () => {
        const previous = useAppStore.getState();
        const prepare = mock(async () => plan);
        let attempts = 0;
        const start = mock(async () => {
            if (++attempts === 1) {
                throw new Error("Another model download is in progress");
            }
            return download;
        });
        useAppStore.setState({ client: createStoreClient({ hubModel: async () => ({ ...model, variants: model.variants.slice(0, 1) }), prepareModelDownload: prepare, startModelDownload: start }) });
        const accepted = mock(() => {});
        const setup = await renderDialog(() => {}, accepted);
        try {
            await setup.click("Download");
            expect(setup.captureCharFrame()).toContain("Another model download is in progress");
            await setup.click("Download");
            expect(start).toHaveBeenCalledTimes(1);
            await setup.click("Check again");
            expect(prepare.mock.calls.length).toBeGreaterThanOrEqual(3);
            await setup.click("Download");
            expect(accepted).toHaveBeenCalledTimes(1);
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });

    for (const scenario of ["unsupported", "full"] as const) {
        test(`keeps the blocking status and Cancel visible in a short terminal: ${scenario}`, async () => {
            const previous = useAppStore.getState();
            const start = mock(async () => download);
            const close = mock(() => {});
            useAppStore.setState({ client: createStoreClient({ hubModel: async () => ({ ...model, variants: model.variants.slice(0, 1) }),
                prepareModelDownload: async () => ({ ...plan, supported: scenario !== "unsupported", enoughSpace: scenario !== "full", reason: "Unsupported architecture" }), startModelDownload: start }) });
            const setup = await renderDialog(close, () => {}, 40, 12);
            try {
                expect(setup.captureCharFrame()).toContain(scenario === "unsupported" ? "Unsupported model" : "Not enough disk space");
                expect(setup.captureCharFrame()).toContain("Cancel");
                await Bun.write("/private/tmp/agenty-ui3-compact-dialog-frame.txt", setup.captureCharFrame());
                await setup.click("Download");
                expect(start).not.toHaveBeenCalled();
                await setup.press("ESCAPE");
                expect(close).toHaveBeenCalledTimes(1);
            } finally {
                await act(async () => {
                    setup.renderer.destroy();
                    useAppStore.setState(previous, true);
                });
            }
        });
    }
});
