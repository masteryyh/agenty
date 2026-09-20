package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/application/apperrors"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	infracompaction "github.com/masteryyh/agenty-core/pkg/infra/compaction"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	infraprompt "github.com/masteryyh/agenty-core/pkg/infra/prompt"
)

const (
	DefaultMaxOutputTokens int64 = infracompaction.DefaultMaxOutputTokens
)

type ExecutionSessionRepository interface {
	Load(ctx context.Context, id uuid.UUID) (*conversation.Session, error)
	Save(ctx context.Context, session *conversation.Session) error
}

type ExecutionCatalogRepository interface {
	Get(ctx context.Context, code shared.Code) (*catalog.Provider, error)
}

type Dependencies struct {
	Sessions              ExecutionSessionRepository
	Catalog               ExecutionCatalogRepository
	Tools                 agentloop.ToolRuntime
	InvokeModel           modelcall.InvokeFunc
	LoopHooks             agentloop.LoopHooks
	Lifecycle             inframiddleware.LifecycleHooks
	PermissionModeChanged func(context.Context, uuid.UUID, conversation.PermissionMode) error
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
	roundID      uuid.UUID
	session      *conversation.Session
	provider     catalog.Provider
	cancel       context.CancelFunc
	sessionReady chan struct{}
	readyOnce    sync.Once
}

