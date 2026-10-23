import { testRender } from "@opentui/react/test-utils";
import { describe, expect, mock, test } from "bun:test";
import { act } from "react";

import type {
    HubModelDetails,
    HubModelPage,
    ModelDownload,
    ModelDownloadPlan,
    ModelStorage
} from "../api/types";
import { useAppStore } from "../state/store";
import { createStoreClient } from "../state/testClient";
import { BottomDialog } from "./BottomDialog";
import {
    ModelDownloadPanel,
    modelDownloadProgress,
    modelFileSizeLabel,
    modelParameterLabel
} from "./ModelDownloadPanel";
import { ModelOverlay } from "./ModelOverlay";

const storage: ModelStorage = { directory: "/data/models", volume: "/data", totalBytes: 100 * 1024 ** 3, usedBytes: 60 * 1024 ** 3, availableBytes: 40 * 1024 ** 3 };
const details: HubModelDetails = { source: "huggingface", id: "Qwen/test", name: "Qwen test", architecture: "Dense", parameters: 800_000_000, sizeBytes: 1024 ** 3, downloads: 1, revision: "revision", variants: [{ id: "q4.gguf", format: "gguf", size: 500 * 1024 ** 2 }] };
const download: ModelDownload = { id: "download", source: "huggingface", repository: details.id, variant: "q4.gguf", directory: "/data/models/snapshot", status: "downloading", totalBytes: 500 * 1024 ** 2, downloadedBytes: 0, files: [{ path: "q4.gguf", totalBytes: 500 * 1024 ** 2, downloadedBytes: 0, status: "downloading" }] };

