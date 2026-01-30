package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ToolCall represents a single tool invocation.
type ToolCall struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
}

// ToolResult represents the result of a tool call.
type ToolResult struct {
	ToolName  string      `json:"tool_name"`
	Success   bool        `json:"success"`
	Data      any         `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Duration  string      `json:"duration,omitempty"`
	TokenCost int         `json:"token_cost,omitempty"`
}

// ChainContext holds state during chain execution.
type ChainContext struct {
	context.Context
	Results    []ToolResult          `json:"results"`
	Vars       map[string]any        `json:"vars"`
	StopReason string                `json:"stop_reason,omitempty"`
	stopped    bool
}

// NewChainContext creates a new chain execution context.
func NewChainContext(ctx context.Context) *ChainContext {
	return &ChainContext{
		Context: ctx,
		Results: make([]ToolResult, 0),
		Vars:    make(map[string]any),
	}
}

// Set stores a variable in the chain context.
func (c *ChainContext) Set(key string, value any) {
	c.Vars[key] = value
}

// Get retrieves a variable from the chain context.
func (c *ChainContext) Get(key string) (any, bool) {
	v, ok := c.Vars[key]
	return v, ok
}

// GetString retrieves a string variable.
func (c *ChainContext) GetString(key string) string {
	v, ok := c.Vars[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// LastResult returns the most recent tool result.
func (c *ChainContext) LastResult() *ToolResult {
	if len(c.Results) == 0 {
		return nil
	}
	return &c.Results[len(c.Results)-1]
}

// LastData returns the data from the last successful result.
func (c *ChainContext) LastData() any {
	for i := len(c.Results) - 1; i >= 0; i-- {
		if c.Results[i].Success {
			return c.Results[i].Data
		}
	}
	return nil
}

// Stop signals the chain to stop execution.
func (c *ChainContext) Stop(reason string) {
	c.stopped = true
	c.StopReason = reason
}

// IsStopped returns true if the chain should stop.
func (c *ChainContext) IsStopped() bool {
	return c.stopped
}

// ToolExecutor executes tool calls.
type ToolExecutor interface {
	Execute(ctx *ChainContext, call ToolCall) ToolResult
}

// Middleware wraps tool execution with pre/post processing.
type Middleware func(next ToolExecutor) ToolExecutor

// Chain represents a sequence of tool calls with middleware.
type Chain struct {
	steps      []ChainStep
	middleware []Middleware
	executor   ToolExecutor
}

// ChainStep represents a step in the chain.
type ChainStep struct {
	Call      ToolCall
	Condition func(*ChainContext) bool
	Transform func(*ChainContext, *ToolResult) error
}

// NewChain creates a new chain.
func NewChain(executor ToolExecutor) *Chain {
	return &Chain{
		steps:      make([]ChainStep, 0),
		middleware: make([]Middleware, 0),
		executor:   executor,
	}
}

// Use adds middleware to the chain.
func (c *Chain) Use(m Middleware) *Chain {
	c.middleware = append(c.middleware, m)
	return c
}

// Add adds a tool call to the chain.
func (c *Chain) Add(call ToolCall) *Chain {
	c.steps = append(c.steps, ChainStep{Call: call})
	return c
}

// AddConditional adds a conditional tool call.
func (c *Chain) AddConditional(call ToolCall, condition func(*ChainContext) bool) *Chain {
	c.steps = append(c.steps, ChainStep{
		Call:      call,
		Condition: condition,
	})
	return c
}

// AddWithTransform adds a tool call with result transformation.
func (c *Chain) AddWithTransform(call ToolCall, transform func(*ChainContext, *ToolResult) error) *Chain {
	c.steps = append(c.steps, ChainStep{
		Call:      call,
		Transform: transform,
	})
	return c
}

// Execute runs the chain.
func (c *Chain) Execute(ctx context.Context) (*ChainContext, error) {
	chainCtx := NewChainContext(ctx)

	// Build executor with middleware
	executor := c.executor
	for i := len(c.middleware) - 1; i >= 0; i-- {
		executor = c.middleware[i](executor)
	}

	for _, step := range c.steps {
		// Check context cancellation
		select {
		case <-ctx.Done():
			chainCtx.Stop("context cancelled")
			return chainCtx, ctx.Err()
		default:
		}

		// Check if chain was stopped
		if chainCtx.IsStopped() {
			break
		}

		// Check condition
		if step.Condition != nil && !step.Condition(chainCtx) {
			continue
		}

		// Resolve parameters with variables
		resolvedCall := c.resolveParams(step.Call, chainCtx)

		// Execute
		result := executor.Execute(chainCtx, resolvedCall)
		chainCtx.Results = append(chainCtx.Results, result)

		// Transform result
		if step.Transform != nil {
			if err := step.Transform(chainCtx, &result); err != nil {
				return chainCtx, err
			}
		}

		// Stop on error
		if !result.Success {
			chainCtx.Stop("tool error: " + result.Error)
			break
		}
	}

	return chainCtx, nil
}

// resolveParams replaces {{var}} placeholders with context variables.
func (c *Chain) resolveParams(call ToolCall, ctx *ChainContext) ToolCall {
	resolved := ToolCall{
		Name:   call.Name,
		Params: make(map[string]any),
	}

	for k, v := range call.Params {
		resolved.Params[k] = c.resolveValue(v, ctx)
	}

	return resolved
}

// resolveValue resolves a single value.
func (c *Chain) resolveValue(v any, ctx *ChainContext) any {
	switch val := v.(type) {
	case string:
		// Check for {{varName}} pattern
		if len(val) > 4 && val[:2] == "{{" && val[len(val)-2:] == "}}" {
			varName := val[2 : len(val)-2]
			if resolved, ok := ctx.Get(varName); ok {
				return resolved
			}
		}
		return val
	case map[string]any:
		resolved := make(map[string]any)
		for k, inner := range val {
			resolved[k] = c.resolveValue(inner, ctx)
		}
		return resolved
	default:
		return v
	}
}

// Summary returns a summary of chain execution.
func (c *ChainContext) Summary() ChainSummary {
	totalDuration := time.Duration(0)
	totalTokens := 0
	successCount := 0

	for _, r := range c.Results {
		if r.Duration != "" {
			if d, err := time.ParseDuration(r.Duration); err == nil {
				totalDuration += d
			}
		}
		totalTokens += r.TokenCost
		if r.Success {
			successCount++
		}
	}

	return ChainSummary{
		TotalSteps:    len(c.Results),
		SuccessSteps:  successCount,
		FailedSteps:   len(c.Results) - successCount,
		TotalDuration: totalDuration.String(),
		TotalTokens:   totalTokens,
		StopReason:    c.StopReason,
	}
}

// ChainSummary provides statistics about chain execution.
type ChainSummary struct {
	TotalSteps    int    `json:"total_steps"`
	SuccessSteps  int    `json:"success_steps"`
	FailedSteps   int    `json:"failed_steps"`
	TotalDuration string `json:"total_duration"`
	TotalTokens   int    `json:"total_tokens"`
	StopReason    string `json:"stop_reason,omitempty"`
}

// FuncExecutor wraps a function as a ToolExecutor.
type FuncExecutor struct {
	fn func(*ChainContext, ToolCall) ToolResult
}

// NewFuncExecutor creates an executor from a function.
func NewFuncExecutor(fn func(*ChainContext, ToolCall) ToolResult) *FuncExecutor {
	return &FuncExecutor{fn: fn}
}

// Execute implements ToolExecutor.
func (e *FuncExecutor) Execute(ctx *ChainContext, call ToolCall) ToolResult {
	return e.fn(ctx, call)
}

// MapExecutor routes calls to specific handlers.
type MapExecutor struct {
	handlers map[string]func(*ChainContext, map[string]any) (any, error)
}

// NewMapExecutor creates a new map-based executor.
func NewMapExecutor() *MapExecutor {
	return &MapExecutor{
		handlers: make(map[string]func(*ChainContext, map[string]any) (any, error)),
	}
}

// Register adds a handler for a tool.
func (e *MapExecutor) Register(name string, handler func(*ChainContext, map[string]any) (any, error)) {
	e.handlers[name] = handler
}

// Execute implements ToolExecutor.
func (e *MapExecutor) Execute(ctx *ChainContext, call ToolCall) ToolResult {
	handler, ok := e.handlers[call.Name]
	if !ok {
		return ToolResult{
			ToolName: call.Name,
			Success:  false,
			Error:    fmt.Sprintf("unknown tool: %s", call.Name),
		}
	}

	start := time.Now()
	data, err := handler(ctx, call.Params)
	duration := time.Since(start)

	result := ToolResult{
		ToolName: call.Name,
		Duration: duration.String(),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
		result.Data = data
		// Estimate token cost
		if data != nil {
			if b, err := json.Marshal(data); err == nil {
				result.TokenCost = len(b) / 4 // ~4 chars per token
			}
		}
	}

	return result
}
