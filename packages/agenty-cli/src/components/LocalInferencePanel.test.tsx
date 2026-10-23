import { testRender } from "@opentui/react/test-utils";
import { describe, expect, mock, test } from "bun:test";
import { act } from "react";

import type { LocalInferenceConfig, ModelProviderDto } from "../api/types";
import { useAppStore } from "../state/store";
import { createStoreClient } from "../state/testClient";
import { BottomDialog } from "./BottomDialog";
import { LocalInferencePanel, localModelFromForm, localWorkerStatus } from "./LocalInferencePanel";
import { isConfiguredProvider } from "./ModelOverlay";

describe("local inference setup", () => {
    test("formats worker installation and running state", () => {
        const config: LocalInferenceConfig = { enabled: false, workers: [{ id: "mlx", backend: "mlx", command: "/workers/mlx", enabled: true }] };
        for (const platform of ["linux", "win32"] as const) {
            expect(localWorkerStatus("mlx", config, null, platform, "x64")).toBe("UNAVAILABLE");
        }
        expect(localWorkerStatus("mlx", config, null, "darwin", "arm64")).toBe("INSTALLED");
        expect(localWorkerStatus("llamaCpp", config, null, "darwin", "arm64")).toBe("NOT INSTALLED");
        expect(localWorkerStatus("mlx", config, { enabled: true, workers: [{ id: "mlx", backend: "mlx", status: "ready" }] }, "darwin", "arm64")).toBe("RUNNING");
        expect(localWorkerStatus("mlx", { ...config, workers: config.workers.map((worker) => ({ ...worker, enabled: false })) }, null, "darwin", "arm64")).toBe("DISABLED");
    });
    test("keeps worker toggles, saving and reinstallation accessible from each row", async () => {
        const previous = useAppStore.getState();
        let config: LocalInferenceConfig = { enabled: true, workers: [{ id: "native", backend: "llamaCpp", command: "/worker", enabled: true }] };
        const save = mock(async (patch: Partial<LocalInferenceConfig>) => {
            config = { ...config, ...patch };
            return config;
        });
        const install = mock(async () => ({ id: "install", status: "installed", downloadedBytes: 10, totalBytes: 10 }));
        useAppStore.setState({ client: createStoreClient({ localInferenceConfig: async () => config, listProviders: async () => [], configureLocalInference: save,
            workerBundles: async () => [{ name: "worker.zip", backend: "llamaCpp", version: "test", url: "https://example.test/worker.zip", sha256: "hash" }], installWorker: install,
        }), localRuntime: { enabled: true, workers: [{ id: "native", backend: "llamaCpp", status: "ready" }] }, workerInstallations: [] });
        const setup = await testRender(<BottomDialog width={78} height={22}><LocalInferencePanel onDone={() => {}} /></BottomDialog>, { width: 80, height: 24 });
        const flush = async () => {
            for (let i = 0; i < 2; i++) {
                await act(async () => {
                    await setup.flush();
                });
            }
        };
        const click = async (label: string) => {
            const lines = setup.captureCharFrame().split("\n");
            const y = lines.findIndex((line) => line.includes(label));
            expect(y).toBeGreaterThanOrEqual(0);
            await act(async () => {
                await setup.mockMouse.click(lines[y]!.indexOf(label), y);
                await setup.flush();
            });
            await flush();
        };
        try {
            await flush();
            await click("llama.cpp worker");
            expect(setup.captureCharFrame().split("\n").find((line) => line.includes("llama.cpp worker"))).toContain("[DISABLED]");
            await act(async () => {
                setup.mockInput.pressKey("r", { ctrl: true });
                await new Promise((resolve) => setTimeout(resolve, 10));
                await setup.flush();
            });
            await flush();
            expect(install).toHaveBeenCalledWith("https://example.test/worker.zip", "hash");
            await click("Save settings");
            expect(save).toHaveBeenCalledWith(expect.objectContaining({ workers: [expect.objectContaining({ id: "native", enabled: false })] }));
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });
    for (const scenario of ["ready", "no-worker", "no-model", "probe-failed"] as const) {
        test(`wizard saves and returns only with a compatible worker and model: ${scenario}`, async () => {
            const previous = useAppStore.getState();
            const done = mock(() => {});
            let config: LocalInferenceConfig = { enabled: false, workers: scenario === "no-worker" ? [] : [{ id: "native", backend: "llamaCpp", enabled: true, command: "/workers/native" }] };
            const provider: ModelProviderDto = { code: "local", name: "Local inference", type: "local", baseUrl: "", apiKey: "", authMethod: "none", builtin: true, localEnabled: false,
                models: scenario === "no-model" ? [] : [{ code: "qwen", name: "Qwen", contextWindow: 8192, maxOutputTokens: 512, multiModal: false, light: false, isDefault: false,
                    local: { backendPreference: "auto", outputParser: "qwen3_5", resources: [{ resourceId: "gguf", backend: "llamaCpp", format: "gguf", weightsPath: "/models/qwen.gguf", codecDirectory: "/models/tokenizer" }] } }], createdAt: "", updatedAt: "" };
            const probe = mock(async () => {
                expect(config.enabled).toBe(false);
                if (scenario === "probe-failed") {
                    throw new Error("Model weight files are missing");
                }
                return {};
            });
            useAppStore.setState({ client: createStoreClient({
                localInferenceConfig: async () => config,
                listProviders: async () => [{ ...provider, localEnabled: config.enabled }],
                configureLocalInference: async (patch) => {
                    config = { ...config, ...patch };
                    return config;
                }, probeLocalModel: probe,
            }), localRuntime: null, workerInstallations: [] });
            const setup = await testRender(<BottomDialog width={78} height={22}><LocalInferencePanel wizard onDone={done} /></BottomDialog>, { width: 80, height: 24 });
            const press = async (key: string) => {
                await act(async () => {
                    await setup.mockInput.pressKeys([key], 10);
                    await setup.flush();
                });
                await act(async () => {
                    await setup.flush();
                });
            };
            try {
                await act(async () => {
                    await setup.flush();
                });
                await act(async () => {
                    await setup.flush();
                });
                expect(setup.captureCharFrame()).toContain("Save and back");
                expect(setup.captureCharFrame()).not.toContain("false");
                for (let i = 0; i < 6; i++) {
                    await press("ARROW_DOWN");
                }
                await press("RETURN");
                await act(async () => {
                    await setup.flush();
                });
                if (scenario === "ready") {
                    expect(config.enabled).toBe(true);
                    expect(done).toHaveBeenCalledWith(expect.objectContaining({ code: "local", localEnabled: true }));
                    expect(probe).toHaveBeenCalledTimes(1);
                } else {
                    expect(config.enabled).toBe(false);
                    expect(done).not.toHaveBeenCalled();
                    expect(setup.captureCharFrame()).toContain(scenario === "no-worker" ? "Install at least one" : scenario === "no-model" ? "Download at least one" : "Model weight files are missing");
                }
            } finally {
                await act(async () => {
                    setup.renderer.destroy();
                    useAppStore.setState(previous, true);
                });
            }
        });
    }
    test("edits registered models while manual registration stays hidden", async () => {
        const previous = useAppStore.getState();
        const probe = mock(async () => ({}));
        const local = localModelFromForm({ codecDirectory: "/models/tokenizer", ggufPath: "/models/qwen.gguf", parser: "qwen3_5" });
        const model = { code: "qwen", name: "Qwen test", contextWindow: 8192, maxOutputTokens: 512, multiModal: false, light: false, isDefault: false, local };
        const provider: ModelProviderDto = { code: "local", name: "Local inference", type: "local", baseUrl: "", apiKey: "", authMethod: "none", builtin: true, localEnabled: true, models: [model], createdAt: "", updatedAt: "" };
        const save = mock(async () => ({ ...model, providerCode: "local", providerName: "Local inference" }));
        useAppStore.setState({ client: createStoreClient({
            localInferenceConfig: async () => ({ enabled: true, workers: [{ id: "native", backend: "llamaCpp", enabled: true, command: "/worker" }] }),
            listProviders: async () => [provider], probeLocalModel: probe, createModel: save,
        }), localRuntime: { enabled: true, workers: [{ id: "native", backend: "llamaCpp", status: "ready" }] }, workerInstallations: [] });
        const setup = await testRender(<BottomDialog width={78} height={22}><LocalInferencePanel onDone={() => {}} /></BottomDialog>, { width: 80, height: 24 });
        const flush = async () => {
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
        };
        const click = async (label: string) => {
            const lines = setup.captureCharFrame().split("\n");
            const y = lines.findIndex((line) => line.includes(label));
            expect(y).toBeGreaterThanOrEqual(0);
            await act(async () => {
                await setup.mockMouse.click(lines[y]!.indexOf(label), y);
                await setup.flush();
            });
            await flush();
        };
        try {
            await flush();
            const frame = setup.captureCharFrame();
            expect(frame).not.toContain("Register local model");
            expect(frame).toContain("Registered models (1)");
            const mlx = frame.split("\n").find((line) => line.includes("MLX worker"))!;
            const llama = frame.split("\n").find((line) => line.includes("llama.cpp worker"))!;
            expect(llama).toContain("[RUNNING]");
            expect(mlx.lastIndexOf("]")).toBe(llama.lastIndexOf("]"));
            await Bun.write("/private/tmp/agenty-ui3-local-frame.txt", frame);
            await click("Qwen test");
            expect(setup.captureCharFrame()).toContain("Local model");
            await click("Check and save");
            expect(probe).toHaveBeenCalledWith(expect.objectContaining({ resources: local.resources }));
            expect(save).toHaveBeenCalledWith(expect.objectContaining({ modelCode: "qwen" }));
        } finally {
            await act(async () => {
                setup.renderer.destroy();
                useAppStore.setState(previous, true);
            });
        }
    });
    test("binds MLX and GGUF separately and advertises only the configured thinking option", () => {
        const local = localModelFromForm({ codecDirectory: "/models/tokenizer", mlxPath: "/models/mlx", ggufPath: "/models/qwen.gguf", parser: "qwen3_5", thinking: "true" });
        expect(local.resources.map((resource) => resource.backend)).toEqual(["mlx", "llamaCpp"]);
        expect(local.thinkingEffort).toBe("medium");
        expect(isConfiguredProvider({ apiKey: "", type: "local", localEnabled: true, models: [] })).toBe(false);
    });
    for (const [width, height] of [[80, 24], [40, 12]]) {
        test(`renders runtime state and exits with Esc at ${width}x${height}`, async () => {
            const previous = useAppStore.getState();
            const done = mock(() => {});
            const provider: ModelProviderDto = { code: "local", name: "Local inference", type: "local", baseUrl: "", apiKey: "", authMethod: "none", builtin: true, localEnabled: true, models: [], createdAt: "", updatedAt: "" };
            useAppStore.setState({ client: createStoreClient({ localInferenceConfig: async () => ({ enabled: true, workers: [] }), listProviders: async () => [provider] }), localRuntime: { enabled: true, workers: [{ id: "mlx", backend: "mlx", status: "ready" }] }, workerInstallations: [] });
            const setup = await testRender(<BottomDialog width={width - 2} height={height - 2}><LocalInferencePanel onDone={done} /></BottomDialog>, { width, height });
            try {
                await act(async () => {
                    await setup.flush();
                });
                await act(async () => {
                    await setup.flush();
                });
                expect(setup.captureCharFrame()).toContain("Local inference");
                expect(setup.captureCharFrame()).not.toContain("API Key");
                await act(async () => {
                    setup.mockInput.pressKey("ESCAPE");
                    await Bun.sleep(30);
                    await setup.flush();
                });
                expect(done).toHaveBeenCalledTimes(1);
            } finally {
                await act(async () => {
                    setup.renderer.destroy();
                    useAppStore.setState(previous, true);
                });
            }
        });
    }
});
