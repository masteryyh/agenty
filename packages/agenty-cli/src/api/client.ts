import type { CoreTransport, EventCursor, StreamFrame } from "../core/http2";
import { formatModelRef, resolveModelInput as resolveModelInputFromList } from "./modelReference";
import type {
    AvailableModelDto,
    ChatMessageDto,
    ChatSessionDto,
    CompactionEvent,
    ContentBlock,
    CoreModelDto,
    CreateModelDto,
    CreateModelProviderDto,
    ExecutionStart,
    InitializeCompleteInput,
    InitializeStatusDto,
    McpEvent,
    McpLogEntry,
    McpServerConfig,
    McpServerDto,
    ModelDto,
    ModelProviderDto,
    ModelRef,
    PagedResponse,
    PermissionMode,
    ReasoningEffort,
    RoundDto,
    SessionEvent,
    SessionSummaryDto,
    SkillDiagnosticDto,
    SkillDto,
    ToolApprovalResolution,
    ToolDialect,
    UpdateModelDto,
    UpdateModelProviderDto,
} from "./types";
import { STANDARD_REASONING_EFFORTS } from "./types";

export interface PreparedSession {
    model: ModelDto;
    session: ChatSessionDto;
}

export class AgentyClient {
    private readonly sessionListeners = new Set<(event: SessionEvent) => void>();
    private readonly compactionListeners = new Set<(event: CompactionEvent) => void>();
    private readonly mcpListeners = new Set<(event: McpEvent) => void>();
    private readonly streamListeners = new Set<(frame: StreamFrame) => void | Promise<void>>();

    constructor(private readonly http: CoreTransport) {
        this.http.onFrame(async (frame) => {
            const event = frame.event as { kind?: string } | undefined;
            if (frame.type === "event" && event?.kind === "session") {
                for (const listener of this.sessionListeners) listener(normalizeSessionEvent(event as SessionEvent));
            } else if (frame.type === "event" && event?.kind === "compaction") {
                for (const listener of this.compactionListeners) listener(event as CompactionEvent);
            } else if (frame.type === "event" && frame.topic === "mcp" && event) {
                for (const listener of this.mcpListeners) listener(event as McpEvent);
            }
            for (const listener of this.streamListeners) await listener(frame);
        });
    }

    close(): void {
        this.http.disconnect();
    }

    subscribeSession(id: string, after?: EventCursor): void {
        this.http.subscribe(`session:${id}`, after);
    }

    subscribeSessionAndWait(id: string, after?: EventCursor): Promise<void> {
        return this.http.subscribeAndWait(`session:${id}`, after);
    }

    unsubscribeSession(id: string): void {
        this.http.unsubscribe(`session:${id}`);
    }

    subscribeMcp(after?: EventCursor): void {
        this.http.subscribe("mcp", after);
    }

    onStreamFrame(listener: (frame: StreamFrame) => void | Promise<void>): () => void {
        this.streamListeners.add(listener);
        return () => this.streamListeners.delete(listener);
    }

    eventCursor(topic: string): EventCursor | undefined {
        return this.http.getCursor(topic);
    }

    onSessionEvent(listener: (event: SessionEvent) => void): () => void {
        this.sessionListeners.add(listener);
        return () => this.sessionListeners.delete(listener);
    }

    onCompactionEvent(listener: (event: CompactionEvent) => void): () => void {
        this.compactionListeners.add(listener);
        return () => this.compactionListeners.delete(listener);
    }

    onMcpEvent(listener: (event: McpEvent) => void): () => void {
        this.mcpListeners.add(listener);
        return () => this.mcpListeners.delete(listener);
    }

    onClose(listener: (reason: Error) => void): () => void {
        return this.http.onClose(listener);
    }

    async initializationStatus(): Promise<InitializeStatusDto> {
        const result = await this.http.request<InitializeStatusDto>("GET", "/v1/initialization");
        return {
            initialized: result?.initialized === true,
            defaultModel: result?.defaultModel,
            defaultReasoningEffort: result?.defaultReasoningEffort,
        };
    }

    async isInitialized(): Promise<boolean> {
        return (await this.initializationStatus()).initialized;
    }

