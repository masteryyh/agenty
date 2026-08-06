package catalog

type APIType string

const (
	APIOpenAI            APIType = "openai"
	APIOpenAICompletions APIType = "openai_completions"
	APIAnthropic         APIType = "anthropic"
	APIGemini            APIType = "gemini"
)

func (t APIType) Valid() bool {
	switch t {
	case APIOpenAI, APIOpenAICompletions, APIAnthropic, APIGemini:
		return true
	default:
		return false
	}
}
