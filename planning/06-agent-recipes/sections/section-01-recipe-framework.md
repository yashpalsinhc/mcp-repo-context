# Section 01: Recipe Framework & Types

## Overview

Define the core Recipe interface, RecipeResult type, RecipeRunner, RecipeInput, Registry, and supporting types. Promote `CompleteRaw` to the `ai.Provider` interface. Define `VectorSearcher` interface for mockable vector search. This is the foundation all recipes build on.

## Dependencies

- Internal: `internal/orchestrator` (Manager), `internal/org` (Manager), `internal/ai` (Provider), `internal/vectors` (SemanticSearch), `internal/tokens` (Budgeter), `internal/logging`

## AI Interface Update

Add `CompleteRaw(ctx context.Context, prompt string, maxTokens int) (string, error)` to the `ai.Provider` interface in `internal/ai/provider.go`. It already exists on `*AnthropicProvider` concretely — just promote it to the interface. This enables recipes to send free-form prompts for risk assessment and narrative generation.

## New Package

`internal/recipes/`

## VectorSearcher Interface

Define in `internal/recipes/interfaces.go`:

```
type VectorSearcher interface {
    SearchFunctions(query string, repoID string, limit int) ([]vectors.SearchResult, error)
    SearchTypes(query string, repoID string, limit int) ([]vectors.SearchResult, error)
}
```

The existing `*vectors.SemanticSearch` satisfies this interface. Tests mock it. Compile-time assertion: `var _ VectorSearcher = (*vectors.SemanticSearch)(nil)`.

## Recipe Interface

```
type Recipe interface {
    Name() string
    Description() string
    InputSchema() map[string]FieldSpec
    Run(ctx context.Context, runner *RecipeRunner, input RecipeInput) (*RecipeResult, error)
}
```

`*RecipeRunner` is passed explicitly (not via context.WithValue) to avoid hiding dependencies.

## FieldSpec

```
type FieldSpec struct {
    Name        string
    Type        string  // "string", "int", "bool", "[]string", "[]ChangedFile"
    Required    bool
    Description string
    Default     any     // default value if not provided
}
```

## RecipeInput

Typed wrapper around `map[string]any`:

```
type RecipeInput struct {
    data map[string]any
}
```

Methods:
- `NewRecipeInput(data map[string]any) RecipeInput`
- `GetString(key string) string` — returns "" if missing or wrong type
- `GetStringSlice(key string) []string` — handles `[]interface{}` -> `[]string` coercion with element-by-element type assertion
- `GetInt(key string, defaultVal int) int` — returns default if missing; handles float64 (from JSON)
- `GetBool(key string, defaultVal bool) bool`
- `Get(key string) (any, bool)` — raw access
- `Validate(schema map[string]FieldSpec) error` — checks required fields present, returns error listing all missing fields

## RecipeResult

```
type RecipeResult struct {
    Recipe        string              // recipe name
    Data          map[string]any      // structured output (facts from code)
    Analysis      string              // AI-generated summary (optional)
    Gaps          []GapNote           // sections that couldn't be computed
    Sources       []RecipeSourceRef   // file:line references
    ContextTokens int                 // context budget tokens used
    AITokens      int                 // AI API tokens consumed
    DurationMS    int64               // execution time
    Confidence    float64             // 0-1
}
```

Named `RecipeSourceRef` to avoid collision with `orchestrator.SourceRef` and `ai.SourceRef`:

```
type RecipeSourceRef struct {
    File  string
    Line  int
    Claim string
}
```

```
type GapNote struct {
    Section    string
    Reason     string
    Suggestion string
}
```

**Confidence scoring:** Start at 1.0. Multiply by 0.7 if AI unavailable. Multiply by 0.7 for each GapNote. Floor at 0.1.

## RecipeRunner

```
type RecipeRunner struct {
    manager    orchestrator.Manager
    orgManager org.Manager
    ai         ai.Provider       // may be nil
    vectors    VectorSearcher    // may be nil
    budgeter   *tokens.Budgeter
    registry   *Registry
    logger     *logging.Logger
}
```

Constructor with functional options:

```
func NewRecipeRunner(manager orchestrator.Manager, orgManager org.Manager, opts ...RunnerOption) *RecipeRunner

type RunnerOption func(*RecipeRunner)

func WithAI(p ai.Provider) RunnerOption
func WithVectors(v VectorSearcher) RunnerOption
func WithBudgeter(b *tokens.Budgeter) RunnerOption
func WithRegistry(r *Registry) RunnerOption
func WithLogger(l *logging.Logger) RunnerOption
```

