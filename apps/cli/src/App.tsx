/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import { useEffect, useState } from "react";
import { useRenderer, useSelectionHandler } from "@opentui/react";
import { Box, Text } from "./components/ui";
import { useApp, useChat, useInput, useWindowSize } from "./hooks";
import { useTuiRuntime } from "./tui/runtime";
import { useAppStore, type OverlayKind } from "./state/store";
import { useCommandPalette } from "./hooks/useCommandPalette";
import { LogoHeader } from "./components/LogoHeader";
import { MessageList } from "./components/MessageList";
import { InputBox } from "./components/InputBox";
import { CommandPalette } from "./components/CommandPalette";
import { SelectOverlay } from "./components/SelectOverlay";
import { ProviderOverlay } from "./components/ProviderOverlay";
import { ConfigOverlay } from "./components/ConfigOverlay";
import { McpOverlay } from "./components/McpOverlay";
import { SkillOverlay } from "./components/SkillOverlay";
import { AgentOverlay } from "./components/AgentOverlay";
import { StatusOverlay } from "./components/StatusOverlay";
import { WizardOverlay } from "./components/WizardOverlay";
import { PanelBox } from "./components/PanelBox";
import { BottomDialog, useBottomDialogSize } from "./components/BottomDialog";
import { commands, parseCommandTokens } from "./commands/registry";
import type { ModelDto, ChatSessionDto } from "./api/types";

const LOGO_HEIGHT = 5;
const INPUT_HEIGHT = 4;
const CONFIG_OVERLAY_HEIGHT = 18;
const PROVIDER_OVERLAY_HEIGHT = 18;
const MCP_OVERLAY_HEIGHT = 18;
const AGENTS_OVERLAY_HEIGHT = 18;
const STATUS_OVERLAY_HEIGHT = 14;
const MODEL_OVERLAY_HEIGHT = 18;

function panelHeight(overlay: OverlayKind): number | null {
	switch (overlay) {
		case "provider":
			return PROVIDER_OVERLAY_HEIGHT;
		case "mcp":
			return MCP_OVERLAY_HEIGHT;
		case "agents":
			return AGENTS_OVERLAY_HEIGHT;
		case "config":
			return CONFIG_OVERLAY_HEIGHT;
		case "status":
			return STATUS_OVERLAY_HEIGHT;
		case "model-select":
			return MODEL_OVERLAY_HEIGHT;
		default:
			return null;
	}
}

export function App() {
	const app = useApp();
	const renderer = useRenderer();
	const { exit } = useTuiRuntime();

	useInput((_input, key, event) => {
		if (key.ctrl && event.name === "c") {
			event.preventDefault();
			event.stopPropagation();
			exit(130);
		}
	});

	useSelectionHandler((selection) => {
		if (selection.isDragging) return;
		const text = selection.getSelectedText();
		if (text) renderer.copyToClipboardOSC52(text);
	});

	useEffect(() => {
		void app.init();
	}, [app.init]);

	if (app.phase === "loading") {
		return (
			<Box padding={1}>
				<Text color="cyan">
					{app.opts.localMode
						? "Starting local agenty server…"
						: `Connecting to agenty server at ${app.opts.serverURL}…`}
				</Text>
			</Box>
		);
	}

	if (app.phase === "wizard") {
		return <WizardOverlay />;
	}

	if (app.phase === "error") {
		return (
			<Box padding={1} flexDirection="column">
				<Text color="red" bold>
					Failed to start agenty-cli:
				</Text>
				<Text color="red">{app.initError}</Text>
				<Text dimColor>
					{app.opts.localMode
						? "The embedded agenty server failed to start. Make sure `make build` succeeded and the configured database is reachable. Press Ctrl+C to exit."
						: "Check that the agenty server is running (agenty) and that agenty-client.yaml points to the right URL. Press Ctrl+C to exit."}
				</Text>
			</Box>
		);
	}

	return <ChatView />;
}

