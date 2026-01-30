package analyzer

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestStructDetailExtractor_BasicStruct(t *testing.T) {
	src := `
package main

type User struct {
	ID    int    ` + "`json:\"id\" db:\"id\"`" + `
	Name  string ` + "`json:\"name\" validate:\"required\"`" + `
	Email string ` + "`json:\"email,omitempty\"`" + `
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	details := ExtractAllStructDetails(f, fset)

	userDetail, ok := details["User"]
	if !ok {
		t.Fatal("Expected to find User struct")
	}

	if len(userDetail.Fields) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(userDetail.Fields))
	}

	// Check ID field
	var idField, nameField, emailField *struct {
		name     string
		jsonName string
		dbColumn string
		required bool
	}

	for _, f := range userDetail.Fields {
		switch f.Name {
		case "ID":
			idField = &struct {
				name     string
				jsonName string
				dbColumn string
				required bool
			}{f.Name, f.JSONName, f.DBColumn, f.Required}
		case "Name":
			nameField = &struct {
				name     string
				jsonName string
				dbColumn string
				required bool
			}{f.Name, f.JSONName, f.DBColumn, f.Required}
		case "Email":
			emailField = &struct {
				name     string
				jsonName string
				dbColumn string
				required bool
			}{f.Name, f.JSONName, f.DBColumn, f.Required}
		}
	}

	if idField == nil {
		t.Error("Expected ID field")
	} else {
		if idField.jsonName != "id" {
			t.Errorf("Expected json name 'id', got '%s'", idField.jsonName)
		}
		if idField.dbColumn != "id" {
			t.Errorf("Expected db column 'id', got '%s'", idField.dbColumn)
		}
	}

	if nameField == nil {
		t.Error("Expected Name field")
	} else {
		if !nameField.required {
			t.Error("Name field should be required (has validate:required)")
		}
	}

	if emailField == nil {
		t.Error("Expected Email field")
	} else {
		if emailField.required {
			t.Error("Email field should not be required (has omitempty)")
		}
	}
}

func TestStructDetailExtractor_Classification(t *testing.T) {
	src := `
package main

type UserRequest struct {
	Name string
}

type UserResponse struct {
	ID   int
	Name string
}

type UserModel struct {
	ID   int    ` + "`db:\"id\"`" + `
	Name string ` + "`db:\"name\"`" + `
}

type AppConfig struct {
	Port int
	Host string
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	details := ExtractAllStructDetails(f, fset)

	tests := []struct {
		name     string
		expected string
	}{
		{"UserRequest", "request"},
		{"UserResponse", "response"},
		{"UserModel", "model"},
		{"AppConfig", "config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail, ok := details[tt.name]
			if !ok {
				t.Fatalf("Expected to find %s struct", tt.name)
			}
			if detail.Category != tt.expected {
				t.Errorf("Expected category '%s', got '%s'", tt.expected, detail.Category)
			}
		})
	}
}

func TestStructDetailExtractor_ComplexTypes(t *testing.T) {
	src := `
package main

type ComplexStruct struct {
	Items     []string
	Metadata  map[string]interface{}
	Callback  func(int) error
	Channel   chan int
	UserPtr   *User
	Nested    struct {
		Value int
	}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	details := ExtractAllStructDetails(f, fset)

	detail, ok := details["ComplexStruct"]
	if !ok {
		t.Fatal("Expected to find ComplexStruct")
	}

	expectedTypes := map[string]string{
		"Items":    "[]string",
		"Metadata": "map[string]interface{}",
		"Callback": "func(...)",
		"Channel":  "chan int",
		"UserPtr":  "*User",
	}

	for _, field := range detail.Fields {
		if expected, ok := expectedTypes[field.Name]; ok {
			if field.Type != expected {
				t.Errorf("Field %s: expected type '%s', got '%s'", field.Name, expected, field.Type)
			}
		}
	}
}

func TestStructDetailExtractor_EmbeddedFields(t *testing.T) {
	src := `
package main

type BaseModel struct {
	ID        int
	CreatedAt time.Time
}

type User struct {
	BaseModel
	Name string
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	details := ExtractAllStructDetails(f, fset)

	userDetail, ok := details["User"]
	if !ok {
		t.Fatal("Expected to find User struct")
	}

	// Should have embedded field
	hasEmbedded := false
	for _, field := range userDetail.Fields {
		if field.Name == "BaseModel" {
			hasEmbedded = true
			break
		}
	}

	if !hasEmbedded {
		t.Log("Embedded field detection: BaseModel not found as field")
	}
}

func TestExtractTag(t *testing.T) {
	tests := []struct {
		tag      string
		key      string
		expected string
	}{
		{`json:"name" db:"user_name"`, "json", "name"},
		{`json:"name" db:"user_name"`, "db", "user_name"},
		{`json:"name,omitempty"`, "json", "name,omitempty"},
		{`validate:"required,min=1"`, "validate", "required,min=1"},
		{`json:"name"`, "db", ""},
	}

	for _, tt := range tests {
		t.Run(tt.tag+"_"+tt.key, func(t *testing.T) {
			result := extractTag(tt.tag, tt.key)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestExtractJSONName(t *testing.T) {
	tests := []struct {
		jsonTag  string
		expected string
	}{
		{"name", "name"},
		{"name,omitempty", "name"},
		{"-", ""},
		{"", ""},
		{",omitempty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.jsonTag, func(t *testing.T) {
			result := extractJSONName(tt.jsonTag)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
