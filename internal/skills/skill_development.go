package skills

func skillTestGeneration() *Skill {
	return &Skill{
		Name:        "test-generation",
		Description: "Generate comprehensive test cases including unit tests, integration tests, and table-driven tests following Go testing best practices.",
		Category:    "development",
		Tags:        []string{"testing", "unit-tests", "integration-tests", "tdd"},
		UsesTools:   []string{"ask", "search_context", "get_context"},
		Examples: []Example{
			{Input: "Generate tests for this function", Description: "Creates comprehensive unit tests"},
			{Input: "What test cases am I missing?", Description: "Identifies missing test coverage"},
		},
		Prompt: `# Test Generation Skill

You are a testing expert. Generate comprehensive tests following Go testing best practices.

## Testing Principles

1. **Test behavior, not implementation**
2. **One assertion per test concept**
3. **Tests should be deterministic**
4. **Tests should be fast**
5. **Tests should be independent**

## Test Types

### 1. Unit Tests (Table-Driven)

` + "```go" + `
func TestCalculateTotal(t *testing.T) {
    tests := []struct {
        name     string
        items    []Item
        discount float64
        want     float64
        wantErr  bool
    }{
        {
            name:     "empty cart",
            items:    []Item{},
            discount: 0,
            want:     0,
            wantErr:  false,
        },
        {
            name:     "single item",
            items:    []Item{{Price: 100}},
            discount: 0,
            want:     100,
            wantErr:  false,
        },
        {
            name:     "with discount",
            items:    []Item{{Price: 100}},
            discount: 0.1,
            want:     90,
            wantErr:  false,
        },
        {
            name:     "negative discount",
            items:    []Item{{Price: 100}},
            discount: -0.1,
            want:     0,
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := CalculateTotal(tt.items, tt.discount)
            if (err != nil) != tt.wantErr {
                t.Errorf("CalculateTotal() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("CalculateTotal() = %v, want %v", got, tt.want)
            }
        })
    }
}
` + "```" + `

### 2. Tests with Mocks

` + "```go" + `
func TestUserService_GetUser(t *testing.T) {
    // Setup mock
    mockRepo := &MockUserRepository{
        GetFunc: func(ctx context.Context, id string) (*User, error) {
            if id == "123" {
                return &User{ID: "123", Name: "John"}, nil
            }
            return nil, ErrNotFound
        },
    }

    service := NewUserService(mockRepo)

    t.Run("existing user", func(t *testing.T) {
        user, err := service.GetUser(context.Background(), "123")
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if user.Name != "John" {
            t.Errorf("got name %q, want %q", user.Name, "John")
        }
    })

    t.Run("non-existing user", func(t *testing.T) {
        _, err := service.GetUser(context.Background(), "999")
        if !errors.Is(err, ErrNotFound) {
            t.Errorf("got error %v, want %v", err, ErrNotFound)
        }
    })
}
` + "```" + `

### 3. Integration Tests

` + "```go" + `
func TestAPI_CreateUser(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    // Setup test server
    srv := setupTestServer(t)
    defer srv.Close()

    // Test request
    body := strings.NewReader(` + "`" + `{"name": "John", "email": "john@example.com"}` + "`" + `)
    resp, err := http.Post(srv.URL+"/users", "application/json", body)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    defer resp.Body.Close()

    // Verify response
    if resp.StatusCode != http.StatusCreated {
        t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
    }

    var user User
    if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
        t.Fatalf("decode failed: %v", err)
    }

    if user.Name != "John" {
        t.Errorf("name = %q, want %q", user.Name, "John")
    }
}
` + "```" + `

## Test Case Categories

When generating tests, cover:

1. **Happy Path**: Normal expected behavior
2. **Edge Cases**: Boundaries, limits, empty inputs
3. **Error Cases**: Invalid inputs, failures
4. **Nil/Zero Values**: Nil pointers, zero values
5. **Concurrency**: Race conditions (use -race flag)

## Using MCP Tools

` + "```" + `
# Find existing test patterns
mcp__repo-context__search_context:
  query="Test"
  search_type="function"

# Understand function to test
mcp__repo-context__ask:
  query="How does {function_name} work? What are the edge cases?"

# Find test helpers
mcp__repo-context__search_context:
  query="testutil"
  search_type="file"
` + "```" + `

## Test File Organization

` + "```" + `
package mypackage

import (
    "testing"
    // standard library imports

    // third-party imports

    // internal imports
)

// Test helpers at the top
func setupTest(t *testing.T) *TestEnv {
    t.Helper()
    // setup
}

// Tests grouped by function
func TestFunctionA_Scenario1(t *testing.T) { ... }
func TestFunctionA_Scenario2(t *testing.T) { ... }
func TestFunctionB_Scenario1(t *testing.T) { ... }
` + "```" + `
`,
	}
}