    async completeInitialization(input: InitializeCompleteInput): Promise<{ initialized: boolean }> {
        const result = await this.http.request<{ initialized?: boolean }>("POST", "/v1/initialization", input);
        return { initialized: result?.initialized === true };
    }

    async listProviders(providerCode?: string): Promise<ModelProviderDto[]> {
        const query = providerCode ? `?providerCode=${encodeURIComponent(providerCode)}` : "";
        const providers = await this.http.request<Array<ModelProviderDto | null>>("GET", `/v1/providers${query}`);
        return (providers ?? [])
            .filter((provider): provider is ModelProviderDto => provider !== null)
            .map(normalizeProvider);
    }

    async listProvidersPage(page = 1, pageSize = 100): Promise<PagedResponse<ModelProviderDto>> {
        return paginate(await this.listProviders(), page, pageSize);
    }

    async listProviderModels(providerCode: string): Promise<AvailableModelDto[]> {
        const models = await this.http.request<Array<AvailableModelDto | null>>(
            "GET", `/v1/providers/${encodeURIComponent(providerCode)}/models`,
        );
        return (models ?? [])
            .filter((model): model is AvailableModelDto => model !== null)
            .map((model) => ({
                ...model,
                reasoningEfforts: Array.isArray(model.reasoningEfforts) ? model.reasoningEfforts : [],
            }));
    }

    async createProvider(input: CreateModelProviderDto): Promise<ModelProviderDto> {
        const provider = await this.http.request<ModelProviderDto>("POST", "/v1/providers", input);
        if (!provider) {
            throw new Error("core returned an empty provider");
        }
        return normalizeProvider(provider);
    }

    async updateProvider(code: string, input: UpdateModelProviderDto): Promise<ModelProviderDto> {
        const provider = await this.http.request<ModelProviderDto>("PATCH", `/v1/providers/${encodeURIComponent(code)}`, input);
        if (!provider) {
            throw new Error(`core returned an empty provider for ${code}`);
        }
        return normalizeProvider(provider);
    }

    async deleteProvider(code: string): Promise<void> {
        await this.http.request("DELETE", `/v1/providers/${encodeURIComponent(code)}`);
    }

    async listModels(): Promise<ModelDto[]> {
        const providers = await this.listProviders();
        return providers.flatMap((provider) => provider.models.map((model) => projectModel(provider, model)));
    }

    async listModelsPage(page = 1, pageSize = 100): Promise<PagedResponse<ModelDto>> {
        return paginate(await this.listModels(), page, pageSize);
    }

    async getDefaultModel(): Promise<ModelDto> {
        const status = await this.initializationStatus();
        if (status.defaultModel) {
            return this.getModel(status.defaultModel);
        }
        const providers = await this.listProviders();
        const models = providers
            .filter((provider) => provider.apiKey.trim() !== "")
            .flatMap((provider) => provider.models.map((model) => projectModel(provider, model)));
        const model = models.find((candidate) => candidate.isDefault) ?? models[0];
        if (!model) {
            throw new Error("no model available");
        }
        return model;
    }

    async getModel(ref: ModelRef): Promise<ModelDto> {
        const providers = await this.listProviders(ref.providerCode);
        const provider = providers.find((candidate) => candidate.code === ref.providerCode);
        const model = provider?.models.find((candidate) => candidate.code === ref.modelCode);
        if (!provider || !model) {
            throw new Error(`model not found: ${formatModelRef(ref)}`);
        }
        return projectModel(provider, model);
    }

    async resolveModelInput(reference?: string): Promise<ModelDto> {
        if (!reference) {
            return this.getDefaultModel();
        }
        return resolveModelInputFromList(await this.listModels(), reference);
    }

    async createModel(input: CreateModelDto): Promise<ModelDto> {
        const provider = await this.http.request<ModelProviderDto>(
            "POST", `/v1/providers/${encodeURIComponent(input.providerCode)}/models`, input,
        );
        return findProjectedModel(provider, input.modelCode);
    }