type Engine struct {
	ctx                   context.Context
	cancel                context.CancelFunc
	sessions              ExecutionSessionRepository
	catalog               ExecutionCatalogRepository
	tools                 agentloop.ToolRuntime
	invokeModel           modelcall.InvokeFunc
	loopHooks             agentloop.LoopHooks
	lifecycle             inframiddleware.LifecycleHooks
	permissionModeChanged func(context.Context, uuid.UUID, conversation.PermissionMode) error
	logger                *slog.Logger
	mu                    sync.Mutex
	sessionLocksMu        sync.Mutex
	sessionLocks          map[uuid.UUID]*sync.Mutex
	active                map[uuid.UUID]*activeExecution
	started               map[uuid.UUID]struct{}
	resources             map[uuid.UUID]executionResources
	waitGroup             sync.WaitGroup
	shutdown              bool
	stopOnce              sync.Once
	stopped               chan struct{}
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
	if dependencies.Lifecycle.OnEvent == nil {
		return nil, apperrors.Validation("execution event hook must not be nil")
	}

	ctx, cancel := context.WithCancel(parentCtx)
	return &Engine{
		ctx:                   ctx,
		cancel:                cancel,
		sessions:              dependencies.Sessions,
		catalog:               dependencies.Catalog,
		tools:                 dependencies.Tools,
		invokeModel:           dependencies.InvokeModel,
		loopHooks:             dependencies.LoopHooks,
		lifecycle:             dependencies.Lifecycle,
		permissionModeChanged: dependencies.PermissionModeChanged,
		logger:                slog.Default(),
		active:                make(map[uuid.UUID]*activeExecution),
		started:               make(map[uuid.UUID]struct{}),
		resources:             make(map[uuid.UUID]executionResources),
		sessionLocks:          make(map[uuid.UUID]*sync.Mutex),
		stopped:               make(chan struct{}),
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

	prepared, err := engine.prepare(ctx, runCtx, id, content, execution)
	if err != nil {
		return nil, err
	}

	engine.mu.Lock()
	execution.roundID = prepared.roundID
	execution.provider = prepared.provider
	engine.mu.Unlock()

	launched = true
	executionCtx := prepared.context
	if executionCtx == nil {
		executionCtx = runCtx
	}
	go engine.run(executionCtx, id, execution, prepared)

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
	sessionLock := engine.sessionLock(id)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	session, err := engine.sessions.Load(ctx, id)
	if err != nil {
		if errors.Is(err, conversation.ErrSessionNotFound) {
			return nil, apperrors.NotFound("session " + id.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load session", err)
	}
	engine.bindExecutionSession(id, execution, session)
	resources, ok := engine.cachedSessionResourcesForModel(id, session)
	if !ok {
		resources, err = engine.loadResources(ctx, runCtx, session)
		if err != nil {
			return nil, err
		}
		resources = engine.applySessionResources(id, resources)
	}
	prepared := &preparedExecution{
		session:         session,
		context:         runCtx,
		provider:        resources.provider,
		model:           resources.model,
		modelCall:       resources.modelCall,
		systemPrompt:    resources.systemPrompt,
		freeFormTool:    resources.freeFormTool,
		maxOutputTokens: modelMaxOutputTokens(resources.model),
		toolRuntime:     toolRuntime,
	}
	event, err := engine.compactPreparedForWindowLocked(
		runCtx,
		prepared,
		conversation.CompactionTriggerManual,
		modelContextWindow(prepared),
		prepared.maxOutputTokens,
	)
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
	sessionLock := engine.sessionLock(id)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	session, err := engine.sessions.Load(ctx, id)
	if err != nil {
		if errors.Is(err, conversation.ErrSessionNotFound) {
			return nil, apperrors.NotFound("session " + id.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load session", err)
	}
	engine.bindExecutionSession(id, execution, session)
	if session.CurrentModel != nil &&
		session.CurrentModel.ProviderCode == providerCodeValue &&
		session.CurrentModel.ModelCode == modelCodeValue {
		return session.VisibleCopy(), nil
	}

	source, ok := engine.cachedSessionResourcesForModel(id, session)
	if !ok {
		source, err = engine.loadResources(ctx, runCtx, session)
		if err != nil {
			return nil, err
		}
		source = engine.applySessionResources(id, source)
	}
	targetRef := shared.NewModelRef(providerCodeValue, modelCodeValue)
	targetProvider, targetModel, err := engine.loadCatalogModel(ctx, targetRef)
	if err != nil {
		return nil, err
	}
	targetContextWindow := int64(targetModel.ContextWindow)
	if targetContextWindow <= 0 {
		return nil, apperrors.Validation("target model context window must be positive")
	}
	if session.CurrentModel == nil || session.CurrentModel.IsZero() {
		session.SetModel(targetRef, targetContextWindow)
		if err := engine.emitForSession(ctx, session, uuid.Nil, targetProvider, targetModel, agentloop.Event{
			Type: agentloop.EventSessionChanged,
		}); err != nil {
			return nil, apperrors.WrapError(apperrors.CodeInternal, "persist session model", err)
		}
		return session.VisibleCopy(), nil
	}
	targetMaxOutputTokens := modelMaxOutputTokens(*targetModel)

	prepared := &preparedExecution{
		context:         runCtx,
		session:         session,
		provider:        source.provider,
		model:           source.model,
		modelCall:       source.modelCall,
		systemPrompt:    source.systemPrompt,
		freeFormTool:    source.freeFormTool,
		maxOutputTokens: modelMaxOutputTokens(source.model),
		toolRuntime:     toolRuntime,
	}
	request := engine.sessionRequestForWindow(prepared, targetContextWindow, targetMaxOutputTokens)
	if infracompaction.ShouldCompact(infracompaction.EstimateRequestTokens(request), targetContextWindow, targetMaxOutputTokens) {
		if _, err := engine.compactPreparedForWindowLocked(
			runCtx,
			prepared,
			conversation.CompactionTriggerModelSwitch,
			targetContextWindow,
			targetMaxOutputTokens,
		); err != nil {
			return nil, fmt.Errorf("compact session before model switch: %w", err)
		}
		request = engine.sessionRequestForWindow(prepared, targetContextWindow, targetMaxOutputTokens)
		if infracompaction.ShouldCompact(infracompaction.EstimateRequestTokens(request), targetContextWindow, targetMaxOutputTokens) {
			return nil, apperrors.Validation("session context remains too large for target model after compaction")
		}
	}

	session.SetModel(targetRef, targetContextWindow)
	if err := engine.emitForSession(ctx, session, uuid.Nil, targetProvider, targetModel, agentloop.Event{
		Type: agentloop.EventSessionChanged,
	}); err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "persist session model", err)
	}
	return session.VisibleCopy(), nil
}

func (engine *Engine) IsRunning(sessionID uuid.UUID) bool {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	_, ok := engine.active[sessionID]
	return ok
}

func (engine *Engine) SetPermissionMode(
	ctx context.Context,
	sessionID string,
	mode conversation.PermissionMode,
) (*conversation.Session, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, apperrors.Validation("invalid session id: " + err.Error())
	}
	if !mode.Valid() {
		return nil, apperrors.Validation("invalid permission mode: " + string(mode))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	active, running, session := engine.waitForActiveSession(ctx, id)
	var roundID uuid.UUID
	var provider catalog.Provider

	sessionLock := engine.sessionLock(id)
	sessionLock.Lock()
	defer sessionLock.Unlock()
	if running {
		// prepare binds the session before it allocates the round. Refresh the
		// execution snapshot after taking the session lock so a permission
		// change cannot retain the pre-prepare zero round ID.
		engine.mu.Lock()
		current, stillRunning := engine.active[id]
		if stillRunning && current == active {
			active = current
			session = current.session
			roundID = current.roundID
			provider = current.provider
		} else if stillRunning && current != nil {
			active = current
			if current.session == nil {
				// A newer Start has reserved the session but has not loaded it
				// yet. Keep the mode change session-scoped; that Start will
				// load the event after this lock is released.
				running = false
				session = nil
			} else {
				session = current.session
				roundID = current.roundID
				provider = current.provider
			}
		} else if !stillRunning {
			running = false
		}
		engine.mu.Unlock()
	}
	if running {
		if roundID == uuid.Nil && session != nil {
			for index := len(session.Rounds) - 1; index >= 0; index-- {
				if session.Rounds[index].Status == conversation.RoundRunning {
					roundID = session.Rounds[index].ID
					break
				}
			}
		}
		round := currentRound(session, roundID)
		if roundID == uuid.Nil || round == nil || round.Status != conversation.RoundRunning {
			running = false
			roundID = uuid.Nil
		}
	}

	if session == nil {
		session, err = engine.sessions.Load(ctx, id)
		if err != nil {
			if errors.Is(err, conversation.ErrSessionNotFound) {
				return nil, apperrors.NotFound("session " + sessionID + " not found")
			}
			return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load session", err)
		}
	}
	if !session.SetPermissionMode(mode, roundID) {
		return session.VisibleCopy(), nil
	}
	if running && roundID != uuid.Nil {
		permissionMode := string(mode)
		text, encodeErr := (conversation.MetadataUpdate{PermissionMode: &permissionMode}).XML()
		if encodeErr != nil {
			return nil, apperrors.WrapError(apperrors.CodeInternal, "encode permission metadata", encodeErr)
		}
		role := conversation.RoleUser
		if provider.SupportsDeveloperMessages() {
			role = conversation.RoleDeveloper
		}
		if _, appendErr := session.AppendHiddenMessage(
			roundID,
			role,
			conversation.Text(text),
			map[string]any{"kind": "metadata", "scope": "round"},
		); appendErr != nil {
			return nil, apperrors.WrapError(apperrors.CodeInternal, "append permission metadata", appendErr)
		}
	}

	change := conversation.SessionPermissionModeChanged{
		SessionID:      session.ID,
		RoundID:        roundID,
		PreviousMode:   session.CurrentPermissionMode(),
		PermissionMode: mode,
	}
	// SetPermissionMode has already recorded the event. Read it back so the
	// emitted payload includes the exact previous mode from the event.
	if pending := session.PendingEvents(); len(pending) > 0 {
		for _, p := range slices.Backward(pending) {
			if recorded, ok := p.(conversation.SessionPermissionModeChanged); ok {
				change = recorded
				break
			}
		}
	}
	if err := engine.emitForSession(ctx, session, roundID, nil, nil, agentloop.Event{
		Type:    agentloop.EventPermissionModeChanged,
		Payload: change,
	}); err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "persist permission mode", err)
	}
	if engine.permissionModeChanged != nil {
		if err := engine.permissionModeChanged(ctx, session.ID, mode); err != nil {
			return nil, apperrors.WrapError(apperrors.CodeInternal, "apply permission mode", err)
		}
	}
	return session.VisibleCopy(), nil
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
			engine.afterSessionStop(context.WithoutCancel(ctx))
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
	execution := &activeExecution{cancel: cancel, sessionReady: make(chan struct{})}
	engine.active[sessionID] = execution
	engine.waitGroup.Add(1)

	return runCtx, execution, nil
}

func (engine *Engine) sessionLock(sessionID uuid.UUID) *sync.Mutex {
	engine.sessionLocksMu.Lock()
	defer engine.sessionLocksMu.Unlock()

	lock, ok := engine.sessionLocks[sessionID]
	if !ok {
		lock = &sync.Mutex{}
		engine.sessionLocks[sessionID] = lock
	}
	return lock
}

func (engine *Engine) bindExecutionSession(
	sessionID uuid.UUID,
	execution *activeExecution,
	session *conversation.Session,
) {
	engine.mu.Lock()
	if engine.active[sessionID] == execution {
		execution.session = session
	}
	engine.mu.Unlock()
	if execution != nil {
		execution.readyOnce.Do(func() { close(execution.sessionReady) })
	}
}

func (engine *Engine) waitForActiveSession(
	ctx context.Context,
	sessionID uuid.UUID,
) (*activeExecution, bool, *conversation.Session) {
	for {
		engine.mu.Lock()
		active, running := engine.active[sessionID]
		var session *conversation.Session
		var ready <-chan struct{}
		if running && active != nil {
			session = active.session
			ready = active.sessionReady
		}
		engine.mu.Unlock()
		if !running || session != nil {
			return active, running, session
		}

		select {
		case <-ready:
		case <-ctx.Done():
			return active, running, nil
		}
	}
}

type preparedExecution struct {
	context         context.Context
	session         *conversation.Session
	roundID         uuid.UUID
	provider        catalog.Provider
	model           catalog.Model
	modelCall       modelcall.ModelCallConfig
	systemPrompt    string
	freeFormTool    bool
	maxOutputTokens int64
	toolRuntime     agentloop.ToolRuntime
	userMessage     conversation.Message
}

type executionResources struct {
	sourceModel           shared.ModelRef
	provider              catalog.Provider
	model                 catalog.Model
	modelCall             modelcall.ModelCallConfig
	baseSystemPrompt      string
	systemPrompt          string
	sessionPromptSuffix   string
	sessionPromptOverride *string
	freeFormTool          bool
}

func (engine *Engine) prepare(
	ctx context.Context,
	runCtx context.Context,
	sessionID uuid.UUID,
	content conversation.Content,
	execution *activeExecution,
) (*preparedExecution, error) {
	toolRuntime := engine.snapshotTools()
	sessionLock := engine.sessionLock(sessionID)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	session, err := engine.sessions.Load(ctx, sessionID)
	if err != nil {
		if errors.Is(err, conversation.ErrSessionNotFound) {
			return nil, apperrors.NotFound("session " + sessionID.String() + " not found")
		}
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to load session", err)
	}
	engine.bindExecutionSession(sessionID, execution, session)

	resources, err := engine.loadResources(ctx, runCtx, session)
	if err != nil {
		return nil, err
	}
	resources = engine.applySessionResources(sessionID, resources)
	if err := runCtx.Err(); err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "execution was cancelled before start", err)
	}
	// Keep the session-level resources selected by BeforeSessionStart separate
	// from per-round mutations made by BeforeRound. In particular, a custom
	// round hook must not accidentally replace the cached skill prompt for
	// later rounds.
	baseSystemPrompt := resources.baseSystemPrompt
	sessionResources := *resources

	type hiddenMessage struct {
		role     conversation.Role
		content  conversation.Content
		metadata map[string]any
	}
	var hiddenMessages []hiddenMessage
	appendHiddenMessage := func(role conversation.Role, content conversation.Content, metadata map[string]any) error {
		if !role.Valid() {
			return fmt.Errorf("hidden message role %q is invalid", role)
		}
		hiddenMessages = append(hiddenMessages, hiddenMessage{
			role:     role,
			content:  content,
			metadata: metadata,
		})
		return nil
	}

	if engine.shouldStartSession(sessionID) {
		model := resources.model
		provider := resources.provider
		systemPrompt := resources.systemPrompt
		freeFormTool := resources.freeFormTool
		tools := toolRuntime
		state := &inframiddleware.SessionStartContext{
			Context:             runCtx,
			Session:             session,
			Provider:            &provider,
			Model:               &model,
			SystemPrompt:        &systemPrompt,
			FreeFormTool:        &freeFormTool,
			Tools:               &tools,
			AppendHiddenMessage: appendHiddenMessage,
			Emit: func(eventCtx context.Context, event agentloop.Event) error {
				return engine.emitForSession(eventCtx, session, uuid.Nil, &provider, &model, event)
			},
		}
		if engine.lifecycle.BeforeSessionStart != nil {
			if err := engine.lifecycle.BeforeSessionStart(runCtx, state); err != nil {
				return nil, apperrors.WrapError(apperrors.CodeInternal, "before session start hook", err)
			}
		}
		if state.Context != nil {
			runCtx = state.Context
		}
		if state.Provider == nil || state.Model == nil {
			return nil, apperrors.Validation("before session start hook cleared required resources")
		}
		provider = *state.Provider
		model = *state.Model
		if state.SystemPrompt == nil {
			systemPrompt = ""
		} else {
			systemPrompt = *state.SystemPrompt
		}
		if state.FreeFormTool != nil {
			freeFormTool = *state.FreeFormTool
		}
		resources.provider = provider
		resources.model = model
		modelCall := newModelCallConfig(provider, model)
		modelCall.FreeFormTool = freeFormTool
		resources.modelCall = modelCall
		resources.baseSystemPrompt = baseSystemPrompt
		resources.systemPrompt = systemPrompt
		resources.freeFormTool = freeFormTool
		if strings.HasPrefix(systemPrompt, baseSystemPrompt) {
			resources.sessionPromptSuffix = strings.TrimPrefix(systemPrompt, baseSystemPrompt)
			resources.sessionPromptOverride = nil
		} else {
			resources.sessionPromptSuffix = ""
			override := systemPrompt
			resources.sessionPromptOverride = &override
		}
		sessionResources = *resources
		if state.Tools == nil {
			toolRuntime = nil
		} else {
			toolRuntime = *state.Tools
		}
	}

	contentValue := content
	state := &inframiddleware.RoundContext{
		Context:             runCtx,
		Session:             session,
		Provider:            &resources.provider,
		Model:               &resources.model,
		Content:             &contentValue,
		Tools:               &toolRuntime,
		SystemPrompt:        &resources.systemPrompt,
		AppendHiddenMessage: appendHiddenMessage,
		Emit: func(eventCtx context.Context, event agentloop.Event) error {
			return engine.emitForSession(
				eventCtx,
				session,
				uuid.Nil,
				&resources.provider,
				&resources.model,
				event,
			)
		},
	}
	if engine.lifecycle.BeforeRound != nil {
		if err := engine.lifecycle.BeforeRound(runCtx, state); err != nil {
			return nil, apperrors.WrapError(apperrors.CodeInternal, "before round hook", err)
		}
	}
	if state.Context != nil {
		runCtx = state.Context
	}
	if err := runCtx.Err(); err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "execution was cancelled before round", err)
	}
	if state.Content == nil {
		return nil, apperrors.Validation("before round hook cleared session content")
	}
	content = *state.Content
	if state.Tools == nil {
		toolRuntime = nil
	} else {
		toolRuntime = *state.Tools
	}
	if state.SystemPrompt == nil {
		resources.systemPrompt = ""
	} else {
		resources.systemPrompt = *state.SystemPrompt
	}

	roundID, err := session.StartRound()
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to start round", err)
	}
	for _, hidden := range hiddenMessages {
		if _, err := session.AppendHiddenMessage(roundID, hidden.role, hidden.content, hidden.metadata); err != nil {
			return nil, apperrors.WrapError(apperrors.CodeInternal, "append hidden round message", err)
		}
	}

	userMessage, err := session.AppendUserMessage(roundID, content)
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to append user message", err)
	}
	prepared := &preparedExecution{
		context:         runCtx,
		session:         session,
		roundID:         roundID,
		provider:        resources.provider,
		model:           resources.model,
		modelCall:       resources.modelCall,
		systemPrompt:    resources.systemPrompt,
		freeFormTool:    resources.freeFormTool,
		maxOutputTokens: modelMaxOutputTokens(resources.model),
		toolRuntime:     toolRuntime,
		userMessage:     userMessage,
	}
	engine.mu.Lock()
	if engine.active[sessionID] == execution {
		execution.roundID = roundID
		execution.provider = prepared.provider
	}
	engine.mu.Unlock()
	if err := engine.emitEvent(runCtx, prepared, agentloop.Event{Type: agentloop.EventSessionChanged}); err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "persist started round", err)
	}
	if engine.shouldStartSession(sessionID) {
		engine.markSessionStarted(sessionID, sessionResources)
	}

	return prepared, nil
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

	systemPrompt, err := infraprompt.ResolveSystemPrompt(infraprompt.SystemPromptOptions{
		UseApplyPatchShell: !provider.FreeFormTool,
	})
	if err != nil {
		return nil, apperrors.WrapError(apperrors.CodeInternal, "failed to resolve system prompt", err)
	}
	return &executionResources{
		sourceModel:      *session.CurrentModel,
		provider:         *provider,
		model:            *model,
		modelCall:        newModelCallConfig(*provider, *model),
		baseSystemPrompt: systemPrompt,
		systemPrompt:     systemPrompt,
		freeFormTool:     provider.FreeFormTool,
	}, nil
}

