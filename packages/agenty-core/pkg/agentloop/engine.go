package agentloop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/application/apperrors"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	domainskill "github.com/masteryyh/agenty-core/pkg/domain/skill"
)

const (
	DefaultMaxOutputTokens int64 = catalog.DefaultMaxOutputTokens

	maxAgentLoopIterations = 20
)

type ExecutionSessionRepository interface {
	Load(ctx context.Context, id uuid.UUID) (*conversation.Session, error)
	Save(ctx context.Context, session *conversation.Session) error
}

type ExecutionCatalogRepository interface {
	Get(ctx context.Context, code shared.Code) (*catalog.Provider, error)
}

type CallerFactory func(
	ctx context.Context,
	provider catalog.Provider,
	model catalog.Model,
) (Caller, error)

type SkillRegistry interface {
	PromptSection() string
	ResolveReferences(text string) ([]domainskill.Resolved, error)
}

type Dependencies struct {
	Sessions    ExecutionSessionRepository
	Catalog     ExecutionCatalogRepository
	Tools       ToolRuntime
	NewCaller   CallerFactory
	Events      SessionEventHandler
	Compactions CompactionEventHandler
	Skills      SkillRegistry
}

type StartResult struct {
	SessionID uuid.UUID                `json:"sessionId"`
	RoundID   uuid.UUID                `json:"roundId"`
	Status    conversation.RoundStatus `json:"status"`
}

type CompactResult struct {
	SessionID           uuid.UUID                      `json:"sessionId"`
	CompactionID        uuid.UUID                      `json:"compactionId"`
	Trigger             conversation.CompactionTrigger `json:"trigger"`
	ContextTokensBefore int64                          `json:"contextTokensBefore"`
	ContextTokensAfter  int64                          `json:"contextTokensAfter"`
	Usage               conversation.TokenUsage        `json:"usage"`
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
	ctx         context.Context
	cancel      context.CancelFunc
	sessions    ExecutionSessionRepository
	catalog     ExecutionCatalogRepository
	tools       ToolRuntime
	newCaller   CallerFactory
	events      SessionEventHandler
	compactions CompactionEventHandler
	skills      SkillRegistry
	logger      *slog.Logger
	mu          sync.Mutex
	active      map[uuid.UUID]*activeExecution
	waitGroup   sync.WaitGroup
	shutdown    bool
	stopOnce    sync.Once
	stopped     chan struct{}
}

