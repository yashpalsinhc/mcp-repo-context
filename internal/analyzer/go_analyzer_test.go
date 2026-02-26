package analyzer

import (
	"context"
	"testing"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/repo"
)

func TestGoAnalyzer_Languages(t *testing.T) {
	a := NewGoAnalyzer()
	langs := a.Languages()

	if len(langs) != 1 || langs[0] != "go" {
		t.Errorf("expected [go], got %v", langs)
	}
}

func TestGoAnalyzer_AnalyzeFile_SimpleFunction(t *testing.T) {
	a := NewGoAnalyzer()
	ctx := context.Background()

	content := []byte(`package main

// HelloWorld prints a greeting.
func HelloWorld(name string) string {
	return "Hello, " + name
}
`)

	file := repo.FileInfo{
		Path:     "main.go",
		Hash:     "abc123",
		Language: "go",
		Size:     int64(len(content)),
		ModTime:  time.Now(),
	}

	result, err := a.AnalyzeFile(ctx, file, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check basic properties
	if result.Path != "main.go" {
		t.Errorf("expected path main.go, got %s", result.Path)
	}
	if result.Language != "go" {
		t.Errorf("expected language go, got %s", result.Language)
	}

	// Check function extraction
	if len(result.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(result.Functions))
	}

	fn := result.Functions[0]
	if fn.Name != "HelloWorld" {
		t.Errorf("expected function HelloWorld, got %s", fn.Name)
	}
	if !fn.IsPublic {
		t.Error("expected HelloWorld to be public")
	}
	if len(fn.Parameters) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(fn.Parameters))
	}
	if fn.Parameters[0].Name != "name" || fn.Parameters[0].Type != "string" {
		t.Errorf("unexpected parameter: %+v", fn.Parameters[0])
	}
	if len(fn.Returns) != 1 || fn.Returns[0] != "string" {
		t.Errorf("expected return type string, got %v", fn.Returns)
	}
	if fn.Description != "HelloWorld prints a greeting." {
		t.Errorf("unexpected description: %s", fn.Description)
	}
}

func TestGoAnalyzer_AnalyzeFile_Struct(t *testing.T) {
	a := NewGoAnalyzer()
	ctx := context.Background()

	content := []byte(`package models

// User represents a system user.
type User struct {
	ID        int64  ` + "`json:\"id\"`" + `
	Name      string ` + "`json:\"name\"`" + `
	email     string // private field
}
`)

	file := repo.FileInfo{
		Path:     "models/user.go",
		Hash:     "def456",
		Language: "go",
		Size:     int64(len(content)),
	}

	result, err := a.AnalyzeFile(ctx, file, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(result.Types))
	}

	typ := result.Types[0]
	if typ.Name != "User" {
		t.Errorf("expected type User, got %s", typ.Name)
	}
	if typ.Kind != "struct" {
		t.Errorf("expected kind struct, got %s", typ.Kind)
	}
	if !typ.IsPublic {
		t.Error("expected User to be public")
	}
	if len(typ.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(typ.Fields))
	}

	// Check fields
	fieldMap := make(map[string]bool)
	for _, f := range typ.Fields {
		fieldMap[f.Name] = f.IsPublic
	}
	if !fieldMap["ID"] || !fieldMap["Name"] {
		t.Error("expected ID and Name to be public")
	}
	if fieldMap["email"] {
		t.Error("expected email to be private")
	}
}

func TestGoAnalyzer_AnalyzeFile_Interface(t *testing.T) {
	a := NewGoAnalyzer()
	ctx := context.Background()

	content := []byte(`package storage

// Store defines storage operations.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}
`)

	file := repo.FileInfo{
		Path:     "storage/store.go",
		Hash:     "ghi789",
		Language: "go",
		Size:     int64(len(content)),
	}

	result, err := a.AnalyzeFile(ctx, file, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(result.Types))
	}

	typ := result.Types[0]
	if typ.Name != "Store" {
		t.Errorf("expected type Store, got %s", typ.Name)
	}
	if typ.Kind != "interface" {
		t.Errorf("expected kind interface, got %s", typ.Kind)
	}
	if len(typ.Methods) != 3 {
		t.Errorf("expected 3 methods, got %d: %v", len(typ.Methods), typ.Methods)
	}
}

