package session_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/event"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

func TestEnginePermissionModePersistenceBoundary(t *testing.T) {
	for _, scenario := range []string{"save failure", "request cancelled", "transcript committed", "notification failure"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newExecutionFixture(t, 8192)
			session := fixture.createSession(t)
			caller := &scriptedCaller{
				responses: []*modelcall.ModelCallResponse{{Content: conversation.Text("done")}},
				started:   make(chan struct{}, 1), release: make(chan struct{}),
			}
			var active *conversation.Session
			requestCtx, cancelRequest := context.WithCancel(t.Context())
			defer cancelRequest()
			failure := errors.New("persistence boundary failure")
			attempts := 0
			persist := storage.NewSessionMiddleware(fixture.sessions).OnEvent
			engine, err := infrasession.NewEngine(t.Context(), infrasession.Dependencies{
				Sessions: fixture.sessions, Catalog: fixture.catalog, Tools: fixture.registry,
				InvokeModel: caller.Call,
				LoopHooks: agentloop.LoopHooks{BeforeModelCall: func(_ context.Context, state *agentloop.ModelCallState) error {
					active = state.Session
					return nil
				}},
				Lifecycle: middleware.LifecycleHooks{OnEvent: func(ctx context.Context, state *middleware.EventContext) error {
					if state.Event.Type != agentloop.EventPermissionModeChanged {
						return persist(ctx, state)
					}
					attempts++
					if active.CurrentPermissionMode() != conversation.PermissionAsk {
						t.Error("uncommitted mode became visible to tool approval")
					}
					if attempts > 1 {
						return persist(ctx, state)
					}
					switch scenario {
					case "request cancelled":
						cancelRequest()
						return ctx.Err()
					case "transcript committed":
						if err := fixture.sessions.Save(ctx, state.Session); err != nil {
							return err
						}
					case "notification failure":
						if err := persist(ctx, state); err != nil {
							return err
						}
					}
					return failure
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := engine.Shutdown(ctx); err != nil {
					t.Error(err)
				}
			})
			if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("hello")); err != nil {
				t.Fatal(err)
			}
			<-caller.started
			before := active.Snapshot()
			if _, err := engine.SetPermissionMode(requestCtx, session.ID.String(), conversation.PermissionYolo); err == nil {
				t.Fatal("expected permission update error")
			}
			committed := scenario == "transcript committed" || scenario == "notification failure"
			wantMode := conversation.PermissionAsk
			if committed {
				wantMode = conversation.PermissionYolo
			} else if !reflect.DeepEqual(active.Snapshot(), before) {
				t.Fatal("failed permission update changed the active session")
			}
			persisted, err := fixture.sessions.Load(t.Context(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if active.CurrentPermissionMode() != wantMode || persisted.CurrentPermissionMode() != wantMode || len(active.PendingEvents()) != 0 {
				t.Fatalf("active and persisted permission mode must be %s with no pending changes", wantMode)
			}
			if _, err := engine.SetPermissionMode(t.Context(), session.ID.String(), conversation.PermissionYolo); err != nil {
				t.Fatal(err)
			}
			wantAttempts := 2
			if committed {
				wantAttempts = 1
			}
			if attempts != wantAttempts || active.CurrentPermissionMode() != conversation.PermissionYolo {
				t.Fatalf("attempts = %d, active mode = %s", attempts, active.CurrentPermissionMode())
			}
			changes := 0
			for _, event := range fixture.sessions.events[session.ID] {
				if change, ok := event.(conversation.SessionPermissionModeChanged); ok {
					changes++
					if change.RoundID != uuid.Nil {
						t.Fatal("permission update must remain session scoped")
					}
				}
			}
			if changes != 1 {
				t.Fatalf("persisted changes = %d, want one", changes)
			}
		})
	}
}

func TestEngineAppliesLatestPermissionBeforeModelHook(t *testing.T) {
	for _, cancelRound := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancelled=%v", cancelRound), func(t *testing.T) {
			t.Parallel()
			fixture := newExecutionFixture(t, 8192)
			session := fixture.createSession(t)
			started := make(chan struct{})
			release := make(chan struct{})
			var calls atomic.Int32
			var hooks atomic.Int32
			engine, err := infrasession.NewEngine(t.Context(), infrasession.Dependencies{
				Sessions: fixture.sessions, Catalog: fixture.catalog, Tools: fixture.registry,
				Lifecycle: middleware.LifecycleHooks{OnEvent: storage.NewSessionMiddleware(fixture.sessions).OnEvent},
				LoopHooks: agentloop.LoopHooks{BeforeModelCall: func(ctx context.Context, state *agentloop.ModelCallState) error {
					want := conversation.PermissionAuto
					if hooks.Add(1) == 2 {
						want = conversation.PermissionYolo
					}
					persisted, err := fixture.sessions.Load(ctx, session.ID)
					if err != nil {
						return err
					}
					if state.Session.CurrentPermissionMode() != want || persisted.CurrentPermissionMode() != want {
						return fmt.Errorf("mode was not applied and persisted before hook: want %s", want)
					}
					last := state.Request.Messages[len(state.Request.Messages)-1]
					if !strings.Contains(last.Content[0].(conversation.TextBlock).Text, "<permission-mode>"+string(want)) {
						return fmt.Errorf("request omitted permission update before hook")
					}
					return nil
				}},
				InvokeModel: func(ctx context.Context, _ modelcall.ModelCallConfig, _ modelcall.ModelCallRequest, _ modelcall.ModelCallStreamHandler) (*modelcall.ModelCallResponse, error) {
					if calls.Add(1) == 1 {
						close(started)
						select {
						case <-release:
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					}
					return &modelcall.ModelCallResponse{Content: conversation.Text("done")}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := engine.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			})

			for _, mode := range []conversation.PermissionMode{
				conversation.PermissionAuto, conversation.PermissionYolo, conversation.PermissionAsk,
			} {
				if _, err := engine.SetPermissionMode(t.Context(), session.ID.String(), mode); err != nil {
					t.Fatal(err)
				}
			}
			if len(fixture.sessions.events[session.ID]) != 4 {
				t.Fatal("permission changes must be persisted immediately")
			}
			if _, err := engine.SetPermissionMode(t.Context(), session.ID.String(), conversation.PermissionAuto); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("first")); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("model did not start")
			}
			for _, mode := range []conversation.PermissionMode{
				conversation.PermissionYolo, conversation.PermissionAsk, conversation.PermissionYolo,
			} {
				updated, err := engine.SetPermissionMode(t.Context(), session.ID.String(), mode)
				if err != nil {
					t.Fatal(err)
				}
				if updated.CurrentPermissionMode() != mode {
					t.Fatal("permission mode did not apply immediately")
				}
			}
			if cancelRound {
				if _, err := engine.Stop(t.Context(), session.ID.String()); err != nil {
					t.Fatal(err)
				}
			} else {
				close(release)
			}
			waitForExecution(t, engine, session.ID)
			if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("second")); err != nil {
				t.Fatal(err)
			}
			waitForExecution(t, engine, session.ID)
			persisted, err := fixture.sessions.Load(t.Context(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if persisted.Rounds[1].Status != conversation.RoundCompleted || hooks.Load() != 2 {
				t.Fatalf("second round = %+v, hooks = %d", persisted.Rounds[1], hooks.Load())
			}
			changes := 0
			for _, event := range fixture.sessions.events[session.ID] {
				if _, ok := event.(conversation.SessionPermissionModeChanged); ok {
					changes++
				}
			}
			if changes != 7 {
				t.Fatalf("permission events = %d", changes)
			}
		})
	}
}

