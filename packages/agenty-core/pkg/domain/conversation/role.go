package conversation

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

func (r Role) Valid() bool {
	switch r {
	case RoleSystem, RoleUser, RoleAssistant:
		return true
	default:
		return false
	}
}
