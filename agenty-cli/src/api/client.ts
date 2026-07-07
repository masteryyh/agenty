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

import { StreamEventType } from "./types";
import type {
	AgentDto,
	ChatDto,
	ChatSessionDto,
	CreateModelProviderDto,
	ModelDto,
	ModelProviderDto,
	StreamEvent,
	UpdateModelProviderDto,
	VersionDto,
	GenericResponse,
	PagedResponse,
} from "./types";

export interface PreparedSession {
	agent: AgentDto;
	model: ModelDto;
	session: ChatSessionDto;
}

export class AgentyClient {
	private readonly baseURL: string;
	private readonly authHeader?: string;

	constructor(baseURL: string, username?: string, password?: string) {
		this.baseURL = baseURL.replace(/\/+$/, "");
		if (username && password) {
			this.authHeader = `Basic ${btoa(`${username}:${password}`)}`;
		}
	}

	private headers(extra?: Record<string, string>): Record<string, string> {
		const h: Record<string, string> = { ...extra };
		if (this.authHeader) h["Authorization"] = this.authHeader;
		return h;
	}

	private async request<T>(
		method: string,
		path: string,
		body?: unknown,
	): Promise<T> {
		const init: RequestInit = {
			method,
			headers: this.headers(
				body !== undefined ? { "Content-Type": "application/json" } : {},
			),
		};
		if (body !== undefined) init.body = JSON.stringify(body);

		const resp = await fetch(this.baseURL + path, init);
		const text = await resp.text();
		let parsed: GenericResponse<T>;
		try {
			parsed = JSON.parse(text) as GenericResponse<T>;
		} catch {
			throw new Error(
				`HTTP ${resp.status}: failed to parse response from ${path}: ${text.slice(0, 200)}`,
			);
		}
		if (parsed.code !== 200) {
			throw new Error(`API error (${parsed.code}): ${parsed.message}`);
		}
		if (parsed.data === undefined || parsed.data === null) {
			throw new Error(`API error: empty data from ${path}`);
		}
		return parsed.data;
	}

	async checkVersion(): Promise<VersionDto> {
		return this.request<VersionDto>("GET", "/api/v1/system/version");
	}

	async listAgents(): Promise<AgentDto[]> {
		const page = await this.request<PagedResponse<AgentDto>>(
			"GET",
			"/api/v1/agents?page=1&pageSize=100",
		);
		return page.data ?? [];
	}

	async resolveAgent(ref?: string): Promise<AgentDto> {
		const agents = await this.listAgents();
		if (agents.length === 0) {
			throw new Error("no agents available; run `agenty init` on the server first");
		}
		if (ref) {
			const lower = ref.toLowerCase();
			const matched = agents.find((a) => a.id === ref) ??
				agents.find((a) => a.name.toLowerCase() === lower);
			if (!matched) throw new Error(`agent not found: ${ref}`);
			return matched;
		}
		return agents.find((a) => a.isDefault) ?? agents[0];
	}

	async getDefaultModel(): Promise<ModelDto> {
		return this.request<ModelDto>("GET", "/api/v1/models/default");
	}

	async listModels(): Promise<ModelDto[]> {
		const page = await this.request<PagedResponse<ModelDto>>(
			"GET",
			"/api/v1/models?page=1&pageSize=200",
		);
		return page.data ?? [];
	}

	async resolveModel(ref?: string): Promise<ModelDto> {
		if (!ref) return this.getDefaultModel();

		const models = await this.listModels();
		const lower = ref.toLowerCase();
		const matched =
			models.find((m) => m.id === ref) ??
			models.find((m) => m.code.toLowerCase() === lower) ??
			models.find((m) => m.name.toLowerCase() === lower) ??
			models.find((m) => {
				const provider = m.provider?.name?.toLowerCase();
				return provider ? `${provider}/${m.name}`.toLowerCase() === lower : false;
			});
		if (!matched) throw new Error(`model not found: ${ref}`);
		if (matched.embeddingModel) {
			throw new Error(`model ${ref} is not a chat model`);
		}
		return matched;
	}

	async createSession(agentId: string): Promise<ChatSessionDto> {
		return this.request<ChatSessionDto>("POST", "/api/v1/chats/session", {
			agentId,
		});
	}

	async listProviders(page = 1, pageSize = 100): Promise<ModelProviderDto[]> {
		const result = await this.request<PagedResponse<ModelProviderDto>>(
			"GET",
			`/api/v1/providers?page=${page}&pageSize=${pageSize}`,
		);
		return result.data ?? [];
	}

