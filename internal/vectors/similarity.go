package vectors

import (
	"math"
	"sort"
)

// CosineSimilarity calculates the cosine similarity between two vectors.
// Returns a value between -1 and 1, where 1 means identical direction.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64

	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// EuclideanDistance calculates the Euclidean distance between two vectors.
func EuclideanDistance(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return math.MaxFloat64
	}

	var sum float64
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return math.Sqrt(sum)
}

// DotProduct calculates the dot product of two vectors.
func DotProduct(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}

	return sum
}

// Magnitude calculates the magnitude (L2 norm) of a vector.
func Magnitude(v []float64) float64 {
	var sum float64
	for _, val := range v {
		sum += val * val
	}
	return math.Sqrt(sum)
}

// Normalize normalizes a vector to unit length in place.
func Normalize(v []float64) {
	mag := Magnitude(v)
	if mag == 0 {
		return
	}
	for i := range v {
		v[i] /= mag
	}
}

// NormalizedCopy returns a normalized copy of the vector.
func NormalizedCopy(v []float64) []float64 {
	result := make([]float64, len(v))
	copy(result, v)
	Normalize(result)
	return result
}

// Add adds two vectors and returns the result.
func Add(a, b []float64) []float64 {
	if len(a) != len(b) {
		return nil
	}
	result := make([]float64, len(a))
	for i := range a {
		result[i] = a[i] + b[i]
	}
	return result
}

// Scale multiplies a vector by a scalar and returns the result.
func Scale(v []float64, scalar float64) []float64 {
	result := make([]float64, len(v))
	for i := range v {
		result[i] = v[i] * scalar
	}
	return result
}

// Average computes the element-wise average of multiple vectors.
func Average(vectors [][]float64) []float64 {
	if len(vectors) == 0 {
		return nil
	}

	dim := len(vectors[0])
	result := make([]float64, dim)

	for _, v := range vectors {
		if len(v) != dim {
			continue
		}
		for i := range v {
			result[i] += v[i]
		}
	}

	n := float64(len(vectors))
	for i := range result {
		result[i] /= n
	}

	return result
}

// TopKSimilar finds the top-k most similar vectors from a list.
func TopKSimilar(query []float64, candidates [][]float64, k int) []int {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}

	type scoredIndex struct {
		index      int
		similarity float64
	}

	scores := make([]scoredIndex, len(candidates))
	for i, candidate := range candidates {
		scores[i] = scoredIndex{
			index:      i,
			similarity: CosineSimilarity(query, candidate),
		}
	}

	// Sort by similarity descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].similarity > scores[j].similarity
	})

	// Return top k indices
	if k > len(scores) {
		k = len(scores)
	}

	result := make([]int, k)
	for i := 0; i < k; i++ {
		result[i] = scores[i].index
	}

	return result
}

// ThresholdFilter returns indices of vectors with similarity above threshold.
func ThresholdFilter(query []float64, candidates [][]float64, threshold float64) []int {
	var result []int
	for i, candidate := range candidates {
		if CosineSimilarity(query, candidate) >= threshold {
			result = append(result, i)
		}
	}
	return result
}
