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
import { Box, Text, useApp as useInkApp, useInput, useWindowSize } from "ink";
import { useApp, useChat } from "./hooks";
import { useAppStore } from "./state/store";
import { useCommandPalette } from "./hooks/useCommandPalette";
import { LogoHeader } from "./components/LogoHeader";
import { MessageList } from "./components/MessageList";
import { InputBox } from "./components/InputBox";
import { CommandPalette } from "./components/CommandPalette";
import { SelectOverlay } from "./components/SelectOverlay";
import { ProviderOverlay } from "./components/ProviderOverlay";
import { commands, parseCommandTokens } from "./commands/registry";
import type { ModelDto, ChatSessionDto } from "./api/types";

const LOGO_HEIGHT = 5;
const INPUT_HEIGHT = 4;

export function App() {
	const app = useApp();

	useEffect(() => {
		void app.init();
	}, [app.init]);

	if (app.phase === "loading") {
		return (
			<Box padding={1}>
				<Text color="cyan">
					Connecting to agenty server at {app.opts.serverURL}…
				</Text>
			</Box>
		);
	}

	if (app.phase === "error") {
		return (
			<Box padding={1} flexDirection="column">
				<Text color="red" bold>
					Failed to start agenty-cli:
				</Text>
				<Text color="red">{app.initError}</Text>
				<Text dimColor>
					Check that the agenty server is running (agenty --server) and that
					agenty-client.yaml points to the right URL. Press Ctrl+C to exit.
				</Text>
			</Box>
		);
	}

	return <ChatView />;
}

function ChatView() {
	const app = useApp();
	const chat = useChat();
	const { exit } = useInkApp();
	const { rows } = useWindowSize();
	const client = useAppStore((s) => s.client);
	const [value, setValue] = useState("");

	const { palette, height: paletteHeight, tab } = useCommandPalette(
		value,
		client,
	);

	const streaming = chat.status === "streaming";
	const messageHeight = Math.max(rows - LOGO_HEIGHT - INPUT_HEIGHT, 1);

	const switchModelByRef = async (ref: string) => {
		if (!client) return;
		try {
			const m = await client.resolveModel(ref);
			await app.switchModel(m);
		} catch (e) {
			app.notify(`model not found: ${ref} (${(e as Error).message})`, true);
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
				case "/provider":
					app.setOverlay("provider");
					return;
				case "/resume":
					app.setOverlay("session-select");
					return;
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

	if (app.overlay === "model-select") {
		return (
			<ModelSelectOverlay
				onClose={() => app.setOverlay(null)}
				onSelect={(m) => void app.switchModel(m)}
			/>
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
	if (app.overlay === "provider") {
		return <ProviderOverlay />;
	}
	if (app.overlay === "help") {
		return <HelpOverlay onClose={() => app.setOverlay(null)} />;
	}

	return (
		<Box flexDirection="column" height={rows}>
			<LogoHeader runtimeVersion={app.runtimeVersion} />
			<MessageList
				history={chat.history}
				current={chat.current}
				height={messageHeight}
			/>
			<CommandPalette palette={palette} marginTop={-paletteHeight} query={value} />
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
				abort={chat.abort}
			/>
		</Box>
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
	return (
		<SelectOverlay<ModelDto>
			title="Switch Model"
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
