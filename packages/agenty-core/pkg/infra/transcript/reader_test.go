package transcript

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReadPreservesPhysicalLines(t *testing.T) {
	t.Parallel()
	raw := "first\r\n\n" + strings.Repeat("x", 2<<20) + "\nlast"
	records := []Record{}
	if err := Read(t.Context(), strings.NewReader(raw), func(record Record) error {
		records = append(records, record)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 || string(records[0].Bytes) != "first" || records[1].Length != 1 {
		t.Fatalf("incorrect framing: %d records", len(records))
	}
	if records[2].Length != (2<<20)+1 || records[3].Terminated || string(records[3].Bytes) != "last" {
		t.Fatal("large or final line was truncated")
	}
	for i := 1; i < len(records); i++ {
		if records[i].Offset != records[i-1].Offset+records[i-1].Length {
			t.Fatal("incorrect physical offset")
		}
	}
}

func TestReadHonorsCancellationAndVisitorErrors(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Read(ctx, strings.NewReader("one\ntwo"), func(Record) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	sentinel := errors.New("stop")
	if err := Read(t.Context(), strings.NewReader("one\ntwo"), func(Record) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("visitor error = %v", err)
	}
}