describe("model download interactions", () => {
    for (const scenario of ["supported", "unsupported", "full", "space-changed"] as const) {
        test(`checks compatibility and partition capacity before download: ${scenario}`, async () => {
            const previous = useAppStore.getState();
            const start = mock(async () => {
                if (scenario === "space-changed") {
                    throw new Error("Insufficient disk space");
                }
                // The stream may deliver completion before POST returns.
                useAppStore.setState({ modelDownloads: [{ ...download, status: "ready", downloadedBytes: download.totalBytes, modelCode: "qwen" }] });
                return download;
            });
            let previews = 0;
            const plan: ModelDownloadPlan = { id: "preview", source: details.source, repository: details.id, variant: "q4.gguf", directory: download.directory, files: [], totalBytes: download.totalBytes, requiredBytes: download.totalBytes, storage, supported: scenario !== "unsupported", reason: "This architecture is unsupported", enoughSpace: scenario !== "full" };
            useAppStore.setState({ client: createStoreClient({
                modelStorage: async () => storage,
                modelDownloads: async () => [],
                searchHubModels: async () => ({ models: [details] }),
                hubModel: async () => details,
                prepareModelDownload: async () => {
                    previews++;
                    return previews > 1 && scenario === "space-changed" ? { ...plan, enoughSpace: false, storage: { ...storage, availableBytes: 0 } } : plan;
                },
                startModelDownload: start,
            }), modelDownloads: [] });
            const setup = await testRender(<BottomDialog width={90} height={28}><ModelDownloadPanel onClose={() => {}} /></BottomDialog>, { width: 92, height: 30 });
            const flush = async () => {
                await act(async () => {
                    await setup.flush();
                });
            };
            const press = async (key: string) => {
                await act(async () => {
                    await setup.mockInput.pressKeys([key], 10);
                    await setup.flush();
                });
                await flush();
            };
            const click = async (label: string) => {
                const lines = setup.captureCharFrame().split("\n");
                const y = lines.findIndex((line) => line.includes(label) && (label !== "Download" || line.includes("Cancel")));
                expect(y).toBeGreaterThanOrEqual(0);
                const x = lines[y]!.indexOf(label) + Math.min(label.length - 1, 2);
                await act(async () => {
                    await setup.mockMouse.click(x, y);
                    await setup.flush();
                });
                await flush();
            };
            try {
                await act(async () => {
                    await new Promise((resolve) => setTimeout(resolve, 350));
                    await setup.flush();
                });
                await flush();
                expect(setup.captureCharFrame()).toContain("40.0 GiB available");
                expect(setup.captureCharFrame()).toContain("800M");
                expect(setup.captureCharFrame()).toContain("1.0 GiB");
                for (const label of ["Dense", "More", "Refresh", "Back", "Tokenizer repository"]) {
                    expect(setup.captureCharFrame()).not.toContain(label);
                }
                await press("ARROW_DOWN");
                await press("RETURN");
                expect(setup.captureCharFrame()).not.toContain("Review download");
                expect(setup.captureCharFrame()).toContain("Download model");
                expect(setup.captureCharFrame()).not.toContain("Download and add");
                expect(setup.captureCharFrame()).not.toContain("Download only");
                if (scenario === "full" || scenario === "unsupported") {
                    expect(setup.captureCharFrame()).toContain(scenario === "full" ? "Not enough disk space" : "Unsupported model");
                    await click("Download");
                    expect(start).not.toHaveBeenCalled();
                } else {
                    expect(setup.captureCharFrame()).toContain("Compatible with local inference");
                    expect(setup.captureCharFrame()).toContain("Download 500.0 MiB");
                    await Bun.write("/private/tmp/agenty-ui3-download-frame.txt", setup.captureCharFrame());
                    await click("Download");
                    expect(start).toHaveBeenCalledWith("preview", false);
                    if (scenario === "space-changed") {
                        expect(setup.captureCharFrame()).toContain("Not enough disk space");
                    } else {
                        expect(setup.captureCharFrame()).toContain("Model downloads");
                        expect(useAppStore.getState().modelDownloads[0]?.status).toBe("ready");
                    }
                }
            } finally {
                await act(async () => {
                    setup.renderer.destroy();
                    useAppStore.setState(previous, true);
                });
            }
        });
    }

    test("keeps progress live and cancels only the selected download", async () => {
        const previous = useAppStore.getState();
        const cancel = mock(async () => {});
        useAppStore.setState({ client: createStoreClient({ modelStorage: async () => storage, modelDownloads: async () => [], searchHubModels: async () => ({ models: [] }), cancelModelDownload: cancel }), modelDownloads: [download] });
        const setup = await testRender(<BottomDialog width={90} height={28}><ModelDownloadPanel onClose={() => {}} /></BottomDialog>, { width: 92, height: 30 });
        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressKey("p", { ctrl: true });
                await new Promise((resolve) => setTimeout(resolve, 20));
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("0%");
            await act(async () => {
                useAppStore.setState({ modelDownloads: [{ ...download, downloadedBytes: download.totalBytes / 2 }] });
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("50%");
            await act(async () => {
                await setup.mockInput.pressKeys(["RETURN"], 10);
                await setup.flush();
            });
            expect(cancel).toHaveBeenCalledWith(download.id);
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });

    test("offers download from /model with no configured providers", async () => {
        const previous = useAppStore.getState();
        useAppStore.setState({ client: createStoreClient({ listProviders: async () => [], modelStorage: async () => storage, modelDownloads: async () => [], searchHubModels: async () => ({ models: [] }) }), modelDownloads: [] });
        const setup = await testRender(<BottomDialog width={78} height={22}><ModelOverlay /></BottomDialog>, { width: 80, height: 24 });
        try {
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Download (Ctrl+O)");
            expect(setup.captureCharFrame()).not.toContain("Add local");
            await act(async () => {
                setup.mockInput.pressKey("o", { ctrl: true });
                await new Promise((resolve) => setTimeout(resolve, 20));
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Download local models");
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });

    test("bounds the dot progress display", () => {
        expect(modelDownloadProgress(20, 10, 4)).toBe("⣿⣿⣿⣿ 100%");
        expect(modelDownloadProgress(10, 0, 4)).toBe("⣀⣀⣀⣀ 0%");
    });

    test("can enable a registered download from the model picker", async () => {
        const previous = useAppStore.getState();
        const enable = mock(async () => ({ enabled: true, workers: [] }));
        const close = mock(() => {});
        useAppStore.setState({ client: createStoreClient({ modelStorage: async () => storage, modelDownloads: async () => [], searchHubModels: async () => ({ models: [] }), configureLocalInference: enable }), modelDownloads: [{ ...download, status: "ready", modelCode: "qwen" }] });
        const setup = await testRender(<BottomDialog width={90} height={28}><ModelDownloadPanel onClose={close} /></BottomDialog>, { width: 92, height: 30 });
        try {
            await act(async () => {
                await setup.flush();
                setup.mockInput.pressKey("p", { ctrl: true });
                await new Promise((resolve) => setTimeout(resolve, 20));
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Enable local inference");
            await act(async () => {
                await setup.mockInput.pressKeys(["RETURN"], 10);
                await setup.flush();
            });
            expect(enable).toHaveBeenCalledWith({ enabled: true });
            expect(close).toHaveBeenCalled();
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });

    test("keeps model selection and available space visible in a short terminal", async () => {
        const previous = useAppStore.getState();
        useAppStore.setState({ client: createStoreClient({ modelStorage: async () => storage, modelDownloads: async () => [], searchHubModels: async () => ({ models: [details] }) }), modelDownloads: [] });
        const setup = await testRender(<BottomDialog width={38} height={10}><ModelDownloadPanel onClose={() => {}} /></BottomDialog>, { width: 40, height: 12 });
        try {
            await act(async () => {
                await new Promise((resolve) => setTimeout(resolve, 350));
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("Qwen test");
            expect(setup.captureCharFrame()).toContain("40.0 GiB available");
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });

    test("shows UNKNOWN for missing metadata", () => {
        for (const value of [undefined, 0, -1, NaN, Infinity]) {
            expect(modelParameterLabel({ parameters: value })).toBe("UNKNOWN");
            expect(modelFileSizeLabel(value)).toBe("UNKNOWN");
        }
    });
});

async function renderBrowser() {
    const setup = await testRender(<BottomDialog width={90} height={28}><ModelDownloadPanel onClose={browserClose} /></BottomDialog>, { width: 92, height: 30 });
    const flush = async (wait = 0) => {
        await act(async () => {
            if (wait) {
                await new Promise((resolve) => setTimeout(resolve, wait));
            }
            await setup.flush();
        });
        await act(async () => {
            await setup.flush();
        });
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
    await flush(350);
    return { ...setup, flush, press };
}
const browserClose = mock(() => {});

describe("model browser navigation", () => {
    test("shows pagination loading below the list with a gap before shortcuts", async () => {
        const previous = useAppStore.getState();
        const pending = Promise.withResolvers<HubModelPage>();
        const last = { ...details, id: "Qwen/last", name: "Last model" };
        useAppStore.setState({ client: createStoreClient({ modelStorage: async () => storage, modelDownloads: async () => [],
            searchHubModels: async (_source, _query, cursor) => cursor ? pending.promise : { models: [details, last], next: "page2" },
        }), modelDownloads: [] });
        const setup = await renderBrowser();
        try {
            await setup.press("ARROW_DOWN", "ARROW_DOWN");
            const lines = setup.captureCharFrame().split("\n");
            const loading = lines.findIndex((line) => line.includes("Loading models…"));
            const row = lines.findIndex((line) => line.includes("Last model"));
            const shortcuts = lines.findIndex((line) => line.includes("Tab source"));
            expect(loading).toBeGreaterThan(row);
            expect(shortcuts).toBeGreaterThan(loading + 1);
            expect(lines[shortcuts - 1]!.replace(/[│\s]/gu, "")).toBe("");
            await Bun.write("/private/tmp/agenty-ui3-browser-loading-frame.txt", setup.captureCharFrame());
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });
    test("loads at the last row once, appends unique models and preserves selection", async () => {
        const previous = useAppStore.getState();
        let resolvePage!: (page: HubModelPage) => void;
        const pending = new Promise<HubModelPage>((resolve) => {
            resolvePage = resolve;
        });
        const first = { ...details, id: "Qwen/first", name: "First", parameters: undefined, sizeBytes: undefined };
        const second = { ...details, id: "Qwen/second", name: "Second" };
        const third = { ...details, id: "Qwen/third", name: "Third" };
        const search = mock(async (_source: string, _query: string, cursor = "") => cursor ? pending : { models: [first, second], next: "page2" });
        const inspect = mock(async (source: string, id: string) => ({ ...details, id }));
        useAppStore.setState({ client: createStoreClient({ modelStorage: async () => storage, modelDownloads: async () => [], searchHubModels: search, hubModel: inspect,
            prepareModelDownload: async () => ({ id: "plan", source: details.source, repository: second.id, variant: "q4.gguf", directory: download.directory, files: [], totalBytes: download.totalBytes, requiredBytes: download.totalBytes, storage, supported: true, enoughSpace: true }),
        }), modelDownloads: [] });
        const setup = await renderBrowser();
        try {
            expect(setup.captureCharFrame()).toContain("UNKNOWN");
            expect(search).toHaveBeenCalledTimes(1);
            await setup.press("ARROW_DOWN", "ARROW_DOWN", "ARROW_DOWN", "ARROW_DOWN");
            expect(search).toHaveBeenCalledTimes(2);
            expect(search).toHaveBeenLastCalledWith("huggingface", "", "page2");
            await act(async () => {
                resolvePage({ models: [second, third] });
            });
            await setup.flush();
            const frame = setup.captureCharFrame();
            expect(frame).toContain("Third");
            expect(frame.match(/Second/g)).toHaveLength(1);
            await setup.press("RETURN");
            expect(inspect).toHaveBeenCalledWith("huggingface", second.id);
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });

    test("discards a pending page when switching sources and resets pagination", async () => {
        const previous = useAppStore.getState();
        let resolvePage!: (page: HubModelPage) => void;
        const pending = new Promise<HubModelPage>((resolve) => {
            resolvePage = resolve;
        });
        const search = mock(async (source: string, _query: string, cursor = "") => {
            if (source === "modelscope") {
                return { models: [{ ...details, name: "New source" }], next: "2" };
            }
            return cursor ? pending : { models: [{ ...details, name: "Old source" }], next: "old-page" };
        });
        useAppStore.setState({ client: createStoreClient({ modelStorage: async () => storage, modelDownloads: async () => [], searchHubModels: search }), modelDownloads: [] });
        const setup = await renderBrowser();
        try {
            await setup.press("ARROW_DOWN");
            expect(search).toHaveBeenLastCalledWith("huggingface", "", "old-page");
            await setup.press("TAB");
            await setup.flush(350);
            await act(async () => {
                resolvePage({ models: [{ ...details, id: "Qwen/stale", name: "Stale model" }], next: "stale-page" });
            });
            await setup.flush();
            expect(setup.captureCharFrame()).toContain("New source");
            expect(setup.captureCharFrame()).not.toContain("Stale model");
            await setup.press("ARROW_DOWN");
            expect(search).toHaveBeenLastCalledWith("modelscope", "", "2");
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });

    test("retries failed pagination by navigating to the last row again", async () => {
        const previous = useAppStore.getState();
        let attempts = 0;
        const search = mock(async (_source: string, _query: string, cursor = "") => {
            if (!cursor) {
                return { models: [details], next: "page2" };
            }
            if (++attempts === 1) {
                throw new Error("Page unavailable");
            }
            return { models: [{ ...details, id: "Qwen/next", name: "Next model" }] };
        });
        useAppStore.setState({ client: createStoreClient({ modelStorage: async () => storage, modelDownloads: async () => [], searchHubModels: search }), modelDownloads: [] });
        const setup = await renderBrowser();
        try {
            await setup.press("ARROW_DOWN");
            expect(setup.captureCharFrame()).toContain("Page unavailable");
            await setup.press("ARROW_DOWN");
            expect(setup.captureCharFrame()).toContain("Next model");
            expect(attempts).toBe(2);
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });

    test("selects variants in a dialog above the browser and returns directly with Esc", async () => {
        const previous = useAppStore.getState();
        browserClose.mockClear();
        const search = mock(async () => ({ models: [details] }));
        const prepare = mock(async () => ({ id: "plan", source: details.source, repository: details.id, variant: "q8.gguf", directory: download.directory, files: [], totalBytes: download.totalBytes, requiredBytes: download.totalBytes, storage, supported: true, enoughSpace: true }));
        useAppStore.setState({ client: createStoreClient({ modelStorage: async () => storage, modelDownloads: async () => [], searchHubModels: search,
            hubModel: async () => ({ ...details, variants: [...details.variants, { id: "q8.gguf", format: "gguf", size: 900 * 1024 ** 2 }] }), prepareModelDownload: prepare,
        }), modelDownloads: [] });
        const setup = await renderBrowser();
        try {
            const lines = setup.captureCharFrame().split("\n");
            const y = lines.findIndex((line) => line.includes("Qwen test"));
            await act(async () => {
                await setup.mockMouse.click(lines[y]!.indexOf("Qwen test"), y);
                await setup.flush();
            });
            await setup.flush();
            expect(setup.captureCharFrame()).toContain("Select weights / quantization");
            expect(setup.captureCharFrame()).not.toContain("Choose model weights");
            expect(setup.captureCharFrame()).not.toContain("Tokenizer repository");
            expect(prepare).not.toHaveBeenCalled();
            await setup.press("RETURN");
            expect(setup.captureCharFrame()).toContain("q8.gguf");
            await setup.press("ARROW_DOWN", "RETURN");
            expect(prepare).toHaveBeenCalledWith({ source: "huggingface", repository: details.id, variant: "q8.gguf" });
            expect(setup.captureCharFrame()).toContain("Compatible with local inference");
            await setup.press("ESCAPE");
            expect(setup.captureCharFrame()).not.toContain("Compatible with local inference");
            expect(browserClose).not.toHaveBeenCalled();
            expect(search).toHaveBeenCalledTimes(1);
            expect(setup.captureCharFrame()).toContain("Download local models");
            await setup.press("ESCAPE");
            expect(browserClose).toHaveBeenCalledTimes(1);
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });
});