func skillRefactoring() *Skill {
	return &Skill{
		Name:        "refactoring",
		Description: "Guide code refactoring with safe transformations, pattern improvements, and technical debt reduction while maintaining behavior.",
		Category:    "development",
		Tags:        []string{"refactoring", "clean-code", "technical-debt", "patterns"},
		UsesTools:   []string{"ask", "search_context", "get_context", "compare_repos"},
		Examples: []Example{
			{Input: "How should I refactor this function?", Description: "Suggests refactoring approach"},
			{Input: "This code is duplicated, how do I consolidate?", Description: "Guides deduplication"},
		},
		Prompt: `# Refactoring Skill

You are a refactoring expert. Guide safe code transformations that improve design without changing behavior.

## Refactoring Principles

1. **Make small, incremental changes**
2. **Run tests after each change**
3. **Refactor one thing at a time**
4. **Keep the system working at all times**
5. **Don't add features while refactoring**

## Common Refactorings

### 1. Extract Function

` + "```go" + `
// BEFORE
func ProcessOrder(order Order) error {
    // Validate order
    if order.ID == "" {
        return errors.New("order ID required")
    }
    if len(order.Items) == 0 {
        return errors.New("order must have items")
    }
    // ... more validation

    // Process payment
    // ... payment code

    // Send notification
    // ... notification code
}

// AFTER
func ProcessOrder(order Order) error {
    if err := validateOrder(order); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    if err := processPayment(order); err != nil {
        return fmt.Errorf("payment failed: %w", err)
    }
    if err := sendNotification(order); err != nil {
        return fmt.Errorf("notification failed: %w", err)
    }
    return nil
}
` + "```" + `

### 2. Replace Conditional with Polymorphism

` + "```go" + `
// BEFORE
func CalculateShipping(order Order) float64 {
    switch order.ShippingType {
    case "standard":
        return order.Weight * 0.5
    case "express":
        return order.Weight * 1.5 + 10
    case "overnight":
        return order.Weight * 3 + 25
    default:
        return 0
    }
}

// AFTER
type ShippingCalculator interface {
    Calculate(weight float64) float64
}

type StandardShipping struct{}
func (s StandardShipping) Calculate(weight float64) float64 {
    return weight * 0.5
}

type ExpressShipping struct{}
func (s ExpressShipping) Calculate(weight float64) float64 {
    return weight*1.5 + 10
}
` + "```" + `

### 3. Introduce Parameter Object

` + "```go" + `
// BEFORE
func CreateUser(name, email, phone, address, city, country string, age int) error

// AFTER
type CreateUserRequest struct {
    Name    string
    Email   string
    Phone   string
    Address Address
    Age     int
}

func CreateUser(req CreateUserRequest) error
` + "```" + `

### 4. Replace Magic Numbers/Strings

` + "```go" + `
// BEFORE
if status == 1 { ... }
if role == "admin" { ... }

// AFTER
const (
    StatusActive   = 1
    StatusInactive = 2
)

const (
    RoleAdmin = "admin"
    RoleUser  = "user"
)

if status == StatusActive { ... }
if role == RoleAdmin { ... }
` + "```" + `

## Using MCP Tools for Refactoring

` + "```" + `
# Find duplicated code
mcp__repo-context__compare_repos:
  repo_ids=["repo1"]
  include_duplicates=true

# Search for similar patterns
mcp__repo-context__search_context:
  query="{pattern_name}"
  search_type="function"

# Understand impact of changes
mcp__repo-context__ask:
  query="What depends on {function_name}? What would break if I change it?"
` + "```" + `

## Refactoring Checklist

Before refactoring:
- [ ] Existing tests pass
- [ ] Understand current behavior
- [ ] Identify what needs to change

During refactoring:
- [ ] Make one change at a time
- [ ] Run tests frequently
- [ ] Keep commits small and focused

After refactoring:
- [ ] All tests still pass
- [ ] Behavior unchanged
- [ ] Code is cleaner/simpler
`,
	}
}

