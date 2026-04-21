package consts

import (
	"math"

	"github.com/google/uuid"
)

const (
	DefaultVectorDimension = 1536
	KnowledgeChunkSize     = 512
	KnowledgeChunkOverlap  = 64
)

var (
	DefaultSystemSettingsID = uuid.MustParse("019cf9b7-a1f4-78f8-9110-15bbe177e7bc")

	Float64Epsilon = math.Nextafter(1.0, 2.0) - 1.0
)
