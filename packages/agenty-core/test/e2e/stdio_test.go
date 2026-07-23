//go:build e2e

package e2e_test

import "testing"

func TestStdioEmptyInputExitsCleanly(t *testing.T) {
	t.Parallel()
	core := startCore(t)
	core.Close(t)
}

func TestStdioProcessesFinalLineWithoutNewline(t *testing.T) {
	t.Parallel()
	core := startCore(t)

	response := decodeRawResponse(t, core.ExchangeFinalRaw(t,
		`{"jsonrpc":"2.0","id":"final-line","method":"agent.list"}`,
	))
	requireSuccess(t, response)
	if string(response.ID) != `"final-line"` {
		t.Fatalf("final-line response id = %s", response.ID)
	}
	core.Close(t)
}