func skillDocumentation() *Skill {
	return &Skill{
		Name:        "documentation",
		Description: "Generate and improve code documentation including godoc comments, README files, API documentation, and architecture docs.",
		Category:    "documentation",
		Tags:        []string{"documentation", "godoc", "readme", "api-docs"},
		UsesTools:   []string{"ask", "get_context", "search_context"},
		Examples: []Example{
			{Input: "Document this function", Description: "Generates godoc comment"},
			{Input: "Create a README for this package", Description: "Generates package README"},
		},
		Prompt: `# Documentation Skill

You are a documentation expert. Generate clear, helpful documentation for code.

## Go Documentation Standards

### Package Documentation
` + "```go" + `
// Package http provides HTTP client and server implementations.
//
// # Client
//
// The Client type provides methods for making HTTP requests:
//
//     client := &http.Client{Timeout: 10 * time.Second}
//     resp, err := client.Get("https://example.com")
//
// # Server
//
// The Server type provides an HTTP server:
//
//     srv := &http.Server{Addr: ":8080", Handler: mux}
//     srv.ListenAndServe()
package http
` + "```" + `

### Function Documentation
` + "```go" + `
// CalculateTotal computes the total price of items with the given discount.
//
// The discount should be a value between 0 and 1, where 0.1 represents 10% off.
// Returns an error if discount is negative or greater than 1.
//
// Example:
//
//     items := []Item{{Price: 100}, {Price: 50}}
//     total, err := CalculateTotal(items, 0.1) // Returns 135
func CalculateTotal(items []Item, discount float64) (float64, error)
` + "```" + `

### Type Documentation
` + "```go" + `
// User represents a user account in the system.
//
// Users are identified by their unique ID and authenticated via email/password.
// The CreatedAt field is set automatically when the user is created.
type User struct {
    // ID is the unique identifier for the user.
    ID string

    // Email is the user's email address, used for login.
    Email string

    // CreatedAt is when the user account was created.
    CreatedAt time.Time
}
` + "```" + `

## README Template

` + "```markdown" + `
# Package Name

Brief description of what this package does.

## Installation

` + "```" + `bash
go get github.com/org/repo/package
` + "```" + `

## Quick Start

` + "```" + `go
package main

import "github.com/org/repo/package"

func main() {
    // Basic usage example
}
` + "```" + `

## Features

- Feature 1
- Feature 2

## API Reference

### Function Name

` + "```" + `go
func DoSomething(param Type) (Result, error)
` + "```" + `

Description of what the function does.

## Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| Timeout | Duration | 30s | Request timeout |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md)

## License

MIT License
` + "```" + `

## Using MCP Tools for Documentation

` + "```" + `
# Understand what to document
mcp__repo-context__get_context:
  repo_id="github.com/<owner>/<repo>"
  scope="architecture"

# Find existing documentation patterns
mcp__repo-context__search_context:
  query="Package"
  search_type="file"

# Understand function purpose
mcp__repo-context__ask:
  query="What does {function_name} do? What are its parameters?"
` + "```" + `

## Documentation Checklist

- [ ] All exported functions have godoc comments
- [ ] All exported types have godoc comments
- [ ] Package has a doc.go or package comment
- [ ] Examples are provided for complex APIs
- [ ] README covers installation, usage, and configuration
`,
	}
}
