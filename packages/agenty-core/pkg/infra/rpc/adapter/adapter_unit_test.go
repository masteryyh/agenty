package adapter

import (
	"errors"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

func TestToRPCError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		code int
	}{
		{name: "not found", err: application.NotFound("missing"), code: rpc.ErrCodeNotFound},
		{name: "already exists", err: application.AlreadyExists("duplicate"), code: rpc.ErrCodeAlreadyExists},
		{name: "validation", err: application.Validation("invalid"), code: rpc.ErrCodeInvalidParams},
		{name: "internal", err: application.Internal("failed"), code: rpc.ErrCodeInternalError},
		{name: "unclassified", err: errors.New("failed"), code: rpc.ErrCodeInternalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toRPCError(tt.err)
			if got.Code != tt.code {
				t.Errorf("code = %d, want %d", got.Code, tt.code)
			}
		})
	}
}
