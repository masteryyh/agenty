package services

import (
	"math"
	"testing"
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
