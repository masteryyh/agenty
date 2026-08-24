export type ReasoningEffort = "" | "off" | "low" | "medium" | "high" | "xhigh" | "max";
export const STANDARD_REASONING_EFFORTS: readonly ReasoningEffort[] = [
    "low",
    "medium",
    "high",
    "xhigh",
    "max",
];
export type APIType = "openai" | "openai_completions" | "anthropic" | "gemini";

export interface ModelRef {
    providerCode: string;
    modelCode: string;
}

export interface AgentDto {
    code: string;
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
    code: string;
    name: string;
    description?: string;
    soul?: string;
    defaultModel?: ModelRef;
    defaultContextWindow?: number;
    defaultReasoningEffort?: ReasoningEffort;
    isDefault?: boolean;
    metadata?: Record<string, unknown>;
}

export type UpdateAgentDto = Partial<Omit<CreateAgentDto, "code">>;

export interface ModelDto {
    code: string;
    providerCode: string;
    providerName: string;
    name: string;
    contextWindow: number;
    /** The exact model limit, or the custom-model fallback when omitted. */
    maxOutputTokens: number;
    multiModal: boolean;
    light: boolean;
    reasoningEfforts?: ReasoningEffort[];
    isDefault: boolean;
    createdAt?: string;
    updatedAt?: string;
}

export interface CoreModelDto extends Omit<ModelDto, "providerCode" | "providerName"> {}

export interface AvailableModelDto {
    code: string;
    name: string;
    contextWindow: number;
    maxOutputTokens: number;
    multiModal: boolean;
    reasoningEfforts: ReasoningEffort[];
}

export interface CreateModelDto {
    providerCode: string;
    modelCode: string;
    name: string;
    contextWindow?: number;
    /** Defaults to the core fallback when omitted. */
    maxOutputTokens?: number;
    multiModal?: boolean;
    light?: boolean;
    reasoning?: boolean;
    isDefault?: boolean;
}

export type UpdateModelDto = Omit<CreateModelDto, "providerCode" | "modelCode">;

export interface ModelProviderDto {
    code: string;
    name: string;
    type: APIType;
    baseUrl: string;
    apiKey: string;
    freeFormTool?: boolean;
    builtin?: boolean;
    official?: boolean;
    modelsUrl?: string;
    tokenCountUrl?: string;
    models: CoreModelDto[];
    /** True when core populated models from its discovery cache. */
    modelsCached?: boolean;
    metadata?: Record<string, unknown>;
    createdAt: string;
    updatedAt: string;
}

export interface CreateModelProviderDto {
    code: string;
    name: string;
    type: APIType;
    baseUrl?: string;
    apiKey?: string;
    freeFormTool?: boolean;
    metadata?: Record<string, unknown>;
}

export type UpdateModelProviderDto = Partial<Omit<CreateModelProviderDto, "code">>;

export type ContentBlock =
    | { type: "text"; text: string }
    | { type: "reasoning"; text: string; signature?: string; redacted?: boolean }
    | { type: "tool_use"; id: string; name: string; input: unknown }
    | {
        type: "shell_call";
        id?: string;
        callId: string;
        commands: string[];
        timeoutMs?: number;
        maxOutputLength?: number;
    }
    | {
        type: "shell_call_output";
        callId: string;
        maxOutputLength: number;
        openAINative?: boolean;
        output: Array<{
            stdout: string;
            stderr: string;
            outcome: { type: string; exitCode?: number };
        }>;
    }
    | {
        type: "apply_patch_call";
        id?: string;
        callId: string;
        source: "native" | "custom";
        operation?: {
            type: "create_file" | "update_file" | "delete_file";
            path: string;
            diff?: string;
            moveTo?: string;
        };
        patch?: string;
    }
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
    agentCode: string;
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
    agentCode: string;
    lastProviderCode: string;
    lastModelCode: string;
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

export interface CompactionEvent {
    type: "started" | "completed" | "failed";
    sessionId: string;
    compactionId?: string;
    trigger: "manual" | "auto" | "model_switch";
    contextTokensBefore?: number;
    contextTokensAfter?: number;
    usage?: TokenUsage;
    error?: string;
}

export interface ExecutionStart {
    sessionId: string;
    roundId: string;
    status: "running";
}

export interface InitializeCompleteInput {
    agentCode: string;
    providerCode: string;
    modelCode: string;
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
