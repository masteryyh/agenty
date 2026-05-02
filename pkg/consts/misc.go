package consts

import (
	"math"

	"github.com/google/uuid"
)

const (
	DefaultVectorDimension = 1536
	KnowledgeChunkSize     = 512
	KnowledgeChunkOverlap  = 64

	FileReadMaxSize = 512 * 1024

	WorkspaceSearchMaxFileSize = 512 * 1024
)

var (
	DefaultSystemSettingsID = uuid.MustParse("019cf9b7-a1f4-78f8-9110-15bbe177e7bc")
	Float64Epsilon          = math.Nextafter(1.0, 2.0) - 1.0
	BlockingPaths           = map[string]struct{}{
		"/dev/zero":       {},
		"/dev/random":     {},
		"/dev/urandom":    {},
		"/dev/full":       {},
		"/dev/stdin":      {},
		"/dev/tty":        {},
		"/dev/console":    {},
		"/dev/stdout":     {},
		"/dev/stderr":     {},
		"/dev/fd/0":       {},
		"/dev/fd/1":       {},
		"/dev/fd/2":       {},
		"/proc/self/fd/0": {},
		"/proc/self/fd/1": {},
		"/proc/self/fd/2": {},
		"/proc/self/mem":  {},
	}
	SensitiveFileToolPathPrefixes = []string{
		"/bin",
		"/boot",
		"/dev",
		"/etc",
		"/lib",
		"/lib64",
		"/private/etc",
		"/private/var/run",
		"/proc",
		"/root",
		"/run",
		"/sbin",
		"/sys",
		"/usr/bin",
		"/usr/sbin",
		"/var/run",
		`c:\program files`,
		`c:\program files (x86)`,
		`c:\programdata`,
		`c:\windows`,
	}
)
