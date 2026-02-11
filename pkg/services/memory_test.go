package services

import (
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/models"
)

func TestNormalizeVector(t *testing.T) {
	vec := []float32{3.0, 4.0}
	normalized := normalizeVector(vec)

	var norm float64
	for _, v := range normalized {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)

	if math.Abs(norm-1.0) > 1e-6 {
		t.Fatalf("expected unit norm, got %f", norm)
	}

	expectedFirst := float32(3.0 / 5.0)
	expectedSecond := float32(4.0 / 5.0)
	if math.Abs(float64(normalized[0]-expectedFirst)) > 1e-6 {
		t.Fatalf("expected %f, got %f", expectedFirst, normalized[0])
	}
	if math.Abs(float64(normalized[1]-expectedSecond)) > 1e-6 {
		t.Fatalf("expected %f, got %f", expectedSecond, normalized[1])
	}
}

func TestNormalizeVectorZero(t *testing.T) {
	vec := []float32{0.0, 0.0, 0.0}
	normalized := normalizeVector(vec)

	for i, v := range normalized {
		if v != 0 {
			t.Fatalf("expected 0 at index %d, got %f", i, v)
		}
	}
}

func TestRRFMergeEmpty(t *testing.T) {
	result := rrfMerge(5)
	if len(result) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result))
	}
}

func TestRRFMergeSingleSource(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	items := []rankedItem{
		{id: id1, memory: &models.MemoryDto{ID: id1, Content: "first"}, rank: 1},
		{id: id2, memory: &models.MemoryDto{ID: id2, Content: "second"}, rank: 2},
	}

	result := rrfMerge(5, items)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	if result[0].Memory.Content != "first" {
		t.Fatalf("expected first result to be 'first', got '%s'", result[0].Memory.Content)
	}
	if result[1].Memory.Content != "second" {
		t.Fatalf("expected second result to be 'second', got '%s'", result[1].Memory.Content)
	}

	if result[0].Score <= result[1].Score {
		t.Fatalf("expected first score > second score, got %f <= %f", result[0].Score, result[1].Score)
	}
}

func TestRRFMergeMultipleSources(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()

	source1 := []rankedItem{
		{id: id1, memory: &models.MemoryDto{ID: id1, Content: "common"}, rank: 1},
		{id: id2, memory: &models.MemoryDto{ID: id2, Content: "only-vector"}, rank: 2},
	}

	source2 := []rankedItem{
		{id: id1, memory: &models.MemoryDto{ID: id1, Content: "common"}, rank: 1},
		{id: id3, memory: &models.MemoryDto{ID: id3, Content: "only-text"}, rank: 2},
	}

	result := rrfMerge(5, source1, source2)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	if result[0].Memory.Content != "common" {
		t.Fatalf("expected 'common' to be ranked first (appears in both sources), got '%s'", result[0].Memory.Content)
	}

	expectedCommonScore := 2.0 / float64(rrfK+1)
	if result[0].Score != expectedCommonScore {
		t.Fatalf("expected common score %f, got %f", expectedCommonScore, result[0].Score)
	}
}

func TestRRFMergeLimit(t *testing.T) {
	items := make([]rankedItem, 10)
	for i := range 10 {
		id := uuid.New()
		items[i] = rankedItem{
			id:     id,
			memory: &models.MemoryDto{ID: id, Content: "item"},
			rank:   i + 1,
		}
	}

	result := rrfMerge(3, items)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
}

func TestRRFMergeScoreOrdering(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	source1 := []rankedItem{
		{id: id1, memory: &models.MemoryDto{ID: id1, Content: "a"}, rank: 1},
		{id: id2, memory: &models.MemoryDto{ID: id2, Content: "b"}, rank: 3},
	}

	source2 := []rankedItem{
		{id: id2, memory: &models.MemoryDto{ID: id2, Content: "b"}, rank: 1},
		{id: id1, memory: &models.MemoryDto{ID: id1, Content: "a"}, rank: 3},
	}

	result := rrfMerge(5, source1, source2)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	if result[0].Score != result[1].Score {
		t.Fatalf("expected equal scores for symmetric ranking, got %f and %f", result[0].Score, result[1].Score)
	}
}

func TestRRFMergeDeduplication(t *testing.T) {
	id := uuid.New()

	source1 := []rankedItem{
		{id: id, memory: &models.MemoryDto{ID: id, Content: "same"}, rank: 1},
	}

	source2 := []rankedItem{
		{id: id, memory: &models.MemoryDto{ID: id, Content: "same"}, rank: 1},
	}

	source3 := []rankedItem{
		{id: id, memory: &models.MemoryDto{ID: id, Content: "same"}, rank: 1},
	}

	result := rrfMerge(5, source1, source2, source3)
	if len(result) != 1 {
		t.Fatalf("expected 1 result after dedup, got %d", len(result))
	}

	expectedScore := 3.0 / float64(rrfK+1)
	if result[0].Score != expectedScore {
		t.Fatalf("expected score %f, got %f", expectedScore, result[0].Score)
	}
}
