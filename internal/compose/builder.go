package compose

import (
	"context"
	"time"
)

// Builder provides a fluent API for building chains.
type Builder struct {
	chain    *Chain
	executor ToolExecutor
}

// NewBuilder creates a new chain builder.
func NewBuilder(executor ToolExecutor) *Builder {
	return &Builder{
		chain:    NewChain(executor),
		executor: executor,
	}
}

// WithLogging adds logging middleware.
func (b *Builder) WithLogging() *Builder {
	b.chain.Use(LoggingMiddleware(nil))
	return b
}

// WithTiming adds timing middleware.
func (b *Builder) WithTiming() *Builder {
	b.chain.Use(TimingMiddleware())
	return b
}

// WithRetry adds retry middleware.
func (b *Builder) WithRetry(maxRetries int, delay time.Duration) *Builder {
	b.chain.Use(RetryMiddleware(maxRetries, delay))
	return b
}

// WithTokenBudget adds token budget middleware.
func (b *Builder) WithTokenBudget(budget int) *Builder {
	b.chain.Use(TokenBudgetMiddleware(budget))
	return b
}

// WithCache adds caching middleware.
func (b *Builder) WithCache() *Builder {
	b.chain.Use(CacheMiddleware())
	return b
}

// WithTimeout adds timeout middleware.
func (b *Builder) WithTimeout(timeout time.Duration) *Builder {
	b.chain.Use(TimeoutMiddleware(timeout))
	return b
}

// WithMiddleware adds custom middleware.
func (b *Builder) WithMiddleware(m Middleware) *Builder {
	b.chain.Use(m)
	return b
}

// Call adds a tool call.
func (b *Builder) Call(name string, params map[string]any) *Builder {
	b.chain.Add(ToolCall{Name: name, Params: params})
	return b
}

// CallIf adds a conditional tool call.
func (b *Builder) CallIf(name string, params map[string]any, condition func(*ChainContext) bool) *Builder {
	b.chain.AddConditional(ToolCall{Name: name, Params: params}, condition)
	return b
}

// CallWithStore adds a tool call that stores result in a variable.
func (b *Builder) CallWithStore(name string, params map[string]any, varName string) *Builder {
	b.chain.AddWithTransform(
		ToolCall{Name: name, Params: params},
		func(ctx *ChainContext, result *ToolResult) error {
			if result.Success && result.Data != nil {
				ctx.Set(varName, result.Data)
			}
			return nil
		},
	)
	return b
}

// Search adds a search_context call.
func (b *Builder) Search(query string, repoID string) *Builder {
	return b.Call("search_context", map[string]any{
		"query":   query,
		"repo_id": repoID,
	})
}

// SearchByConcept adds a search_by_concept call.
func (b *Builder) SearchByConcept(repoID, concept string) *Builder {
	return b.Call("search_by_concept", map[string]any{
		"repo_id": repoID,
		"concept": concept,
	})
}

// SearchBySideEffect adds a search_by_side_effect call.
func (b *Builder) SearchBySideEffect(repoID, effect string) *Builder {
	return b.Call("search_by_side_effect", map[string]any{
		"repo_id": repoID,
		"effect":  effect,
	})
}

// GetFunctionContext adds a get_function_context call.
func (b *Builder) GetFunctionContext(repoID, filePath, funcName string) *Builder {
	return b.Call("get_function_context", map[string]any{
		"repo_id":       repoID,
		"file_path":     filePath,
		"function_name": funcName,
	})
}

// GetCallers adds a get_callers call.
func (b *Builder) GetCallers(repoID, funcName string) *Builder {
	return b.Call("get_callers", map[string]any{
		"repo_id":       repoID,
		"function_name": funcName,
	})
}

// Build returns the constructed chain.
func (b *Builder) Build() *Chain {
	return b.chain
}

// Execute runs the chain.
func (b *Builder) Execute(ctx context.Context) (*ChainContext, error) {
	return b.chain.Execute(ctx)
}

// QuickChain provides shorthand for common chain operations.
type QuickChain struct {
	executor ToolExecutor
}

// NewQuickChain creates a quick chain helper.
func NewQuickChain(executor ToolExecutor) *QuickChain {
	return &QuickChain{executor: executor}
}

// SearchAndGetContext searches and gets context for first result.
func (q *QuickChain) SearchAndGetContext(ctx context.Context, repoID, query string) (*ChainContext, error) {
	return NewBuilder(q.executor).
		WithTiming().
		CallWithStore("search_context", map[string]any{
			"query":   query,
			"repo_id": repoID,
		}, "search_results").
		CallIf("get_function_context", map[string]any{
			"repo_id":       repoID,
			"file_path":     "{{first_file}}",
			"function_name": "{{first_func}}",
		}, func(ctx *ChainContext) bool {
			// Extract first result
			results, ok := ctx.Get("search_results")
			if !ok {
				return false
			}
			switch r := results.(type) {
			case []map[string]any:
				if len(r) > 0 {
					if file, ok := r[0]["file"].(string); ok {
						ctx.Set("first_file", file)
					}
					if name, ok := r[0]["name"].(string); ok {
						ctx.Set("first_func", name)
					}
					return true
				}
			}
			return false
		}).
		Execute(ctx)
}

// AnalyzeFunction gets full context and callers for a function.
func (q *QuickChain) AnalyzeFunction(ctx context.Context, repoID, filePath, funcName string) (*ChainContext, error) {
	return NewBuilder(q.executor).
		WithTiming().
		CallWithStore("get_function_context", map[string]any{
			"repo_id":       repoID,
			"file_path":     filePath,
			"function_name": funcName,
		}, "function_context").
		CallWithStore("get_callers", map[string]any{
			"repo_id":       repoID,
			"function_name": funcName,
		}, "callers").
		Execute(ctx)
}

// FindDatabaseOperations finds all database operations in a repo.
func (q *QuickChain) FindDatabaseOperations(ctx context.Context, repoID string) (*ChainContext, error) {
	return NewBuilder(q.executor).
		WithTiming().
		CallWithStore("search_by_side_effect", map[string]any{
			"repo_id": repoID,
			"effect":  "db_query",
		}, "db_reads").
		CallWithStore("search_by_side_effect", map[string]any{
			"repo_id": repoID,
			"effect":  "db_write",
		}, "db_writes").
		Execute(ctx)
}

// FindHTTPEndpoints finds HTTP handlers and their routes.
func (q *QuickChain) FindHTTPEndpoints(ctx context.Context, repoID string) (*ChainContext, error) {
	return NewBuilder(q.executor).
		WithTiming().
		CallWithStore("search_by_concept", map[string]any{
			"repo_id": repoID,
			"concept": "handler",
		}, "handlers").
		CallWithStore("search_by_side_effect", map[string]any{
			"repo_id": repoID,
			"effect":  "http_call",
		}, "http_calls").
		Execute(ctx)
}
