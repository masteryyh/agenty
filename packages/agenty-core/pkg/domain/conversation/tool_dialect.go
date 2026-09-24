package conversation

// ToolDialect selects the built-in file-tool contract for a session.
// Codex mode is intentionally one-way: start a new session to return to the
// default editor contract.
type ToolDialect string

const (
	ToolDialectDefault ToolDialect = "default"
	ToolDialectCodex   ToolDialect = "codex"
)

func (dialect ToolDialect) Valid() bool {
	switch dialect {
	case ToolDialectDefault, ToolDialectCodex:
		return true
	default:
		return false
	}
}

func (dialect ToolDialect) Normalized() ToolDialect {
	if dialect == ToolDialectCodex {
		return ToolDialectCodex
	}
	return ToolDialectDefault
}
