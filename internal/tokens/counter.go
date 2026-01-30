package tokens

import (
	"encoding/json"
	"sync"
)

// TokenCounter counts tokens using a character-based approximation.
// Uses ~4 characters per token which is a reasonable approximation for code.
// For production use with exact counts, integrate tiktoken-go.
type TokenCounter struct {
	// CharPerToken is the average characters per token (default: 4)
	CharPerToken float64

	mu sync.RWMutex
}

// NewTokenCounter creates a new token counter with default settings.
func NewTokenCounter() *TokenCounter {
	return &TokenCounter{
		CharPerToken: 4.0, // Claude's approximate ratio for code
	}
}

// NewTokenCounterWithRatio creates a counter with custom char-per-token ratio.
func NewTokenCounterWithRatio(ratio float64) *TokenCounter {
	if ratio <= 0 {
		ratio = 4.0
	}
	return &TokenCounter{
		CharPerToken: ratio,
	}
}

// Count estimates the token count for text.
func (c *TokenCounter) Count(text string) int {
	if text == "" {
		return 0
	}
	c.mu.RLock()
	ratio := c.CharPerToken
	c.mu.RUnlock()

	return int(float64(len(text)) / ratio)
}

// CountJSON estimates tokens for a JSON-serializable value.
func (c *TokenCounter) CountJSON(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return c.Count(string(data))
}

// CountSlice estimates tokens for a slice of strings.
func (c *TokenCounter) CountSlice(items []string) int {
	total := 0
	for _, item := range items {
		total += c.Count(item)
	}
	return total
}

// SetRatio updates the character-per-token ratio.
func (c *TokenCounter) SetRatio(ratio float64) {
	if ratio <= 0 {
		return
	}
	c.mu.Lock()
	c.CharPerToken = ratio
	c.mu.Unlock()
}

// TokenBudget represents a token allocation.
type TokenBudget struct {
	Total     int `json:"total"`
	Used      int `json:"used"`
	Remaining int `json:"remaining"`
}

// NewBudget creates a new token budget.
func NewBudget(total int) *TokenBudget {
	return &TokenBudget{
		Total:     total,
		Used:      0,
		Remaining: total,
	}
}

// Use consumes tokens from the budget. Returns true if within budget.
func (b *TokenBudget) Use(tokens int) bool {
	if b.Remaining < tokens {
		return false
	}
	b.Used += tokens
	b.Remaining = b.Total - b.Used
	return true
}

// CanFit checks if tokens would fit in remaining budget.
func (b *TokenBudget) CanFit(tokens int) bool {
	return b.Remaining >= tokens
}

// Reset resets the budget to full.
func (b *TokenBudget) Reset() {
	b.Used = 0
	b.Remaining = b.Total
}

// UsagePercent returns the percentage of budget used.
func (b *TokenBudget) UsagePercent() float64 {
	if b.Total == 0 {
		return 0
	}
	return float64(b.Used) / float64(b.Total) * 100
}
