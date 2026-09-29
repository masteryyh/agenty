package conversation

// ToolDialect selects the built-in file-tool contract for a session.
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
