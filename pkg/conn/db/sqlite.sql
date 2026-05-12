PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS system_settings (
	id TEXT PRIMARY KEY,
	initialized INTEGER NOT NULL DEFAULT 0,
	embedding_model_id TEXT,
	context_compression_model_id TEXT,
	web_search_provider TEXT NOT NULL DEFAULT 'disabled',
	brave_api_key TEXT NOT NULL DEFAULT '',
	tavily_api_key TEXT NOT NULL DEFAULT '',
	firecrawl_api_key TEXT NOT NULL DEFAULT '',
	firecrawl_base_url TEXT NOT NULL DEFAULT 'https://api.firecrawl.dev',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agents (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	soul TEXT NOT NULL,
	is_default INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS model_providers (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	base_url TEXT NOT NULL,
	bailian_multi_modal_embedding_base_url TEXT,
	api_key TEXT NOT NULL DEFAULT '',
	is_preset INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS models (
	id TEXT PRIMARY KEY,
	provider_id TEXT NOT NULL,
	name TEXT NOT NULL,
	code TEXT NOT NULL,
	default_model INTEGER NOT NULL DEFAULT 0,
	embedding_model INTEGER NOT NULL DEFAULT 0,
	context_compression_model INTEGER NOT NULL DEFAULT 0,
	multi_modal INTEGER NOT NULL DEFAULT 0,
	light INTEGER NOT NULL DEFAULT 0,
	thinking INTEGER NOT NULL DEFAULT 0,
	thinking_levels TEXT NOT NULL DEFAULT '[]',
	anthropic_adaptive_thinking INTEGER NOT NULL DEFAULT 0,
	is_preset INTEGER NOT NULL DEFAULT 0,
	context_window INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS agent_models (
	agent_id TEXT NOT NULL,
	model_id TEXT NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (agent_id, model_id)
);

CREATE TABLE IF NOT EXISTS chat_sessions (
	id TEXT PRIMARY KEY,
	agent_id TEXT NOT NULL,
	token_consumed INTEGER NOT NULL DEFAULT 0,
	context_tokens INTEGER NOT NULL DEFAULT 0,
	last_used_model TEXT,
	last_used_thinking_level TEXT,
	active_compaction_id TEXT,
	cwd TEXT,
	agents_md TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS chat_messages (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	round_id TEXT,
	agent_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT,
	tool_calls TEXT,
	tool_results TEXT,
	model_id TEXT,
	reasoning_content TEXT,
	provider_specifics TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS chat_round_token_usages (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	model_id TEXT NOT NULL,
	round_id TEXT NOT NULL,
	total_tokens INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS chat_compactions (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	model_id TEXT NOT NULL,
	type TEXT NOT NULL,
	compacted_until_round_id TEXT,
	compacted_until_message_id TEXT,
	summary TEXT NOT NULL DEFAULT '',
	compacted_messages TEXT,
	source_token_estimate INTEGER NOT NULL DEFAULT 0,
	compacted_token_estimate INTEGER NOT NULL DEFAULT 0,
	threshold_tokens INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS mcp_servers (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	transport TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	command TEXT,
	args TEXT,
	env TEXT,
	url TEXT,
	headers TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS knowledge_items (
	id TEXT PRIMARY KEY,
	agent_id TEXT NOT NULL,
	category TEXT NOT NULL,
	content_type TEXT NOT NULL DEFAULT 'text',
	title TEXT,
	content TEXT NOT NULL,
	language TEXT,
	metadata TEXT,
	source_session_id TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS kb_data (
	id TEXT PRIMARY KEY,
	item_id TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	chunk_index INTEGER NOT NULL DEFAULT 0,
	chunk_content TEXT NOT NULL,
	text_embedding BLOB,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS skills (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT NOT NULL,
	skill_md_path TEXT NOT NULL,
	metadata TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agents_deleted_at ON agents (deleted_at);
CREATE INDEX IF NOT EXISTS idx_chat_sessions_agent_id ON chat_sessions (agent_id);
CREATE INDEX IF NOT EXISTS idx_chat_messages_session_id ON chat_messages (session_id);
CREATE INDEX IF NOT EXISTS idx_chat_round_token_usages_session_id ON chat_round_token_usages (session_id);
CREATE INDEX IF NOT EXISTS idx_chat_round_token_usages_round_id ON chat_round_token_usages (round_id);
CREATE INDEX IF NOT EXISTS idx_chat_compactions_session_id ON chat_compactions (session_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_servers_name ON mcp_servers (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_knowledge_items_agent_id ON knowledge_items (agent_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_items_category ON knowledge_items (category);
CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_items_source_session_id ON knowledge_items (source_session_id) WHERE source_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kb_data_item_id ON kb_data (item_id);
CREATE INDEX IF NOT EXISTS idx_kb_data_agent_id ON kb_data (agent_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_skill_md_path ON skills (skill_md_path);

CREATE VIRTUAL TABLE IF NOT EXISTS kb_data_fts USING fts5(id UNINDEXED, chunk_content);
CREATE VIRTUAL TABLE IF NOT EXISTS skills_fts USING fts5(id UNINDEXED, name, description);

CREATE TRIGGER IF NOT EXISTS kb_data_fts_ai AFTER INSERT ON kb_data BEGIN
	INSERT INTO kb_data_fts(rowid, id, chunk_content) VALUES (new.rowid, new.id, new.chunk_content);
END;

CREATE TRIGGER IF NOT EXISTS kb_data_fts_ad AFTER DELETE ON kb_data BEGIN
	DELETE FROM kb_data_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS kb_data_fts_au AFTER UPDATE ON kb_data BEGIN
	DELETE FROM kb_data_fts WHERE rowid = old.rowid;
	INSERT INTO kb_data_fts(rowid, id, chunk_content) VALUES (new.rowid, new.id, new.chunk_content);
END;

CREATE TRIGGER IF NOT EXISTS skills_fts_ai AFTER INSERT ON skills BEGIN
	INSERT INTO skills_fts(rowid, id, name, description) VALUES (new.rowid, new.id, new.name, new.description);
END;

CREATE TRIGGER IF NOT EXISTS skills_fts_ad AFTER DELETE ON skills BEGIN
	DELETE FROM skills_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS skills_fts_au AFTER UPDATE ON skills BEGIN
	DELETE FROM skills_fts WHERE rowid = old.rowid;
	INSERT INTO skills_fts(rowid, id, name, description) VALUES (new.rowid, new.id, new.name, new.description);
END;

INSERT INTO kb_data_fts(rowid, id, chunk_content)
SELECT kb_data.rowid, kb_data.id, kb_data.chunk_content
FROM kb_data
WHERE NOT EXISTS (SELECT 1 FROM kb_data_fts WHERE kb_data_fts.rowid = kb_data.rowid);

INSERT INTO skills_fts(rowid, id, name, description)
SELECT skills.rowid, skills.id, skills.name, skills.description
FROM skills
WHERE NOT EXISTS (SELECT 1 FROM skills_fts WHERE skills_fts.rowid = skills.rowid);
