package recipes

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// AIProvider defines the interface for AI operations.
type AIProvider interface {
	CompleteRaw(ctx context.Context, prompt string, maxTokens int) (string, int, error)
	IsConfigured() bool
}

// OrchestratorManager defines the interface for orchestrator operations.
type OrchestratorManager interface {
	GetContext(repoID string, scope string) (interface{}, error)
	ListRepos() []string
	SearchContext(query string, repoID string) ([]interface{}, error)
	GetFunctionContext(repoID string, filePath string, functionName string) (interface{}, error)
	GetCallers(repoID string, functionName string) ([]interface{}, error)
	GetPRContext(repoID string, changedFiles []interface{}) (interface{}, error)
}

// OrgManager defines the interface for org operations.
type OrgManager interface {
	GetOrg(orgID string) (interface{}, error)
	ListOrgs() []string
}

// TokenBudgeter defines the interface for token budgeting.
type TokenBudgeter interface {
	Available() int
	Reserve(tokens int) bool
	Use(tokens int) bool
}

// Logger defines the interface for logging.
type Logger interface {
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// RecipeRunner coordinates recipe execution.
type RecipeRunner struct {
	manager    OrchestratorManager
	orgManager OrgManager
	ai         AIProvider     // may be nil
	vectors    VectorSearcher // may be nil
	budgeter   TokenBudgeter
	registry   *Registry
	logger     Logger
}

// RunnerOption is a functional option for RecipeRunner.
type RunnerOption func(*RecipeRunner)

// WithAI sets the AI provider.
func WithAI(p AIProvider) RunnerOption {
	return func(r *RecipeRunner) {
		r.ai = p
	}
}

// WithVectors sets the vector searcher.
func WithVectors(v VectorSearcher) RunnerOption {
	return func(r *RecipeRunner) {
		r.vectors = v
	}
}

// WithBudgeter sets the token budgeter.
func WithBudgeter(b TokenBudgeter) RunnerOption {
	return func(r *RecipeRunner) {
		r.budgeter = b
	}
}

// WithRegistry sets the recipe registry.
func WithRegistry(reg *Registry) RunnerOption {
	return func(r *RecipeRunner) {
		r.registry = reg
	}
}

// WithLogger sets the logger.
func WithLogger(l Logger) RunnerOption {
	return func(r *RecipeRunner) {
		r.logger = l
	}
}

// NewRecipeRunner creates a new recipe runner with the given manager and options.
func NewRecipeRunner(manager OrchestratorManager, orgManager OrgManager, opts ...RunnerOption) *RecipeRunner {
	r := &RecipeRunner{
		manager:    manager,
		orgManager: orgManager,
		registry:   NewRegistry(),
		logger:     &simpleLogger{},
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// simpleLogger is a simple logger implementation.
type simpleLogger struct{}

func (s *simpleLogger) Info(msg string, fields ...interface{})   {}
func (s *simpleLogger) Warn(msg string, fields ...interface{})   {}
func (s *simpleLogger) Error(msg string, fields ...interface{})  {}


// Manager returns the orchestrator manager.
func (r *RecipeRunner) Manager() OrchestratorManager {
	return r.manager
}

// OrgManager returns the org manager.
func (r *RecipeRunner) OrgManager() OrgManager {
	return r.orgManager
}

// AI returns the AI provider (may be nil).
func (r *RecipeRunner) AI() AIProvider {
	return r.ai
}

// Vectors returns the vector searcher (may be nil).
func (r *RecipeRunner) Vectors() VectorSearcher {
	return r.vectors
}

// Budgeter returns the token budgeter.
func (r *RecipeRunner) Budgeter() TokenBudgeter {
	return r.budgeter
}

// Registry returns the recipe registry.
func (r *RecipeRunner) Registry() *Registry {
	return r.registry
}

// Logger returns the logger.
func (r *RecipeRunner) Logger() Logger {
	return r.logger
}

// Execute looks up a recipe by name and runs it with the given input.
// Returns error if recipe not found or if validation fails.
func (r *RecipeRunner) Execute(ctx context.Context, recipeName string, input RecipeInput) (*RecipeResult, error) {
	recipe, ok := r.registry.Get(recipeName)
	if !ok {
		// Provide helpful error with available names
		available := r.registry.List()
		names := make([]string, 0, len(available))
		for _, info := range available {
			names = append(names, info.Name)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("recipe not found: %q (available: %s)", recipeName, strings.Join(names, ", "))
	}

	// Validate input against schema
	schema := recipe.InputSchema()
	if err := input.Validate(schema); err != nil {
		return nil, fmt.Errorf("invalid input for recipe %q: %w", recipeName, err)
	}

	// Execute the recipe
	result, err := recipe.Run(ctx, r, input)
	if err != nil {
		return nil, fmt.Errorf("recipe %q failed: %w", recipeName, err)
	}

	return result, nil
}
