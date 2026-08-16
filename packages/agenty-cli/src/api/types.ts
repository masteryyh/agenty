export type ReasoningEffort = "" | "off" | "low" | "medium" | "high" | "xhigh" | "max";
export type APIType = "openai" | "openai_completions" | "anthropic" | "gemini";

export interface ModelRef {
    providerSlug: string;
    modelSlug: string;
}

export interface AgentDto {
    slug: string;
    name: string;
    description?: string;
    soul: string;
    defaultModel?: ModelRef;
    defaultContextWindow: number;
    defaultReasoningEffort?: ReasoningEffort;
    isDefault: boolean;
    metadata?: Record<string, unknown>;
    createdAt: string;
    updatedAt: string;
}

export interface CreateAgentDto {
    slug: string;
    name: string;
    description?: string;
    soul?: string;
    defaultModel?: ModelRef;
    defaultContextWindow?: number;
    defaultReasoningEffort?: ReasoningEffort;
    isDefault?: boolean;
    metadata?: Record<string, unknown>;
}

export type UpdateAgentDto = Partial<Omit<CreateAgentDto, "slug">>;

export interface ModelDto {
    slug: string;
    providerSlug: string;
    providerName: string;
    name: string;
    contextWindow: number;
    maxOutputTokens: number;
    multiModal: boolean;
    light: boolean;
    reasoningEffortMapping?: Record<string, ReasoningEffort>;
    isDefault: boolean;
    createdAt?: string;
    updatedAt?: string;
}

export interface CoreModelDto extends Omit<ModelDto, "providerSlug" | "providerName"> {}

export interface CreateModelDto {
    providerSlug: string;
    modelSlug: string;
    name: string;
    contextWindow?: number;
    maxOutputTokens: number;
    multiModal?: boolean;
    light?: boolean;
    reasoningEffortMapping?: Record<string, ReasoningEffort>;
    isDefault?: boolean;
}

export type UpdateModelDto = Omit<CreateModelDto, "providerSlug" | "modelSlug">;

export interface ModelProviderDto {
    slug: string;
    name: string;
    type: APIType;
    baseUrl: string;
    apiKey: string;
    models: CoreModelDto[];
    metadata?: Record<string, unknown>;
    createdAt: string;
    updatedAt: string;
}

export interface CreateModelProviderDto {
    slug: string;
    name: string;
    type: APIType;
    baseUrl?: string;
    apiKey?: string;
    metadata?: Record<string, unknown>;
}

export type UpdateModelProviderDto = Partial<Omit<CreateModelProviderDto, "slug">>;

export type ContentBlock =
    | { type: "text"; text: string }
    | { type: "reasoning"; text: string; signature?: string; redacted?: boolean }
    | { type: "tool_use"; id: string; name: string; input: unknown }
    | { type: "tool_result"; toolUseId: string; content: ContentBlock[]; isError: boolean }
    | { type: "image"; mediaType: string; data: string };

export type MessageRole = "user" | "assistant" | "system";

export interface TokenUsage {
    input: number;
    output: number;
    total: number;
}

export interface ChatMessageDto {
    id: string;
    roundId: string;
    role: MessageRole;
    content: ContentBlock[];
    model?: ModelRef;
    usage?: TokenUsage;
    createdAt: string;
}

export type RoundStatus = "running" | "completed" | "failed" | "cancelled";

export interface RoundDto {
    id: string;
    sessionId: string;
    sequence: number;
    status: RoundStatus;
    model: ModelRef;
    contextWindow: number;
    reasoningEffort?: ReasoningEffort;
    cwd?: string;
    messages: ChatMessageDto[];
    usage: TokenUsage;
    error?: string;
    startedAt: string;
    endedAt?: string;
}

export interface ChatSessionDto {
    id: string;
    agentSlug: string;
    title?: string;
    cwd?: string;
    currentModel?: ModelRef;
    contextWindow: number;
    currentReasoningEffort?: ReasoningEffort;
    rounds: RoundDto[];
    createdAt: string;
    updatedAt: string;
}

export interface SessionSummaryDto {
    id: string;
    title: string;
    agentSlug: string;
    lastProviderSlug: string;
    lastModelSlug: string;
    contextWindow: number;
    lastReasoningEffort?: ReasoningEffort;
    createdAt: string;
    updatedAt: string;
}

export interface StreamEvent {
    type: "text_delta" | "reasoning_delta" | "tool_use_start" | "tool_input_delta" | "tool_use_done" | "completed";
    index?: number;
    delta?: string;
    toolUseId?: string;
    toolName?: string;
    toolInput?: unknown;
}

export interface SessionEvent {
    type: "round_started" | "message_appended" | "model_stream" | "round_ended";
    sessionId: string;
    roundId: string;
    sequence: number;
    iteration?: number;
    stream?: StreamEvent;
    message?: ChatMessageDto;
    status?: RoundStatus;
    usage?: TokenUsage;
    error?: string;
}

export interface ExecutionStart {
    sessionId: string;
    roundId: string;
    status: "running";
}

export interface InitializeCompleteInput {
    agentSlug: string;
    providerSlug: string;
    modelSlug: string;
}

export interface PagedResponse<T> {
    total: number;
    pageSize: number;
    page: number;
    data: T[];
}

export interface ToolResult {
    callId: string;
    name: string;
    content: string;
    isError: boolean;
}
