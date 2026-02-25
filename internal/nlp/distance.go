package nlp

// LevenshteinDistance computes the edit distance between two strings.
func LevenshteinDistance(a, b string) int {
	return LevenshteinDistanceMax(a, b, max(len(a), len(b)))
}

// LevenshteinDistanceMax computes edit distance with early termination.
// Returns maxDist+1 if actual distance exceeds maxDist.
func LevenshteinDistanceMax(a, b string, maxDist int) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	if la-lb > maxDist || lb-la > maxDist {
		return maxDist + 1
	}

	// Two-row optimization
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		rowMin := curr[0]

		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,       // deletion
				curr[j-1]+1,     // insertion
				prev[j-1]+cost,  // substitution
			)
			if curr[j] < rowMin {
				rowMin = curr[j]
			}
		}

		if rowMin > maxDist {
			return maxDist + 1
		}

		prev, curr = curr, prev
	}

	return prev[lb]
}

// FuzzyMatch returns all candidates within maxDistance of input.
func FuzzyMatch(input string, candidates []string, maxDistance int) []string {
	var result []string
	for _, c := range candidates {
		if LevenshteinDistanceMax(input, c, maxDistance) <= maxDistance {
			result = append(result, c)
		}
	}
	if result == nil {
		result = []string{}
	}
	return result
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