	async createProvider(
		dto: CreateModelProviderDto,
	): Promise<ModelProviderDto> {
		return this.request<ModelProviderDto>("POST", "/api/v1/providers", dto);
	}

	async updateProvider(
		id: string,
		dto: UpdateModelProviderDto,
	): Promise<ModelProviderDto> {
		return this.request<ModelProviderDto>("PUT", `/api/v1/providers/${id}`, dto);
	}

	async deleteProvider(id: string): Promise<void> {
		const init: RequestInit = { method: "DELETE", headers: this.headers() };
		const resp = await fetch(this.baseURL + `/api/v1/providers/${id}`, init);
		const text = await resp.text();
		let parsed: GenericResponse<unknown>;
		try {
			parsed = JSON.parse(text) as GenericResponse<unknown>;
		} catch {
			throw new Error(
				`HTTP ${resp.status}: failed to parse response from /api/v1/providers/${id}`,
			);
		}
		if (parsed.code !== 200) {
			throw new Error(`API error (${parsed.code}): ${parsed.message}`);
		}
	}

	async getLastSession(): Promise<ChatSessionDto | null> {
		try {
			return await this.request<ChatSessionDto>(
				"GET",
				"/api/v1/chats/session/last",
			);
		} catch {
			return null;
		}
	}

	async listSessions(page = 1, pageSize = 50): Promise<ChatSessionDto[]> {
		const result = await this.request<PagedResponse<ChatSessionDto>>(
			"GET",
			`/api/v1/chats/sessions?page=${page}&pageSize=${pageSize}`,
		);
		return result.data ?? [];
	}

	async getSession(sessionId: string): Promise<ChatSessionDto> {
		return this.request<ChatSessionDto>(
			"GET",
			`/api/v1/chats/session/${sessionId}`,
		);
	}

	async compactSessionForModel(
		sessionId: string,
		modelId: string,
	): Promise<boolean> {
		const result = await this.request<{ compacted: boolean }>(
			"POST",
			`/api/v1/chats/session/${sessionId}/compact`,
			{ modelId },
		);
		return result.compacted;
	}

	async prepareSession(opts: {
		agentRef?: string;
		modelRef?: string;
		newSession: boolean;
	}): Promise<PreparedSession> {
		const agent = await this.resolveAgent(opts.agentRef);
		const model = await this.resolveModel(opts.modelRef);

		let session: ChatSessionDto | null = null;
		if (!opts.newSession) {
			session = await this.getLastSession();
			if (session && session.agentId !== agent.id) session = null;
		}
		if (!session) session = await this.createSession(agent.id);

		return { agent, model, session };
	}

	async streamChat(
		sessionId: string,
		dto: ChatDto,
		onEvent: (event: StreamEvent) => void,
		signal?: AbortSignal,
	): Promise<void> {
		const resp = await fetch(
			this.baseURL + `/api/v1/chats/stream?sessionId=${sessionId}`,
			{
				method: "POST",
				headers: this.headers({
					"Content-Type": "application/json",
					Accept: "text/event-stream",
				}),
				body: JSON.stringify(dto),
				signal,
			},
		);

		if (!resp.ok || resp.body === null) {
			const text = await resp.text().catch(() => "");
			throw new Error(
				`stream chat failed (status ${resp.status}): ${text.slice(0, 200)}`,
			);
		}

		const reader = resp.body.getReader();
		const decoder = new TextDecoder();
		let buffer = "";
		let dataLines: string[] = [];

		const dispatch = () => {
			if (dataLines.length === 0) return;
			const payload = dataLines.join("\n");
			dataLines = [];
			if (payload.trim() === "") return;
			try {
				const evt = JSON.parse(payload) as StreamEvent;
				onEvent(evt);
				if (evt.type === StreamEventType.Done) {
					return true;
				}
			} catch (err) {
				onEvent({
					type: StreamEventType.Error,
					error: `failed to parse SSE payload: ${(err as Error).message}`,
				});
			}
			return false;
		};

		for (;;) {
			const { done, value } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });

			let nlIndex: number;
			while ((nlIndex = buffer.indexOf("\n")) !== -1) {
				const line = buffer.slice(0, nlIndex).replace(/\r$/, "");
				buffer = buffer.slice(nlIndex + 1);

				if (line === "") {
					const finished = dispatch();
					if (finished) return;
					continue;
				}
				if (line.startsWith(":")) continue;
				if (line.startsWith("data:")) {
					dataLines.push(line.slice(5).replace(/^ /, ""));
				}
			}
		}
		dispatch();
	}
}
