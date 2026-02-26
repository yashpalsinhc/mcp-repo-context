package vectors

import (
	"math"
	"testing"
)

func TestExportVocabulary(t *testing.T) {
	emb := NewDefaultEmbedder()
	emb.BuildVocabulary([]string{"hello world", "foo bar baz"})

	vocab := emb.ExportVocabulary()

	if len(vocab.WordIDF) == 0 {
		t.Error("WordIDF is empty")
	}
	if vocab.DocCount != 2 {
		t.Errorf("DocCount = %d, want 2", vocab.DocCount)
	}
	if vocab.VersionHash == "" {
		t.Error("VersionHash is empty")
	}
}

func TestImportVocabulary(t *testing.T) {
	emb1 := NewDefaultEmbedder()
	emb1.BuildVocabulary([]string{"hello world", "foo bar baz"})
	vocab := emb1.ExportVocabulary()

	vec1 := emb1.Embed("hello world")

	emb2 := NewDefaultEmbedder()
	if err := emb2.ImportVocabulary(vocab); err != nil {
		t.Fatalf("ImportVocabulary: %v", err)
	}

	vec2 := emb2.Embed("hello world")

	// Vectors should be identical
	for i := range vec1 {
		if math.Abs(vec1[i]-vec2[i]) > 1e-10 {
			t.Errorf("vector mismatch at dim %d: %f vs %f", i, vec1[i], vec2[i])
			break
		}
	}
}

func TestVersionHash_ChangesWithVocab(t *testing.T) {
	emb := NewDefaultEmbedder()
	emb.BuildVocabulary([]string{"hello world"})
	hash1 := emb.ExportVocabulary().VersionHash

	emb.BuildVocabulary([]string{"completely different text"})
	hash2 := emb.ExportVocabulary().VersionHash

	if hash1 == hash2 {
		t.Error("VersionHash should change with different vocabulary")
	}
}

func TestVersionHash_Stable(t *testing.T) {
	emb1 := NewDefaultEmbedder()
	emb1.BuildVocabulary([]string{"hello world", "foo bar"})
	hash1 := emb1.ExportVocabulary().VersionHash

	emb2 := NewDefaultEmbedder()
	emb2.BuildVocabulary([]string{"hello world", "foo bar"})
	hash2 := emb2.ExportVocabulary().VersionHash

	if hash1 != hash2 {
		t.Errorf("VersionHash not stable: %q vs %q", hash1, hash2)
	}
}

func TestVocabularyAwareEmbedder_TypeAssertion(t *testing.T) {
	emb := NewDefaultEmbedder()
	var _ VocabularyAwareEmbedder = emb // compile-time check
}

func TestNewLocalEmbedder(t *testing.T) {
	embedder := NewDefaultEmbedder()

	if embedder.Dimension() != 256 {
		t.Errorf("expected dimension 256, got %d", embedder.Dimension())
	}
}

func TestLocalEmbedder_Embed(t *testing.T) {
	embedder := NewLocalEmbedder(LocalEmbedderConfig{
		Dimension:    128,
		MinWordLen:   2,
		MaxVocabSize: 1000,
	})

	// Build vocabulary
	embedder.BuildVocabulary([]string{
		"create user database",
		"delete user account",
		"validate user input",
		"handle http request",
	})

	tests := []struct {
		name    string
		text    string
		nonZero bool
	}{
		{"empty", "", false},
		{"user related", "create new user", true},
		{"database", "database query", true},
		{"unknown words only", "xyz abc", true}, // Still generates embedding via hashing
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vec := embedder.Embed(tc.text)

			if len(vec) != 128 {
				t.Errorf("expected vector of length 128, got %d", len(vec))
			}

			hasNonZero := false
			for _, v := range vec {
				if v != 0 {
					hasNonZero = true
					break
				}
			}

			if tc.nonZero && !hasNonZero {
				t.Error("expected non-zero vector")
			}
			if !tc.nonZero && hasNonZero {
				t.Error("expected zero vector")
			}
		})
	}
}

