package analyzer

import (
	"go/ast"
	"go/token"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// StructDetailExtractor extracts detailed information about struct types.
type StructDetailExtractor struct {
	fset *token.FileSet
}

// NewStructDetailExtractor creates a new struct detail extractor.
func NewStructDetailExtractor(fset *token.FileSet) *StructDetailExtractor {
	return &StructDetailExtractor{fset: fset}
}

// ExtractStructDetails extracts detailed field information from a struct type.
func (e *StructDetailExtractor) ExtractStructDetails(typeSpec *ast.TypeSpec) *ctxpkg.StructDetail {
	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return nil
	}

	detail := &ctxpkg.StructDetail{
		Name:   typeSpec.Name.Name,
		Fields: []ctxpkg.StructField{},
	}

	if structType.Fields == nil {
		return detail
	}

	for _, field := range structType.Fields.List {
		fieldInfo := e.extractFieldInfo(field)
		for _, f := range fieldInfo {
			detail.Fields = append(detail.Fields, f)
		}
	}

	// Classify the struct type
	detail.Category = e.classifyStruct(detail)

	return detail
}

// extractFieldInfo extracts information about a struct field.
func (e *StructDetailExtractor) extractFieldInfo(field *ast.Field) []ctxpkg.StructField {
	var fields []ctxpkg.StructField

	// Get field type
	fieldType := e.typeToString(field.Type)

	// Get field names (could be multiple for same type, or embedded)
	var names []string
	if len(field.Names) == 0 {
		// Embedded field
		names = []string{fieldType}
	} else {
		for _, n := range field.Names {
			names = append(names, n.Name)
		}
	}

	// Parse struct tag
	var jsonTag, validationTag, dbTag string
	var required bool
	if field.Tag != nil {
		tag := strings.Trim(field.Tag.Value, "`")
		jsonTag = extractTag(tag, "json")
		validationTag = extractTag(tag, "validate")
		dbTag = extractTag(tag, "db")

		// Check for omitempty (means not required)
		required = !strings.Contains(jsonTag, "omitempty")

		// Check for required validation
		if strings.Contains(validationTag, "required") {
			required = true
		}
	}

	for _, name := range names {
		sf := ctxpkg.StructField{
			Name:       name,
			Type:       fieldType,
			JSONName:   extractJSONName(jsonTag),
			DBColumn:   extractJSONName(dbTag), // DB tags usually have same format
			Validation: validationTag,
			Required:   required,
			IsExported: len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z',
		}

		// Add description from comments if available
		if field.Doc != nil {
			sf.Description = strings.TrimSpace(field.Doc.Text())
		} else if field.Comment != nil {
			sf.Description = strings.TrimSpace(field.Comment.Text())
		}

		fields = append(fields, sf)
	}

	return fields
}

// extractTag extracts a specific tag value from a struct tag string.
func extractTag(tag, key string) string {
	// Look for key:"value"
	search := key + `:"`
	idx := strings.Index(tag, search)
	if idx == -1 {
		return ""
	}

	start := idx + len(search)
	end := strings.Index(tag[start:], `"`)
	if end == -1 {
		return ""
	}

	return tag[start : start+end]
}

// extractJSONName extracts the field name from a json tag.
func extractJSONName(jsonTag string) string {
	if jsonTag == "" || jsonTag == "-" {
		return ""
	}
	// Handle json:"name,omitempty"
	parts := strings.Split(jsonTag, ",")
	if len(parts) > 0 && parts[0] != "" && parts[0] != "-" {
		return parts[0]
	}
	return ""
}

// classifyStruct determines what kind of struct this is.
func (e *StructDetailExtractor) classifyStruct(detail *ctxpkg.StructDetail) string {
	nameLower := strings.ToLower(detail.Name)

	// Request/Response types
	if strings.HasSuffix(nameLower, "request") || strings.HasSuffix(nameLower, "req") {
		return "request"
	}
	if strings.HasSuffix(nameLower, "response") || strings.HasSuffix(nameLower, "resp") {
		return "response"
	}
	if strings.HasSuffix(nameLower, "input") {
		return "input"
	}
	if strings.HasSuffix(nameLower, "output") {
		return "output"
	}

	// Database models
	hasDBTags := false
	for _, f := range detail.Fields {
		if f.DBColumn != "" {
			hasDBTags = true
			break
		}
	}
	if hasDBTags || strings.HasSuffix(nameLower, "model") || strings.HasSuffix(nameLower, "entity") {
		return "model"
	}

	// Config
	if strings.Contains(nameLower, "config") || strings.Contains(nameLower, "options") ||
		strings.Contains(nameLower, "settings") {
		return "config"
	}

	// DTO
	if strings.HasSuffix(nameLower, "dto") {
		return "dto"
	}

	return "struct"
}

// typeToString converts an AST type expression to a string.
func (e *StructDetailExtractor) typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	case *ast.StarExpr:
		return "*" + e.typeToString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + e.typeToString(t.Elt)
		}
		return "[...]" + e.typeToString(t.Elt)
	case *ast.MapType:
		return "map[" + e.typeToString(t.Key) + "]" + e.typeToString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + e.typeToString(t.Value)
	default:
		return "unknown"
	}
}

// ExtractAllStructDetails extracts details for all structs in a file.
func ExtractAllStructDetails(f *ast.File, fset *token.FileSet) map[string]*ctxpkg.StructDetail {
	extractor := NewStructDetailExtractor(fset)
	details := make(map[string]*ctxpkg.StructDetail)

	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			detail := extractor.ExtractStructDetails(typeSpec)
			if detail != nil {
				details[detail.Name] = detail
			}
		}
	}

	return details
}
