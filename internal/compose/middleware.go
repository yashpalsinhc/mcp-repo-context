package compose

import (
	"fmt"
	"log"
	"time"
)

// LoggingMiddleware logs tool execution.
func LoggingMiddleware(logger *log.Logger) Middleware {
	return func(next ToolExecutor) ToolExecutor {
		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			if logger != nil {
				logger.Printf("[CALL] %s with %v", call.Name, call.Params)
			}

			result := next.Execute(ctx, call)

			if logger != nil {
				if result.Success {
					logger.Printf("[OK] %s (%s)", call.Name, result.Duration)
				} else {
					logger.Printf("[ERR] %s: %s", call.Name, result.Error)
				}
			}

			return result
		})
	}
}

// TimingMiddleware adds timing information to results.
func TimingMiddleware() Middleware {
	return func(next ToolExecutor) ToolExecutor {
		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			start := time.Now()
			result := next.Execute(ctx, call)
			result.Duration = time.Since(start).String()
			return result
		})
	}
}

// RetryMiddleware retries failed calls.
func RetryMiddleware(maxRetries int, delay time.Duration) Middleware {
	return func(next ToolExecutor) ToolExecutor {
		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			var result ToolResult

			for i := 0; i <= maxRetries; i++ {
				result = next.Execute(ctx, call)
				if result.Success {
					return result
				}

				// Check if context is cancelled
				select {
				case <-ctx.Done():
					result.Error = "context cancelled during retry"
					return result
				default:
				}

				// Wait before retry (except on last attempt)
				if i < maxRetries {
					time.Sleep(delay)
				}
			}

			result.Error = fmt.Sprintf("failed after %d retries: %s", maxRetries+1, result.Error)
			return result
		})
	}
}

// TokenBudgetMiddleware tracks and limits token usage.
func TokenBudgetMiddleware(budget int) Middleware {
	return func(next ToolExecutor) ToolExecutor {
		usedTokens := 0

		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			result := next.Execute(ctx, call)

			usedTokens += result.TokenCost
			ctx.Set("_total_tokens", usedTokens)

			if usedTokens > budget {
				ctx.Stop(fmt.Sprintf("token budget exceeded: %d > %d", usedTokens, budget))
			}

			return result
		})
	}
}

// CacheMiddleware caches results based on tool name and params.
func CacheMiddleware() Middleware {
	cache := make(map[string]ToolResult)

	return func(next ToolExecutor) ToolExecutor {
		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			key := fmt.Sprintf("%s:%v", call.Name, call.Params)

			if cached, ok := cache[key]; ok {
				return cached
			}

			result := next.Execute(ctx, call)
			if result.Success {
				cache[key] = result
			}

			return result
		})
	}
}

// ErrorHandlerMiddleware provides custom error handling.
func ErrorHandlerMiddleware(handler func(call ToolCall, err string) bool) Middleware {
	return func(next ToolExecutor) ToolExecutor {
		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			result := next.Execute(ctx, call)

			if !result.Success && handler != nil {
				// Handler returns true to continue, false to stop
				if !handler(call, result.Error) {
					ctx.Stop("error handler stopped chain")
				}
			}

			return result
		})
	}
}

// TransformMiddleware transforms results.
func TransformMiddleware(transform func(ToolResult) ToolResult) Middleware {
	return func(next ToolExecutor) ToolExecutor {
		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			result := next.Execute(ctx, call)
			return transform(result)
		})
	}
}

// StoreResultMiddleware stores results in context variables.
func StoreResultMiddleware(varPrefix string) Middleware {
	return func(next ToolExecutor) ToolExecutor {
		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			result := next.Execute(ctx, call)

			if result.Success && result.Data != nil {
				varName := varPrefix + "_" + call.Name
				ctx.Set(varName, result.Data)
			}

			return result
		})
	}
}

// TimeoutMiddleware adds timeout to tool execution.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next ToolExecutor) ToolExecutor {
		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			done := make(chan ToolResult, 1)

			go func() {
				done <- next.Execute(ctx, call)
			}()

			select {
			case result := <-done:
				return result
			case <-time.After(timeout):
				return ToolResult{
					ToolName: call.Name,
					Success:  false,
					Error:    fmt.Sprintf("timeout after %s", timeout),
				}
			}
		})
	}
}

// MetricsMiddleware collects execution metrics.
type ExecutionMetrics struct {
	CallCount    int
	SuccessCount int
	ErrorCount   int
	TotalTokens  int
	TotalTime    time.Duration
}

func MetricsMiddleware(metrics *ExecutionMetrics) Middleware {
	return func(next ToolExecutor) ToolExecutor {
		return NewFuncExecutor(func(ctx *ChainContext, call ToolCall) ToolResult {
			start := time.Now()
			result := next.Execute(ctx, call)
			elapsed := time.Since(start)

			metrics.CallCount++
			metrics.TotalTime += elapsed
			metrics.TotalTokens += result.TokenCost

			if result.Success {
				metrics.SuccessCount++
			} else {
				metrics.ErrorCount++
			}

			return result
		})
	}
}