function ChatView() {
	const app = useApp();
	const chat = useChat();
	const { exit } = useTuiRuntime();
	const { rows, columns } = useWindowSize();
	const client = useAppStore((s) => s.client);
	const thinkingLevel = useAppStore((s) => s.thinkingLevel);
	const [value, setValue] = useState("");

	const { palette, height: paletteHeight, tab } = useCommandPalette(
		value,
		client,
	);

	const streaming = chat.status === "streaming";
	const reasoningActive = streaming && !!chat.current?.reasoning && !chat.current.content;

	const panelH = panelHeight(app.overlay);
	const hasPanelOverlay = panelH !== null;
	// Bottom dialogs float over the chat and input instead of changing the main
	// flex flow. This keeps scroll position and message layout stable.
	const messageHeight = Math.max(rows - INPUT_HEIGHT, 1);

	const switchModelByRef = async (ref: string) => {
		if (!client) return;
		try {
			const m = await client.resolveModel(ref);
			await app.switchModel(m);
		} catch (e) {
			app.notify(`model not found: ${ref} (${(e as Error).message})`, true);
		}
	};

	const switchAgentByRef = async (ref: string) => {
		if (!client) return;
		try {
			const a = await client.resolveAgent(ref);
			await app.switchAgent(a);
		} catch (e) {
			app.notify(`agent not found: ${ref} (${(e as Error).message})`, true);
		}
	};

	const handleSubmit = (text: string) => {
		const trimmed = text.trim();
		if (!trimmed) return;
		setValue("");
		if (trimmed.startsWith("/")) {
			const tokens = parseCommandTokens(trimmed);
			const cmd = (tokens[0] ?? "").toLowerCase();
			const arg = tokens.slice(1).join(" ").trim();
			switch (cmd) {
				case "/exit":
				case "/quit":
					exit();
					return;
				case "/help":
					app.setOverlay("help");
					return;
				case "/model":
					if (arg) void switchModelByRef(arg);
					else app.setOverlay("model-select");
					return;
				case "/new":
					void app.newSession();
					return;
				case "/provider":
					app.setOverlay("provider");
					return;
				case "/agents":
					if (arg) void switchAgentByRef(arg);
					else app.setOverlay("agents");
					return;
				case "/config":
					app.setOverlay("config");
					return;
				case "/resume":
					app.setOverlay("session-select");
					return;
				case "/mcp":
					app.setOverlay("mcp");
					return;
				case "/think": {
					const a = arg.toLowerCase();
					if (!a) {
						if (app.thinkingEnabled) {
							const lvl = app.thinkingLevel || "on";
							app.setToast(`thinking: ${lvl}${app.thinkingLevel ? ` (${app.thinkingLevel} effort)` : ""}`);
						} else {
							app.setToast("thinking: off");
						}
					} else if (a === "off") {
						app.setThinking(false, "");
					} else if (a === "on") {
						app.setThinking(true, "");
					} else {
						app.setThinking(true, a);
					}
					return;
				}
				case "/compact":
					void app.compactSession();
					return;
				case "/skill":
					app.setOverlay("skill");
					return;
				case "/status":
					app.setOverlay("status");
					return;
				case "/cwd": {
					if (!arg) {
						app.setToast(`CWD: ${app.session?.cwd ?? process.cwd()}`);
					} else if (arg === "clear") {
						void app.setCwd(null);
					} else {
						let resolved = arg;
						if (resolved.startsWith("~")) {
							const home = process.env.HOME ?? process.env.USERPROFILE ?? "";
							if (home) resolved = home + resolved.slice(1);
						}
						if (resolved.includes("..")) {
							app.setToast("cwd: path traversal (..) is not allowed", true);
						} else {
							void app.setCwd(resolved);
						}
					}
					return;
				}
				default:
					app.notify(`unknown command: ${cmd}`, true);
			}
			return;
		}
		void chat.sendMessage(trimmed);
	};

	const handleTab = (): boolean => {
		const v = tab();
		if (v !== null) {
			setValue(v);
			return true;
		}
		return false;
	};

	// Full-screen overlays
	if (app.overlay === "skill") {
		return (
			<Box flexDirection="column" height={rows}>
				<LogoHeader runtimeVersion={app.runtimeVersion} />
				<PanelBox height={Math.max(rows - LOGO_HEIGHT, 1)}>
					<SkillOverlay />
				</PanelBox>
			</Box>
		);
	}
	if (app.overlay === "session-select") {
		return (
			<SessionSelectOverlay
				onClose={() => app.setOverlay(null)}
				onSelect={(s) => void app.resumeSession(s)}
			/>
		);
	}
	if (app.overlay === "help") {
		return <HelpOverlay onClose={() => app.setOverlay(null)} />;
	}

	return (
		<Box flexDirection="column" height={rows}>
			<MessageList
				history={chat.history}
				current={chat.current}
				height={messageHeight}
				header={<LogoHeader runtimeVersion={app.runtimeVersion} />}
				interactive={!hasPanelOverlay && paletteHeight === 0}
			/>
			<CommandPalette
				palette={palette}
				marginTop={-paletteHeight}
				onChoose={setValue}
			/>
			<InputBox
				value={value}
				onChange={setValue}
				onSubmit={handleSubmit}
				onTab={handleTab}
				streaming={streaming}
				phrase={chat.phrase}
				modelName={`${app.model?.provider?.name ?? "?"}/${app.model?.name ?? "?"}`}
				cwd={app.session?.cwd ?? process.cwd()}
				tokenConsumed={chat.tokenConsumed}
				thinkingLevel={thinkingLevel}
				reasoningActive={reasoningActive}
				abort={chat.abort}
				toast={app.toast}
				active={!hasPanelOverlay}
			/>
			{hasPanelOverlay ? (
				<BottomDialog
					width={Math.max(columns - 2, 1)}
					height={Math.max(Math.min(panelH!, Math.max(rows - 2, 1)), 1)}
				>
					<OverlayPanel kind={app.overlay!} />
				</BottomDialog>
			) : null}
		</Box>
	);
}

