package vectors

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
)

// VocabularyData holds an exported vocabulary state for persistence and versioning.
type VocabularyData struct {
	WordIDF     map[string]float64 `json:"word_idf"`
	WordSlots   map[string]int     `json:"word_slots"`   // word -> dimension slot index
	DocCount    int                `json:"doc_count"`
	BuiltAt     time.Time          `json:"built_at"`
	VersionHash string             `json:"version_hash"` // SHA256 of sorted vocabulary keys
}

// ComputeVersionHash computes a deterministic hash from vocabulary keys.
// Sort all keys alphabetically, join with newline, SHA256 the result.
func ComputeVersionHash(wordIDF map[string]float64) string {
	keys := make([]string, 0, len(wordIDF))
	for k := range wordIDF {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	joined := strings.Join(keys, "\n")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(joined)))
}

// VocabularyAwareEmbedder extends Embedder with vocabulary export/import.
type VocabularyAwareEmbedder interface {
	Embedder
	ExportVocabulary() *VocabularyData
	ImportVocabulary(vocab *VocabularyData) error
}
