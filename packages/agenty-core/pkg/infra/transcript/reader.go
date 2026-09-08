// Package transcript reads the append-only session format without opening a
// database or initializing the data directory.
package transcript

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Record preserves physical positions, including empty and unterminated lines.
// Bytes excludes the line ending; Length includes it.
type Record struct {
	Line       int
	Offset     int64
	Length     int64
	Bytes      []byte
	Terminated bool
}

func Path(dir string, id uuid.UUID, createdAt time.Time) string {
	y, m, d := createdAt.Date()
	return filepath.Join(dir, fmt.Sprintf("%04d", y), fmt.Sprintf("%02d", int(m)),
		fmt.Sprintf("%02d", d), id.String()+".jsonl")
}

// Read visits each physical line, including a final line without a newline.
// The caller determines whether a malformed trailing fragment is an error.
func Read(ctx context.Context, input io.Reader, visit func(Record) error) error {
	reader := bufio.NewReader(input)
	var offset int64
	for lineNumber := 1; ; lineNumber++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			record := Record{
				Line: lineNumber, Offset: offset, Length: int64(len(line)),
				Terminated: line[len(line)-1] == '\n',
				Bytes:      bytes.TrimSuffix(bytes.TrimSuffix(line, []byte{'\n'}), []byte{'\r'}),
			}
			if err := visit(record); err != nil {
				return err
			}
			offset += record.Length
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read transcript at byte %d: %w", offset, readErr)
		}
	}
}
