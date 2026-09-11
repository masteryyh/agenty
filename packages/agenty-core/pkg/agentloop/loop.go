package agentloop

import (
	"context"
	"fmt"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

type agentLoopConfig struct {
	name string

	request func(context.Context, int) (Request, conversation.TokenUsage, error)
	invoke  func(context.Context, int, Request) (*Response, error)

	toolRuntime ToolRuntime
	callContext func() CallContext

	appendAssistant   func(context.Context, int, *Response, bool) error
	appendToolResults func(context.Context, int, conversation.Content) error
	afterResponse     func(*Response)
}

type agentLoopResult struct {
	response *Response
	usage    conversation.TokenUsage
}

func runAgentLoop(
	ctx context.Context,
	config agentLoopConfig,
) (agentLoopResult, error) {
	name := config.name
	if name == "" {
		name = "agent loop"
	}
	if config.request == nil {
		return agentLoopResult{}, fmt.Errorf("%s request builder is nil", name)
	}
	if config.invoke == nil {
		return agentLoopResult{}, fmt.Errorf("%s invoker is nil", name)
	}

	// Keep model continuation, tool dispatch, and usage accounting in one place.
	var totalUsage conversation.TokenUsage
	for iteration := 1; iteration <= maxAgentLoopIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			return agentLoopResult{usage: totalUsage}, err
		}

		request, requestUsage, err := config.request(ctx, iteration)
		if err != nil {
			return agentLoopResult{usage: totalUsage}, fmt.Errorf(
				"%s iteration %d: build request: %w",
				name,
				iteration,
				err,
			)
		}
		totalUsage = totalUsage.Add(requestUsage)

		response, err := config.invoke(ctx, iteration, request)
		if err != nil {
			return agentLoopResult{usage: totalUsage}, fmt.Errorf(
				"%s iteration %d: invoke model: %w",
				name,
				iteration,
				err,
			)
		}
		if response == nil {
			return agentLoopResult{usage: totalUsage}, fmt.Errorf(
				"%s iteration %d: empty response",
				name,
				iteration,
			)
		}

		totalUsage = totalUsage.Add(response.Usage)
		calls := toolCalls(response.Content)
		if config.afterResponse != nil {
			config.afterResponse(response)
		}
		if config.appendAssistant != nil {
			if err := config.appendAssistant(ctx, iteration, response, len(calls) > 0); err != nil {
				return agentLoopResult{usage: totalUsage}, err
			}
		}

		if len(calls) == 0 {
			if response.StopReason == StopReasonError {
				return agentLoopResult{usage: totalUsage}, fmt.Errorf(
					"%s iteration %d: model stopped with an error",
					name,
					iteration,
				)
			}
			response.Usage = totalUsage
			return agentLoopResult{response: response, usage: totalUsage}, nil
		}

		if config.toolRuntime == nil {
			return agentLoopResult{usage: totalUsage}, fmt.Errorf(
				"%s iteration %d: tool runtime is nil",
				name,
				iteration,
			)
		}
		callContext := CallContext{}
		if config.callContext != nil {
			callContext = config.callContext()
		}
		results := config.toolRuntime.ExecuteBatch(ctx, callContext, calls)
		markNativeShellResults(response.Content, results)
		if err := ctx.Err(); err != nil {
			return agentLoopResult{usage: totalUsage}, err
		}

		if config.appendToolResults != nil {
			content := make(conversation.Content, 0, len(results))
			for _, result := range results {
				content = append(content, result)
			}
			if err := config.appendToolResults(ctx, iteration, content); err != nil {
				return agentLoopResult{usage: totalUsage}, err
			}
		}
	}

	return agentLoopResult{usage: totalUsage}, fmt.Errorf(
		"%s exceeded %d iterations",
		name,
		maxAgentLoopIterations,
	)
}
