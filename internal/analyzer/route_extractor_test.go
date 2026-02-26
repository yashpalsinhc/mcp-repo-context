package analyzer

import (
	"go/parser"
	"go/token"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestRouteExtractorGorillaMux(t *testing.T) {
	code := `
package main

import "github.com/gorilla/mux"

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/api/users", GetUsers).Methods("GET")
	r.HandleFunc("/api/users", CreateUser).Methods("POST")
	r.HandleFunc("/api/users/{id}", DeleteUser).Methods("DELETE")
}
`
	routes := extractRoutesFromCode(t, code, "test.go")

	if len(routes) != 3 {
		t.Errorf("Expected 3 routes, got %d", len(routes))
	}

	// Check first route
	if routes[0].Method != "GET" {
		t.Errorf("Expected GET, got %s", routes[0].Method)
	}
	if routes[0].Path != "/api/users" {
		t.Errorf("Expected /api/users, got %s", routes[0].Path)
	}
	if routes[0].Handler != "GetUsers" {
		t.Errorf("Expected GetUsers, got %s", routes[0].Handler)
	}
	if routes[0].Framework != "gorilla/mux" {
		t.Errorf("Expected gorilla/mux framework, got %s", routes[0].Framework)
	}
}

func TestRouteExtractorGorillaMuxWithoutMethods(t *testing.T) {
	code := `
package main

import "github.com/gorilla/mux"

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/api/users", GetUsers)
}
`
	routes := extractRoutesFromCode(t, code, "test.go")

	if len(routes) != 1 {
		t.Errorf("Expected 1 route, got %d", len(routes))
	}

	// Should default to ANY method
	if routes[0].Method != "ANY" {
		t.Errorf("Expected ANY, got %s", routes[0].Method)
	}
	if routes[0].Handler != "GetUsers" {
		t.Errorf("Expected GetUsers, got %s", routes[0].Handler)
	}
}

func TestRouteExtractorPathParamNormalization(t *testing.T) {
	tests := []struct {
		name     string
		rawPath  string
		expected string
	}{
		{
			name:     "Gorilla regex in braces",
			rawPath:  "/users/{id:[0-9]+}",
			expected: "/users/{id}",
		},
		{
			name:     "Gin colon params",
			rawPath:  "/users/:id",
			expected: "/users/{id}",
		},
		{
			name:     "Already normalized braces",
			rawPath:  "/users/{id}",
			expected: "/users/{id}",
		},
		{
			name:     "Multiple path params",
			rawPath:  "/users/:userId/posts/:postId",
			expected: "/users/{userId}/posts/{postId}",
		},
		{
			name:     "Gorilla multiple params",
			rawPath:  "/users/{id:[0-9]+}/posts/{pid:[a-z]+}",
			expected: "/users/{id}/posts/{pid}",
		},
		{
			name:     "URL with protocol (not normalized)",
			rawPath:  "http://example.com/path",
			expected: "http://example.com/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewRouteExtractor(token.NewFileSet(), "test.go")
			result := extractor.normalizePathParams(tt.rawPath)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestRouteExtractorGo122Pattern(t *testing.T) {
	code := `
package main

import "net/http"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tasks", GetTasks)
	mux.HandleFunc("POST /tasks/{id}", UpdateTask)
	mux.HandleFunc("GET /users/:uid", GetUser)
}
`
	routes := extractRoutesFromCode(t, code, "test.go")

	if len(routes) != 3 {
		t.Errorf("Expected 3 routes, got %d", len(routes))
	}

	// Check first route
	if routes[0].Method != "GET" {
		t.Errorf("Expected GET, got %s", routes[0].Method)
	}
	if routes[0].Path != "/tasks" {
		t.Errorf("Expected /tasks, got %s", routes[0].Path)
	}

	// Check second route with path param
	if routes[1].Method != "POST" {
		t.Errorf("Expected POST, got %s", routes[1].Method)
	}
	if routes[1].Path != "/tasks/{id}" {
		t.Errorf("Expected /tasks/{id}, got %s", routes[1].Path)
	}

	// Check third route with colon param (should be normalized)
	if routes[2].Method != "GET" {
		t.Errorf("Expected GET, got %s", routes[2].Method)
	}
	if routes[2].Path != "/users/{uid}" {
		t.Errorf("Expected /users/{uid}, got %s", routes[2].Path)
	}
}

func TestRouteExtractorChiNestedSimple(t *testing.T) {
	code := `
package main

import "github.com/go-chi/chi"

func main() {
	r := chi.NewRouter()

	r.Route("/api", func(r chi.Router) {
		r.Get("/users", GetUsers)
	})
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	extractor := NewRouteExtractor(fset, "test.go")
	routes := extractor.Extract(f)

	if len(routes) != 1 {
		t.Errorf("Expected 1 route, got %d", len(routes))
		for i, route := range routes {
			t.Logf("  %d: %s %s", i, route.Method, route.Path)
		}
		return
	}

	if routes[0].Path != "/api/users" {
		t.Errorf("Expected /api/users, got %s", routes[0].Path)
	}
}

func TestRouteExtractorChiNestedRouting(t *testing.T) {
	code := `
package main

import "github.com/go-chi/chi"

func main() {
	r := chi.NewRouter()

	r.Route("/api", func(r chi.Router) {
		r.Get("/users", GetUsers)
		r.Post("/users", CreateUser)

		r.Route("/admin", func(r chi.Router) {
			r.Get("/stats", GetStats)
		})
	})

	r.Get("/health", Health)
}
`
	routes := extractRoutesFromCode(t, code, "test.go")

	t.Logf("Found %d routes:", len(routes))
	for i, route := range routes {
		t.Logf("  %d: %s %s (handler=%s, rawpath=%s, framework=%s)", i, route.Method, route.Path, route.Handler, route.RawPath, route.Framework)
	}

	if len(routes) < 4 {
		t.Errorf("Expected at least 4 routes, got %d", len(routes))
	}

	// Check nested routes have correct paths
	foundNestedUser := false
	foundNestedPost := false
	foundNestedStats := false
	foundHealth := false

	for _, route := range routes {
		if route.Path == "/api/users" && route.Method == "GET" {
			foundNestedUser = true
		}
		if route.Path == "/api/users" && route.Method == "POST" {
			foundNestedPost = true
		}
		if route.Path == "/api/admin/stats" && route.Method == "GET" {
			foundNestedStats = true
		}
		if route.Path == "/health" && route.Method == "GET" {
			foundHealth = true
		}
	}

	if !foundNestedUser {
		t.Error("Expected /api/users GET route")
	}
	if !foundNestedPost {
		t.Error("Expected /api/users POST route")
	}
	if !foundNestedStats {
		t.Error("Expected /api/admin/stats GET route")
	}
	if !foundHealth {
		t.Error("Expected /health GET route")
	}
}

func TestRouteExtractorGinEchoStyleRoutes(t *testing.T) {
	code := `
package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/users", GetUsers)
	r.POST("/users", CreateUser)
	r.PUT("/users/:id", UpdateUser)
	r.DELETE("/users/{id:[0-9]+}", DeleteUser)
}
`
	routes := extractRoutesFromCode(t, code, "test.go")

	if len(routes) != 4 {
		t.Errorf("Expected 4 routes, got %d", len(routes))
	}

	// Check GET route
	if routes[0].Method != "GET" {
		t.Errorf("Expected GET, got %s", routes[0].Method)
	}
	if routes[0].Framework != "gin" {
		t.Errorf("Expected gin framework, got %s", routes[0].Framework)
	}

	// Check PUT route with colon param
	if routes[2].Path != "/users/{id}" {
		t.Errorf("Expected /users/{id}, got %s", routes[2].Path)
	}

	// Check DELETE route with gorilla-style regex
	if routes[3].Path != "/users/{id}" {
		t.Errorf("Expected /users/{id}, got %s", routes[3].Path)
	}
}

func TestRouteExtractorChiCamelCaseStyle(t *testing.T) {
	code := `
package main

import "github.com/go-chi/chi"

func main() {
	r := chi.NewRouter()
	r.Get("/users", GetUsers)
	r.Post("/users", CreateUser)
	r.Put("/users/:id", UpdateUser)
	r.Delete("/users/{id}", DeleteUser)
}
`
	routes := extractRoutesFromCode(t, code, "test.go")

	if len(routes) != 4 {
		t.Errorf("Expected 4 routes, got %d", len(routes))
	}

	// Check that camelCase methods work
	methods := []string{"GET", "POST", "PUT", "DELETE"}
	for i, method := range methods {
		if routes[i].Method != method {
			t.Errorf("Route %d: expected %s, got %s", i, method, routes[i].Method)
		}
	}

	// Check framework detection
	if routes[0].Framework != "chi" {
		t.Errorf("Expected chi framework, got %s", routes[0].Framework)
	}
}

func TestRouteExtractorWithMiddleware(t *testing.T) {
	code := `
package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/protected", AuthMiddleware, GetProtected)
	r.POST("/admin", AuthMiddleware, AdminMiddleware, CreateAdmin)
}
`
	routes := extractRoutesFromCode(t, code, "test.go")

	if len(routes) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(routes))
	}

	// Check first route middleware
	if len(routes[0].Middleware) != 1 || routes[0].Middleware[0] != "AuthMiddleware" {
		t.Errorf("Expected [AuthMiddleware], got %v", routes[0].Middleware)
	}

	// Check second route middleware
	if len(routes[1].Middleware) != 2 {
		t.Errorf("Expected 2 middleware, got %d", len(routes[1].Middleware))
	}
}

func TestRouteExtractorFileAndLine(t *testing.T) {
	code := `
package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/users", GetUsers)
}
`
	routes := extractRoutesFromCode(t, code, "handlers/users.go")

	if len(routes) != 1 {
		t.Errorf("Expected 1 route, got %d", len(routes))
	}

	if routes[0].File != "handlers/users.go" {
		t.Errorf("Expected handlers/users.go, got %s", routes[0].File)
	}

	if routes[0].Line < 1 {
		t.Errorf("Expected positive line number, got %d", routes[0].Line)
	}
}

func TestExtractRoutesFromComments(t *testing.T) {
	comments := `
	// GetUser handles user retrieval
	// @Router /users/{id} [GET]
	// @Description Get user by ID
	// @Router /users [GET]
	`

	routes := ExtractRoutesFromComments(comments)

	if len(routes) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(routes))
	}

	if routes[0].Path != "/users/{id}" {
		t.Errorf("Expected /users/{id}, got %s", routes[0].Path)
	}

	if routes[0].Method != "GET" {
		t.Errorf("Expected GET, got %s", routes[0].Method)
	}

	if routes[1].Path != "/users" {
		t.Errorf("Expected /users, got %s", routes[1].Path)
	}
}

func TestRouteExtractorDuplicateGinEchoFix(t *testing.T) {
	// This tests that the duplicate Gin/Echo handling bug is fixed
	// Previously lines 63-67 were a duplicate of 57-61
	code := `
package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/users", GetUsers)
	r.POST("/users", CreateUser)
}
`
	routes := extractRoutesFromCode(t, code, "test.go")

	// Should get exactly 2 routes, not duplicates
	if len(routes) != 2 {
		t.Errorf("Expected 2 routes (no duplicates), got %d", len(routes))
	}

	if routes[0].Path != "/users" || routes[0].Method != "GET" {
		t.Error("First route should be GET /users")
	}

	if routes[1].Path != "/users" || routes[1].Method != "POST" {
		t.Error("Second route should be POST /users")
	}
}

// Helper function to extract routes from Go code
func extractRoutesFromCode(t *testing.T, code string, filename string) []ctxpkg.Route {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	extractor := NewRouteExtractor(fset, filename)
	return extractor.Extract(f)
}

// Test that routes are returned with proper context type
func TestRouteExtractorReturnsContextType(t *testing.T) {
	code := `
package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.GET("/test", Handler)
}
`
	routes := extractRoutesFromCode(t, code, "test.go")

	if len(routes) != 1 {
		t.Fatalf("Expected 1 route, got %d", len(routes))
	}

	route := routes[0]

	// Verify it's a context.Route type
	var _ ctxpkg.Route = route

	// Check new fields are available
	if route.RawPath == "" && route.Path != "" {
		t.Errorf("RawPath should be set: %s", route.RawPath)
	}

	if route.Framework == "" {
		t.Error("Framework field should be populated")
	}

	if route.File == "" {
		t.Error("File field should be populated")
	}
}

// Test path param normalization with various edge cases
func TestPathParamNormalizationEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		rawPath  string
		expected string
	}{
		{
			name:     "Empty regex in braces",
			rawPath:  "/users/{id:}",
			expected: "/users/{id}",
		},
		{
			name:     "Complex regex",
			rawPath:  "/files/{filename:.*}",
			expected: "/files/{filename}",
		},
		{
			name:     "Multiple colons in path",
			rawPath:  "/api:v1/:resource",
			expected: "/api:v1/{resource}",
		},
		{
			name:     "Path with port (preserved)",
			rawPath:  "localhost:8080/api",
			expected: "localhost:8080/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewRouteExtractor(token.NewFileSet(), "test.go")
			result := extractor.normalizePathParams(tt.rawPath)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
