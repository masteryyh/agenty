import { testRender } from "@opentui/react/test-utils";
import { describe, expect, spyOn, test } from "bun:test";
import { act, useState } from "react";

import { AgentyClient } from "../api/client";
import * as oauth from "../api/openRouterOAuth";
import type { ModelProviderDto, UpdateModelProviderDto } from "../api/types";
import { draftForProvider, modelDraftsForProvider } from "../consts/providerPresets";
import type { CoreTransport } from "../core/http2";
import { useAppStore } from "../state/store";
import { TuiRuntimeProvider } from "../tui/runtime";
import { BottomDialog } from "./BottomDialog";
import { ProviderOverlay } from "./ProviderOverlay";
import type { WizardListFocus } from "./wizardNavigation";
import { ModelStep, WizardOverlay } from "./WizardOverlay";
import { buildWizardModelRows } from "./wizardRows";

function provider(code: string, discovered: boolean, modelCount = 2): ModelProviderDto {
    return {
        code, name: code, type: "openai", baseUrl: "https://example.invalid/v1",
        apiKey: "existing-key", builtin: true, modelsUrl: "models", modelsCached: discovered,
        models: Array.from({ length: modelCount }, (_, index) => ({
            code: `${code}-model-${index}`,
            name: index === 0 ? `${code} Claude` : `${code} GPT ${index}`,
            contextWindow: 128_000, maxOutputTokens: 8_192,
            multiModal: false, light: false, isDefault: false, cached: discovered,
        })),
        createdAt: "", updatedAt: "",
    };
}

function ModelHarness({ providers }: { providers: ModelProviderDto[] }) {
    const [focus, setFocus] = useState<WizardListFocus>({ kind: "row", index: 0 });
    const drafts = providers.map(draftForProvider);
    const models = providers.flatMap((resource, index) => modelDraftsForProvider(drafts[index], resource));
    return (
        <BottomDialog width={98} height={30}>
            <ModelStep rows={buildWizardModelRows(drafts, models)} focus={focus}
                selectedModelId={models[0]?.id ?? null} error={null} onFocus={setFocus}
                onSelect={() => undefined} onAdd={() => undefined} onEdit={() => undefined}
                onComplete={() => undefined} onBack={() => undefined} />
        </BottomDialog>
    );
}