    async updateModel(providerCode: string, modelCode: string, input: UpdateModelDto): Promise<ModelDto> {
        const inputWithTarget = {
            providerCode,
            modelCode,
            ...input,
        };
        const provider = await this.http.request<ModelProviderDto>(
            "POST", `/v1/providers/${encodeURIComponent(providerCode)}/models`, inputWithTarget,
        );
        return findProjectedModel(provider, modelCode);
    }

    async deleteModel(providerCode: string, modelCode: string): Promise<void> {
        await this.http.request(
            "DELETE", `/v1/providers/${encodeURIComponent(providerCode)}/models/${encodeURIComponent(modelCode)}`,
        );
    }

    async createSession(
        model: ModelDto,
        effort: ReasoningEffort = "off",
        permissionMode: PermissionMode = "ask",
        cwd = process.cwd(),
        toolDialect: ToolDialect = "default",
    ): Promise<ChatSessionDto> {
        const session = await this.http.request<ChatSessionDto>("POST", "/v1/sessions", {
            providerCode: model.providerCode,
            modelCode: model.code,
            contextWindow: model.contextWindow,
            reasoningEffort: effort,
            permissionMode,
            ...(toolDialect === "codex" ? { toolDialect } : {}),
            cwd,
        });
        return requireSession(session, "session.create");
    }

    async getSession(id: string): Promise<ChatSessionDto> {
        const session = await this.http.request<ChatSessionDto>("GET", `/v1/sessions/${encodeURIComponent(id)}`);
        return requireSession(session, `session.get ${id}`);
    }

    async listSessionSummaries(): Promise<SessionSummaryDto[]> {
        const summaries = await this.http.request<Array<SessionSummaryDto | null>>("GET", "/v1/sessions");
        return (summaries ?? []).filter((summary): summary is SessionSummaryDto => summary !== null);
    }

    async listSkills(): Promise<{ skills: SkillDto[]; diagnostics: SkillDiagnosticDto[] }> {
        const result = await this.http.request<{
            skills?: SkillDto[];
            diagnostics?: SkillDiagnosticDto[];
        }>("GET", "/v1/skills");
        return {
            skills: result?.skills ?? [],
            diagnostics: result?.diagnostics ?? [],
        };
    }

    async listMcpServers(): Promise<McpServerDto[]> {
        const servers = await this.http.request<Array<McpServerDto | null>>("GET", "/v1/mcp");
        return (servers ?? []).filter((server): server is McpServerDto => server !== null);
    }

    async listMcpServerLogs(name: string): Promise<McpLogEntry[]> {
        const logs = await this.http.request<Array<McpLogEntry | null>>(
            "GET", `/v1/mcp/${encodeURIComponent(name)}/logs`,
        );
        return (logs ?? []).filter((log): log is McpLogEntry => log !== null);
    }

