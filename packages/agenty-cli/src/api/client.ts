import type { StdioRPCClient } from "../core/rpc";
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
    UpdateModelDto,
    UpdateModelProviderDto,
} from "./types";
import { STANDARD_REASONING_EFFORTS } from "./types";

export interface PreparedSession {
    model: ModelDto;
    session: ChatSessionDto;
}

export class AgentyClient {
    constructor(private readonly rpc: StdioRPCClient) {}

    onSessionEvent(listener: (event: SessionEvent) => void): () => void {
        return this.rpc.onNotification<SessionEvent | null | undefined>("session.event", (event) => {
            if (event) {
                listener(normalizeSessionEvent(event));
            }
        });
    }

    onCompactionEvent(listener: (event: CompactionEvent) => void): () => void {
        return this.rpc.onNotification<CompactionEvent | null | undefined>("session.compaction", (event) => {
            if (event) {
                listener(event);
            }
        });
    }

    onMcpEvent(listener: (event: McpEvent) => void): () => void {
        return this.rpc.onNotification<McpEvent | null | undefined>("mcp.event", (event) => {
            if (event) {
                listener(event);
            }
        });
    }

    onClose(listener: (reason: Error) => void): () => void {
        return this.rpc.onClose(listener);
    }

    async initializationStatus(): Promise<InitializeStatusDto> {
        const result = await this.rpc.call<InitializeStatusDto | null>("initialize.already");
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
        const result = await this.rpc.call<{ initialized?: boolean } | null>("initialize.complete", input);
        return { initialized: result?.initialized === true };
    }

    async listProviders(providerCode?: string): Promise<ModelProviderDto[]> {
        const providers = providerCode
            ? await this.rpc.call<Array<ModelProviderDto | null> | null>("provider.list", { providerCode })
            : await this.rpc.call<Array<ModelProviderDto | null> | null>("provider.list");
        return (providers ?? [])
            .filter((provider): provider is ModelProviderDto => provider !== null)
            .map(normalizeProvider);
    }

    async listProvidersPage(page = 1, pageSize = 100): Promise<PagedResponse<ModelProviderDto>> {
        return paginate(await this.listProviders(), page, pageSize);
    }

    async listProviderModels(providerCode: string): Promise<AvailableModelDto[]> {
        const models = await this.rpc.call<Array<AvailableModelDto | null> | null>("provider.listModels", {
            providerCode,
        });
        return (models ?? [])
            .filter((model): model is AvailableModelDto => model !== null)
            .map((model) => ({
                ...model,
                reasoningEfforts: Array.isArray(model.reasoningEfforts) ? model.reasoningEfforts : [],
            }));
    }

    async createProvider(input: CreateModelProviderDto): Promise<ModelProviderDto> {
        const provider = await this.rpc.call<ModelProviderDto | null>("provider.create", input);
        if (!provider) {
            throw new Error("core returned an empty provider");
        }
        return normalizeProvider(provider);
    }

    async updateProvider(code: string, input: UpdateModelProviderDto): Promise<ModelProviderDto> {
        const provider = await this.rpc.call<ModelProviderDto | null>("provider.update", { code, ...input });
        if (!provider) {
            throw new Error(`core returned an empty provider for ${code}`);
        }
        return normalizeProvider(provider);
    }

    async deleteProvider(code: string): Promise<void> {
        await this.rpc.call("provider.delete", { code });
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
        const provider = await this.rpc.call<ModelProviderDto | null>("provider.addModel", input);
        return findProjectedModel(provider, input.modelCode);
    }

    async updateModel(providerCode: string, modelCode: string, input: UpdateModelDto): Promise<ModelDto> {
        const provider = await this.rpc.call<ModelProviderDto | null>("provider.addModel", {
            providerCode,
            modelCode,
            ...input,
        });
        return findProjectedModel(provider, modelCode);
    }

    async deleteModel(providerCode: string, modelCode: string): Promise<void> {
        await this.rpc.call("provider.removeModel", { providerCode, modelCode });
    }

    async createSession(
        model: ModelDto,
        effort: ReasoningEffort = "off",
        permissionMode: PermissionMode = "ask",
    ): Promise<ChatSessionDto> {
        const session = await this.rpc.call<ChatSessionDto | null>("session.create", {
            providerCode: model.providerCode,
            modelCode: model.code,
            contextWindow: model.contextWindow,
            reasoningEffort: effort,
            permissionMode,
        });
        return requireSession(session, "session.create");
    }

