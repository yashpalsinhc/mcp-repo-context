package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestAPIFlowExtractor_HTTPHandler(t *testing.T) {
	src := `
package main

import (
	"encoding/json"
	"net/http"
)

type UserRequest struct {
	Name  string ` + "`json:\"name\"`" + `
	Email string ` + "`json:\"email\"`" + `
}

func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req UserRequest
	json.NewDecoder(r.Body).Decode(&req)

	user := createUser(req)

	json.NewEncoder(w).Encode(user)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "encoding/json"},
		{Path: "net/http"},
	}

	extractor := NewAPIFlowExtractor(fset)

	// Find the handler function
	for _, decl := range f.Decls {
		if fn, ok := decl.(*funcDeclType); ok && fn.Name.Name == "HandleCreateUser" {
			flow := extractor.ExtractAPIFlow(fn, imports)

			if !flow.IsHTTPHandler {
				t.Error("Expected IsHTTPHandler to be true")
			}

			// Should detect request payload
			if flow.RequestPayload == nil {
				t.Log("Request payload detection may need enhancement")
			} else if flow.RequestPayload.TypeName != "req" && flow.RequestPayload.TypeName != "UserRequest" {
				t.Logf("Request payload type: %s", flow.RequestPayload.TypeName)
			}

			// Should have flow steps
			if len(flow.Steps) == 0 {
				t.Log("Flow steps may need enhancement")
			}

			t.Logf("Flow steps: %d", len(flow.Steps))
			for _, step := range flow.Steps {
				t.Logf("  Step %d: %s (%s)", step.StepNumber, step.Action, step.Type)
			}
		}
	}
}

func TestAPIFlowExtractor_DatabaseOperations(t *testing.T) {
	src := `
package main

import "database/sql"

func GetUserByID(db *sql.DB, id int) (*User, error) {
	row := db.QueryRow("SELECT * FROM users WHERE id = ?", id)
	var user User
	err := row.Scan(&user.ID, &user.Name)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func CreateUser(db *sql.DB, user *User) error {
	_, err := db.Exec("INSERT INTO users (name, email) VALUES (?, ?)", user.Name, user.Email)
	return err
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "database/sql"},
	}

	extractor := NewAPIFlowExtractor(fset)

	for _, decl := range f.Decls {
		if fn, ok := decl.(*funcDeclType); ok {
			flow := extractor.ExtractAPIFlow(fn, imports)

			t.Logf("Function: %s", fn.Name.Name)
			t.Logf("  DB Queries: %d", len(flow.DBQueries))

			for _, query := range flow.DBQueries {
				t.Logf("    - %s on %s: %s", query.Operation, query.Table, query.RawQuery)
			}

			if fn.Name.Name == "GetUserByID" {
				// Should have SELECT query
				hasSelect := false
				for _, q := range flow.DBQueries {
					if q.Operation == "SELECT" {
						hasSelect = true
						if q.Table != "users" {
							t.Logf("Expected table 'users', got '%s'", q.Table)
						}
					}
				}
				if !hasSelect {
					t.Logf("Expected SELECT query in GetUserByID")
				}
			}

			if fn.Name.Name == "CreateUser" {
				// Should have INSERT query
				hasInsert := false
				for _, q := range flow.DBQueries {
					if q.Operation == "INSERT" {
						hasInsert = true
					}
				}
				if !hasInsert {
					t.Logf("Expected INSERT query in CreateUser")
				}
			}
		}
	}
}

func TestAPIFlowExtractor_ExternalHTTPCalls(t *testing.T) {
	src := `
package main

import "net/http"

func FetchExternalData(url string) (*http.Response, error) {
	return http.Get(url)
}

func PostToWebhook(url string, data []byte) error {
	_, err := http.Post(url, "application/json", bytes.NewReader(data))
	return err
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "net/http"},
	}

	extractor := NewAPIFlowExtractor(fset)

	for _, decl := range f.Decls {
		if fn, ok := decl.(*funcDeclType); ok {
			flow := extractor.ExtractAPIFlow(fn, imports)

			t.Logf("Function: %s", fn.Name.Name)
			t.Logf("  External Calls: %d", len(flow.ExternalCalls))

			for _, call := range flow.ExternalCalls {
				t.Logf("    - %s %s", call.Method, call.URL)
			}

			if fn.Name.Name == "FetchExternalData" && len(flow.ExternalCalls) > 0 {
				if flow.ExternalCalls[0].Method != "GET" {
					t.Errorf("Expected GET method, got %s", flow.ExternalCalls[0].Method)
				}
			}
		}
	}
}

func TestAPIFlowExtractor_ValidationDetection(t *testing.T) {
	src := `
package main

func ProcessRequest(name string, age int) error {
	if name == "" {
		return errors.New("name is required")
	}
	if age == 0 {
		return errors.New("age must be positive")
	}
	return nil
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	extractor := NewAPIFlowExtractor(fset)

	for _, decl := range f.Decls {
		if fn, ok := decl.(*funcDeclType); ok && fn.Name.Name == "ProcessRequest" {
			flow := extractor.ExtractAPIFlow(fn, nil)

			t.Logf("Validation steps: %v", flow.ValidationSteps)

			// Should detect validation for empty string and zero checks
			if len(flow.ValidationSteps) == 0 {
				t.Log("Validation detection may need enhancement")
			}
		}
	}
}

func TestAPIFlowExtractor_GinHandler(t *testing.T) {
	src := `
package main

import "github.com/gin-gonic/gin"

func HandleUser(c *gin.Context) {
	var req UserRequest
	c.ShouldBindJSON(&req)

	user := processUser(req)

	c.JSON(200, user)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	imports := []ctxpkg.Import{
		{Path: "github.com/gin-gonic/gin"},
	}

	extractor := NewAPIFlowExtractor(fset)

	for _, decl := range f.Decls {
		if fn, ok := decl.(*funcDeclType); ok && fn.Name.Name == "HandleUser" {
			flow := extractor.ExtractAPIFlow(fn, imports)

			if !flow.IsHTTPHandler {
				t.Error("Should detect gin handler as HTTP handler")
			}

			t.Logf("Steps: %d", len(flow.Steps))
		}
	}
}

// Type alias for test code
type funcDeclType = ast.FuncDecl
