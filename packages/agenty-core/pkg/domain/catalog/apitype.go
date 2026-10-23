package catalog

type APIType string

const (
	APIOpenAI            APIType = "openai"
	APIOpenAICompletions APIType = "openai_completions"
	APIAnthropic         APIType = "anthropic"
	APIGemini            APIType = "gemini"
	APILocal             APIType = "local"
)

func (t APIType) Valid() bool {
	switch t {
	case APIOpenAI, APIOpenAICompletions, APIAnthropic, APIGemini, APILocal:
		return true
	default:
		return false
	}
}
