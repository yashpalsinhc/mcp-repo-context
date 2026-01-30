package vectors

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed generates a vector embedding for the given text.
	Embed(text string) []float64

	// EmbedBatch generates embeddings for multiple texts.
	EmbedBatch(texts []string) [][]float64

	// Dimension returns the embedding dimension.
	Dimension() int
}

// LocalEmbedder implements Embedder using TF-IDF style local embeddings.
// This doesn't require external API calls and works offline.
type LocalEmbedder struct {
	vocabulary  map[string]int    // word -> index
	idf         map[string]float64 // word -> inverse document frequency
	dimension   int
	mu          sync.RWMutex

	// Configuration
	minWordLen  int
	maxVocabSize int
}

// LocalEmbedderConfig configures the local embedder.
type LocalEmbedderConfig struct {
	Dimension    int // Target embedding dimension (default: 256)
	MinWordLen   int // Minimum word length (default: 2)
	MaxVocabSize int // Maximum vocabulary size (default: 10000)
}

// DefaultConfig returns the default embedder configuration.
func DefaultConfig() LocalEmbedderConfig {
	return LocalEmbedderConfig{
		Dimension:    256,
		MinWordLen:   2,
		MaxVocabSize: 10000,
	}
}

// NewLocalEmbedder creates a new local embedder.
func NewLocalEmbedder(config LocalEmbedderConfig) *LocalEmbedder {
	if config.Dimension <= 0 {
		config.Dimension = 256
	}
	if config.MinWordLen <= 0 {
		config.MinWordLen = 2
	}
	if config.MaxVocabSize <= 0 {
		config.MaxVocabSize = 10000
	}

	return &LocalEmbedder{
		vocabulary:   make(map[string]int),
		idf:          make(map[string]float64),
		dimension:    config.Dimension,
		minWordLen:   config.MinWordLen,
		maxVocabSize: config.MaxVocabSize,
	}
}

// NewDefaultEmbedder creates an embedder with default settings.
func NewDefaultEmbedder() *LocalEmbedder {
	return NewLocalEmbedder(DefaultConfig())
}

// Dimension returns the embedding dimension.
func (e *LocalEmbedder) Dimension() int {
	return e.dimension
}

// BuildVocabulary builds the vocabulary from a corpus of documents.
func (e *LocalEmbedder) BuildVocabulary(documents []string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Count document frequency for each word
	docFreq := make(map[string]int)
	totalDocs := len(documents)

	for _, doc := range documents {
		words := e.tokenize(doc)
		seen := make(map[string]bool)
		for _, word := range words {
			if !seen[word] {
				docFreq[word]++
				seen[word] = true
			}
		}
	}

	// Sort words by frequency and take top maxVocabSize
	type wordFreq struct {
		word string
		freq int
	}
	wfs := make([]wordFreq, 0, len(docFreq))
	for word, freq := range docFreq {
		wfs = append(wfs, wordFreq{word, freq})
	}
	sort.Slice(wfs, func(i, j int) bool {
		return wfs[i].freq > wfs[j].freq
	})

	// Build vocabulary with top words
	e.vocabulary = make(map[string]int)
	e.idf = make(map[string]float64)

	limit := e.maxVocabSize
	if limit > len(wfs) {
		limit = len(wfs)
	}

	for i := 0; i < limit; i++ {
		word := wfs[i].word
		e.vocabulary[word] = i % e.dimension // Hash to dimension
		// IDF = log(N/df)
		e.idf[word] = math.Log(float64(totalDocs+1) / float64(wfs[i].freq+1))
	}
}

// Embed generates a vector embedding for text.
func (e *LocalEmbedder) Embed(text string) []float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if text == "" {
		return make([]float64, e.dimension)
	}

	// Tokenize
	words := e.tokenize(text)
	if len(words) == 0 {
		return make([]float64, e.dimension)
	}

	// Calculate TF (term frequency)
	tf := make(map[string]float64)
	for _, word := range words {
		tf[word]++
	}

	// Normalize TF
	for word := range tf {
		tf[word] = 1 + math.Log(tf[word]) // Log normalization
	}

	// Build embedding vector using TF-IDF
	vec := make([]float64, e.dimension)

	for word, termFreq := range tf {
		idx, ok := e.vocabulary[word]
		if !ok {
			// Unknown word - use hash
			idx = e.hashWord(word) % e.dimension
		}

		idf := e.idf[word]
		if idf == 0 {
			idf = 1.0 // Default IDF for unknown words
		}

		vec[idx] += termFreq * idf
	}

	// L2 normalize
	e.normalize(vec)

	return vec
}

// EmbedBatch generates embeddings for multiple texts.
func (e *LocalEmbedder) EmbedBatch(texts []string) [][]float64 {
	embeddings := make([][]float64, len(texts))
	for i, text := range texts {
		embeddings[i] = e.Embed(text)
	}
	return embeddings
}

// EmbedCode generates an embedding optimized for code.
func (e *LocalEmbedder) EmbedCode(code string) []float64 {
	// Preprocess code - split camelCase, snake_case, etc.
	processed := e.preprocessCode(code)
	return e.Embed(processed)
}

// preprocessCode expands code identifiers for better matching.
func (e *LocalEmbedder) preprocessCode(code string) string {
	var result strings.Builder

	// Split camelCase
	camelRe := regexp.MustCompile(`([a-z])([A-Z])`)
	code = camelRe.ReplaceAllString(code, `$1 $2`)

	// Split snake_case
	code = strings.ReplaceAll(code, "_", " ")

	// Split on common separators
	code = strings.ReplaceAll(code, ".", " ")
	code = strings.ReplaceAll(code, "/", " ")
	code = strings.ReplaceAll(code, "::", " ")

	result.WriteString(code)

	return result.String()
}

// tokenize splits text into normalized tokens.
func (e *LocalEmbedder) tokenize(text string) []string {
	// Preprocess for code
	text = e.preprocessCode(text)

	// Convert to lowercase
	text = strings.ToLower(text)

	// Split on non-alphanumeric
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() >= e.minWordLen {
				tokens = append(tokens, current.String())
			}
			current.Reset()
		}
	}

	if current.Len() >= e.minWordLen {
		tokens = append(tokens, current.String())
	}

	// Remove stopwords
	return e.removeStopwords(tokens)
}

// stopwords that should be excluded from embeddings.
var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true,
	"but": true, "in": true, "on": true, "at": true, "to": true,
	"for": true, "of": true, "with": true, "by": true, "from": true,
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"been": true, "being": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true,
	"this": true, "that": true, "these": true, "those": true,
	"it": true, "its": true, "if": true, "then": true, "else": true,
	"var": true, "let": true, "const": true, "func": true, "function": true,
}

// removeStopwords filters out common stopwords.
func (e *LocalEmbedder) removeStopwords(tokens []string) []string {
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if !stopwords[token] {
			result = append(result, token)
		}
	}
	return result
}

// hashWord generates a consistent hash for unknown words.
func (e *LocalEmbedder) hashWord(word string) int {
	hash := 0
	for i, r := range word {
		hash = hash*31 + int(r) + i
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

// normalize performs L2 normalization in place.
func (e *LocalEmbedder) normalize(vec []float64) {
	var sum float64
	for _, v := range vec {
		sum += v * v
	}
	if sum == 0 {
		return
	}
	norm := math.Sqrt(sum)
	for i := range vec {
		vec[i] /= norm
	}
}

// VocabularySize returns the current vocabulary size.
func (e *LocalEmbedder) VocabularySize() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.vocabulary)
}
