CREATE EXTENSION IF NOT EXISTS vector;

CREATE EXTENSION IF NOT EXISTS pg_search;

CREATE TABLE IF NOT EXISTS system_settings (
	id UUID PRIMARY KEY,
	initialized BOOLEAN NOT NULL DEFAULT FALSE,
	embedding_model_id UUID,
	context_compression_model_id UUID,
	web_search_provider VARCHAR(50) NOT NULL DEFAULT 'disabled',
	brave_api_key VARCHAR(255) NOT NULL DEFAULT '',
	tavily_api_key VARCHAR(255) NOT NULL DEFAULT '',
	firecrawl_api_key VARCHAR(255) NOT NULL DEFAULT '',
	firecrawl_base_url VARCHAR(255) NOT NULL DEFAULT 'https://api.firecrawl.dev',
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agents (
	id UUID PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	soul TEXT NOT NULL,
	is_default BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS model_providers (
	id UUID PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	type VARCHAR(50) NOT NULL,
	base_url VARCHAR(255) NOT NULL,
	bailian_multi_modal_embedding_base_url VARCHAR(255),
	api_key VARCHAR(255) NOT NULL DEFAULT '',
	is_preset BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS models (
	id UUID PRIMARY KEY,
	provider_id UUID NOT NULL,
	name VARCHAR(255) NOT NULL,
	code VARCHAR(255) NOT NULL,
	default_model BOOLEAN NOT NULL DEFAULT FALSE,
	embedding_model BOOLEAN NOT NULL DEFAULT FALSE,
	context_compression_model BOOLEAN NOT NULL DEFAULT FALSE,
	multi_modal BOOLEAN NOT NULL DEFAULT FALSE,
	light BOOLEAN NOT NULL DEFAULT FALSE,
	thinking BOOLEAN NOT NULL DEFAULT FALSE,
	thinking_levels JSONB NOT NULL DEFAULT '[]'::jsonb,
	anthropic_adaptive_thinking BOOLEAN NOT NULL DEFAULT FALSE,
	is_preset BOOLEAN NOT NULL DEFAULT FALSE,
	context_window INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS agent_models (
	agent_id UUID NOT NULL,
	model_id UUID NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (agent_id, model_id)
);

CREATE TABLE IF NOT EXISTS chat_sessions (
	id UUID PRIMARY KEY,
	agent_id UUID NOT NULL,
	token_consumed BIGINT NOT NULL DEFAULT 0,
	last_used_model UUID,
	last_used_thinking_level TEXT,
	cwd TEXT,
	agents_md TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS chat_messages (
	id UUID PRIMARY KEY,
	session_id UUID NOT NULL,
	round_id UUID,
	agent_id UUID NOT NULL,
	role VARCHAR(50) NOT NULL,
	content TEXT,
	tool_calls JSONB,
	tool_results JSONB,
	model_id UUID,
	reasoning_content TEXT,
	provider_specifics JSONB,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS chat_round_token_usages (
	id UUID PRIMARY KEY,
	session_id UUID NOT NULL,
	agent_id UUID NOT NULL,
	model_id UUID NOT NULL,
	round_id UUID NOT NULL,
	total_tokens BIGINT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS mcp_servers (
	id UUID PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	transport VARCHAR(50) NOT NULL,
	enabled BOOLEAN NOT NULL DEFAULT TRUE,
	command VARCHAR(512),
	args JSONB,
	env JSONB,
	url VARCHAR(512),
	headers JSONB,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS knowledge_items (
	id UUID PRIMARY KEY,
	agent_id UUID NOT NULL,
	category VARCHAR(50) NOT NULL,
	content_type VARCHAR(50) NOT NULL DEFAULT 'text',
	title VARCHAR(500),
	content TEXT NOT NULL,
	language VARCHAR(20),
	metadata JSONB,
	source_session_id UUID,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS kb_data (
	id UUID PRIMARY KEY,
	item_id UUID NOT NULL,
	agent_id UUID NOT NULL,
	chunk_index INTEGER NOT NULL DEFAULT 0,
	chunk_content TEXT NOT NULL,
	text_embedding vector(1024),
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS skills (
	id UUID PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	description TEXT NOT NULL,
	skill_md_path TEXT NOT NULL,
	metadata JSONB,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agents_deleted_at ON agents (deleted_at);
CREATE INDEX IF NOT EXISTS idx_chat_sessions_agent_id ON chat_sessions (agent_id);
CREATE INDEX IF NOT EXISTS idx_chat_messages_session_id ON chat_messages (session_id);
CREATE INDEX IF NOT EXISTS idx_chat_round_token_usages_session_id ON chat_round_token_usages (session_id);
CREATE INDEX IF NOT EXISTS idx_chat_round_token_usages_round_id ON chat_round_token_usages (round_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_servers_name ON mcp_servers (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_knowledge_items_agent_id ON knowledge_items (agent_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_items_category ON knowledge_items (category);
CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_items_source_session_id ON knowledge_items (source_session_id) WHERE source_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kb_data_item_id ON kb_data (item_id);
CREATE INDEX IF NOT EXISTS idx_kb_data_agent_id ON kb_data (agent_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_skill_md_path ON skills (skill_md_path);
CREATE INDEX IF NOT EXISTS idx_kb_data_text_embedding_hnsw ON kb_data USING hnsw (text_embedding vector_ip_ops);
CREATE INDEX IF NOT EXISTS idx_kb_data_bm25 ON kb_data USING bm25 (id, agent_id, item_id, chunk_content, created_at) WITH (key_field = 'id');
CREATE INDEX IF NOT EXISTS idx_skills_bm25 ON skills USING bm25 (id, name, description) WITH (key_field = 'id');
