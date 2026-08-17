package agentloop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/application/apperrors"
	"github.com/masteryyh/agenty-core/pkg/domain/agent"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

const (
	DefaultMaxOutputTokens int64 = 65_536

	maxAgentLoopIterations = 20
)

type ExecutionSessionRepository interface {
	Load(ctx context.Context, id uuid.UUID) (*conversation.Session, error)
	Save(ctx context.Context, session *conversation.Session) error
}

type ExecutionAgentRepository interface {
	Get(ctx context.Context, slug shared.Slug) (*agent.Agent, error)
}

type ExecutionCatalogRepository interface {
	Get(ctx context.Context, slug shared.Slug) (*catalog.Provider, error)
}

type CallerFactory func(
	ctx context.Context,
	provider catalog.Provider,
	model catalog.Model,
) (Caller, error)

type Dependencies struct {
	Sessions  ExecutionSessionRepository
	Agents    ExecutionAgentRepository
	Catalog   ExecutionCatalogRepository
	Tools     ToolRuntime
	NewCaller CallerFactory
	Events    SessionEventHandler
}

type StartResult struct {
	SessionID uuid.UUID                `json:"sessionId"`
	RoundID   uuid.UUID                `json:"roundId"`
	Status    conversation.RoundStatus `json:"status"`
}

type StopResult struct {
	SessionID     uuid.UUID `json:"sessionId"`
	RoundID       uuid.UUID `json:"roundId"`
	StopRequested bool      `json:"stopRequested"`
}

type activeExecution struct {
	roundID uuid.UUID
	cancel  context.CancelFunc
}

type Engine struct {
	ctx       context.Context
	cancel    context.CancelFunc
	sessions  ExecutionSessionRepository
	agents    ExecutionAgentRepository
	catalog   ExecutionCatalogRepository
	tools     ToolRuntime
	newCaller CallerFactory
	events    SessionEventHandler
	logger    *slog.Logger
	mu        sync.Mutex
	active    map[uuid.UUID]*activeExecution
	waitGroup sync.WaitGroup
	shutdown  bool
	stopOnce  sync.Once
	stopped   chan struct{}
}

func NewEngine(parentCtx context.Context, dependencies Dependencies) (*Engine, error) {
	if parentCtx == nil {
		return nil, apperrors.Validation("execution parent context must not be nil")
	}
	if dependencies.Sessions == nil {
		return nil, apperrors.Validation("execution session repository must not be nil")
	}
	if dependencies.Agents == nil {
		return nil, apperrors.Validation("execution agent repository must not be nil")
	}
	if dependencies.Catalog == nil {
		return nil, apperrors.Validation("execution catalog repository must not be nil")
	}
	if dependencies.Tools == nil {
		return nil, apperrors.Validation("tool registry must not be nil")
	}
	if dependencies.NewCaller == nil {
		return nil, apperrors.Validation("LLM caller factory must not be nil")
	}

	ctx, cancel := context.WithCancel(parentCtx)
	return &Engine{
		ctx:       ctx,
		cancel:    cancel,
		sessions:  dependencies.Sessions,
		agents:    dependencies.Agents,
		catalog:   dependencies.Catalog,
		tools:     dependencies.Tools,
		newCaller: dependencies.NewCaller,
		events:    dependencies.Events,
		logger:    slog.Default(),
		active:    make(map[uuid.UUID]*activeExecution),
		stopped:   make(chan struct{}),
	}, nil
}

func (engine *Engine) Start(
	ctx context.Context,
	sessionID string,
	content conversation.Content,
) (*StartResult, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, apperrors.Validation("invalid session id: " + err.Error())
	}
	if len(content) == 0 {
		return nil, apperrors.Validation("session content must not be empty")
	}

	runCtx, execution, err := engine.reserve(id)
	if err != nil {
		return nil, err
	}
	launched := false
	defer func() {
		if !launched {
			engine.release(id, execution)
		}
	}()

	prepared, err := engine.prepare(ctx, runCtx, id, content)
	if err != nil {
		return nil, err
	}

	engine.mu.Lock()
	execution.roundID = prepared.roundID
	engine.mu.Unlock()

	launched = true
	go engine.run(runCtx, id, execution, prepared)

	return &StartResult{
		SessionID: id,
		RoundID:   prepared.roundID,
		Status:    conversation.RoundRunning,
	}, nil
}

