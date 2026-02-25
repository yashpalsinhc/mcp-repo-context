package nlp

import "strings"

var stopWords = map[string]bool{
	"get": true, "set": true, "new": true, "make": true,
	"handle": true, "error": true, "init": true, "close": true,
	"open": true, "read": true, "write": true, "string": true,
	"reset": true, "is": true, "has": true,
}

// IsStopWord checks if a word is in the stop word list.
func IsStopWord(word string) bool {
	return stopWords[strings.ToLower(word)]
}

// ConceptSimilarity returns the fraction of non-stop words present in domainProfile.
func ConceptSimilarity(words []string, domainProfile map[string]bool) float64 {
	if len(words) == 0 || len(domainProfile) == 0 {
		return 0.0
	}

	var nonStop int
	var matches int
	for _, w := range words {
		if IsStopWord(w) {
			continue
		}
		nonStop++
		stemmed := Stem(w)
		if domainProfile[stemmed] {
			matches++
		}
	}

	if nonStop == 0 {
		return 0.0
	}

	return float64(matches) / float64(nonStop)
}

// BuildDomainProfile builds a stemmed domain profile from a list of identifiers.
func BuildDomainProfile(names []string) map[string]bool {
	profile := make(map[string]bool)
	for _, name := range names {
		parts := SplitCamelCase(name)
		for _, p := range parts {
			s := Stem(p)
			if s != "" && !IsStopWord(p) {
				profile[s] = true
			}
		}
	}
	return profile
}
