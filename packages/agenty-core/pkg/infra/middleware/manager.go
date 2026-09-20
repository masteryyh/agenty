package middleware

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
)

// MiddlewareManager collects middleware hooks once and exposes the immutable
// execution chains used by a session engine.
type MiddlewareManager struct {
	mu          sync.Mutex
	middlewares []Middleware
	compiled    *HookChain
}

func NewMiddlewareManager() *MiddlewareManager {
	return &MiddlewareManager{}
}

func NewManager() *MiddlewareManager {
	return NewMiddlewareManager()
}

func (manager *MiddlewareManager) Register(middleware Middleware) error {
	if middleware.Name == "" {
		return errors.New("middleware manager: middleware name must not be empty")
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.compiled != nil {
		return errors.New("middleware manager: registration is closed after compile")
	}
	for _, existing := range manager.middlewares {
		if existing.Name == middleware.Name {
			return fmt.Errorf("middleware manager: middleware %q is already registered", middleware.Name)
		}
	}
	manager.middlewares = append(manager.middlewares, middleware)
	return nil
}

func (manager *MiddlewareManager) Compile() (*HookChain, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.compiled != nil {
		return manager.compiled, nil
	}

	chain := &HookChain{}
	for _, middleware := range manager.middlewares {
		if middleware.BeforeSessionStart != nil {
			chain.beforeSessionStart = append(chain.beforeSessionStart, namedHook[BeforeSessionStartHook]{
				middleware: middleware.Name,
				hook:       middleware.BeforeSessionStart,
			})
		}
		if middleware.BeforeRound != nil {
			chain.beforeRound = append(chain.beforeRound, namedHook[BeforeRoundHook]{
				middleware: middleware.Name,
				hook:       middleware.BeforeRound,
			})
		}
		if middleware.AfterRound != nil {
			chain.afterRound = append(chain.afterRound, namedHook[AfterRoundHook]{
				middleware: middleware.Name,
				hook:       middleware.AfterRound,
			})
		}
		if middleware.BeforeModelCall != nil {
			chain.beforeModelCall = append(chain.beforeModelCall, namedHook[BeforeModelCallHook]{
				middleware: middleware.Name,
				hook:       middleware.BeforeModelCall,
			})
		}
		if middleware.AfterModelCall != nil {
			chain.afterModelCall = append(chain.afterModelCall, namedHook[AfterModelCallHook]{
				middleware: middleware.Name,
				hook:       middleware.AfterModelCall,
			})
		}
		if middleware.BeforeToolCall != nil {
			chain.beforeToolCall = append(chain.beforeToolCall, namedHook[BeforeToolCallHook]{
				middleware: middleware.Name,
				hook:       middleware.BeforeToolCall,
			})
		}
		if middleware.AfterToolCall != nil {
			chain.afterToolCall = append(chain.afterToolCall, namedHook[AfterToolCallHook]{
				middleware: middleware.Name,
				hook:       middleware.AfterToolCall,
			})
		}
		if middleware.AfterSessionStop != nil {
			chain.afterSessionStop = append(chain.afterSessionStop, namedHook[AfterSessionStopHook]{
				middleware: middleware.Name,
				hook:       middleware.AfterSessionStop,
			})
		}
		if middleware.OnEvent != nil {
			chain.onEvent = append(chain.onEvent, namedHook[OnEventHook]{
				middleware: middleware.Name,
				hook:       middleware.OnEvent,
			})
		}
	}
	manager.compiled = chain
	return chain, nil
}

// Build is an alias for Compile kept as a readable assembly-time operation.
func (manager *MiddlewareManager) Build() (*HookChain, error) {
	return manager.Compile()
}

type namedHook[T any] struct {
	middleware string
	hook       T
}

type HookChain struct {
	beforeSessionStart []namedHook[BeforeSessionStartHook]
	beforeRound        []namedHook[BeforeRoundHook]
	afterRound         []namedHook[AfterRoundHook]
	beforeModelCall    []namedHook[BeforeModelCallHook]
	afterModelCall     []namedHook[AfterModelCallHook]
	beforeToolCall     []namedHook[BeforeToolCallHook]
	afterToolCall      []namedHook[AfterToolCallHook]
	afterSessionStop   []namedHook[AfterSessionStopHook]
	onEvent            []namedHook[OnEventHook]
}

func (chain *HookChain) BeforeSessionStart(ctx context.Context, state *SessionStartContext) error {
	for _, entry := range chain.beforeSessionStart {
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
		if err := entry.hook(ctx, state); err != nil {
			return &HookError{Middleware: entry.middleware, Phase: PhaseBeforeSessionStart, Err: err}
		}
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
	}
	return nil
}

func (chain *HookChain) BeforeRound(ctx context.Context, state *RoundContext) error {
	for _, entry := range chain.beforeRound {
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
		if err := entry.hook(ctx, state); err != nil {
			return &HookError{Middleware: entry.middleware, Phase: PhaseBeforeRound, Err: err}
		}
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
	}
	return nil
}

func (chain *HookChain) AfterRound(ctx context.Context, state *RoundContext) error {
	var errs []error
	for _, entry := range chain.afterRound {
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
		if err := entry.hook(ctx, state); err != nil {
			errs = append(errs, &HookError{Middleware: entry.middleware, Phase: PhaseAfterRound, Err: err})
		}
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
	}
	return errors.Join(errs...)
}

func (chain *HookChain) BeforeModelCall(ctx context.Context, state *ModelCallContext) error {
	for _, entry := range chain.beforeModelCall {
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
		if err := entry.hook(ctx, state); err != nil {
			return &HookError{Middleware: entry.middleware, Phase: PhaseBeforeModelCall, Err: err}
		}
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
	}
	return nil
}

func (chain *HookChain) AfterModelCall(ctx context.Context, state *ModelCallContext) error {
	var errs []error
	for _, entry := range chain.afterModelCall {
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
		if err := entry.hook(ctx, state); err != nil {
			errs = append(errs, &HookError{Middleware: entry.middleware, Phase: PhaseAfterModelCall, Err: err})
		}
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
	}
	return errors.Join(errs...)
}

func (chain *HookChain) BeforeToolCall(ctx context.Context, state *ToolCallContext) error {
	for _, entry := range chain.beforeToolCall {
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
		if err := entry.hook(ctx, state); err != nil {
			return &HookError{Middleware: entry.middleware, Phase: PhaseBeforeToolCall, Err: err}
		}
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
	}
	return nil
}

func (chain *HookChain) AfterToolCall(ctx context.Context, state *ToolCallContext) error {
	var errs []error
	for _, entry := range chain.afterToolCall {
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
		if err := entry.hook(ctx, state); err != nil {
			errs = append(errs, &HookError{Middleware: entry.middleware, Phase: PhaseAfterToolCall, Err: err})
		}
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
	}
	return errors.Join(errs...)
}

func (chain *HookChain) AfterSessionStop(ctx context.Context, state *SessionStopContext) error {
	var errs []error
	for _, entry := range chain.afterSessionStop {
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
		if err := entry.hook(ctx, state); err != nil {
			errs = append(errs, &HookError{Middleware: entry.middleware, Phase: PhaseAfterSessionStop, Err: err})
		}
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
	}
	return errors.Join(errs...)
}

// OnEvent delivers an emitted runtime event in registration order. Unlike
// after hooks, an error stops delivery so an unpersisted event cannot reach
// the CLI notification consumer.
func (chain *HookChain) OnEvent(ctx context.Context, state *EventContext) error {
	for _, entry := range chain.onEvent {
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
		if err := entry.hook(ctx, state); err != nil {
			return &HookError{Middleware: entry.middleware, Phase: PhaseOnEvent, Err: err}
		}
		ctx = hookContext(ctx, state.Context)
		state.Context = ctx
	}
	return nil
}

// AgentLoopHooks adapts the infrastructure hook contexts to the low-level
// callbacks understood by agentloop. The returned value only captures the
// immutable chain; every call receives a fresh context wrapper.
func (chain *HookChain) AgentLoopHooks() agentloop.LoopHooks {
	return agentloop.LoopHooks{
		BeforeModelCall: func(ctx context.Context, state *agentloop.ModelCallState) error {
			middlewareState := &ModelCallContext{
				Context:      hookContext(ctx, state.Context),
				Session:      state.Session,
				Round:        state.Round,
				Iteration:    state.Iteration,
				Request:      state.Request,
				Response:     state.Response,
				Err:          state.Err,
				RequestUsage: state.RequestUsage,
				Emit:         state.Emit,
			}
			err := chain.BeforeModelCall(ctx, middlewareState)
			copyModelCallState(state, middlewareState)
			return err
		},
		AfterModelCall: func(ctx context.Context, state *agentloop.ModelCallState) error {
			middlewareState := &ModelCallContext{
				Context:      hookContext(ctx, state.Context),
				Session:      state.Session,
				Round:        state.Round,
				Iteration:    state.Iteration,
				Request:      state.Request,
				Response:     state.Response,
				Err:          state.Err,
				RequestUsage: state.RequestUsage,
				Emit:         state.Emit,
			}
			err := chain.AfterModelCall(ctx, middlewareState)
			copyModelCallState(state, middlewareState)
			return err
		},
		BeforeToolCall: func(ctx context.Context, state *agentloop.ToolCallState) error {
			middlewareState := &ToolCallContext{
				Context:         hookContext(ctx, state.Context),
				Session:         state.Session,
				SessionSnapshot: state.SessionSnapshot,
				Round:           state.Round,
				Iteration:       state.Iteration,
				Call:            state.Call,
				Tools:           state.Tools,
				Model:           state.Model,
				Result:          state.Result,
				Err:             state.Err,
				Emit:            state.Emit,
			}
			err := chain.BeforeToolCall(ctx, middlewareState)
			copyToolCallState(state, middlewareState)
			return err
		},
		AfterToolCall: func(ctx context.Context, state *agentloop.ToolCallState) error {
			middlewareState := &ToolCallContext{
				Context:         hookContext(ctx, state.Context),
				Session:         state.Session,
				SessionSnapshot: state.SessionSnapshot,
				Round:           state.Round,
				Iteration:       state.Iteration,
				Call:            state.Call,
				Tools:           state.Tools,
				Model:           state.Model,
				Result:          state.Result,
				Err:             state.Err,
				Emit:            state.Emit,
			}
			err := chain.AfterToolCall(ctx, middlewareState)
			copyToolCallState(state, middlewareState)
			return err
		},
	}
}

func (chain *HookChain) LifecycleHooks() LifecycleHooks {
	return LifecycleHooks{
		BeforeSessionStart: func(ctx context.Context, state *SessionStartContext) error {
			if state.Context == nil {
				state.Context = ctx
			}
			return chain.BeforeSessionStart(ctx, state)
		},
		BeforeRound: func(ctx context.Context, state *RoundContext) error {
			if state.Context == nil {
				state.Context = ctx
			}
			return chain.BeforeRound(ctx, state)
		},
		AfterRound: func(ctx context.Context, state *RoundContext) error {
			if state.Context == nil {
				state.Context = ctx
			}
			return chain.AfterRound(ctx, state)
		},
		AfterSessionStop: func(ctx context.Context, state *SessionStopContext) error {
			if state.Context == nil {
				state.Context = ctx
			}
			return chain.AfterSessionStop(ctx, state)
		},
		OnEvent: func(ctx context.Context, state *EventContext) error {
			if state.Context == nil {
				state.Context = ctx
			}
			return chain.OnEvent(ctx, state)
		},
	}
}

func copyModelCallState(state *agentloop.ModelCallState, middlewareState *ModelCallContext) {
	state.Context = middlewareState.Context
	state.Request = middlewareState.Request
	state.Response = middlewareState.Response
	state.Err = middlewareState.Err
	state.RequestUsage = middlewareState.RequestUsage
	state.Emit = middlewareState.Emit
}

func copyToolCallState(state *agentloop.ToolCallState, middlewareState *ToolCallContext) {
	state.Context = middlewareState.Context
	state.Call = middlewareState.Call
	state.Result = middlewareState.Result
	state.Err = middlewareState.Err
	state.Emit = middlewareState.Emit
}

func hookContext(current, candidate context.Context) context.Context {
	if candidate != nil {
		return candidate
	}
	return current
}