func (engine *Engine) Stop(_ context.Context, sessionID string) (*StopResult, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, apperrors.Validation("invalid session id: " + err.Error())
	}

	engine.mu.Lock()
	execution, ok := engine.active[id]
	var roundID uuid.UUID
	if ok {
		roundID = execution.roundID
		execution.cancel()
	}
	engine.mu.Unlock()
	if !ok {
		return nil, apperrors.NotFound("session " + sessionID + " is not running")
	}

	return &StopResult{
		SessionID:     id,
		RoundID:       roundID,
		StopRequested: true,
	}, nil
}

func (engine *Engine) IsRunning(sessionID uuid.UUID) bool {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	_, ok := engine.active[sessionID]
	return ok
}

func (engine *Engine) ExecuteSessionIfIdle(
	sessionID uuid.UUID,
	execute func() error,
) (bool, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	if _, running := engine.active[sessionID]; running {
		return false, nil
	}

	return true, execute()
}

func (engine *Engine) Shutdown(ctx context.Context) error {
	engine.stopOnce.Do(func() {
		engine.mu.Lock()
		engine.shutdown = true
		engine.cancel()
		for _, execution := range engine.active {
			execution.cancel()
		}
		engine.mu.Unlock()

		go func() {
			engine.waitGroup.Wait()
			close(engine.stopped)
		}()
	})

	select {
	case <-engine.stopped:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("execution: wait for shutdown: %w", ctx.Err())
	}
}

func (engine *Engine) reserve(
	sessionID uuid.UUID,
) (context.Context, *activeExecution, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	if engine.shutdown || engine.ctx.Err() != nil {
		return nil, nil, apperrors.Internal("execution engine is shutting down")
	}
	if _, exists := engine.active[sessionID]; exists {
		return nil, nil, apperrors.AlreadyExists("session " + sessionID.String() + " is already running")
	}

	runCtx, cancel := context.WithCancel(engine.ctx)
	execution := &activeExecution{cancel: cancel}
	engine.active[sessionID] = execution
	engine.waitGroup.Add(1)

	return runCtx, execution, nil
}

type preparedExecution struct {
	session         *conversation.Session
	roundID         uuid.UUID
	model           catalog.Model
	caller          Caller
	systemPrompt    string
	maxOutputTokens int64
	userMessage     conversation.Message
	eventSequence   uint64
}

func (engine *Engine) prepare(
	ctx context.Context,
	runCtx context.Context,
	sessionID uuid.UUID,
	content conversation.Content,
) (*preparedExecution, error) {
	session, err := engine.sessions.Load(ctx, sessionID)
	if err != nil {
		if errors.Is(err, conversation.ErrSessionNotFound) {
			return nil, apperrors.NotFound("session " + sessionID.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load session", err)
	}

	agentDefinition, err := engine.agents.Get(ctx, session.AgentSlug)
	if err != nil {
		if errors.Is(err, agent.ErrNotFound) {
			return nil, apperrors.NotFound("agent " + session.AgentSlug.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load agent", err)
	}
	if session.CurrentModel == nil || session.CurrentModel.IsZero() {
		return nil, apperrors.Validation("session model is not configured")
	}

	provider, err := engine.catalog.Get(ctx, session.CurrentModel.ProviderSlug)
	if err != nil {
		if errors.Is(err, catalog.ErrProviderNotFound) {
			return nil, apperrors.NotFound("provider " + session.CurrentModel.ProviderSlug.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load provider", err)
	}
	model, err := provider.Model(session.CurrentModel.ModelSlug)
	if err != nil {
		if errors.Is(err, catalog.ErrModelNotFound) {
			return nil, apperrors.NotFound("model " + session.CurrentModel.ModelSlug.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load model", err)
	}
	if model.MaxOutputTokens <= 0 {
		return nil, apperrors.Validation("model " + model.Slug.String() + " has invalid max output tokens")
	}

	systemPrompt, err := agentDefinition.ResolveSystemPrompt()
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to resolve system prompt", err)
	}
	caller, err := engine.newCaller(runCtx, *provider, *model)
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to create LLM caller", err)
	}
	if err := runCtx.Err(); err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "execution was cancelled before start", err)
	}

	roundID, err := session.StartRound()
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to start round", err)
	}
	round := session.Rounds[len(session.Rounds)-1]

	metadata, err := metadataForRound(session, round)
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to build session metadata", err)
	}
	if len(metadata) > 0 {
		if _, err := session.AppendHiddenUserMessage(roundID, metadata); err != nil {
			return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to append session metadata", err)
		}
	}

	userMessage, err := session.AppendUserMessage(roundID, content)
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to append user message", err)
	}
	if err := engine.sessions.Save(ctx, session); err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to save started round", err)
	}
	session.ClearPending()

	return &preparedExecution{
		session:         session,
		roundID:         roundID,
		model:           *model,
		caller:          caller,
		systemPrompt:    systemPrompt,
		maxOutputTokens: min(DefaultMaxOutputTokens, model.MaxOutputTokens),
		userMessage:     userMessage,
	}, nil
}

