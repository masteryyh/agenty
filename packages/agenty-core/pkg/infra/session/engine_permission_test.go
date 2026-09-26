package session_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

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
			if engine.PendingPermissionMode(session.ID) != "" || len(fixture.sessions.events[session.ID]) != 1 {
				t.Fatal("switching back to the effective mode must discard all staged changes")
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
				if updated.CurrentPermissionMode() != conversation.PermissionAuto {
					t.Fatal("staged mode affected the current model/tool iteration")
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
			if engine.PendingPermissionMode(session.ID) != conversation.PermissionYolo {
				t.Fatal("pending mode was lost at round completion")
			}
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
			if changes != 2 || engine.PendingPermissionMode(session.ID) != "" {
				t.Fatalf("permission events = %d, pending = %s", changes, engine.PendingPermissionMode(session.ID))
			}
		})
	}
}