// applySessionResources restores session-level prompt additions while retaining
// the provider-specific base prompt selected by the newly loaded model.
func (engine *Engine) applySessionResources(
	sessionID uuid.UUID,
	loaded *executionResources,
) *executionResources {
	if loaded == nil {
		return nil
	}
	cached, ok := engine.cachedSessionResources(sessionID)
	if !ok {
		return loaded
	}

	merged := *loaded
	merged.baseSystemPrompt = loaded.baseSystemPrompt
	if cached.sessionPromptOverride != nil {
		merged.systemPrompt = *cached.sessionPromptOverride
		merged.sessionPromptOverride = cached.sessionPromptOverride
		merged.sessionPromptSuffix = ""
	} else {
		merged.systemPrompt = loaded.baseSystemPrompt + cached.sessionPromptSuffix
		merged.sessionPromptOverride = nil
		merged.sessionPromptSuffix = cached.sessionPromptSuffix
	}
	return &merged
}

func (engine *Engine) shouldStartSession(sessionID uuid.UUID) bool {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	_, started := engine.started[sessionID]
	return !started
}

func (engine *Engine) markSessionStarted(sessionID uuid.UUID, resources executionResources) {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	engine.started[sessionID] = struct{}{}
	engine.resources[sessionID] = resources
}