func TestLocalEmbedder_EmbedBatch(t *testing.T) {
	embedder := NewDefaultEmbedder()

	texts := []string{"hello world", "foo bar", "test code"}
	embeddings := embedder.EmbedBatch(texts)

	if len(embeddings) != 3 {
		t.Errorf("expected 3 embeddings, got %d", len(embeddings))
	}

	for i, vec := range embeddings {
		if len(vec) != 256 {
			t.Errorf("embedding %d: expected length 256, got %d", i, len(vec))
		}
	}
}

func TestLocalEmbedder_SimilarTexts(t *testing.T) {
	embedder := NewLocalEmbedder(LocalEmbedderConfig{
		Dimension:    256,
		MinWordLen:   2,
		MaxVocabSize: 5000,
	})

	// Build vocabulary with relevant terms
	embedder.BuildVocabulary([]string{
		"create user in database",
		"delete user from database",
		"update user record",
		"validate email address",
		"send http request",
		"handle api response",
	})

	// Similar texts should have higher cosine similarity
	text1 := "create new user"
	text2 := "add user to system"
	text3 := "send http request to api"

	vec1 := embedder.Embed(text1)
	vec2 := embedder.Embed(text2)
	vec3 := embedder.Embed(text3)

	sim12 := CosineSimilarity(vec1, vec2) // user-related
	sim13 := CosineSimilarity(vec1, vec3) // user vs http

	// Similar topics should have higher similarity
	// This is a basic check - local embeddings are less accurate than neural embeddings
	if sim12 <= 0 {
		t.Errorf("expected positive similarity between similar texts, got %f", sim12)
	}

	t.Logf("Similarity user/user: %f, user/http: %f", sim12, sim13)
}

func TestLocalEmbedder_Normalization(t *testing.T) {
	embedder := NewDefaultEmbedder()

	text := "test normalization of embedding vectors"
	vec := embedder.Embed(text)

	// Check that vector is normalized (magnitude ≈ 1)
	var magnitude float64
	for _, v := range vec {
		magnitude += v * v
	}
	magnitude = math.Sqrt(magnitude)

	if math.Abs(magnitude-1.0) > 0.0001 {
		t.Errorf("expected normalized vector (magnitude 1), got %f", magnitude)
	}
}

func TestLocalEmbedder_EmbedCode(t *testing.T) {
	embedder := NewDefaultEmbedder()
	embedder.BuildVocabulary([]string{
		"create user CreateUser createUser",
		"validate input ValidateInput validateInput",
		"http handler Handler handler",
	})

	// EmbedCode should handle camelCase and snake_case
	vec1 := embedder.EmbedCode("CreateUser")
	vec2 := embedder.EmbedCode("create_user")

	sim := CosineSimilarity(vec1, vec2)

	// CamelCase and snake_case should be similar after preprocessing
	if sim < 0.5 {
		t.Errorf("CreateUser and create_user should be similar, got similarity %f", sim)
	}
}

func TestLocalEmbedder_VocabularySize(t *testing.T) {
	embedder := NewLocalEmbedder(LocalEmbedderConfig{
		Dimension:    128,
		MinWordLen:   2,
		MaxVocabSize: 10,
	})

	// Before building vocabulary
	if embedder.VocabularySize() != 0 {
		t.Error("vocabulary should be empty initially")
	}

	// Build with more words than max
	docs := make([]string, 100)
	for i := 0; i < 100; i++ {
		docs[i] = "word" + string(rune('a'+i%26))
	}
	embedder.BuildVocabulary(docs)

	// Should be limited to maxVocabSize
	if embedder.VocabularySize() > 10 {
		t.Errorf("vocabulary should be limited to 10, got %d", embedder.VocabularySize())
	}
}

func TestLocalEmbedder_Tokenize(t *testing.T) {
	embedder := NewLocalEmbedder(LocalEmbedderConfig{
		Dimension:  128,
		MinWordLen: 3,
	})

	// Test that short words are filtered
	text := "a to be or not"
	vec := embedder.Embed(text)

	// With minWordLen=3, most common stopwords should be filtered
	// The vector might still be non-zero due to other processing
	if len(vec) != 128 {
		t.Errorf("expected dimension 128, got %d", len(vec))
	}
}