function OverlayPanel({
	kind,
}: {
	kind: "provider" | "config" | "mcp" | "agents" | "status" | "model-select";
}) {
	const app = useApp();
	return kind === "model-select" ? (
		<ModelSelectOverlay
			onClose={() => app.setOverlay(null)}
			onSelect={(model) => void app.switchModel(model)}
		/>
	) : kind === "provider" ? (
		<ProviderOverlay />
	) : kind === "mcp" ? (
		<McpOverlay />
	) : kind === "agents" ? (
		<AgentOverlay />
	) : kind === "status" ? (
		<StatusOverlay />
	) : (
		<ConfigOverlay />
	);
}

function ModelSelectOverlay({
	onClose,
	onSelect,
}: {
	onClose: () => void;
	onSelect: (model: ModelDto) => void;
}) {
	const client = useAppStore((s) => s.client);
	const dialogSize = useBottomDialogSize();
	return (
		<SelectOverlay<ModelDto>
			title="Switch Model"
			dialog
			visibleOptionCount={Math.max(dialogSize.height - 2, 1)}
			emptyHint="No switchable chat models found"
			onClose={onClose}
			onSelect={onSelect}
			load={async () => {
				const models = client ? await client.listModels() : [];
				return models
					.filter((m) => !m.embeddingModel)
					.map((m) => ({
						label: `${m.provider?.name ?? "?"}/${m.name}`,
						data: m,
					}));
			}}
		/>
	);
}

function SessionSelectOverlay({
	onClose,
	onSelect,
}: {
	onClose: () => void;
	onSelect: (session: ChatSessionDto) => void;
}) {
	const client = useAppStore((s) => s.client);
	return (
		<SelectOverlay<ChatSessionDto>
			title="Resume Session"
			emptyHint="No previous sessions found"
			onClose={onClose}
			onSelect={onSelect}
			load={async () => {
				const sessions = client ? await client.listSessions() : [];
				return sessions.map((s) => ({
					label: `${s.id.slice(0, 8)}… · ${s.messages?.length ?? 0} msgs · ${s.updatedAt}`,
					data: s,
				}));
			}}
		/>
	);
}

function HelpOverlay({ onClose }: { onClose: () => void }) {
	useInput((_input, key) => {
		if (key.escape) onClose();
	});
	return (
		<Box flexDirection="column" paddingX={2} paddingY={1}>
			<Box marginBottom={1}>
				<Text color="magenta" bold>
					Commands
				</Text>
				<Text dimColor> · Esc to close</Text>
			</Box>
			{commands.map((c) => (
				<Box key={c.name} gap={1}>
					<Text color="cyan" bold>
						{c.name}
					</Text>
					<Text color="gray">— {c.description}</Text>
				</Box>
			))}
		</Box>
	);
}
