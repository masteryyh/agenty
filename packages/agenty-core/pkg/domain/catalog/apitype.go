package catalog

type APIType string

const (
	APIOpenAI       APIType = "openai"
	APIOpenAILegacy APIType = "openai_legacy"
	APIAnthropic    APIType = "anthropic"
	APIGemini       APIType = "gemini"
	APIDeepSeek     APIType = "deepseek"
	APIKimi         APIType = "kimi"
	APIQwen         APIType = "qwen"
	APIBigModel     APIType = "bigmodel"
)

func (t APIType) Valid() bool {
	switch t {
	case APIOpenAI, APIOpenAILegacy, APIAnthropic, APIGemini, APIDeepSeek, APIKimi, APIQwen, APIBigModel:
		return true
	default:
		return false
	}
}
