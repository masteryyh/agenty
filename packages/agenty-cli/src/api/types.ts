export type ReasoningEffort = "" | "off" | "low" | "medium" | "high" | "xhigh" | "max";
export type PermissionMode = "ask" | "auto" | "yolo";
export type ToolDialect = "default" | "codex";
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
    reasoning?: boolean;
    reasoningEfforts?: ReasoningEffort[];
    isDefault: boolean;
    cached?: boolean;
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
    reasoning?: boolean;
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
    reasoningEfforts?: ReasoningEffort[];
    isDefault?: boolean;
}

export type UpdateModelDto = Omit<CreateModelDto, "providerCode" | "modelCode">;

export interface ModelProviderDto {
    code: string;
    name: string;
    type: APIType;
    baseUrl: string;
    apiKey: string;
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

export interface SkillDto {
    name: string;
    directoryName: string;
    description: string;
    location: string;
    source: string;
    autoEnabled: boolean;
    warning?: string;
}

export interface SkillDiagnosticDto {
    severity: string;
    code: string;
    message: string;
    path?: string;
    name?: string;
}

export type McpTransport = "stdio" | "http" | "sse";
export type McpServerStatus =
    | "disabled"
    | "connecting"
    | "connected"
    | "auth-required"
    | "authenticating"
    | "error"
    | "closing";

export interface McpServerConfig {
    type: McpTransport;
    enabled: boolean;
    command?: string;
    args?: string[];
    env?: Record<string, string>;
    url?: string;
    headers?: Record<string, string>;
}

export interface McpServerDto {
    name: string;
    config: McpServerConfig;
    status: McpServerStatus;
    error?: string;
    errorStage?: string;
    toolCount: number;
    lastChanged?: string;
    deprecated?: boolean;
    authUrl?: string;
}

export interface McpLogEntry {
    time: string;
    level: string;
    source: string;
    stage?: string;
    message: string;
}

export interface McpEvent {
    type: string;
    name: string;
    status?: McpServerStatus;
    error?: string;
    errorStage?: string;
    toolCount?: number;
    authUrl?: string;
}

export interface CreateModelProviderDto {
    code: string;
    name: string;
    type: APIType;
    baseUrl?: string;
    apiKey?: string;
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

export type MessageRole = "user" | "assistant" | "system" | "developer";

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
    title?: string;
    cwd?: string;
    currentModel?: ModelRef;
    contextWindow: number;
    currentReasoningEffort?: ReasoningEffort;
    permissionMode?: PermissionMode;
    pendingPermissionMode?: PermissionMode;
    toolDialect?: ToolDialect;
    rounds: RoundDto[];
    createdAt: string;
    updatedAt: string;
}

export interface SessionSummaryDto {
    id: string;
    title: string;
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

export interface ToolApprovalRequest {
    message?: string;
    approvalId: string;
    toolCall: { type: "tool_use"; id: string; name: string; input: unknown };
    cwd: string;
    preview: { title: string; detail: string };
}

export interface ToolApprovalResolution {
    sessionId: string;
    roundId: string;
    approvalId: string;
    decision: "allow" | "deny";
}

export interface ToolReviewEvent {
    toolUseId: string;
}

export interface SessionEvent {
    type: "round_started" | "message_appended" | "model_stream" | "round_ended" | "tool_approval_requested" | "tool_approval_resolved" | "tool_review_started" | "tool_review_resolved" | "permission_mode_changed" | "tool_dialect_changed";
    sessionId: string;
    roundId: string;
    sequence: number;
    iteration?: number;
    stream?: StreamEvent;
    message?: ChatMessageDto;
    status?: RoundStatus;
    usage?: TokenUsage;
    error?: string;
    approval?: ToolApprovalRequest;
    resolution?: ToolApprovalResolution;
    review?: ToolReviewEvent;
    permissionMode?: PermissionMode;
    previousPermissionMode?: PermissionMode;
    toolDialect?: ToolDialect;
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
    providerCode: string;
    modelCode: string;
    reasoningEffort?: ReasoningEffort;
}

export interface InitializeStatusDto {
    initialized: boolean;
    defaultModel?: ModelRef;
    defaultReasoningEffort?: ReasoningEffort;
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
