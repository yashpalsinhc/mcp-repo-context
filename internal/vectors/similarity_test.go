package vectors

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
		epsilon  float64
	}{
		{
			name:     "identical vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 1.0,
			epsilon:  0.0001,
		},
		{
			name:     "orthogonal vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{0, 1, 0},
			expected: 0.0,
			epsilon:  0.0001,
		},
		{
			name:     "opposite vectors",
			a:        []float64{1, 0, 0},
			b:        []float64{-1, 0, 0},
			expected: -1.0,
			epsilon:  0.0001,
		},
		{
			name:     "similar vectors",
			a:        []float64{1, 1, 0},
			b:        []float64{1, 0, 0},
			expected: 0.7071, // 1/sqrt(2)
			epsilon:  0.001,
		},
		{
			name:     "empty vectors",
			a:        []float64{},
			b:        []float64{},
			expected: 0.0,
			epsilon:  0.0001,
		},
		{
			name:     "zero vectors",
			a:        []float64{0, 0, 0},
			b:        []float64{0, 0, 0},
			expected: 0.0,
			epsilon:  0.0001,
		},
		{
			name:     "mismatched dimensions",
			a:        []float64{1, 2, 3},
			b:        []float64{1, 2},
			expected: 0.0,
			epsilon:  0.0001,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := CosineSimilarity(tc.a, tc.b)
			if math.Abs(result-tc.expected) > tc.epsilon {
				t.Errorf("CosineSimilarity(%v, %v) = %f, expected %f", tc.a, tc.b, result, tc.expected)
			}
		})
	}
}

func TestEuclideanDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
		epsilon  float64
	}{
		{
			name:     "same point",
			a:        []float64{1, 2, 3},
			b:        []float64{1, 2, 3},
			expected: 0.0,
			epsilon:  0.0001,
		},
		{
			name:     "unit distance",
			a:        []float64{0, 0, 0},
			b:        []float64{1, 0, 0},
			expected: 1.0,
			epsilon:  0.0001,
		},
		{
			name:     "diagonal",
			a:        []float64{0, 0},
			b:        []float64{3, 4},
			expected: 5.0,
			epsilon:  0.0001,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := EuclideanDistance(tc.a, tc.b)
			if math.Abs(result-tc.expected) > tc.epsilon {
				t.Errorf("EuclideanDistance(%v, %v) = %f, expected %f", tc.a, tc.b, result, tc.expected)
			}
		})
	}
}

func TestDotProduct(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{4, 5, 6}

	result := DotProduct(a, b)
	expected := 1*4 + 2*5 + 3*6 // 32

	if result != float64(expected) {
		t.Errorf("DotProduct = %f, expected %f", result, float64(expected))
	}
}

func TestMagnitude(t *testing.T) {
	v := []float64{3, 4}
	result := Magnitude(v)
	expected := 5.0

	if math.Abs(result-expected) > 0.0001 {
		t.Errorf("Magnitude = %f, expected %f", result, expected)
	}
}

func TestNormalize(t *testing.T) {
	v := []float64{3, 4}
	Normalize(v)

	// Should be unit vector
	mag := Magnitude(v)
	if math.Abs(mag-1.0) > 0.0001 {
		t.Errorf("normalized magnitude = %f, expected 1.0", mag)
	}

	// Direction should be preserved
	if math.Abs(v[0]-0.6) > 0.0001 || math.Abs(v[1]-0.8) > 0.0001 {
		t.Errorf("normalized vector = %v, expected [0.6, 0.8]", v)
	}
}

func TestNormalizedCopy(t *testing.T) {
	original := []float64{3, 4}
	normalized := NormalizedCopy(original)

	// Original should be unchanged
	if original[0] != 3 || original[1] != 4 {
		t.Error("original vector was modified")
	}

	// Normalized should be unit length
	if math.Abs(Magnitude(normalized)-1.0) > 0.0001 {
		t.Error("copy was not normalized")
	}
}

func TestAdd(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{4, 5, 6}

	result := Add(a, b)
	expected := []float64{5, 7, 9}

	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("Add result[%d] = %f, expected %f", i, result[i], expected[i])
		}
	}
}

func TestScale(t *testing.T) {
	v := []float64{1, 2, 3}
	result := Scale(v, 2.0)
	expected := []float64{2, 4, 6}

	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("Scale result[%d] = %f, expected %f", i, result[i], expected[i])
		}
	}
}

func TestAverage(t *testing.T) {
	vectors := [][]float64{
		{1, 2, 3},
		{3, 4, 5},
		{5, 6, 7},
	}

	result := Average(vectors)
	expected := []float64{3, 4, 5}

	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("Average result[%d] = %f, expected %f", i, result[i], expected[i])
		}
	}
}

func TestTopKSimilar(t *testing.T) {
	query := []float64{1, 0, 0}
	candidates := [][]float64{
		{0, 1, 0},    // orthogonal
		{1, 0, 0},    // identical
		{0.5, 0.5, 0}, // somewhat similar
		{-1, 0, 0},   // opposite
	}

	result := TopKSimilar(query, candidates, 2)

	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}

	// First should be the identical vector (index 1)
	if result[0] != 1 {
		t.Errorf("top result should be index 1, got %d", result[0])
	}

	// Second should be the similar vector (index 2)
	if result[1] != 2 {
		t.Errorf("second result should be index 2, got %d", result[1])
	}
}

func TestThresholdFilter(t *testing.T) {
	query := []float64{1, 0, 0}
	candidates := [][]float64{
		{1, 0, 0},      // sim = 1.0
		{0.9, 0.1, 0},  // sim ≈ 0.99
		{0, 1, 0},      // sim = 0
		{-1, 0, 0},     // sim = -1
	}

	// Normalize candidates for accurate similarity
	for i := range candidates {
		Normalize(candidates[i])
	}

	result := ThresholdFilter(query, candidates, 0.9)

	// Should include first two (high similarity)
	if len(result) < 1 || len(result) > 2 {
		t.Errorf("expected 1-2 results above threshold, got %d", len(result))
	}

	// Should include index 0 (identical)
	found := false
	for _, idx := range result {
		if idx == 0 {
			found = true
		}
	}
	if !found {
		t.Error("should include identical vector (index 0)")
	}
}