func (engine *Engine) cachedSessionResources(sessionID uuid.UUID) (executionResources, bool) {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	resources, ok := engine.resources[sessionID]
	return resources, ok
}

func (engine *Engine) cachedSessionResourcesForModel(
	sessionID uuid.UUID,
	session *conversation.Session,
) (*executionResources, bool) {
	if session == nil || session.CurrentModel == nil {
		return nil, false
	}
	cached, ok := engine.cachedSessionResources(sessionID)
	if !ok {
		return nil, false
	}
	cachedModelMatches := cached.provider.Code == session.CurrentModel.ProviderCode &&
		cached.model.Code == session.CurrentModel.ModelCode
	sourceModelMatches := cached.sourceModel == *session.CurrentModel
	if !cachedModelMatches && !sourceModelMatches {
		return nil, false
	}
	return &cached, true
}

func (engine *Engine) afterSessionStop(ctx context.Context) {
	if engine.lifecycle.AfterSessionStop == nil {
		return
	}

	engine.mu.Lock()
	ids := make([]uuid.UUID, 0, len(engine.started))
	for sessionID := range engine.started {
		ids = append(ids, sessionID)
	}
	engine.mu.Unlock()

	for _, sessionID := range ids {
		session, err := engine.sessions.Load(ctx, sessionID)
		if err != nil {
			engine.logger.ErrorContext(ctx, "failed to load session for after session stop hook",
				"sessionId", sessionID,
				"error", err,
			)
			continue
		}
		if err := engine.lifecycle.AfterSessionStop(ctx, &inframiddleware.SessionStopContext{
			Context: ctx,
			Session: session,
			Emit: func(eventCtx context.Context, event agentloop.Event) error {
				return engine.emitForSession(eventCtx, session, uuid.Nil, nil, nil, event)
			},
		}); err != nil {
			engine.logger.ErrorContext(ctx, "after session stop hook failed",
				"sessionId", sessionID,
				"error", err,
			)
		}
		if err := engine.emitForSession(ctx, session, uuid.Nil, nil, nil, agentloop.Event{
			Type: agentloop.EventSessionChanged,
		}); err != nil {
			engine.logger.ErrorContext(ctx, "failed to persist session after session stop hook",
				"sessionId", sessionID,
				"error", err,
			)
		}
	}
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

func (engine *Engine) sessionRequest(prepared *preparedExecution) modelcall.ModelCallRequest {
	lock := engine.sessionLock(prepared.session.ID)
	lock.Lock()
	defer lock.Unlock()

	return engine.sessionRequestForWindow(prepared, modelContextWindow(prepared), prepared.maxOutputTokens)
}

func (engine *Engine) sessionRequestForWindow(
	prepared *preparedExecution,
	contextWindow int64,
	maxOutputTokens int64,
) modelcall.ModelCallRequest {
	messages := infracompaction.FitCompactedMessages(
		sessionMessages(prepared.session),
		contextWindow,
		maxOutputTokens,
	)
	request := modelcall.ModelCallRequest{
		SystemPrompt:    prepared.systemPrompt,
		Messages:        modelMessages(messages),
		Tools:           engine.toolDefinitions(prepared.toolRuntime, prepared.freeFormTool),
		MaxOutputTokens: maxOutputTokens,
		ReasoningEffort: sessionReasoningEffort(prepared.session),
	}
	return request
}

func (engine *Engine) compactPrepared(
	ctx context.Context,
	prepared *preparedExecution,
	trigger conversation.CompactionTrigger,
) (*conversation.SessionCompacted, error) {
	return engine.compactPreparedForWindow(
		ctx,
		prepared,
		trigger,
		modelContextWindow(prepared),
		prepared.maxOutputTokens,
	)
}

func (engine *Engine) compactPreparedForWindow(
	ctx context.Context,
	prepared *preparedExecution,
	trigger conversation.CompactionTrigger,
	contextWindow int64,
	maxOutputTokens int64,
) (*conversation.SessionCompacted, error) {
	lock := engine.sessionLock(prepared.session.ID)
	lock.Lock()
	defer lock.Unlock()

	return engine.compactPreparedForWindowLocked(ctx, prepared, trigger, contextWindow, maxOutputTokens)
}

func (engine *Engine) compactPreparedForWindowLocked(
	ctx context.Context,
	prepared *preparedExecution,
	trigger conversation.CompactionTrigger,
	contextWindow int64,
	maxOutputTokens int64,
) (*conversation.SessionCompacted, error) {
	return (infracompaction.Executor{
		Emit: func(eventCtx context.Context, event agentloop.Event) error {
			return engine.emitEvent(eventCtx, prepared, event)
		},
		Logger: engine.logger,
	}).Compact(ctx, infracompaction.Prepared{
		Session:         prepared.session,
		Model:           prepared.modelCall,
		InvokeModel:     engine.invokeModel,
		SystemPrompt:    prepared.systemPrompt,
		MaxOutputTokens: prepared.maxOutputTokens,
		RequestForWindow: func(window, outputTokens int64) modelcall.ModelCallRequest {
			return engine.sessionRequestForWindow(prepared, window, outputTokens)
		},
	}, trigger, contextWindow, maxOutputTokens)
}

func (engine *Engine) snapshotTools() agentloop.ToolRuntime {
	if snapshotter, ok := engine.tools.(interface{ SnapshotToolRuntime() agentloop.ToolRuntime }); ok {
		return snapshotter.SnapshotToolRuntime()
	}
	return engine.tools
}

func (engine *Engine) toolDefinitions(
	toolRuntime agentloop.ToolRuntime,
	freeFormTool bool,
) []modelcall.ToolDefinition {
	if toolRuntime == nil {
		return nil
	}
	definitions := toolRuntime.Definitions()
	if freeFormTool {
		return definitions
	}

	filtered := make([]modelcall.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Type != modelcall.ToolTypeApplyPatch {
			filtered = append(filtered, definition)
		}
	}
	return filtered
}

func sessionReasoningEffort(session *conversation.Session) shared.ReasoningEffort {
	if len(session.Rounds) == 0 {
		return session.CurrentReasoningEffort
	}
	return session.Rounds[len(session.Rounds)-1].ReasoningEffort
}

func (engine *Engine) emitEvent(
	ctx context.Context,
	prepared *preparedExecution,
	event agentloop.Event,
) error {
	if prepared == nil {
		return fmt.Errorf("emit event without prepared execution")
	}
	return engine.emitForSession(
		ctx,
		prepared.session,
		prepared.roundID,
		&prepared.provider,
		&prepared.model,
		event,
	)
}

func (engine *Engine) emitForSession(
	ctx context.Context,
	session *conversation.Session,
	roundID uuid.UUID,
	provider *catalog.Provider,
	model *catalog.Model,
	event agentloop.Event,
) error {
	if session == nil {
		return fmt.Errorf("emit event session is nil")
	}
	if engine.lifecycle.OnEvent == nil {
		return fmt.Errorf("event hook is not configured")
	}
	if event.SessionID == uuid.Nil {
		event.SessionID = session.ID
	}
	if event.RoundID == uuid.Nil {
		event.RoundID = roundID
	}

	state := &inframiddleware.EventContext{
		Context:  ctx,
		Session:  session,
		Round:    currentRound(session, event.RoundID),
		RoundID:  event.RoundID,
		Provider: provider,
		Model:    model,
		Event:    &event,
	}
	state.Emit = func(eventCtx context.Context, next agentloop.Event) error {
		return engine.emitForSession(eventCtx, session, roundID, provider, model, next)
	}
	return engine.lifecycle.OnEvent(ctx, state)
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
	lock := engine.sessionLock(prepared.session.ID)
	lock.Lock()
	if err := engine.emitEvent(ctx, prepared, agentloop.Event{Type: agentloop.EventRoundStarted}); err != nil {
		lock.Unlock()
		status = conversation.RoundFailed
		runErr = err
		return
	}
	if err := engine.emitEvent(ctx, prepared, agentloop.Event{
		Type:    agentloop.EventMessageAppended,
		Message: &prepared.userMessage,
	}); err != nil {
		lock.Unlock()
		status = conversation.RoundFailed
		runErr = err
		return
	}
	lock.Unlock()

	usage, runErr = engine.executeLoop(ctx, prepared)
	if runErr != nil {
		status = conversation.RoundFailed
	}
}

func (engine *Engine) executeLoop(
	ctx context.Context,
	prepared *preparedExecution,
) (conversation.TokenUsage, error) {
	toolRuntime := prepared.toolRuntime
	compactionAttempted := false

	ctx = infracompaction.WithOperations(ctx, infracompaction.Operations{
		ContextWindow:   modelContextWindow(prepared),
		MaxOutputTokens: prepared.maxOutputTokens,
		Attempted:       &compactionAttempted,
		Compact: func(compactCtx context.Context) (conversation.TokenUsage, error) {
			compaction, err := engine.compactPrepared(compactCtx, prepared, conversation.CompactionTriggerAuto)
			if err != nil {
				return conversation.TokenUsage{}, err
			}
			return compaction.Usage, nil
		},
		RebuildRequest: func(_ context.Context) (modelcall.ModelCallRequest, error) {
			return engine.sessionRequest(prepared), nil
		},
	})

	loop, err := agentloop.NewAgentLoop(agentloop.AgentLoopOptions{
		Name:        "agent loop",
		Model:       prepared.modelCall,
		InvokeModel: engine.invokeModel,
		Emit: func(eventCtx context.Context, event agentloop.Event) error {
			return engine.handleLoopEvent(eventCtx, prepared, event)
		},
		Hooks:   engine.loopHooks,
		Session: prepared.session,
		Round: func() *conversation.Round {
			return engine.roundSnapshot(prepared.session, prepared.roundID)
		},
		SessionSnapshot: func() *conversation.Session {
			lock := engine.sessionLock(prepared.session.ID)
			lock.Lock()
			defer lock.Unlock()
			return prepared.session.Snapshot()
		},
		BuildRequest: func(ctx context.Context, iteration int) (modelcall.ModelCallRequest, conversation.TokenUsage, error) {
			request := engine.sessionRequest(prepared)
			return request, conversation.TokenUsage{}, nil
		},
		ToolRuntime: toolRuntime,
		CallContext: func() agentloop.CallContext {
			cwd := ""
			if round := engine.roundSnapshot(prepared.session, prepared.roundID); round != nil && round.Cwd != nil {
				cwd = *round.Cwd
			}
			return agentloop.CallContext{
				SessionID: prepared.session.ID,
				RoundID:   prepared.roundID,
				Cwd:       cwd,
			}
		},
	})
	if err != nil {
		return conversation.TokenUsage{}, err
	}
	result, err := loop.Run(ctx)
	return result.Usage, err
}

func newModelCallConfig(provider catalog.Provider, model catalog.Model) modelcall.ModelCallConfig {
	return modelcall.ModelCallConfig{
		BaseURL:           provider.BaseURL,
		APIType:           modelcall.APIType(provider.Type),
		APIKey:            provider.APIKey,
		ModelCode:         model.Code.String(),
		Official:          provider.Official,
		FreeFormTool:      provider.FreeFormTool,
		SupportsReasoning: model.SupportsReasoning(),
		ReasoningEfforts:  append([]shared.ReasoningEffort(nil), model.ReasoningEfforts...),
	}
}

func modelMessages(messages []conversation.Message) []modelcall.ModelCallMessage {
	projected := make([]modelcall.ModelCallMessage, 0, len(messages))
	for _, message := range messages {
		projected = append(projected, modelcall.ModelCallMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return projected
}

func (engine *Engine) handleLoopEvent(
	ctx context.Context,
	prepared *preparedExecution,
	event agentloop.Event,
) error {
	lock := engine.sessionLock(prepared.session.ID)
	lock.Lock()
	defer lock.Unlock()

	switch event.Type {
	case agentloop.EventModelStream:
		return engine.emitEvent(ctx, prepared, event)
	case agentloop.EventAssistantResponse:
		if event.Response == nil {
			return fmt.Errorf("agent loop emitted assistant response without response")
		}
		message, err := prepared.session.AppendAssistantMessage(
			prepared.roundID,
			event.Response.Content,
			prepared.session.Rounds[len(prepared.session.Rounds)-1].Model,
			&event.Response.Usage,
		)
		if err != nil {
			return fmt.Errorf("append assistant message at iteration %d: %w", event.Iteration, err)
		}
		event.Type = agentloop.EventMessageAppended
		event.Message = &message
		event.Response = nil
		return engine.emitEvent(ctx, prepared, event)
	case agentloop.EventToolResults:
		message, err := prepared.session.AppendUserMessage(prepared.roundID, event.ToolResults)
		if err != nil {
			return fmt.Errorf("append tool results at iteration %d: %w", event.Iteration, err)
		}
		event.Type = agentloop.EventMessageAppended
		event.Message = &message
		event.ToolResults = nil
		return engine.emitEvent(ctx, prepared, event)
	default:
		return engine.emitEvent(ctx, prepared, event)
	}
}

func modelContextWindow(prepared *preparedExecution) int64 {
	if prepared.model.ContextWindow > 0 {
		return int64(prepared.model.ContextWindow)
	}
	return prepared.session.ContextWindow
}

func (engine *Engine) finish(
	ctx context.Context,
	prepared *preparedExecution,
	status conversation.RoundStatus,
	usage conversation.TokenUsage,
	runErr error,
) {
	lock := engine.sessionLock(prepared.session.ID)
	lock.Lock()
	defer lock.Unlock()

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
	if engine.lifecycle.AfterRound != nil {
		tools := prepared.toolRuntime
		systemPrompt := prepared.systemPrompt
		state := &inframiddleware.RoundContext{
			Context:      finishCtx,
			Session:      prepared.session,
			Round:        currentRound(prepared.session, prepared.roundID),
			RoundID:      prepared.roundID,
			Provider:     &prepared.provider,
			Model:        &prepared.model,
			Tools:        &tools,
			SystemPrompt: &systemPrompt,
			Status:       status,
			Usage:        usage,
			Err:          runErr,
			Emit: func(eventCtx context.Context, event agentloop.Event) error {
				return engine.emitEvent(eventCtx, prepared, event)
			},
		}
		if err := engine.lifecycle.AfterRound(finishCtx, state); err != nil {
			engine.logger.ErrorContext(finishCtx, "after round hook failed",
				"sessionId", prepared.session.ID,
				"roundId", prepared.roundID,
				"error", err,
			)
		}
		prepared.systemPrompt = systemPrompt
		if state.Tools == nil {
			prepared.toolRuntime = nil
		} else {
			prepared.toolRuntime = *state.Tools
		}
	}

	if err := engine.emitEvent(finishCtx, prepared, agentloop.Event{
		Type:   agentloop.EventRoundEnded,
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
	execution.readyOnce.Do(func() { close(execution.sessionReady) })

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

func currentRound(session *conversation.Session, roundID uuid.UUID) *conversation.Round {
	if session == nil {
		return nil
	}
	for index := range session.Rounds {
		if session.Rounds[index].ID == roundID {
			return &session.Rounds[index]
		}
	}
	return nil
}

func (engine *Engine) roundSnapshot(
	session *conversation.Session,
	roundID uuid.UUID,
) *conversation.Round {
	if session == nil {
		return nil
	}
	lock := engine.sessionLock(session.ID)
	lock.Lock()
	defer lock.Unlock()

	round := currentRound(session, roundID)
	if round == nil {
		return nil
	}
	snapshot := *round
	snapshot.Cwd = nil
	if round.Cwd != nil {
		cwd := *round.Cwd
		snapshot.Cwd = &cwd
	}
	snapshot.Messages = append([]conversation.Message(nil), round.Messages...)
	return &snapshot
}