func (engine *Engine) run(
	ctx context.Context,
	sessionID uuid.UUID,
	execution *activeExecution,
	prepared *preparedExecution,
) {
	defer engine.release(sessionID, execution)

	status := conversation.RoundCompleted
	usage := conversation.TokenUsage{}
	var runErr error

	if err := engine.emit(ctx, prepared, SessionEvent{Type: SessionEventRoundStarted}); err != nil {
		runErr = err
		status = conversation.RoundFailed
		engine.finish(ctx, prepared, status, usage, runErr)
		return
	}
	if err := engine.emit(ctx, prepared, SessionEvent{
		Type:    SessionEventMessageAppended,
		Message: &prepared.userMessage,
	}); err != nil {
		runErr = err
		status = conversation.RoundFailed
		engine.finish(ctx, prepared, status, usage, runErr)
		return
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			status = conversation.RoundFailed
			runErr = fmt.Errorf("agent loop panicked: %v", recovered)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			status = conversation.RoundCancelled
			runErr = nil
		}
		engine.finish(ctx, prepared, status, usage, runErr)
	}()

	usage, runErr = engine.executeLoop(ctx, prepared)
	if runErr != nil {
		status = conversation.RoundFailed
	}
}

func (engine *Engine) executeLoop(
	ctx context.Context,
	prepared *preparedExecution,
) (conversation.TokenUsage, error) {
	totalUsage := conversation.TokenUsage{}

	for iteration := 1; iteration <= maxAgentLoopIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			return totalUsage, err
		}

		request := Request{
			SystemPrompt:    prepared.systemPrompt,
			Messages:        sessionMessages(prepared.session),
			Tools:           engine.tools.Definitions(),
			MaxOutputTokens: prepared.maxOutputTokens,
			ReasoningEffort: prepared.session.Rounds[len(prepared.session.Rounds)-1].ReasoningEffort,
		}
		response, err := engine.call(ctx, prepared, iteration, request)
		if err != nil {
			return totalUsage, fmt.Errorf("invoke LLM at iteration %d: %w", iteration, err)
		}
		if response == nil {
			return totalUsage, fmt.Errorf("invoke LLM at iteration %d: empty response", iteration)
		}

		totalUsage = totalUsage.Add(response.Usage)
		message, err := prepared.session.AppendAssistantMessage(
			prepared.roundID,
			response.Content,
			prepared.session.Rounds[len(prepared.session.Rounds)-1].Model,
			&response.Usage,
		)
		if err != nil {
			return totalUsage, fmt.Errorf("append assistant message at iteration %d: %w", iteration, err)
		}
		if err := engine.saveProgress(ctx, prepared.session); err != nil {
			return totalUsage, fmt.Errorf("save assistant message at iteration %d: %w", iteration, err)
		}
		if err := engine.emit(ctx, prepared, SessionEvent{
			Type:      SessionEventMessageAppended,
			Iteration: iteration,
			Message:   &message,
		}); err != nil {
			return totalUsage, fmt.Errorf("emit assistant message at iteration %d: %w", iteration, err)
		}

		toolCalls := toolCalls(response.Content)
		if len(toolCalls) == 0 {
			if response.StopReason == StopReasonError {
				return totalUsage, fmt.Errorf("LLM stopped with an error")
			}
			return totalUsage, nil
		}

		cwd := ""
		round := prepared.session.Rounds[len(prepared.session.Rounds)-1]
		if round.Cwd != nil {
			cwd = *round.Cwd
		}
		results := engine.tools.ExecuteBatch(ctx, CallContext{
			SessionID: prepared.session.ID,
			RoundID:   prepared.roundID,
			Cwd:       cwd,
		}, toolCalls)
		if err := ctx.Err(); err != nil {
			return totalUsage, err
		}

		content := make(conversation.Content, 0, len(results))
		for _, result := range results {
			content = append(content, result)
		}
		message, err = prepared.session.AppendUserMessage(prepared.roundID, content)
		if err != nil {
			return totalUsage, fmt.Errorf("append tool results at iteration %d: %w", iteration, err)
		}
		if err := engine.saveProgress(ctx, prepared.session); err != nil {
			return totalUsage, fmt.Errorf("save tool results at iteration %d: %w", iteration, err)
		}
		if err := engine.emit(ctx, prepared, SessionEvent{
			Type:      SessionEventMessageAppended,
			Iteration: iteration,
			Message:   &message,
		}); err != nil {
			return totalUsage, fmt.Errorf("emit tool results at iteration %d: %w", iteration, err)
		}
	}

	return totalUsage, fmt.Errorf("agent loop exceeded %d iterations", maxAgentLoopIterations)
}

