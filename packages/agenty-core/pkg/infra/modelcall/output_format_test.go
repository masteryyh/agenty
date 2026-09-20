package modelcall

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type formatTransport func(*http.Request) (*http.Response, error)

func (transport formatTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func formatTestResponse(api APIType) string {
	switch api {
	case APIOpenAI:
		return `{"id":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{}","annotations":[]}]}]}`
	case APIOpenAICompletions:
		return `{"id":"completion","choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`
	case APIAnthropic:
		return `{"id":"message","type":"message","role":"assistant","content":[{"type":"text","text":"{}"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	default:
		return `{"candidates":[{"content":{"role":"model","parts":[{"text":"{}"}]},"finishReason":"STOP"}]}`
	}
}

func TestStructuredOutputReachesEveryProtocol(t *testing.T) {
	for _, api := range []APIType{APIOpenAI, APIOpenAICompletions, APIAnthropic, APIGemini} {
		for _, structured := range []bool{true, false} {
			name := string(api)
			if structured {
				name += "/structured"
			} else {
				name += "/ordinary"
			}
			t.Run(name, func(t *testing.T) {
				calls := 0
				client := &http.Client{Transport: formatTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Fatal(err)
					}
					var request map[string]any
					if err := json.Unmarshal(body, &request); err != nil {
						t.Fatal(err)
					}
					var schema any
					switch api {
					case APIOpenAI:
						text, _ := request["text"].(map[string]any)
						format, _ := text["format"].(map[string]any)
						schema = format["schema"]
						if structured && (format["type"] != "json_schema" || format["strict"] != true || format["name"] != "review") {
							t.Errorf("format = %v", format)
						}
						reasoning, _ := request["reasoning"].(map[string]any)
						if reasoning["effort"] != "low" {
							t.Errorf("reasoning lost: %v", request)
						}
					case APIOpenAICompletions:
						format, _ := request["response_format"].(map[string]any)
						jsonFormat, _ := format["json_schema"].(map[string]any)
						schema = jsonFormat["schema"]
						if structured && (format["type"] != "json_schema" || jsonFormat["strict"] != true) {
							t.Errorf("format = %v", format)
						}
					case APIAnthropic:
						config, _ := request["output_config"].(map[string]any)
						format, _ := config["format"].(map[string]any)
						schema = format["schema"]
						if config["effort"] != "low" {
							t.Errorf("effort lost: %v", config)
						}
						if structured && format["type"] != "json_schema" {
							t.Errorf("format = %v", format)
						}
					case APIGemini:
						config, _ := request["generationConfig"].(map[string]any)
						schema = config["responseJsonSchema"]
						if structured && config["responseMimeType"] != "application/json" {
							t.Errorf("config = %v", config)
						}
					}
					if structured {
						object, ok := schema.(map[string]any)
						if !ok || object["type"] != "object" || object["additionalProperties"] != false {
							t.Errorf("schema = %v", schema)
						}
						properties, _ := object["properties"].(map[string]any)
						message, _ := properties["message"].(map[string]any)
						nullable, _ := message["anyOf"].([]any)
						if len(nullable) != 2 {
							t.Errorf("nullable schema lost: %v", message)
						}
					} else if schema != nil {
						t.Errorf("unsolicited output schema: %v", schema)
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(formatTestResponse(api))), Request: r}, nil
				})}
				request := ModelCallRequest{
					Messages:        []ModelCallMessage{{Role: conversation.RoleUser, Content: conversation.Text("review")}},
					MaxOutputTokens: 512, ReasoningEffort: shared.ReasoningLow,
				}
				if structured {
					request.OutputFormat = &OutputFormat{Name: "review", Schema: JSONSchema{
						Type:       JSONSchemaTypeObject,
						Properties: map[string]JSONSchema{"message": {AnyOf: []JSONSchema{{Type: JSONSchemaTypeString}, {Type: JSONSchemaTypeNull}}}},
						Required:   []string{"message"}, AdditionalProperties: AllowAdditionalProperties(false),
					}}
				}
				model := ModelCallConfig{APIType: api, APIKey: "test", ModelCode: "test", SupportsReasoning: true, ReasoningEfforts: []shared.ReasoningEffort{shared.ReasoningLow}}
				_, err := Call(t.Context(), model, request, WithHTTPClient(client), WithMaxRetries(0))
				if err != nil || calls != 1 {
					t.Fatalf("calls = %d, error = %v", calls, err)
				}
			})
		}
	}
}

func TestHTTPRetryOptionForEveryProtocol(t *testing.T) {
	for _, api := range []APIType{APIOpenAI, APIOpenAICompletions, APIAnthropic, APIGemini} {
		for _, status := range []int{503, 401, 400} {
			t.Run(string(api)+"/"+http.StatusText(status), func(t *testing.T) {
				calls := 0
				client := &http.Client{Transport: formatTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					code, body := status, `{"error":{"message":"failed"}}`
					if calls == 2 {
						code, body = 200, formatTestResponse(api)
					}
					return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}, "Retry-After": []string{"0.001"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})}
				_, err := Call(t.Context(), ModelCallConfig{APIType: api, APIKey: "test", ModelCode: "test"}, ModelCallRequest{MaxOutputTokens: 512}, WithHTTPClient(client), WithMaxRetries(1))
				if status == 503 {
					if err != nil || calls != 2 {
						t.Fatalf("calls = %d, error = %v", calls, err)
					}
				} else if err == nil || calls != 1 {
					t.Fatalf("non-retryable calls = %d, error = %v", calls, err)
				}
			})
		}
	}
}

func TestOutputAndRetryValidationBeforeNetwork(t *testing.T) {
	model := ModelCallConfig{APIType: APIOpenAI, APIKey: "test", ModelCode: "test"}
	for _, format := range []OutputFormat{
		{Schema: JSONSchema{Type: JSONSchemaTypeObject}},
		{Name: "bad name", Schema: JSONSchema{Type: JSONSchemaTypeObject}},
		{Name: strings.Repeat("a", 65), Schema: JSONSchema{Type: JSONSchemaTypeObject}},
		{Name: "valid", Schema: JSONSchema{Type: JSONSchemaTypeArray}},
		{Name: "valid", Schema: JSONSchema{Type: JSONSchemaTypeObject, AnyOf: []JSONSchema{{Type: JSONSchemaTypeObject}}}},
	} {
		_, err := Call(context.Background(), model, ModelCallRequest{MaxOutputTokens: 512, OutputFormat: &format})
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("format %#v: %v", format, err)
		}
	}
	for _, retries := range []int{-1, math.MaxInt32} {
		_, err := Call(t.Context(), model, ModelCallRequest{MaxOutputTokens: 512}, WithMaxRetries(retries))
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("retries %d: %v", retries, err)
		}
	}
}