    async createMcpServer(name: string, config: McpServerConfig): Promise<McpServerDto> {
        const server = await this.http.request<McpServerDto>("POST", "/v1/mcp", { name, config });
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async updateMcpServer(name: string, config: McpServerConfig): Promise<McpServerDto> {
        const server = await this.http.request<McpServerDto>("PUT", `/v1/mcp/${encodeURIComponent(name)}`, { config });
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async setMcpEnabled(name: string, enabled: boolean): Promise<McpServerDto> {
        const server = await this.http.request<McpServerDto>(
            "PUT", `/v1/mcp/${encodeURIComponent(name)}/enabled`, { enabled },
        );
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async reconnectMcpServer(name: string): Promise<McpServerDto> {
        const server = await this.http.request<McpServerDto>("POST", `/v1/mcp/${encodeURIComponent(name)}/reconnect`);
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async loginMcpServer(name: string): Promise<McpServerDto> {
        const server = await this.http.request<McpServerDto>("POST", `/v1/mcp/${encodeURIComponent(name)}/login`);
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async logoutMcpServer(name: string): Promise<McpServerDto> {
        const server = await this.http.request<McpServerDto>("POST", `/v1/mcp/${encodeURIComponent(name)}/logout`);
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async removeMcpServer(name: string): Promise<void> {
        await this.http.request("DELETE", `/v1/mcp/${encodeURIComponent(name)}`);
    }

    async listSessions(): Promise<ChatSessionDto[]> {
        const summaries = await this.listSessionSummaries();
        return Promise.all(summaries.map((session) => this.getSession(session.id)));
    }

    async getLastSession(): Promise<ChatSessionDto | null> {
        const sessions = await this.listSessionSummaries();
        return sessions.length > 0 ? this.getSession(sessions[0].id) : null;
    }

    async setSessionModel(id: string, model: ModelDto): Promise<ChatSessionDto> {
        const session = await this.http.request<ChatSessionDto>("PUT", `/v1/sessions/${encodeURIComponent(id)}/model`, {
            providerCode: model.providerCode,
            modelCode: model.code,
        });
        return requireSession(session, `session.setModel ${id}`);
    }

    async setSessionReasoningEffort(id: string, reasoningEffort: ReasoningEffort): Promise<ChatSessionDto> {
        const session = await this.http.request<ChatSessionDto>(
            "PUT", `/v1/sessions/${encodeURIComponent(id)}/reasoning-effort`, { reasoningEffort },
        );
        return requireSession(session, `session.setReasoningEffort ${id}`);
    }

    async setSessionCwd(id: string, cwd: string | null): Promise<ChatSessionDto> {
        const session = await this.http.request<ChatSessionDto>("PUT", `/v1/sessions/${encodeURIComponent(id)}/cwd`, { cwd });
        return requireSession(session, `session.setCwd ${id}`);
    }

    async setSessionPermissionMode(id: string, permissionMode: PermissionMode): Promise<ChatSessionDto> {
        const session = await this.http.request<ChatSessionDto>(
            "PUT", `/v1/sessions/${encodeURIComponent(id)}/permission-mode`, {
            permissionMode,
            },
        );
        return requireSession(session, `session.setPermissionMode ${id}`);
    }

    async enableCodexMode(id: string): Promise<ChatSessionDto> {
        const session = await this.http.request<ChatSessionDto>("POST", `/v1/sessions/${encodeURIComponent(id)}/codex-mode`);
        return requireSession(session, `session.enableCodexMode ${id}`);
    }

    async setToolDialect(id: string, toolDialect: ToolDialect): Promise<ChatSessionDto> {
        const session = await this.http.request<ChatSessionDto>(
            "PUT", `/v1/sessions/${encodeURIComponent(id)}/tool-dialect`, { toolDialect },
        );
        return requireSession(session, `session.setToolDialect ${id}`);
    }

    startSession(id: string, text: string): Promise<ExecutionStart> {
        return this.http.request("POST", `/v1/sessions/${encodeURIComponent(id)}/rounds`, { content: [{ type: "text", text }] });
    }

    async stopSession(id: string, roundId: string): Promise<void> {
        await this.http.request(
            "POST", `/v1/sessions/${encodeURIComponent(id)}/rounds/${encodeURIComponent(roundId)}/cancel`,
        );
    }

    async resolveToolApproval(resolution: ToolApprovalResolution): Promise<void> {
        await this.http.request(
            "POST", `/v1/tool-approvals/${encodeURIComponent(resolution.approvalId)}/resolution`, resolution,
        );
    }

    async compactSession(id: string): Promise<void> {
        await this.http.request("POST", `/v1/sessions/${encodeURIComponent(id)}/compact`);
    }

    async prepareSession(options: {
        modelInput?: string;
        newSession: boolean;
        reasoningEffort?: ReasoningEffort;
    }): Promise<PreparedSession> {
        const requestedModel = options.modelInput ? await this.resolveModelInput(options.modelInput) : undefined;
        let session = options.newSession ? null : await this.getLastSession();
        if (!session) {
            const status = await this.initializationStatus();
            const model = requestedModel ?? await this.getDefaultModel();
            session = await this.createSession(
                model,
                options.reasoningEffort ?? status.defaultReasoningEffort ?? "off",
            );
            return { model, session };
        }

        if (requestedModel) {
            const matchesCurrent = session.currentModel?.providerCode === requestedModel.providerCode &&
                session.currentModel.modelCode === requestedModel.code;
            if (!matchesCurrent) {
                session = await this.setSessionModel(session.id, requestedModel);
            }
            return { model: requestedModel, session };
        }

        if (session.currentModel) {
            const model = await this.getModel(session.currentModel);
            return { model, session };
        }

        const model = await this.getDefaultModel();
        session = await this.setSessionModel(session.id, model);
        return { model, session };
    }
}

function projectModel(provider: ModelProviderDto, model: CoreModelDto): ModelDto {
    return {
        ...model,
        providerCode: provider.code,
        providerName: provider.name,
    };
}

function normalizeProvider(provider: ModelProviderDto): ModelProviderDto {
    return {
        ...provider,
        builtin: provider.builtin === true,
        official: provider.official === true,
        oauth: provider.oauth === true,
        authMethod: provider.authMethod === "oauth" ? "oauth" : "apiKey",
        modelsCached: provider.modelsCached === true,
        models: Array.isArray(provider.models)
            ? provider.models
                .filter((model): model is CoreModelDto => model !== null)
                .map((model) => {
                    const configuredEfforts = Array.isArray(model.reasoningEfforts)
                        ? model.reasoningEfforts
                        : [];
                    const reasoning = model.reasoning === true || configuredEfforts.length > 0;
                    return {
                        ...model,
                        reasoning,
                        reasoningEfforts: reasoning
                            ? configuredEfforts.length > 0
                                ? configuredEfforts
                                : [...STANDARD_REASONING_EFFORTS]
                            : [],
                    };
                })
            : [],
    };
}

function normalizeContent(content: ChatMessageDto["content"] | null | undefined): ContentBlock[] {
    if (!Array.isArray(content)) {
        return [];
    }
    return content
        .filter((block): block is ContentBlock => block !== null && typeof block === "object")
        .map((block) => {
            if (block?.type !== "tool_result") {
                return block;
            }
            return {
                ...block,
                content: normalizeContent(block.content),
            };
        });
}

function normalizeMessage(message: ChatMessageDto | null | undefined): ChatMessageDto | undefined {
    if (!message) {
        return undefined;
    }
    return {
        ...message,
        content: normalizeContent(message.content),
    };
}

function normalizeSession(session: ChatSessionDto): ChatSessionDto {
    const rounds: RoundDto[] = Array.isArray(session.rounds)
        ? session.rounds.filter((round): round is RoundDto => round !== null)
        : [];
    return {
        ...session,
        permissionMode: session.permissionMode === "yolo" || session.permissionMode === "auto"
            ? session.permissionMode
            : "ask",
        toolDialect: session.toolDialect === "codex" ? "codex" : "default",
        rounds: rounds.map((round) => ({
            ...round,
            messages: Array.isArray(round.messages)
                ? round.messages
                    .filter((message): message is ChatMessageDto => message !== null)
                    .map((message) => normalizeMessage(message)!)
                : [],
        })),
    };
}

function normalizeSessionEvent(event: SessionEvent): SessionEvent {
    return {
        ...event,
        message: normalizeMessage(event.message),
    };
}

function requireSession(session: ChatSessionDto | null, operation: string): ChatSessionDto {
    if (!session) {
        throw new Error(`core returned an empty session for ${operation}`);
    }
    return normalizeSession(session);
}

function findProjectedModel(provider: ModelProviderDto | null, modelCode: string): ModelDto {
    if (!provider) {
        throw new Error("core returned an empty provider while adding a model");
    }
    const normalizedProvider = normalizeProvider(provider);
    const model = normalizedProvider.models.find((candidate) => candidate.code === modelCode);
    if (!model) {
        throw new Error(`core did not return model ${normalizedProvider.code}/${modelCode}`);
    }
    return projectModel(normalizedProvider, model);
}

function paginate<T>(data: T[] | null | undefined, page: number, pageSize: number): PagedResponse<T> {
    const normalized = data ?? [];
    const start = Math.max(0, (page - 1) * pageSize);
    return {
        total: normalized.length,
        page,
        pageSize,
        data: normalized.slice(start, start + pageSize),
    };
}