func (engine *Engine) call(
	ctx context.Context,
	prepared *preparedExecution,
	iteration int,
	request Request,
) (*Response, error) {
	if engine.events == nil {
		return prepared.caller.Invoke(ctx, request)
	}

	return prepared.caller.Stream(ctx, request, func(stream StreamEvent) error {
		return engine.emit(ctx, prepared, SessionEvent{
			Type:      SessionEventModelStream,
			Iteration: iteration,
			Stream:    &stream,
		})
	})
}

func (engine *Engine) emit(
	ctx context.Context,
	prepared *preparedExecution,
	event SessionEvent,
) error {
	if engine.events == nil {
		return nil
	}
	prepared.eventSequence++
	event.SessionID = prepared.session.ID
	event.RoundID = prepared.roundID
	event.Sequence = prepared.eventSequence
	return engine.events(ctx, event)
}

func (engine *Engine) saveProgress(
	ctx context.Context,
	session *conversation.Session,
) error {
	if err := engine.sessions.Save(ctx, session); err != nil {
		return err
	}
	session.ClearPending()
	return nil
}

func (engine *Engine) finish(
	ctx context.Context,
	prepared *preparedExecution,
	status conversation.RoundStatus,
	usage conversation.TokenUsage,
	runErr error,
) {
	var errorMessage *string
	if runErr != nil {
		message := runErr.Error()
		errorMessage = &message
	}
	if err := prepared.session.CompleteRound(prepared.roundID, status, usage, errorMessage); err != nil {
		engine.logger.ErrorContext(context.WithoutCancel(ctx), "failed to complete agent round",
			"sessionId", prepared.session.ID,
			"roundId", prepared.roundID,
			"error", err,
		)
		return
	}

	finishCtx := context.WithoutCancel(ctx)
	if err := engine.sessions.Save(finishCtx, prepared.session); err != nil {
		engine.logger.ErrorContext(finishCtx, "failed to save completed agent round",
			"sessionId", prepared.session.ID,
			"roundId", prepared.roundID,
			"status", status,
			"error", err,
		)
		return
	}
	prepared.session.ClearPending()
	if err := engine.emit(finishCtx, prepared, SessionEvent{
		Type:   SessionEventRoundEnded,
		Status: status,
		Usage:  &usage,
		Error:  errorMessage,
	}); err != nil {
		engine.logger.ErrorContext(finishCtx, "failed to emit completed agent round",
			"sessionId", prepared.session.ID,
			"roundId", prepared.roundID,
			"status", status,
			"error", err,
		)
	}
}

func (engine *Engine) release(
	sessionID uuid.UUID,
	execution *activeExecution,
) {
	execution.cancel()

	engine.mu.Lock()
	if engine.active[sessionID] == execution {
		delete(engine.active, sessionID)
	}
	engine.mu.Unlock()

	engine.waitGroup.Done()
}

func sessionMessages(session *conversation.Session) []conversation.Message {
	count := 0
	for _, round := range session.Rounds {
		count += len(round.Messages)
	}

	messages := make([]conversation.Message, 0, count)
	for _, round := range session.Rounds {
		messages = append(messages, round.Messages...)
	}

	return messages
}

func toolCalls(content conversation.Content) []conversation.ToolUseBlock {
	calls := make([]conversation.ToolUseBlock, 0)
	for _, block := range content {
		call, ok := block.(conversation.ToolUseBlock)
		if ok {
			calls = append(calls, call)
		}
	}

	return calls
}
