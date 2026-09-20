// Package agentloop contains the atomic provider-neutral model/tool loop.
package agentloop

import (
	"context"
	"errors"
	"fmt"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

const maxAgentLoopIterations = 20

type agentLoopConfig struct {
	name string

	request func(context.Context, int) (modelcall.ModelCallRequest, conversation.TokenUsage, error)
	model   modelcall.ModelCallConfig
	invoke  modelcall.InvokeFunc
	emit    EventEmitter
	hooks   LoopHooks
	session *conversation.Session
	round   func() *conversation.Round

	toolRuntime ToolRuntime
	callContext func() CallContext
}

type agentLoopResult struct {
	response *modelcall.ModelCallResponse
	usage    conversation.TokenUsage
}

// AgentLoopOptions describes one atomic model/tool execution. The loop owns
// continuation, tool dispatch, usage accounting, cancellation, and the
// iteration limit; persistence and presentation are supplied as callbacks.
type AgentLoopOptions struct {
	// Name labels the loop in errors and diagnostic messages.
	Name string

	// BuildRequest builds the request for one iteration and reports preparation usage.
	BuildRequest func(context.Context, int) (modelcall.ModelCallRequest, conversation.TokenUsage, error)
	// Model identifies the connection and protocol used for every iteration.
	Model modelcall.ModelCallConfig
	// InvokeModel overrides modelcall.Call for focused tests. Production callers
	// leave it nil and invoke modelcall directly.
	InvokeModel modelcall.InvokeFunc
	// Emit receives model and tool events. Leaving it nil selects non-streaming
	// invocation and omits runtime event delivery.
	Emit EventEmitter

	// ToolRuntime executes the tool calls returned by the model.
	ToolRuntime ToolRuntime
	// CallContext supplies session and working-directory data for a tool batch.
	CallContext func() CallContext
	// Hooks observes and can modify model-call and tool-call state.
	Hooks LoopHooks
	// Session is the session exposed to loop hooks and callbacks.
	Session *conversation.Session
	// Round returns the persisted round currently being executed.
	Round func() *conversation.Round
}

type AgentLoop struct {
	config agentLoopConfig
}

type AgentLoopResult struct {
	Response *modelcall.ModelCallResponse
	Usage    conversation.TokenUsage
}

func NewAgentLoop(options AgentLoopOptions) (*AgentLoop, error) {
	if options.BuildRequest == nil {
		return nil, fmt.Errorf("agent loop request builder is nil")
	}
	invoke := options.InvokeModel
	if invoke == nil {
		invoke = invokeModel
	}

	return &AgentLoop{config: agentLoopConfig{
		name:        options.Name,
		request:     options.BuildRequest,
		model:       options.Model,
		invoke:      invoke,
		emit:        options.Emit,
		toolRuntime: options.ToolRuntime,
		callContext: options.CallContext,
		hooks:       options.Hooks,
		session:     options.Session,
		round:       options.Round,
	}}, nil
}

func (loop *AgentLoop) Run(ctx context.Context) (AgentLoopResult, error) {
	result, err := runAgentLoop(ctx, loop.config)
	return AgentLoopResult{Response: result.response, Usage: result.usage}, err
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
		return agentLoopResult{}, fmt.Errorf("%s model call function is nil", name)
	}

	// Keep model continuation, tool dispatch, and usage accounting in one place.
	var totalUsage conversation.TokenUsage
	loopCtx := ctx
	for iteration := 1; iteration <= maxAgentLoopIterations; iteration++ {
		if err := loopCtx.Err(); err != nil {
			return agentLoopResult{usage: totalUsage}, err
		}

		request, requestUsage, err := config.request(loopCtx, iteration)
		if err != nil {
			return agentLoopResult{usage: totalUsage}, fmt.Errorf(
				"%s iteration %d: build request: %w",
				name,
				iteration,
				err,
			)
		}
		modelCall := &ModelCallState{
			Context:      loopCtx,
			Session:      config.session,
			Iteration:    iteration,
			Request:      &request,
			RequestUsage: requestUsage,
			Emit:         config.emit,
		}
		if config.round != nil {
			modelCall.Round = config.round()
		}
		if config.hooks.BeforeModelCall != nil {
			if err := config.hooks.BeforeModelCall(loopCtx, modelCall); err != nil {
				loopCtx = contextOr(loopCtx, modelCall.Context)
				totalUsage = totalUsage.Add(modelCall.RequestUsage)
				return agentLoopResult{usage: totalUsage}, fmt.Errorf(
					"%s iteration %d: before model call: %w",
					name,
					iteration,
					err,
				)
			}
		}
		loopCtx = contextOr(loopCtx, modelCall.Context)
		modelCall.Context = loopCtx
		if modelCall.Request == nil {
			totalUsage = totalUsage.Add(modelCall.RequestUsage)
			return agentLoopResult{usage: totalUsage}, fmt.Errorf(
				"%s iteration %d: before model call cleared request",
				name,
				iteration,
			)
		}
		var streamHandler modelcall.ModelCallStreamHandler
		if config.emit != nil {
			streamHandler = func(event modelcall.ModelCallStreamEvent) error {
				return config.emit(loopCtx, Event{
					Type:      EventModelStream,
					Iteration: iteration,
					Stream:    &event,
				})
			}
		}
		response, err := config.invoke(loopCtx, config.model, *modelCall.Request, streamHandler)
		modelCall.Response = response
		modelCall.Err = err
		if config.hooks.AfterModelCall != nil {
			if hookErr := config.hooks.AfterModelCall(loopCtx, modelCall); hookErr != nil {
				loopCtx = contextOr(loopCtx, modelCall.Context)
				totalUsage = totalUsage.Add(modelCall.RequestUsage)
				response = modelCall.Response
				if response != nil {
					totalUsage = totalUsage.Add(response.Usage)
				}
				err = errors.Join(modelCall.Err, hookErr)
				return agentLoopResult{usage: totalUsage}, fmt.Errorf(
					"%s iteration %d: after model call: %w",
					name, iteration, err,
				)
			}
		}
		loopCtx = contextOr(loopCtx, modelCall.Context)
		totalUsage = totalUsage.Add(modelCall.RequestUsage)
		response = modelCall.Response
		err = modelCall.Err
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
		if config.emit != nil {
			if err := config.emit(loopCtx, Event{
				Type:      EventAssistantResponse,
				Iteration: iteration,
				Response:  response,
			}); err != nil {
				return agentLoopResult{usage: totalUsage}, err
			}
		}

		if len(calls) == 0 {
			if response.StopReason == modelcall.ModelCallStopReasonError {
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
		mutableCalls := append([]conversation.ToolUseBlock(nil), calls...)
		results := make([]conversation.ToolResultBlock, len(calls))
		executableCalls := make([]conversation.ToolUseBlock, 0, len(calls))
		executableIndexes := make([]int, 0, len(calls))
		for callIndex := range mutableCalls {
			if err := loopCtx.Err(); err != nil {
				return agentLoopResult{usage: totalUsage}, err
			}
			toolCall := &ToolCallState{
				Context:   loopCtx,
				Session:   config.session,
				Iteration: iteration,
				Call:      &mutableCalls[callIndex],
				Tools:     config.toolRuntime,
				Model:     config.model,
				Emit:      config.emit,
			}
			if config.round != nil {
				toolCall.Round = config.round()
			}
			if config.hooks.BeforeToolCall != nil {
				if err := config.hooks.BeforeToolCall(loopCtx, toolCall); err != nil {
					loopCtx = contextOr(loopCtx, toolCall.Context)
					return agentLoopResult{usage: totalUsage}, fmt.Errorf(
						"%s iteration %d tool %d: before tool call: %w",
						name,
						iteration,
						callIndex,
						err,
					)
				}
			}
			loopCtx = contextOr(loopCtx, toolCall.Context)
			toolCall.Context = loopCtx
			if toolCall.Call == nil {
				return agentLoopResult{usage: totalUsage}, fmt.Errorf(
					"%s iteration %d tool %d: before tool call cleared call",
					name,
					iteration,
					callIndex,
				)
			}
			mutableCalls[callIndex] = *toolCall.Call
			if toolCall.Result != nil {
				results[callIndex] = *toolCall.Result
				results[callIndex].ToolUseID = calls[callIndex].ID
			} else {
				executableCalls = append(executableCalls, *toolCall.Call)
				executableIndexes = append(executableIndexes, callIndex)
			}
		}
		if err := loopCtx.Err(); err != nil {
			return agentLoopResult{usage: totalUsage}, err
		}
		if len(executableCalls) > 0 {
			executed := config.toolRuntime.ExecuteBatch(loopCtx, callContext, executableCalls)
			for index, callIndex := range executableIndexes {
				if index < len(executed) {
					results[callIndex] = executed[index]
				} else {
					results[callIndex] = conversation.ToolResultBlock{
						ToolUseID: mutableCalls[callIndex].ID,
						IsError:   true,
						Content:   conversation.Text("Tool returned no result."),
					}
				}
			}
		}
		markNativeShellResults(response.Content, results)
		if config.hooks.AfterToolCall != nil {
			for callIndex := range mutableCalls {
				toolCall := &ToolCallState{
					Context:   loopCtx,
					Session:   config.session,
					Iteration: iteration,
					Call:      &mutableCalls[callIndex],
					Tools:     config.toolRuntime,
					Emit:      config.emit,
				}
				if config.round != nil {
					toolCall.Round = config.round()
				}
				if callIndex < len(results) {
					toolCall.Result = &results[callIndex]
				}
				if err := config.hooks.AfterToolCall(loopCtx, toolCall); err != nil {
					loopCtx = contextOr(loopCtx, toolCall.Context)
					return agentLoopResult{usage: totalUsage}, fmt.Errorf(
						"%s iteration %d tool %d: after tool call: %w",
						name,
						iteration,
						callIndex,
						err,
					)
				}
				loopCtx = contextOr(loopCtx, toolCall.Context)
				if toolCall.Result != nil {
					results[callIndex] = *toolCall.Result
				}
				if toolCall.Call != nil {
					mutableCalls[callIndex] = *toolCall.Call
				}
			}
		}
		if err := loopCtx.Err(); err != nil {
			return agentLoopResult{usage: totalUsage}, err
		}

		if config.emit != nil {
			content := make(conversation.Content, 0, len(results))
			for _, result := range results {
				content = append(content, result)
			}
			if err := config.emit(loopCtx, Event{
				Type:        EventToolResults,
				Iteration:   iteration,
				ToolResults: content,
			}); err != nil {
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

func invokeModel(
	ctx context.Context,
	model modelcall.ModelCallConfig,
	request modelcall.ModelCallRequest,
	handler modelcall.ModelCallStreamHandler,
) (*modelcall.ModelCallResponse, error) {
	if handler == nil {
		return modelcall.Call(ctx, model, request)
	}

	return modelcall.Call(ctx, model, request, modelcall.WithStreamHandler(handler))
}
