package nlp

import (
	"strings"
	"unicode"
)

// Stem applies a simplified Porter stemmer to a single word.
// Empty input returns empty. Single-character words return unchanged.
// ASCII input is lowercased. Unicode identifiers pass through unchanged.
func Stem(word string) string {
	if len(word) <= 1 {
		return strings.ToLower(word)
	}

	// Check if ASCII-only
	for _, r := range word {
		if r > 127 {
			return word // pass through unicode
		}
	}

	w := strings.ToLower(word)
	if len(w) <= 2 {
		return w
	}

	w = step1a(w)
	w = step1b(w)
	w = step1c(w)
	w = step2(w)
	w = step3(w)
	w = step4(w)
	w = step5a(w)
	w = step5b(w)

	return w
}

// SplitCamelCase splits a Go-style identifier into word parts.
// "ServeHTTP" -> ["Serve", "HTTP"], "getURL" -> ["get", "URL"]
func SplitCamelCase(s string) []string {
	if s == "" {
		return nil
	}

	runes := []rune(s)
	var parts []string
	start := 0

	for i := 1; i < len(runes); i++ {
		prev := runes[i-1]
		cur := runes[i]

		// lowercase -> uppercase: split before uppercase
		if unicode.IsLower(prev) && unicode.IsUpper(cur) {
			parts = append(parts, string(runes[start:i]))
			start = i
			continue
		}

		// uppercase -> uppercase -> lowercase: split before last uppercase
		if i+1 < len(runes) && unicode.IsUpper(prev) && unicode.IsUpper(cur) && unicode.IsLower(runes[i+1]) {
			if start < i {
				parts = append(parts, string(runes[start:i]))
				start = i
			}
		}
	}

	parts = append(parts, string(runes[start:]))
	return parts
}

// StemAll applies Stem to each word.
func StemAll(words []string) []string {
	result := make([]string, len(words))
	for i, w := range words {
		result[i] = Stem(w)
	}
	return result
}

// StemIdentifier splits camelCase and stems each part.
func StemIdentifier(ident string) []string {
	parts := SplitCamelCase(ident)
	return StemAll(parts)
}

// --- Porter Stemmer helpers ---

func isVowel(c byte) bool {
	return c == 'a' || c == 'e' || c == 'i' || c == 'o' || c == 'u'
}

func hasVowel(s string) bool {
	for i := 0; i < len(s); i++ {
		if isVowel(s[i]) {
			return true
		}
	}
	return false
}

// measure returns the "m" value of a stem (number of VC sequences).
func measure(s string) int {
	if len(s) == 0 {
		return 0
	}
	m := 0
	i := 0
	// skip initial consonants
	for i < len(s) && !isVowel(s[i]) {
		i++
	}
	for i < len(s) {
		// skip vowels
		for i < len(s) && isVowel(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		// skip consonants
		for i < len(s) && !isVowel(s[i]) {
			i++
		}
		m++
	}
	return m
}

func endsWith(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

func stem(s, suffix string) string {
	return s[:len(s)-len(suffix)]
}

func step1a(w string) string {
	if endsWith(w, "sses") {
		return stem(w, "sses") + "ss"
	}
	if endsWith(w, "ies") {
		return stem(w, "ies") + "i"
	}
	if endsWith(w, "ss") {
		return w
	}
	if endsWith(w, "s") {
		return w[:len(w)-1]
	}
	return w
}

func step1b(w string) string {
	if endsWith(w, "eed") {
		s := stem(w, "eed")
		if measure(s) > 0 {
			return s + "ee"
		}
		return w
	}

	var stemPart string
	changed := false
	if endsWith(w, "ed") {
		stemPart = stem(w, "ed")
		if hasVowel(stemPart) {
			w = stemPart
			changed = true
		}
	} else if endsWith(w, "ing") {
		stemPart = stem(w, "ing")
		if hasVowel(stemPart) {
			w = stemPart
			changed = true
		}
	}

	if changed {
		if endsWith(w, "at") || endsWith(w, "bl") || endsWith(w, "iz") {
			w += "e"
		} else if len(w) >= 2 && w[len(w)-1] == w[len(w)-2] &&
			w[len(w)-1] != 'l' && w[len(w)-1] != 's' && w[len(w)-1] != 'z' {
			w = w[:len(w)-1]
		} else if measure(w) == 1 && cvc(w) {
			w += "e"
		}
	}

	return w
}

func cvc(s string) bool {
	n := len(s)
	if n < 3 {
		return false
	}
	c1, v, c2 := s[n-3], s[n-2], s[n-1]
	if isVowel(c1) || !isVowel(v) || isVowel(c2) {
		return false
	}
	// don't match w, x, y
	return c2 != 'w' && c2 != 'x' && c2 != 'y'
}

func step1c(w string) string {
	if endsWith(w, "y") {
		s := stem(w, "y")
		if hasVowel(s) {
			return s + "i"
		}
	}
	return w
}

func step2(w string) string {
	replacements := []struct{ suffix, replacement string }{
		{"ational", "ate"},
		{"tional", "tion"},
		{"enci", "ence"},
		{"anci", "ance"},
		{"izer", "ize"},
		{"abli", "able"},
		{"alli", "al"},
		{"entli", "ent"},
		{"eli", "e"},
		{"ousli", "ous"},
		{"ization", "ize"},
		{"ation", "ate"},
		{"ator", "ate"},
		{"alism", "al"},
		{"iveness", "ive"},
		{"fulness", "ful"},
		{"ousness", "ous"},
		{"aliti", "al"},
		{"iviti", "ive"},
		{"biliti", "ble"},
	}
	for _, r := range replacements {
		if endsWith(w, r.suffix) {
			s := stem(w, r.suffix)
			if measure(s) > 0 {
				return s + r.replacement
			}
			return w
		}
	}
	return w
}

func step3(w string) string {
	replacements := []struct{ suffix, replacement string }{
		{"icate", "ic"},
		{"ative", ""},
		{"alize", "al"},
		{"iciti", "ic"},
		{"ical", "ic"},
		{"ful", ""},
		{"ness", ""},
	}
	for _, r := range replacements {
		if endsWith(w, r.suffix) {
			s := stem(w, r.suffix)
			if measure(s) > 0 {
				return s + r.replacement
			}
			return w
		}
	}
	return w
}

func step4(w string) string {
	suffixes := []string{
		"al", "ance", "ence", "er", "ic", "able", "ible", "ant",
		"ement", "ment", "ent", "ion", "ou", "ism", "ate", "iti",
		"ous", "ive", "ize",
	}
	for _, suf := range suffixes {
		if endsWith(w, suf) {
			s := stem(w, suf)
			if suf == "ion" {
				if len(s) > 0 && (s[len(s)-1] == 's' || s[len(s)-1] == 't') {
					if measure(s) > 1 {
						return s
					}
				}
			} else {
				if measure(s) > 1 {
					return s
				}
			}
			return w
		}
	}
	return w
}

func step5a(w string) string {
	if endsWith(w, "e") {
		s := stem(w, "e")
		if measure(s) > 1 {
			return s
		}
		if measure(s) == 1 && !cvc(s) {
			return s
		}
	}
	return w
}

func step5b(w string) string {
	if measure(w) > 1 && len(w) >= 2 && w[len(w)-1] == 'l' && w[len(w)-2] == 'l' {
		return w[:len(w)-1]
	}
	return w
}
