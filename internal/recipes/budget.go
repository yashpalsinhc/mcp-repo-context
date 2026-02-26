package recipes

// BudgetAllocation defines how to split token budget across context passes.
type BudgetAllocation struct {
	Structural float64 // proportion for structural data pass
	Vector     float64 // proportion for vector search pass
	Keyword    float64 // proportion for keyword search pass
	Metadata   float64 // proportion for metadata pass
}

// DefaultBudgetAllocation returns the default allocation (40/30/20/10).
func DefaultBudgetAllocation() BudgetAllocation {
	return BudgetAllocation{
		Structural: 0.40,
		Vector:     0.30,
		Keyword:    0.20,
		Metadata:   0.10,
	}
}

// NoBudgetAllocation returns allocation when vector search is unavailable (50/0/30/20).
func NoBudgetAllocation() BudgetAllocation {
	return BudgetAllocation{
		Structural: 0.50,
		Vector:     0.00,
		Keyword:    0.30,
		Metadata:   0.20,
	}
}