    async getSession(id: string): Promise<ChatSessionDto> {
        const session = await this.rpc.call<ChatSessionDto | null>("session.get", { id });
        return requireSession(session, `session.get ${id}`);
    }

    async listSessionSummaries(): Promise<SessionSummaryDto[]> {
        const summaries = await this.rpc.call<Array<SessionSummaryDto | null> | null>("session.list", {});
        return (summaries ?? []).filter((summary): summary is SessionSummaryDto => summary !== null);
    }

    async listSkills(): Promise<{ skills: SkillDto[]; diagnostics: SkillDiagnosticDto[] }> {
        const result = await this.rpc.call<{
            skills?: SkillDto[];
            diagnostics?: SkillDiagnosticDto[];
        } | null>("skill.list");
        return {
            skills: result?.skills ?? [],
            diagnostics: result?.diagnostics ?? [],
        };
    }

    async listMcpServers(): Promise<McpServerDto[]> {
        const servers = await this.rpc.call<Array<McpServerDto | null> | null>("mcp.list");
        return (servers ?? []).filter((server): server is McpServerDto => server !== null);
    }

    async listMcpServerLogs(name: string): Promise<McpLogEntry[]> {
        const logs = await this.rpc.call<Array<McpLogEntry | null> | null>("mcp.logs", { name });
        return (logs ?? []).filter((log): log is McpLogEntry => log !== null);
    }

    async createMcpServer(name: string, config: McpServerConfig): Promise<McpServerDto> {
        const server = await this.rpc.call<McpServerDto | null>("mcp.create", { name, config });
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async updateMcpServer(name: string, config: McpServerConfig): Promise<McpServerDto> {
        const server = await this.rpc.call<McpServerDto | null>("mcp.update", { name, config });
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async setMcpEnabled(name: string, enabled: boolean): Promise<McpServerDto> {
        const server = await this.rpc.call<McpServerDto | null>("mcp.enable", { name, enabled });
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async reconnectMcpServer(name: string): Promise<McpServerDto> {
        const server = await this.rpc.call<McpServerDto | null>("mcp.reconnect", { name });
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async loginMcpServer(name: string): Promise<McpServerDto> {
        const server = await this.rpc.call<McpServerDto | null>("mcp.login", { name });
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async logoutMcpServer(name: string): Promise<McpServerDto> {
        const server = await this.rpc.call<McpServerDto | null>("mcp.logout", { name });
        if (!server) {
            throw new Error(`core returned an empty MCP server for ${name}`);
        }
        return server;
    }

    async removeMcpServer(name: string): Promise<void> {
        await this.rpc.call("mcp.remove", { name });
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
        const session = await this.rpc.call<ChatSessionDto | null>("session.setModel", {
            id,
            providerCode: model.providerCode,
            modelCode: model.code,
        });
        return requireSession(session, `session.setModel ${id}`);
    }

    async setSessionReasoningEffort(id: string, reasoningEffort: ReasoningEffort): Promise<ChatSessionDto> {
        const session = await this.rpc.call<ChatSessionDto | null>("session.setReasoningEffort", { id, reasoningEffort });
        return requireSession(session, `session.setReasoningEffort ${id}`);
    }

    async setSessionCwd(id: string, cwd: string | null): Promise<ChatSessionDto> {
        const session = await this.rpc.call<ChatSessionDto | null>("session.setCwd", { id, cwd });
        return requireSession(session, `session.setCwd ${id}`);
    }

    async setSessionPermissionMode(id: string, permissionMode: PermissionMode): Promise<ChatSessionDto> {
        const session = await this.rpc.call<ChatSessionDto | null>("session.setPermissionMode", {
            id,
            permissionMode,
        });
        return requireSession(session, `session.setPermissionMode ${id}`);
    }

    async enableCodexMode(id: string): Promise<ChatSessionDto> {
        const session = await this.rpc.call<ChatSessionDto | null>("session.enableCodexMode", { id });
        return requireSession(session, `session.enableCodexMode ${id}`);
    }

    startSession(id: string, text: string): Promise<ExecutionStart> {
        return this.rpc.call("session.start", { id, content: [{ type: "text", text }] });
    }

    async stopSession(id: string): Promise<void> {
        await this.rpc.call("session.stop", { id });
    }

    async resolveToolApproval(resolution: ToolApprovalResolution): Promise<void> {
        await this.rpc.call("session.resolveToolApproval", resolution);
    }

    async compactSession(id: string): Promise<void> {
        await this.rpc.call("session.compact", { id });
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