func NewEngine(parentCtx context.Context, dependencies Dependencies) (*Engine, error) {
	if parentCtx == nil {
		return nil, apperrors.Validation("execution parent context must not be nil")
	}
	if dependencies.Sessions == nil {
		return nil, apperrors.Validation("execution session repository must not be nil")
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
		ctx:         ctx,
		cancel:      cancel,
		sessions:    dependencies.Sessions,
		catalog:     dependencies.Catalog,
		tools:       dependencies.Tools,
		newCaller:   dependencies.NewCaller,
		events:      dependencies.Events,
		compactions: dependencies.Compactions,
		skills:      dependencies.Skills,
		logger:      slog.Default(),
		active:      make(map[uuid.UUID]*activeExecution),
		stopped:     make(chan struct{}),
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

func (engine *Engine) Compact(
	ctx context.Context,
	sessionID string,
) (*CompactResult, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, apperrors.Validation("invalid session id: " + err.Error())
	}

	runCtx, execution, err := engine.reserve(id)
	if err != nil {
		return nil, err
	}
	defer engine.release(id, execution)
	toolRuntime := engine.snapshotTools()

	session, err := engine.sessions.Load(ctx, id)
	if err != nil {
		if errors.Is(err, conversation.ErrSessionNotFound) {
			return nil, apperrors.NotFound("session " + id.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load session", err)
	}
	resources, err := engine.loadResources(ctx, runCtx, session)
	if err != nil {
		return nil, err
	}
	prepared := &preparedExecution{
		session:         session,
		model:           resources.model,
		caller:          resources.caller,
		systemPrompt:    resources.systemPrompt,
		freeFormTool:    resources.freeFormTool,
		maxOutputTokens: modelMaxOutputTokens(resources.model),
		toolRuntime:     toolRuntime,
	}
	event, err := engine.compactPrepared(runCtx, prepared, conversation.CompactionTriggerManual)
	if err != nil {
		return nil, err
	}
	return &CompactResult{
		SessionID:           event.SessionID,
		CompactionID:        event.CompactionID,
		Trigger:             event.Trigger,
		ContextTokensBefore: event.ContextTokensBefore,
		ContextTokensAfter:  event.ContextTokensAfter,
		Usage:               event.Usage,
	}, nil
}

func (engine *Engine) SetModel(
	ctx context.Context,
	sessionID string,
	providerCode string,
	modelCode string,
) (*conversation.Session, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, apperrors.Validation("invalid session id: " + err.Error())
	}
	providerCodeValue, err := shared.NewCode(providerCode)
	if err != nil {
		return nil, apperrors.Validation(err.Error())
	}
	modelCodeValue, err := shared.NewModelCode(modelCode)
	if err != nil {
		return nil, apperrors.Validation(err.Error())
	}

	runCtx, execution, err := engine.reserve(id)
	if err != nil {
		return nil, err
	}
	defer engine.release(id, execution)
	toolRuntime := engine.snapshotTools()

	session, err := engine.sessions.Load(ctx, id)
	if err != nil {
		if errors.Is(err, conversation.ErrSessionNotFound) {
			return nil, apperrors.NotFound("session " + id.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load session", err)
	}
	if session.CurrentModel != nil &&
		session.CurrentModel.ProviderCode == providerCodeValue &&
		session.CurrentModel.ModelCode == modelCodeValue {
		return session.VisibleCopy(), nil
	}

	source, err := engine.loadResources(ctx, runCtx, session)
	if err != nil {
		return nil, err
	}
	targetRef := shared.NewModelRef(providerCodeValue, modelCodeValue)
	_, targetModel, err := engine.loadCatalogModel(ctx, targetRef)
	if err != nil {
		return nil, err
	}
	targetContextWindow := int64(targetModel.ContextWindow)
	if targetContextWindow <= 0 {
		return nil, apperrors.Validation("target model context window must be positive")
	}
	if session.CurrentModel == nil || session.CurrentModel.IsZero() {
		session.SetModel(targetRef, targetContextWindow)
		if err := engine.saveProgress(ctx, session); err != nil {
			return nil, apperrors.WrapError(apperrors.CodeInternal, "save session model", err)
		}
		return session.VisibleCopy(), nil
	}
	targetMaxOutputTokens := modelMaxOutputTokens(*targetModel)

	prepared := &preparedExecution{
		session:         session,
		model:           source.model,
		caller:          source.caller,
		systemPrompt:    source.systemPrompt,
		freeFormTool:    source.freeFormTool,
		maxOutputTokens: modelMaxOutputTokens(source.model),
		toolRuntime:     toolRuntime,
	}
	request := engine.sessionRequestForWindow(prepared, targetContextWindow, targetMaxOutputTokens)
	if ShouldCompact(estimateRequestTokens(request), targetContextWindow, targetMaxOutputTokens) {
		if _, err := engine.compactPreparedForWindow(
			runCtx,
			prepared,
			conversation.CompactionTriggerModelSwitch,
			targetContextWindow,
			targetMaxOutputTokens,
		); err != nil {
			return nil, fmt.Errorf("compact session before model switch: %w", err)
		}
		request = engine.sessionRequestForWindow(prepared, targetContextWindow, targetMaxOutputTokens)
		if ShouldCompact(estimateRequestTokens(request), targetContextWindow, targetMaxOutputTokens) {
			return nil, apperrors.Validation("session context remains too large for target model after compaction")
		}
	}

	session.SetModel(targetRef, targetContextWindow)
	if err := engine.saveProgress(ctx, session); err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "save session model", err)
	}
	return session.VisibleCopy(), nil
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
	freeFormTool    bool
	maxOutputTokens int64
	toolRuntime     ToolRuntime
	userMessage     conversation.Message
	eventSequence   uint64
}

type executionResources struct {
	model        catalog.Model
	caller       Caller
	systemPrompt string
	freeFormTool bool
}

func (engine *Engine) prepare(
	ctx context.Context,
	runCtx context.Context,
	sessionID uuid.UUID,
	content conversation.Content,
) (*preparedExecution, error) {
	toolRuntime := engine.snapshotTools()

	session, err := engine.sessions.Load(ctx, sessionID)
	if err != nil {
		if errors.Is(err, conversation.ErrSessionNotFound) {
			return nil, apperrors.NotFound("session " + sessionID.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load session", err)
	}

	resources, err := engine.loadResources(ctx, runCtx, session)
	if err != nil {
		return nil, err
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

	resolvedSkills, err := engine.resolveExplicitSkills(content)
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeValidation, "failed to load referenced skill", err)
	}
	if len(resolvedSkills) > 0 {
		if _, err := session.AppendHiddenUserMessageWithMetadata(
			roundID,
			conversation.Text(formatSkillMarkdown(resolvedSkills)),
			map[string]any{
				"kind":  "skill-md",
				"names": skillNames(resolvedSkills),
				"paths": skillPaths(resolvedSkills),
			},
		); err != nil {
			return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to append referenced skill", err)
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
		model:           resources.model,
		caller:          resources.caller,
		systemPrompt:    resources.systemPrompt,
		freeFormTool:    resources.freeFormTool,
		maxOutputTokens: modelMaxOutputTokens(resources.model),
		toolRuntime:     toolRuntime,
		userMessage:     userMessage,
	}, nil
}

func modelMaxOutputTokens(model catalog.Model) int64 {
	if model.MaxOutputTokens > 0 {
		return model.MaxOutputTokens
	}
	return DefaultMaxOutputTokens
}

func (engine *Engine) loadResources(
	ctx context.Context,
	runCtx context.Context,
	session *conversation.Session,
) (*executionResources, error) {
	if session.CurrentModel == nil || session.CurrentModel.IsZero() {
		return nil, apperrors.Validation("session model is not configured")
	}

	provider, model, err := engine.loadCatalogModel(ctx, *session.CurrentModel)
	if err != nil {
		return nil, err
	}

	skillPrompt := ""
	if engine.skills != nil {
		skillPrompt = engine.skills.PromptSection()
	}
	systemPrompt, err := ResolveSystemPrompt(SystemPromptOptions{
		UseApplyPatchShell: !provider.FreeFormTool,
		SkillCatalog:       skillPrompt,
	})
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to resolve system prompt", err)
	}
	caller, err := engine.newCaller(runCtx, *provider, *model)
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to create LLM caller", err)
	}
	return &executionResources{
		model:        *model,
		caller:       caller,
		systemPrompt: systemPrompt,
		freeFormTool: provider.FreeFormTool,
	}, nil
}

func (engine *Engine) loadCatalogModel(
	ctx context.Context,
	modelRef shared.ModelRef,
) (*catalog.Provider, *catalog.Model, error) {
	provider, err := engine.catalog.Get(ctx, modelRef.ProviderCode)
	if err != nil {
		if errors.Is(err, catalog.ErrProviderNotFound) {
			return nil, nil, apperrors.NotFound("provider " + modelRef.ProviderCode.String() + " not found")
		}
		return nil, nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load provider", err)
	}
	model, err := provider.Model(modelRef.ModelCode)
	if err != nil {
		if errors.Is(err, catalog.ErrModelNotFound) {
			return nil, nil, apperrors.NotFound("model " + modelRef.ModelCode.String() + " not found")
		}
		return nil, nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load model", err)
	}
	return provider, model, nil
}

func (engine *Engine) sessionRequest(prepared *preparedExecution) Request {
	return engine.sessionRequestForWindow(prepared, modelContextWindow(prepared), prepared.maxOutputTokens)
}

func (engine *Engine) sessionRequestForWindow(
	prepared *preparedExecution,
	contextWindow int64,
	maxOutputTokens int64,
) Request {
	request := Request{
		SystemPrompt:    prepared.systemPrompt,
		Messages:        sessionMessages(prepared.session),
		Tools:           engine.toolDefinitions(prepared.toolRuntime, prepared.freeFormTool),
		MaxOutputTokens: maxOutputTokens,
		ReasoningEffort: preparedReasoningEffort(prepared),
	}
	return fitCompactedRequest(request, contextWindow)
}

func (engine *Engine) snapshotTools() ToolRuntime {
	if snapshotter, ok := engine.tools.(interface{ SnapshotToolRuntime() ToolRuntime }); ok {
		return snapshotter.SnapshotToolRuntime()
	}
	return engine.tools
}

func (engine *Engine) toolDefinitions(toolRuntime ToolRuntime, freeFormTool bool) []ToolDefinition {
	if toolRuntime == nil {
		toolRuntime = engine.tools
	}
	definitions := toolRuntime.Definitions()
	if freeFormTool {
		return definitions
	}

	filtered := make([]ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Type != ToolTypeApplyPatch {
			filtered = append(filtered, definition)
		}
	}
	return filtered
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
	lastCompacted := false
	toolRuntime := prepared.toolRuntime
	if toolRuntime == nil {
		toolRuntime = engine.tools
	}

	result, err := runAgentLoop(ctx, agentLoopConfig{
		name: "agent loop",
		request: func(ctx context.Context, iteration int) (Request, conversation.TokenUsage, error) {
			// Automatic compaction is a request-building policy of real sessions.
			request := engine.sessionRequest(prepared)
			var compactionUsage conversation.TokenUsage
			if ShouldCompact(
				estimateRequestTokens(request),
				modelContextWindow(prepared),
				prepared.maxOutputTokens,
			) && !lastCompacted {
				compaction, err := engine.compactPrepared(ctx, prepared, conversation.CompactionTriggerAuto)
				if err != nil {
					return Request{}, conversation.TokenUsage{}, fmt.Errorf(
						"compact session before iteration %d: %w",
						iteration,
						err,
					)
				}
				compactionUsage = compaction.Usage
				lastCompacted = true
				request = engine.sessionRequest(prepared)
			}
			return request, compactionUsage, nil
		},
		invoke: func(ctx context.Context, iteration int, request Request) (*Response, error) {
			return engine.call(ctx, prepared, iteration, request)
		},
		toolRuntime: toolRuntime,
		callContext: func() CallContext {
			cwd := ""
			round := prepared.session.Rounds[len(prepared.session.Rounds)-1]
			if round.Cwd != nil {
				cwd = *round.Cwd
			}
			return CallContext{
				SessionID: prepared.session.ID,
				RoundID:   prepared.roundID,
				Cwd:       cwd,
			}
		},
		appendAssistant: func(ctx context.Context, iteration int, response *Response, _ bool) error {
			// Persist each assistant response before continuing with tools.
			message, err := prepared.session.AppendAssistantMessage(
				prepared.roundID,
				response.Content,
				prepared.session.Rounds[len(prepared.session.Rounds)-1].Model,
				&response.Usage,
			)
			if err != nil {
				return fmt.Errorf("append assistant message at iteration %d: %w", iteration, err)
			}
			if err := engine.saveProgress(ctx, prepared.session); err != nil {
				return fmt.Errorf("save assistant message at iteration %d: %w", iteration, err)
			}
			if err := engine.emit(ctx, prepared, SessionEvent{
				Type:      SessionEventMessageAppended,
				Iteration: iteration,
				Message:   &message,
			}); err != nil {
				return fmt.Errorf("emit assistant message at iteration %d: %w", iteration, err)
			}
			return nil
		},
		appendToolResults: func(ctx context.Context, iteration int, content conversation.Content) error {
			// Tool results remain ordinary user-role transcript messages.
			message, err := prepared.session.AppendUserMessage(prepared.roundID, content)
			if err != nil {
				return fmt.Errorf("append tool results at iteration %d: %w", iteration, err)
			}
			if err := engine.saveProgress(ctx, prepared.session); err != nil {
				return fmt.Errorf("save tool results at iteration %d: %w", iteration, err)
			}
			if err := engine.emit(ctx, prepared, SessionEvent{
				Type:      SessionEventMessageAppended,
				Iteration: iteration,
				Message:   &message,
			}); err != nil {
				return fmt.Errorf("emit tool results at iteration %d: %w", iteration, err)
			}
			return nil
		},
		afterResponse: func(*Response) {
			lastCompacted = false
		},
	})
	if err != nil {
		return result.usage, err
	}
	return result.usage, nil
}

func modelContextWindow(prepared *preparedExecution) int64 {
	if prepared.model.ContextWindow > 0 {
		return int64(prepared.model.ContextWindow)
	}
	return prepared.session.ContextWindow
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
	return session.ContextMessages()
}

func toolCalls(content conversation.Content) []conversation.ToolUseBlock {
	calls := make([]conversation.ToolUseBlock, 0)
	for _, block := range content {
		switch call := block.(type) {
		case conversation.ToolUseBlock:
			calls = append(calls, call)
		case conversation.ShellCallBlock:
			calls = append(calls, call.ToolUseBlock())
		case conversation.ApplyPatchCallBlock:
			calls = append(calls, call.ToolUseBlock())
		}
	}

	return calls
}