func TestEnginePermissionChangesDuringOtherSessionRounds(t *testing.T) {
	fixture := newExecutionFixture(t, 8192)
	engine := fixture.newEngine(t, func(context.Context, modelcall.ModelCallConfig, modelcall.ModelCallRequest, modelcall.ModelCallStreamHandler) (*modelcall.ModelCallResponse, error) {
		return &modelcall.ModelCallResponse{Content: conversation.Text("done")}, nil
	})
	changing := fixture.createSession(t)
	running := fixture.createSession(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		mode := conversation.PermissionAuto
		for ctx.Err() == nil {
			if _, err := engine.SetPermissionMode(ctx, changing.ID.String(), mode); err != nil {
				if errors.Is(err, context.Canceled) {
					break
				}
				done <- err
				return
			}
			if mode == conversation.PermissionAuto {
				mode = conversation.PermissionAsk
			} else {
				mode = conversation.PermissionAuto
			}
		}
		done <- nil
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	for range 30 {
		if _, err := engine.Start(t.Context(), running.ID.String(), conversation.Text("hello")); err != nil {
			t.Fatal(err)
		}
		waitForExecution(t, engine, running.ID)
	}
}

func TestEngineDeleteDoesNotDeadlockWithPermissionEventBarrier(t *testing.T) {
	fixture := newExecutionFixture(t, 8192)
	session := fixture.createSession(t)
	broker := event.NewStreamBroker(nil)
	manager := middleware.NewManager()
	if err := manager.Register(storage.NewSessionMiddleware(fixture.sessions)); err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(event.NewSessionEventMiddleware(broker)); err != nil {
		t.Fatal(err)
	}
	chain, err := manager.Compile()
	if err != nil {
		t.Fatal(err)
	}
	chain.SetEventBarrier(broker.TopicBarrier)
	emitting := make(chan struct{})
	engine, err := infrasession.NewEngine(t.Context(), infrasession.Dependencies{
		Sessions: fixture.sessions, Catalog: fixture.catalog, Tools: fixture.registry,
		Lifecycle: middleware.LifecycleHooks{OnEvent: func(ctx context.Context, state *middleware.EventContext) error {
			close(emitting)
			return chain.OnEvent(ctx, state)
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Shutdown(context.Background())

	// DELETE enters with the topic barrier held; the permission update holds
	// the session lock and is about to wait for that same barrier.
	_, unlockBarrier := broker.TopicBarrier(t.Context(), "session:"+session.ID.String())
	releaseBarrier := sync.OnceFunc(unlockBarrier)
	defer releaseBarrier()
	changed := make(chan error, 1)
	go func() {
		_, err := engine.SetPermissionMode(t.Context(), session.ID.String(), conversation.PermissionAuto)
		changed <- err
	}()
	<-emitting
	service := application.NewSessionService(fixture.sessions, application.WithSessionExecutionState(engine))
	deleted := make(chan error, 1)
	go func() { deleted <- service.Delete(t.Context(), session.ID.String()) }()
	select {
	case err := <-deleted:
		if appErrorCode(err) != application.CodeAlreadyExists {
			t.Fatalf("delete = %v, want busy", err)
		}
	case <-time.After(time.Second):
		t.Fatal("delete waited on the session lock while holding its event barrier")
	}
	releaseBarrier()
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(t.Context(), session.ID.String()); err != nil {
		t.Fatal(err)
	}
}
