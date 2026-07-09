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

export const StreamEventType = {
	ReasoningDelta: "reasoning_delta",
	ContentDelta: "content_delta",
	ToolCallStart: "tool_call_start",
	ToolCallDelta: "tool_call_delta",
	ToolCallDone: "tool_call_done",
	ToolResult: "tool_result",
	MessageDone: "message_done",
	Usage: "usage",
	Error: "error",
	Done: "done",
	ModelSwitch: "model_switch",
	CompactionStart: "compaction_start",
	CompactionDone: "compaction_done",
} as const;

export type StreamEventType = (typeof StreamEventType)[keyof typeof StreamEventType];

export type MessageRole = "user" | "assistant" | "tool" | "system";

export interface ToolCall {
	id: string;
	name: string;
	arguments: string;
}

export interface ToolResult {
	callId: string;
	name: string;
	content: string;
	isError: boolean;
}

export interface ReasoningBlock {
	summary: string;
	signature?: string;
	redacted?: boolean;
}

export interface ProviderMessage {
	role: MessageRole;
	content: string;
	toolCalls?: ToolCall[];
	toolResult?: ToolResult;
	reasoningContent?: string;
	reasoningDurationMillis?: number;
	reasoningBlocks?: ReasoningBlock[];
}

export interface StreamUsage {
	totalTokens: number;
	contextTokens?: number;
}

export interface StreamEvent {
	type: StreamEventType;
	content?: string;
	reasoning?: string;
	toolCall?: ToolCall;
	toolResult?: ToolResult;
	usage?: StreamUsage;
	error?: string;
	message?: ProviderMessage;
	modelId?: string;
	modelName?: string;
	modelThinking?: boolean;
	modelThinkingLevels?: string[];
}

export interface ChatDto {
	modelId: string;
	message: string;
	thinking: boolean;
	thinkingLevel: string;
}

export interface ModelProviderDto {
	id: string;
	name: string;
	type: string;
	baseUrl: string;
	apiKeyCensored: string;
	isPreset: boolean;
}

export interface CreateModelProviderDto {
	name: string;
	type: string;
	baseUrl: string;
	apiKey: string;
}

export interface UpdateModelProviderDto {
	name?: string;
	type?: string;
	baseUrl?: string;
	apiKey?: string;
}

export interface ModelDto {
	id: string;
	provider?: ModelProviderDto;
	name: string;
	code: string;
	defaultModel: boolean;
	embeddingModel: boolean;
	contextCompressionModel: boolean;
	multiModal: boolean;
	light: boolean;
	thinking: boolean;
	thinkingLevels: string[];
	anthropicAdaptiveThinking: boolean;
	isPreset: boolean;
	contextWindow: number;
}

export interface AgentDto {
	id: string;
	name: string;
	soul: string;
	isDefault: boolean;
	models?: ModelDto[];
}

export interface CreateAgentDto {
	name: string;
	soul?: string;
	isDefault: boolean;
	modelIds?: string[];
}

export interface UpdateAgentDto {
	name?: string;
	soul?: string;
	isDefault?: boolean;
	modelIds?: string[];
}

export interface ChatMessageDto {
	id: string;
	roundId: string;
	agentId: string;
	role: MessageRole;
	content: string;
	toolCalls?: ToolCall[];
	toolResult?: ToolResult;
	model?: ModelDto;
	reasoningContent?: string;
	reasoningDurationMillis?: number;
	createdAt: string;
}

export interface ChatSessionDto {
	id: string;
	agentId: string;
	tokenConsumed: number;
	contextTokens: number;
	messages: ChatMessageDto[];
	lastUsedModel: string;
	lastUsedThinkingLevel?: string;
	cwd?: string;
	createdAt: string;
	updatedAt: string;
}

export interface VersionDto {
	version: string;
}

export interface SystemSettingsDto {
	initialized: boolean;
	embeddingModelId?: string;
	contextCompressionModelId?: string;
	webSearchProvider: string;
	configuredWebSearchProviders?: string[];
	lastConfiguredWebSearchProvider?: string;
	braveApiKey?: string;
	tavilyApiKey?: string;
	firecrawlApiKey?: string;
	firecrawlBaseUrl?: string;
}

export interface UpdateSystemSettingsDto {
	initialized?: boolean;
	embeddingModelId?: string;
	contextCompressionModelId?: string;
	webSearchProvider?: string;
	braveApiKey?: string;
	tavilyApiKey?: string;
	firecrawlApiKey?: string;
	firecrawlBaseUrl?: string;
}

export type MCPTransportType = "stdio" | "sse" | "streamable-http";

export interface MCPServerDto {
	id: string;
	name: string;
	transport: MCPTransportType;
	enabled: boolean;
	command?: string;
	args?: string[];
	env?: Record<string, string>;
	url?: string;
	headers?: Record<string, string>;
	status?: string;
	tools?: string[];
	error?: string;
	createdAt: string;
	updatedAt: string;
}

export interface CreateMCPServerDto {
	name: string;
	transport: MCPTransportType;
	enabled?: boolean;
	command?: string;
	args?: string[];
	env?: Record<string, string>;
	url?: string;
	headers?: Record<string, string>;
}

export interface UpdateMCPServerDto {
	name?: string;
	transport?: MCPTransportType;
	enabled?: boolean;
	command?: string;
	args?: string[];
	env?: Record<string, string>;
	url?: string;
	headers?: Record<string, string>;
}

export interface SkillDto {
	id: string;
	name: string;
	description: string;
	skillMdPath: string;
	scope?: "global" | "project";
	sourceDir?: string;
	createdAt: string;
	updatedAt: string;
}

export interface SkillContentResult {
	content: string;
}

export interface GenericResponse<T> {
	code: number;
	message: string;
	data?: T;
}

export interface PagedResponse<T> {
	total: number;
	pageSize: number;
	page: number;
	data: T[];
}
