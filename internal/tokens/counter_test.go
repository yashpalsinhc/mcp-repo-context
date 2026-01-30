package tokens

import (
	"testing"
)

func TestTokenCounter_Count(t *testing.T) {
	counter := NewTokenCounter()

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"empty", "", 0},
		{"short", "test", 1},      // 4 chars / 4 = 1
		{"medium", "hello world", 2}, // 11 chars / 4 ~= 2
		{"code", "func main() { fmt.Println(\"Hello\") }", 9}, // 36 chars / 4 = 9
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := counter.Count(tc.text)
			if result != tc.expected {
				t.Errorf("Count(%q) = %d, expected %d", tc.text, result, tc.expected)
			}
		})
	}
}

func TestTokenCounter_CountJSON(t *testing.T) {
	counter := NewTokenCounter()

	tests := []struct {
		name  string
		value any
		minTokens int
		maxTokens int
	}{
		{"int", 123, 0, 2},
		{"string", "hello", 1, 3},
		{"struct", struct{ Name string }{Name: "test"}, 3, 10},
		{"nil", nil, 0, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := counter.CountJSON(tc.value)
			if result < tc.minTokens || result > tc.maxTokens {
				t.Errorf("CountJSON(%v) = %d, expected between %d and %d",
					tc.value, result, tc.minTokens, tc.maxTokens)
			}
		})
	}
}

func TestTokenCounter_SetRatio(t *testing.T) {
	counter := NewTokenCounter()

	// Default ratio
	result1 := counter.Count("12345678") // 8 chars / 4 = 2
	if result1 != 2 {
		t.Errorf("expected 2 with default ratio, got %d", result1)
	}

	// Change ratio
	counter.SetRatio(2.0)
	result2 := counter.Count("12345678") // 8 chars / 2 = 4
	if result2 != 4 {
		t.Errorf("expected 4 with ratio 2.0, got %d", result2)
	}

	// Invalid ratio (should be ignored)
	counter.SetRatio(0)
	result3 := counter.Count("12345678") // Should still be 4
	if result3 != 4 {
		t.Errorf("expected 4 after invalid ratio, got %d", result3)
	}
}

func TestTokenCounter_CountSlice(t *testing.T) {
	counter := NewTokenCounter()

	items := []string{"hello", "world", "test"}
	result := counter.CountSlice(items)

	// "hello" (5/4=1) + "world" (5/4=1) + "test" (4/4=1) = 3
	if result != 3 {
		t.Errorf("CountSlice expected 3, got %d", result)
	}
}

func TestTokenBudget(t *testing.T) {
	budget := NewBudget(100)

	if budget.Total != 100 || budget.Used != 0 || budget.Remaining != 100 {
		t.Error("initial budget state incorrect")
	}

	// Use some tokens
	if !budget.Use(30) {
		t.Error("Use(30) should succeed")
	}
	if budget.Used != 30 || budget.Remaining != 70 {
		t.Errorf("after Use(30): used=%d, remaining=%d", budget.Used, budget.Remaining)
	}

	// Check CanFit
	if !budget.CanFit(70) {
		t.Error("CanFit(70) should be true")
	}
	if budget.CanFit(71) {
		t.Error("CanFit(71) should be false")
	}

	// Use more than remaining
	if budget.Use(80) {
		t.Error("Use(80) should fail when only 70 remaining")
	}

	// Reset
	budget.Reset()
	if budget.Used != 0 || budget.Remaining != 100 {
		t.Error("Reset should clear used tokens")
	}
}

func TestTokenBudget_UsagePercent(t *testing.T) {
	budget := NewBudget(100)

	if budget.UsagePercent() != 0 {
		t.Error("initial usage should be 0%")
	}

	budget.Use(50)
	if budget.UsagePercent() != 50 {
		t.Errorf("expected 50%%, got %f%%", budget.UsagePercent())
	}

	// Edge case: zero total
	zeroBudget := NewBudget(0)
	if zeroBudget.UsagePercent() != 0 {
		t.Error("zero budget should have 0% usage")
	}
}

func TestNewTokenCounterWithRatio(t *testing.T) {
	// Valid ratio
	counter := NewTokenCounterWithRatio(2.0)
	result := counter.Count("12345678") // 8 chars / 2 = 4
	if result != 4 {
		t.Errorf("expected 4 with ratio 2.0, got %d", result)
	}

	// Invalid ratio (should default to 4.0)
	counter2 := NewTokenCounterWithRatio(-1.0)
	result2 := counter2.Count("12345678") // 8 chars / 4 = 2
	if result2 != 2 {
		t.Errorf("expected 2 with default ratio, got %d", result2)
	}
}
