import { useEffect, useState } from "react";
import { useInput } from "../hooks/useInput";
import { useTuiRuntime } from "../tui/runtime";
import { Box, Spinner, Text, TextInput } from "./ui";
import { useAppStore } from "../state/store";
import type { ModelDto, ModelProviderDto } from "../api/types";

// Mirrors pkg/models/system.go WebSearchProvider constants.
const WS_PROVIDERS = [
	{ label: "Tavily", value: "tavily" },
	{ label: "Brave", value: "brave" },
	{ label: "Firecrawl", value: "firecrawl" },
];
const MAX_MODELS = 4;
const NOT_SET = "<not set>";

type Step =
	| "welcome"
	| "providers"
	| "providerInput"
	| "webSearch"
	| "webSearchKey"
	| "firecrawlUrl"
	| "models"
	| "embed"
	| "saving"
	| "done";

type Feedback = { msg: string; kind: "ok" | "err" | "warn" } | null;

function modelLabel(m: ModelDto): string {
	return `${m.provider?.name ?? "?"}/${m.name}`;
}

function maskKey(key: string): string {
	if (key.length <= 4) return "*".repeat(key.length);
	return `${key.slice(0, 2)}****${key.slice(-2)}`;
}

export function WizardOverlay() {
	const client = useAppStore((s) => s.client);
	const finishWizard = useAppStore((s) => s.finishWizard);
	const { exit } = useTuiRuntime();

	const [step, setStep] = useState<Step>("welcome");
	const [providers, setProviders] = useState<ModelProviderDto[]>([]);
	const [configuredIds, setConfiguredIds] = useState<Set<string>>(new Set());
	const [provCursor, setProvCursor] = useState(0);
	const [selectedProvIdx, setSelectedProvIdx] = useState(0);
	const [wsCursor, setWsCursor] = useState(0);
	const [chatModels, setChatModels] = useState<ModelDto[]>([]);
	const [embedModels, setEmbedModels] = useState<ModelDto[]>([]);
	const [selectedModels, setSelectedModels] = useState<number[]>([]);
	const [modelCursor, setModelCursor] = useState(0);
	const [embedCursor, setEmbedCursor] = useState(0);
	const [input, setInput] = useState("");
	const [feedback, setFeedback] = useState<Feedback>(null);
	const [savingLabel, setSavingLabel] = useState("");
	const [wsKey, setWsKey] = useState("");
	const [loadingProviders, setLoadingProviders] = useState(true);

	useEffect(() => {
		if (!client) return;
		let cancelled = false;
		(async () => {
			try {
				const list = await client.listProviders();
				if (cancelled) return;
				setProviders(list);
				setConfiguredIds(
					new Set(
						list.filter((p) => p.apiKeyCensored !== NOT_SET).map((p) => p.id),
					),
				);
			} catch (e) {
				if (!cancelled) {
					setFeedback({
						msg: `Failed to load providers: ${(e as Error).message}`,
						kind: "err",
					});
				}
			} finally {
				if (!cancelled) setLoadingProviders(false);
			}
		})();
		return () => {
			cancelled = true;
		};
	}, [client]);

	const provListLen = providers.length + 1; // trailing "Continue"
	const wsListLen = WS_PROVIDERS.length + 1; // trailing "Skip"

	const loadModels = async (): Promise<{ chat: ModelDto[]; embed: ModelDto[] }> => {
		if (!client) return { chat: [], embed: [] };
		const all = await client.listModels();
		const configured = configuredIds;
		const chat = all.filter(
			(m) => !m.embeddingModel && m.provider && configured.has(m.provider.id),
		);
		const embed = all.filter(
			(m) => m.embeddingModel && m.provider && configured.has(m.provider.id),
		);
		setChatModels(chat);
		setEmbedModels(embed);
		setModelCursor(0);
		setEmbedCursor(0);
		setSelectedModels([]);
		return { chat, embed };
	};

	const saveProviderKey = async (key: string) => {
		const prov = providers[selectedProvIdx];
		if (!prov || !client) return;
		setStep("saving");
		setSavingLabel("Saving API key…");
		try {
			await client.updateProvider(prov.id, { apiKey: key });
			setConfiguredIds((s) => new Set(s).add(prov.id));
			setProviders((list) =>
				list.map((p) =>
					p.id === prov.id ? { ...p, apiKeyCensored: maskKey(key) } : p,
				),
			);
			setFeedback({ msg: `✓ API key saved for ${prov.name}`, kind: "ok" });
			setStep("providers");
		} catch (e) {
			setFeedback({
				msg: `Failed to save API key: ${(e as Error).message}`,
				kind: "err",
			});
			setStep("providers");
		}
	};

	const saveWebSearch = async (
		provider: string,
		key: string,
		firecrawlUrl?: string,
	) => {
		if (!client) return;
		setStep("saving");
		setSavingLabel("Saving web search config…");
		const dto: Record<string, string> = { webSearchProvider: provider };
		if (provider === "tavily") dto.tavilyApiKey = key;
		else if (provider === "brave") dto.braveApiKey = key;
		else if (provider === "firecrawl") {
			dto.firecrawlApiKey = key;
			if (firecrawlUrl) dto.firecrawlBaseUrl = firecrawlUrl;
		}
		try {
			await client.updateConfig(dto);
			setFeedback(null);
			setStep("models");
		} catch (e) {
			setFeedback({
				msg: `Failed to save web search config: ${(e as Error).message}`,
				kind: "err",
			});
			setStep("webSearch");
		}
	};

	const saveAgentModels = async () => {
		if (!client) return;
		setStep("saving");
		setSavingLabel("Saving agent models…");
		const modelIds = selectedModels.map((i) => chatModels[i].id);
		try {
			const agents = await client.listAgents();
			const def =
				agents.find((a) => a.isDefault) ?? agents.find((a) => a.name === "default");
			if (def) {
				await client.updateAgent(def.id, { modelIds });
			} else {
				await client.createAgent({ name: "default", isDefault: true, modelIds });
			}
			setFeedback(null);
			if (embedModels.length > 0) {
				setStep("embed");
			} else {
				await finish();
			}
		} catch (e) {
			setFeedback({
				msg: `Failed to save agent models: ${(e as Error).message}`,
				kind: "err",
			});
			setStep("models");
		}
	};

	const saveEmbed = async (selectedIndex = embedCursor) => {
		if (!client) return;
		const m = embedModels[selectedIndex];
		if (!m) {
			await finish();
			return;
		}
		setStep("saving");
		setSavingLabel("Saving embedding model…");
		try {
			await client.updateConfig({ embeddingModelId: m.id });
			await finish();
		} catch (e) {
			setFeedback({
				msg: `Failed to save embedding model: ${(e as Error).message}`,
				kind: "err",
			});
			setStep("embed");
		}
	};

	const finish = async () => {
		setStep("done");
		await finishWizard();
	};

	const beginProviderInput = (index: number) => {
		setProvCursor(index);
		setSelectedProvIdx(index);
		setInput("");
		setFeedback(null);
		setStep("providerInput");
	};

	const continueFromProviders = () => {
		setProvCursor(providers.length);
		if (configuredIds.size === 0) {
			setFeedback({
				msg: "Configure at least one provider to continue",
				kind: "warn",
			});
			return;
		}
		setFeedback(null);
		setStep("saving");
		setSavingLabel("Loading models…");
		void loadModels()
			.then(({ chat }) => {
				if (chat.length === 0) {
					setFeedback({
						msg: "No models found for configured providers",
						kind: "warn",
					});
					setStep("providers");
				} else {
					setStep("webSearch");
				}
			})
			.catch((error) => {
				setFeedback({
					msg: `Failed to load models: ${(error as Error).message}`,
					kind: "err",
				});
				setStep("providers");
			});
	};

	const chooseWebSearch = (index: number) => {
		setWsCursor(index);
		setInput("");
		setFeedback(null);
		setStep("webSearchKey");
	};

	const skipWebSearch = () => {
		setWsCursor(WS_PROVIDERS.length);
		setFeedback(null);
		setStep("models");
	};

	const toggleModel = (index: number) => {
		setModelCursor(index);
		setSelectedModels((selected) => {
			const existing = selected.indexOf(index);
			if (existing >= 0) return selected.filter((value) => value !== index);
			if (selected.length >= MAX_MODELS) {
				setFeedback({
					msg: `Maximum ${MAX_MODELS} models allowed (1 primary + ${MAX_MODELS - 1} fallbacks)`,
					kind: "warn",
				});
				return selected;
			}
			setFeedback(null);
			return [...selected, index];
		});
	};

	useInput((ch, key, event) => {
		if (step === "saving" || step === "done") return;
		switch (step) {
			case "welcome":
				if (ch === "y" || key.return) {
					setStep("providers");
					setFeedback(null);
				} else if (ch === "n" || key.escape) {
					exit();
				}
				return;
			case "providers": {
				if (loadingProviders) return;
				if (key.upArrow) {
					setProvCursor((c) => Math.max(0, c - 1));
					return;
				}
				if (key.downArrow) {
					setProvCursor((c) => Math.min(provListLen - 1, c + 1));
					return;
				}
				if (key.return) {
					if (provCursor >= providers.length) {
						continueFromProviders();
						return;
					}
					beginProviderInput(provCursor);
				}
				return;
			}
			case "providerInput":
				if (key.escape) {
					event.preventDefault();
					setStep("providers");
					setFeedback(null);
				}
				return;
			case "webSearch":
				if (key.upArrow) {
					setWsCursor((c) => Math.max(0, c - 1));
					return;
				}
				if (key.downArrow) {
					setWsCursor((c) => Math.min(wsListLen - 1, c + 1));
					return;
				}
				if (key.return) {
					if (wsCursor >= WS_PROVIDERS.length) {
						skipWebSearch();
						return;
					}
					chooseWebSearch(wsCursor);
				}
				return;
			case "webSearchKey":
				if (key.escape) {
					event.preventDefault();
					setStep("webSearch");
					setFeedback(null);
				}
				return;
			case "firecrawlUrl":
				if (key.escape) {
					event.preventDefault();
					setStep("models");
					setFeedback(null);
				}
				return;
			case "models": {
				if (chatModels.length === 0) return;
				if (key.upArrow) {
					setModelCursor((c) => Math.max(0, c - 1));
					return;
				}
				if (key.downArrow) {
					setModelCursor((c) => Math.min(chatModels.length - 1, c + 1));
					return;
				}
				if (key.return) {
					if (selectedModels.length === 0) {
						setFeedback({
							msg: "Select at least one model to continue",
							kind: "warn",
						});
						return;
					}
					setFeedback(null);
					void saveAgentModels();
					return;
				}
				if (ch === " ") {
					toggleModel(modelCursor);
				}
				return;
			}
			case "embed": {
				if (embedModels.length === 0) {
					void finish();
					return;
				}
				if (key.upArrow) {
					setEmbedCursor((c) => Math.max(0, c - 1));
					return;
				}
				if (key.downArrow) {
					setEmbedCursor((c) => Math.min(embedModels.length - 1, c + 1));
					return;
				}
				if (key.return) {
					setFeedback(null);
					void saveEmbed();
					return;
				}
				if (key.escape) {
					void finish();
				}
				return;
			}
		}
	});

	const feedbackNode = feedback ? (
		<Text
			color={
				feedback.kind === "ok"
					? "green"
					: feedback.kind === "err"
						? "red"
						: "yellow"
			}
		>
			{feedback.msg}
		</Text>
	) : null;

	if (step === "saving" || step === "done") {
		return (
			<Box paddingX={2} paddingY={1} flexDirection="column">
				{step === "done" ? (
					<Text color="green" bold>
						✓ Setup complete. Starting agenty-cli…
					</Text>
				) : (
					<Spinner label={savingLabel} />
				)}
				{feedbackNode}
			</Box>
		);
	}

	if (step === "welcome") {
		return (
			<Box paddingX={2} paddingY={1} flexDirection="column">
				<Text color="magenta" bold>
					Welcome to agenty
				</Text>
				<Box marginTop={1} flexDirection="column">
					<Text>Let's set up your model providers, web search, and default models.</Text>
					<Text dimColor>
						You can skip web search and embedding if you don't need them.
					</Text>
				</Box>
				<Box marginTop={1}>
					<Text dimColor>Press </Text>
					<Text color="cyan" bold>y</Text>
					<Text dimColor> to begin, </Text>
					<Text color="cyan" bold>n</Text>
					<Text dimColor> to exit.</Text>
				</Box>
				<Box marginTop={1} gap={2}>
					<Text color="cyan" bold onMouseClick={() => setStep("providers")}>[Begin]</Text>
					<Text color="gray" onMouseClick={() => exit()}>[Exit]</Text>
				</Box>
			</Box>
		);
	}

	if (step === "providerInput") {
		const prov = providers[selectedProvIdx];
		return (
			<Box paddingX={2} paddingY={1} flexDirection="column">
				<Text color="magenta" bold>
					API Key for {prov?.name ?? "provider"}
				</Text>
				<Text dimColor>{prov ? `Type: ${prov.type}` : ""}</Text>
				<Box marginTop={1}>
					<Text color="cyan">❯ </Text>
					<TextInput
						value={input}
						onChange={setInput}
						onSubmit={(v) => {
							const k = v.trim();
							if (k) void saveProviderKey(k);
						}}
						placeholder="paste API key, then Enter"
					/>
				</Box>
				<Text dimColor>Esc to go back</Text>
				{feedbackNode}
			</Box>
		);
	}

	if (step === "webSearchKey") {
		const ws = WS_PROVIDERS[wsCursor];
		return (
			<Box paddingX={2} paddingY={1} flexDirection="column">
				<Text color="magenta" bold>
					{ws?.label ?? "Web Search"} API Key
				</Text>
				<Box marginTop={1}>
					<Text color="cyan">❯ </Text>
					<TextInput
						value={input}
						onChange={setInput}
						onSubmit={(v) => {
							const k = v.trim();
							if (!k) return;
							setWsKey(k);
							if (ws?.value === "firecrawl") {
								setInput("");
								setStep("firecrawlUrl");
							} else {
								void saveWebSearch(ws.value, k);
							}
						}}
						placeholder="paste API key, then Enter"
					/>
				</Box>
				<Text dimColor>Esc to go back</Text>
				{feedbackNode}
			</Box>
		);
	}

	if (step === "firecrawlUrl") {
		return (
			<Box paddingX={2} paddingY={1} flexDirection="column">
				<Text color="magenta" bold>
					Firecrawl Base URL
				</Text>
				<Text dimColor>Leave empty for the default and press Enter.</Text>
				<Box marginTop={1}>
					<Text color="cyan">❯ </Text>
					<TextInput
						value={input}
						onChange={setInput}
						onSubmit={(v) => {
							void saveWebSearch("firecrawl", wsKey, v.trim() || undefined);
						}}
						placeholder="https://api.firecrawl.dev"
					/>
				</Box>
				<Text dimColor>Esc to skip</Text>
				{feedbackNode}
			</Box>
		);
	}

	if (step === "providers") {
		return (
			<Box paddingX={2} paddingY={1} flexDirection="column">
				<Text color="magenta" bold>
					Configure model providers
				</Text>
				<Text dimColor>
					Select a provider to set its API key. ✓ = configured. ↑↓ move, Enter select.
				</Text>
				<Box marginTop={1} flexDirection="column">
					{loadingProviders ? (
						<Spinner label="Loading providers…" />
					) : (
						providers.map((p, i) => {
							const active = i === provCursor;
							const configured = configuredIds.has(p.id);
							return (
								<Box key={p.id} gap={1} onMouseClick={() => beginProviderInput(i)}>
									<Text color={active ? "cyan" : "gray"}>{active ? "❯" : " "}</Text>
									<Text color={configured ? "green" : "white"}>
										{configured ? "✓" : "☐"} {p.name}
									</Text>
									<Text dimColor>{p.type}</Text>
								</Box>
							);
						})
					)}
					{!loadingProviders && (
						<Box gap={1} onMouseClick={continueFromProviders}>
							<Text color={provCursor === providers.length ? "cyan" : "gray"}>
								{provCursor === providers.length ? "❯" : " "}
							</Text>
							<Text color={provCursor === providers.length ? "cyan" : "white"} bold>
								Continue →
							</Text>
						</Box>
					)}
				</Box>
				{feedbackNode}
			</Box>
		);
	}

	if (step === "webSearch") {
		return (
			<Box paddingX={2} paddingY={1} flexDirection="column">
				<Text color="magenta" bold>
					Web search provider
				</Text>
				<Text dimColor>↑↓ move, Enter select. Pick Skip to disable web search.</Text>
				<Box marginTop={1} flexDirection="column">
					{WS_PROVIDERS.map((ws, i) => {
						const active = i === wsCursor;
						return (
							<Box key={ws.value} gap={1} onMouseClick={() => chooseWebSearch(i)}>
								<Text color={active ? "cyan" : "gray"}>{active ? "❯" : " "}</Text>
								<Text color={active ? "cyan" : "white"} bold={active}>
									{ws.label}
								</Text>
							</Box>
						);
					})}
					<Box gap={1} onMouseClick={skipWebSearch}>
						<Text color={wsCursor === WS_PROVIDERS.length ? "cyan" : "gray"}>
							{wsCursor === WS_PROVIDERS.length ? "❯" : " "}
						</Text>
						<Text color={wsCursor === WS_PROVIDERS.length ? "cyan" : "white"} bold>
							Skip
						</Text>
					</Box>
				</Box>
				{feedbackNode}
			</Box>
		);
	}

	if (step === "models") {
		return (
			<Box paddingX={2} paddingY={1} flexDirection="column">
				<Text color="magenta" bold>
					Select chat models
				</Text>
				<Text dimColor>
					Space toggles (max {MAX_MODELS}, first selected is primary). Enter to confirm.
				</Text>
				<Box marginTop={1} flexDirection="column">
					{chatModels.map((m, i) => {
						const active = i === modelCursor;
						const rank = selectedModels.indexOf(i);
						return (
							<Box key={m.id} gap={1} onMouseClick={() => toggleModel(i)}>
								<Text color={active ? "cyan" : "gray"}>{active ? "❯" : " "}</Text>
								<Text color={rank >= 0 ? "green" : "white"}>
									{rank >= 0 ? `①②③④`[rank] ?? "✓" : "☐"} {modelLabel(m)}
								</Text>
							</Box>
						);
					})}
				</Box>
				<Box marginTop={1}>
					<Text color="cyan" bold onMouseClick={() => void saveAgentModels()}>[Confirm models]</Text>
				</Box>
				{feedbackNode}
			</Box>
		);
	}

	// embed
	return (
		<Box paddingX={2} paddingY={1} flexDirection="column">
			<Text color="magenta" bold>
				Select embedding model
			</Text>
			<Text dimColor>↑↓ move, Enter to confirm, Esc to skip.</Text>
			<Box marginTop={1} flexDirection="column">
				{embedModels.map((m, i) => {
					const active = i === embedCursor;
					return (
						<Box
							key={m.id}
							gap={1}
							onMouseClick={() => {
								setEmbedCursor(i);
								void saveEmbed(i);
							}}
						>
							<Text color={active ? "cyan" : "gray"}>{active ? "❯" : " "}</Text>
							<Text color={active ? "cyan" : "white"} bold={active}>
								{modelLabel(m)}
							</Text>
						</Box>
					);
				})}
			</Box>
			<Box marginTop={1}>
				<Text color="gray" onMouseClick={() => void finish()}>[Skip]</Text>
			</Box>
			{feedbackNode}
		</Box>
	);
}