func TestGoAnalyzer_AnalyzeFile_Imports(t *testing.T) {
	a := NewGoAnalyzer()
	ctx := context.Background()

	content := []byte(`package main

import (
	"fmt"
	"os"

	mylog "github.com/example/log"
	_ "github.com/example/driver"
)

func main() {}
`)

	file := repo.FileInfo{
		Path:     "main.go",
		Hash:     "jkl012",
		Language: "go",
		Size:     int64(len(content)),
	}

	result, err := a.AnalyzeFile(ctx, file, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Imports) != 4 {
		t.Fatalf("expected 4 imports, got %d", len(result.Imports))
	}

	importMap := make(map[string]string)
	for _, imp := range result.Imports {
		importMap[imp.Path] = imp.Alias
	}

	if importMap["fmt"] != "" {
		t.Error("expected fmt to have no alias")
	}
	if importMap["github.com/example/log"] != "mylog" {
		t.Errorf("expected alias mylog for log package, got %s", importMap["github.com/example/log"])
	}
	if importMap["github.com/example/driver"] != "_" {
		t.Error("expected blank import for driver")
	}
}

func TestGoAnalyzer_AnalyzeFile_Constants(t *testing.T) {
	a := NewGoAnalyzer()
	ctx := context.Background()

	content := []byte(`package config

const (
	MaxRetries = 3
	Timeout    = 30
	apiKey     = "secret"
)

const Version = "1.0.0"
`)

	file := repo.FileInfo{
		Path:     "config/config.go",
		Hash:     "mno345",
		Language: "go",
		Size:     int64(len(content)),
	}

	result, err := a.AnalyzeFile(ctx, file, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Constants) != 4 {
		t.Fatalf("expected 4 constants, got %d", len(result.Constants))
	}

	constMap := make(map[string]bool)
	for _, c := range result.Constants {
		constMap[c.Name] = c.IsPublic
	}

	if !constMap["MaxRetries"] || !constMap["Timeout"] || !constMap["Version"] {
		t.Error("expected MaxRetries, Timeout, Version to be public")
	}
	if constMap["apiKey"] {
		t.Error("expected apiKey to be private")
	}
}

func TestGoAnalyzer_AnalyzeFile_MethodReceiver(t *testing.T) {
	a := NewGoAnalyzer()
	ctx := context.Background()

	content := []byte(`package models

type User struct {
	Name string
}

func (u *User) SetName(name string) {
	u.Name = name
}

func (u User) GetName() string {
	return u.Name
}
`)

	file := repo.FileInfo{
		Path:     "models/user.go",
		Hash:     "pqr678",
		Language: "go",
		Size:     int64(len(content)),
	}

	result, err := a.AnalyzeFile(ctx, file, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(result.Functions))
	}

	for _, fn := range result.Functions {
		if fn.Name == "SetName" && fn.Receiver != "*User" {
			t.Errorf("expected receiver *User for SetName, got %s", fn.Receiver)
		}
		if fn.Name == "GetName" && fn.Receiver != "User" {
			t.Errorf("expected receiver User for GetName, got %s", fn.Receiver)
		}
	}
}

func TestGoAnalyzer_AnalyzeFile_Complexity(t *testing.T) {
	a := NewGoAnalyzer()
	ctx := context.Background()

	content := []byte(`package main

func complexFunction(x int) int {
	if x < 0 {
		return -1
	} else if x == 0 {
		return 0
	}

	for i := 0; i < x; i++ {
		if i%2 == 0 {
			continue
		}
	}

	switch x {
	case 1:
		return 1
	case 2:
		return 2
	default:
		return x
	}
}
`)

	file := repo.FileInfo{
		Path:     "main.go",
		Hash:     "stu901",
		Language: "go",
		Size:     int64(len(content)),
	}

	result, err := a.AnalyzeFile(ctx, file, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(result.Functions))
	}

	fn := result.Functions[0]
	// Complexity should be > 1 due to if/else, for, switch
	if fn.Complexity <= 1 {
		t.Errorf("expected complexity > 1, got %d", fn.Complexity)
	}
}

func TestGoAnalyzer_AnalyzeFile_ParseError(t *testing.T) {
	a := NewGoAnalyzer()
	ctx := context.Background()

	// Invalid Go code
	content := []byte(`package main

func broken( {

}
`)

	file := repo.FileInfo{
		Path:     "broken.go",
		Hash:     "vwx234",
		Language: "go",
		Size:     int64(len(content)),
	}

	result, err := a.AnalyzeFile(ctx, file, content)
	// Should not return error, just basic context
	if err != nil {
		t.Fatalf("expected no error for parse failure, got %v", err)
	}

	if result.Path != "broken.go" {
		t.Errorf("expected path broken.go, got %s", result.Path)
	}
	if result.Purpose != "Go source file (parse error)" {
		t.Errorf("expected parse error purpose, got %s", result.Purpose)
	}
}