describe("wizard provider model search", () => {
    test("places independent searches below discovered provider names with spacing", async () => {
        const setup = await testRender(<ModelHarness providers={[
            provider("OpenRouter", true), provider("Other", true), provider("Fixed", false),
        ]} />, { width: 100, height: 32 });
        try {
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            const lines = setup.captureCharFrame().split("\n");
            const titleIndex = lines.findIndex((line) => line.includes("02 / Default session model"));
            expect(titleIndex).toBeGreaterThanOrEqual(0);
            expect(lines[titleIndex + 1]).toContain("Choose a model.");
            const providerIndex = lines.findIndex((line) => line.includes("OpenRouter (OpenRouter)"));
            const searchIndex = lines.findIndex((line) => line.includes("Search:"));
            expect(searchIndex).toBe(providerIndex + 2);
            expect(lines[searchIndex - 1].replace(/[│┃ ]/g, "")).toBe("");
            expect(lines[searchIndex + 1].replace(/[│┃ ]/g, "")).toBe("");
            expect(setup.captureCharFrame().match(/Search:/g)?.length).toBe(2);
            expect(setup.captureCharFrame()).toContain("Fixed Claude");
            await act(async () => {
                setup.mockInput.pressArrow("down");
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await setup.mockInput.typeText("claude");
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            let frame = setup.captureCharFrame();
            expect(frame).toContain("OpenRouter Claude");
            expect(frame).not.toContain("OpenRouter GPT");
            expect(frame).toContain("Other GPT");
            expect(frame).toContain("Fixed GPT");
            await act(async () => {
                await setup.mockInput.typeText("missing");
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            frame = setup.captureCharFrame();
            expect(frame).toContain("No models match.");
            expect(frame).toContain("Other GPT");
            expect(frame).toContain("Fixed GPT");
            await act(async () => {
                setup.mockInput.pressArrow("up");
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("❯ OpenRouter");
        } finally {
            await act(async () => {
                setup.renderer.destroy();
            });
        }
    });

    test("keeps focused models and actions reachable in a large discovered list", async () => {
        const setup = await testRender(<ModelHarness providers={[provider("OpenRouter", true, 80)]} />,
            { width: 100, height: 32 });
        try {
            await act(async () => {
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            for (let index = 0; index < 82; index++) {
                await act(async () => {
                    setup.mockInput.pressArrow("down");
                    await setup.flush();
                });
                await act(async () => {
                    await setup.flush();
                });
            }
            expect(setup.captureCharFrame()).toContain("[Complete setup]");
            await act(async () => {
                setup.mockInput.pressArrow("up");
                await setup.flush();
            });
            await act(async () => {
                await setup.flush();
            });
            expect(setup.captureCharFrame()).toContain("OpenRouter GPT 79");
        } finally {
            await act(async () => {
                setup.renderer.destroy();
            });
        }
    });
});

for (const screen of ["provider", "wizard"] as const) {
    describe(`${screen} OAuth form`, () => {
        for (const outcome of ["success", "failure"] as const) {
            test(`starts sign-in with Enter and displays ${outcome} inline`, async () => {
                const resource = { ...provider("OpenRouter", true), oauth: true };
                const oldState = useAppStore.getState();
                const updates: UpdateModelProviderDto[] = [];
                const transport: CoreTransport = {
                    request: async <T,>(method: string, _path: string, body?: unknown): Promise<T> => {
                        if (method === "PATCH") {
                            updates.push(body as UpdateModelProviderDto);
                            return resource as T;
                        }
                        return [resource] as T;
                    },
                    onFrame: () => () => undefined, onClose: () => () => undefined,
                    subscribe: () => undefined, unsubscribe: () => undefined,
                    getCursor: () => undefined, setCursor: () => undefined,
                    subscribeAndWait: async () => undefined, disconnect: () => undefined,
                };
                useAppStore.setState({ client: new AgentyClient(transport) });
                let resolveLogin: (key: string) => void = () => undefined;
                let rejectLogin: (error: Error) => void = () => undefined;
                const login = spyOn(oauth, "loginWithOpenRouter").mockImplementation(() =>
                    new Promise<string>((resolve, reject) => {
                        resolveLogin = resolve;
                        rejectLogin = reject;
                    }),
                );
                const setup = await testRender(screen === "provider" ? (
                    <BottomDialog width={98} height={25}><ProviderOverlay /></BottomDialog>
                ) : (
                    <TuiRuntimeProvider runtime={{ exit: () => undefined }}><WizardOverlay /></TuiRuntimeProvider>
                ), { width: 100, height: 32 });
                try {
                    await act(async () => {
                        await setup.flush();
                    });
                    await act(async () => {
                        await setup.flush();
                    });
                    if (screen === "wizard") {
                        await act(async () => {
                            setup.mockInput.pressEnter();
                            await setup.flush();
                        });
                        await act(async () => {
                            await setup.flush();
                        });
                    }
                    await act(async () => {
                        setup.mockInput.pressEnter();
                        await setup.flush();
                    });
                    await act(async () => {
                        await setup.flush();
                    });
                    expect(setup.captureCharFrame()).toContain("API");
                    expect(setup.captureCharFrame()).toContain("Sign in with OAuth...");
                    expect(setup.captureCharFrame()).not.toContain("OAuth sign-in:");
                    await act(async () => {
                        setup.mockInput.pressArrow("down");
                        await setup.flush();
                    });
                    await act(async () => {
                        await setup.flush();
                    });
                    await act(async () => {
                        setup.mockInput.pressEnter();
                        await setup.flush();
                    });
                    await act(async () => {
                        await setup.flush();
                    });
                    expect(login).toHaveBeenCalledTimes(1);
                    await act(async () => {
                        await setup.waitForFrame((frame) => frame.includes("Waiting for OpenRouter"));
                    });
                    expect(setup.captureCharFrame()).toContain("Waiting for OpenRouter");
                    await act(async () => {
                        setup.mockInput.pressEnter();
                        await setup.flush();
                    });
                    await act(async () => {
                        await setup.flush();
                    });
                    expect(login).toHaveBeenCalledTimes(1);
                    await act(async () => {
                        if (outcome === "success") {
                            resolveLogin("oauth-fixture-key");
                        } else {
                            rejectLogin(new Error("Authorization rejected: fixture error"));
                        }
                        await setup.flush();
                    });
                    await act(async () => {
                        await setup.flush();
                    });
                    const frame = setup.captureCharFrame();
                    if (outcome === "success") {
                        expect(frame).toContain("Signed in with OAuth.");
                        expect(frame).not.toContain("API Key:");
                        expect(frame).not.toContain("API key:");
                        expect(frame).not.toContain("Sign in with OAuth...");
                        expect(frame).not.toContain("Sign in again");
                        expect(frame).not.toContain("oauth-fixture-key");
                        await act(async () => {
                            setup.mockInput.pressEnter();
                            await setup.flush();
                        });
                        await act(async () => {
                            await setup.flush();
                        });
                        expect(updates[0]).toMatchObject({ apiKey: "oauth-fixture-key", authMethod: "oauth" });
                    } else {
                        const lines = frame.split("\n");
                        expect(lines.findIndex((line) => line.includes("Authorization rejected")))
                            .toBeGreaterThan(lines.findIndex((line) => line.includes("Sign in with OAuth...")));
                        expect(frame).toContain("API");
                        expect(frame).not.toContain("Signed in with OAuth.");
                    }
                } finally {
                    await act(async () => {
                        setup.renderer.destroy();
                    });
                    login.mockRestore();
                    useAppStore.setState(oldState);
                }
            });
        }
    });
}