**Execute method** (enables recipe composability):

```
func (r *RecipeRunner) Execute(ctx context.Context, recipeName string, input RecipeInput) (*RecipeResult, error)
```

Looks up recipe in registry, validates input against schema, calls `recipe.Run(ctx, r, input)`. Returns error if recipe not found (with list of available names).

**Accessor methods** for recipes to use: `Manager()`, `OrgManager()`, `AI()`, `Vectors()`, `Budgeter()`.

## BudgetAllocation

```
type BudgetAllocation struct {
    Structural float64  // default 0.40
    Vector     float64  // default 0.30
    Keyword    float64  // default 0.20
    Metadata   float64  // default 0.10
}

func DefaultBudgetAllocation() BudgetAllocation
func NoBudgetAllocation() BudgetAllocation  // no vector: 0.50/0.00/0.30/0.20
```

## Registry

```
type Registry struct {
    mu      sync.RWMutex
    recipes map[string]Recipe
}
```

Methods:
- `NewRegistry() *Registry` — empty registry
- `Register(recipe Recipe)` — add recipe (last-write-wins on name collision)
- `Get(name string) (Recipe, bool)`
- `List() []RecipeInfo` — returns name, description, input schema for each

`DefaultRegistry()` returns a registry pre-populated with all built-in recipes (added in sections 3-5).

## RecipeRiskAssessment

Shared type used by recipes (avoids collision with `orchestrator.ImpactAnalysis`):

```
type RecipeRiskAssessment struct {
    Level      string   // "low", "medium", "high"
    Reasoning  string
    Confidence float64
}
```

## Tests

### `internal/recipes/framework_test.go`

**Test: RecipeInput validates required fields**
- Create input missing required "repo_id"
- Validate against schema with repo_id required
- Assert error listing missing field

**Test: RecipeInput GetString returns value**
- Input with "repo_id" = "github.com/org/repo"
- Assert GetString returns correct value
- Assert GetString("missing") returns ""

**Test: RecipeInput GetStringSlice coerces []interface{}**
- Input with "repo_ids" = []interface{}{"a", "b"}
- Assert GetStringSlice returns []string{"a", "b"}

**Test: RecipeInput GetInt returns default when missing**
- Assert GetInt("budget", 8000) returns 8000 when not set
- Assert GetInt("budget", 8000) returns 4000 when set to 4000

**Test: Registry register and get**
- Register recipe, Get returns it, Get("unknown") returns false

**Test: Registry list returns all recipes**
- Register 3 recipes, List() returns 3

**Test: RecipeRunner Execute calls correct recipe**
- Register mock recipe, Execute invokes it with correct input

**Test: RecipeRunner Execute returns error for unknown recipe**
- Assert error mentions available names

**Test: Confidence scoring**
- All steps: 1.0; no AI: 0.7; one gap: ~0.7; both: ~0.49

**Test: Provider interface includes CompleteRaw**
- Compile-time: var _ ai.Provider = (*ai.AnthropicProvider)(nil)

**Test: SemanticSearch satisfies VectorSearcher**
- Compile-time: var _ VectorSearcher = (*vectors.SemanticSearch)(nil)

## File Inventory

| File | Purpose |
|------|---------|
| `internal/recipes/recipe.go` | Recipe interface, FieldSpec |
| `internal/recipes/input.go` | RecipeInput struct and methods |
| `internal/recipes/result.go` | RecipeResult, GapNote, RecipeSourceRef, RecipeRiskAssessment |
| `internal/recipes/runner.go` | RecipeRunner struct, options, Execute |
| `internal/recipes/registry.go` | Registry, DefaultRegistry |
| `internal/recipes/budget.go` | BudgetAllocation |
| `internal/recipes/interfaces.go` | VectorSearcher interface |
| `internal/ai/provider.go` | Updated Provider interface with CompleteRaw |
| `internal/recipes/framework_test.go` | All framework tests |

## Acceptance Criteria

1. Recipe interface defined with explicit RecipeRunner parameter
2. RecipeInput validates and coerces types correctly
3. RecipeResult tracks context and AI tokens separately
4. RecipeRunner.Execute enables recipe composability
5. Registry manages recipes with thread-safe access
6. VectorSearcher interface mockable in tests
7. CompleteRaw on ai.Provider interface
8. All 11 tests pass
