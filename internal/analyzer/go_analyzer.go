package analyzer

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"time"
	"unicode"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/repo"
)

// Ensure goAnalyzer implements Analyzer interface.
var _ Analyzer = (*goAnalyzer)(nil)

// goAnalyzer extracts context from Go source files (private struct).
type goAnalyzer struct{}

// NewGoAnalyzer creates a new Go analyzer.
func NewGoAnalyzer() Analyzer {
	return &goAnalyzer{}
}

// Name returns the name of this analyzer.
func (a *goAnalyzer) Name() string {
	return "go"
}

// Languages returns file extensions this analyzer handles.
func (a *goAnalyzer) Languages() []string {
	return []string{"go"}
}

// AnalyzeFile extracts context from a Go file.
func (a *goAnalyzer) AnalyzeFile(ctx context.Context, file repo.FileInfo, content []byte) (*ctxpkg.FileContext, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file.Path, content, parser.ParseComments)
	if err != nil {
		// Return basic context even if parsing fails
		return &ctxpkg.FileContext{
			Path:       file.Path,
			Hash:       file.Hash,
			Language:   "go",
			Size:       file.Size,
			LineCount:  countLines(content),
			Purpose:    "Go source file (parse error)",
			AnalyzedAt: time.Now(),
		}, nil
	}

	fileCtx := &ctxpkg.FileContext{
		Path:       file.Path,
		Hash:       file.Hash,
		Language:   "go",
		Package:    f.Name.Name, // Extract package name from AST
		Size:       file.Size,
		LineCount:  countLines(content),
		AnalyzedAt: time.Now(),
	}

	// Extract imports
	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		fileCtx.Imports = append(fileCtx.Imports, ctxpkg.Import{
			Path:  importPath,
			Alias: alias,
		})
	}

	// Extract declarations
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			a.extractGenDecl(fset, d, fileCtx)
		case *ast.FuncDecl:
			a.extractFuncDecl(fset, d, fileCtx, f.Decls)
		}
	}

	// Generate purpose from package doc or first comment
	fileCtx.Purpose = a.extractPurpose(f)

	// Extract concepts (keywords that describe what this file does)
	fileCtx.Concepts = a.extractConcepts(fileCtx)

	// Extract HTTP routes
	routeExtractor := NewRouteExtractor(fset, file.Path)
	fileCtx.Routes = routeExtractor.Extract(f)

	return fileCtx, nil
}

func (a *goAnalyzer) extractGenDecl(fset *token.FileSet, decl *ast.GenDecl, fileCtx *ctxpkg.FileContext) {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			typeDef := a.extractTypeSpec(fset, s, decl.Doc)
			fileCtx.Types = append(fileCtx.Types, typeDef)

			if typeDef.IsPublic {
				fileCtx.Exports = append(fileCtx.Exports, ctxpkg.Export{
					Name:        typeDef.Name,
					Kind:        "type",
					Signature:   typeDef.Kind,
					Description: typeDef.Description,
					LineStart:   typeDef.LineStart,
					LineEnd:     typeDef.LineEnd,
				})
			}

		case *ast.ValueSpec:
			for i, name := range s.Names {
				isPublic := isExported(name.Name)
				pos := fset.Position(name.Pos())

				var value string
				if i < len(s.Values) {
					value = formatNode(s.Values[i])
				}

				var typeName string
				if s.Type != nil {
					typeName = formatNode(s.Type)
				}

				if decl.Tok == token.CONST {
					fileCtx.Constants = append(fileCtx.Constants, ctxpkg.ConstantDef{
						Name:      name.Name,
						Type:      typeName,
						Value:     value,
						IsPublic:  isPublic,
						LineStart: pos.Line,
					})
				}

				if isPublic {
					kind := "var"
					if decl.Tok == token.CONST {
						kind = "const"
					}
					fileCtx.Exports = append(fileCtx.Exports, ctxpkg.Export{
						Name:      name.Name,
						Kind:      kind,
						Signature: typeName,
						LineStart: pos.Line,
					})
				}
			}
		}
	}
}

func (a *goAnalyzer) extractTypeSpec(fset *token.FileSet, spec *ast.TypeSpec, doc *ast.CommentGroup) ctxpkg.TypeDef {
	typeDef := ctxpkg.TypeDef{
		Name:      spec.Name.Name,
		IsPublic:  isExported(spec.Name.Name),
		LineStart: fset.Position(spec.Pos()).Line,
		LineEnd:   fset.Position(spec.End()).Line,
	}

	if doc != nil {
		typeDef.Description = strings.TrimSpace(doc.Text())
	}

	switch t := spec.Type.(type) {
	case *ast.StructType:
		typeDef.Kind = "struct"
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				fieldType := formatNode(field.Type)
				var tag string
				if field.Tag != nil {
					tag = field.Tag.Value
				}

				if len(field.Names) == 0 {
					// Embedded field
					typeDef.Fields = append(typeDef.Fields, ctxpkg.Field{
						Name:     fieldType,
						Type:     fieldType,
						Tag:      tag,
						IsPublic: isExported(fieldType),
					})
				} else {
					for _, name := range field.Names {
						typeDef.Fields = append(typeDef.Fields, ctxpkg.Field{
							Name:     name.Name,
							Type:     fieldType,
							Tag:      tag,
							IsPublic: isExported(name.Name),
						})
					}
				}
			}
		}

	case *ast.InterfaceType:
		typeDef.Kind = "interface"
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				if len(method.Names) > 0 {
					typeDef.Methods = append(typeDef.Methods, method.Names[0].Name)
				}
			}
		}

	default:
		typeDef.Kind = "alias"
	}

	return typeDef
}

func (a *goAnalyzer) extractFuncDecl(fset *token.FileSet, decl *ast.FuncDecl, fileCtx *ctxpkg.FileContext, fileDecls []ast.Decl) {
	funcDef := ctxpkg.FunctionDef{
		Name:      decl.Name.Name,
		IsPublic:  isExported(decl.Name.Name),
		LineStart: fset.Position(decl.Pos()).Line,
		LineEnd:   fset.Position(decl.End()).Line,
	}

	// Extract receiver
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		recv := decl.Recv.List[0]
		funcDef.Receiver = formatNode(recv.Type)
	}

	// Extract parameters
	if decl.Type.Params != nil {
		for _, param := range decl.Type.Params.List {
			paramType := formatNode(param.Type)
			if len(param.Names) == 0 {
				funcDef.Parameters = append(funcDef.Parameters, ctxpkg.Param{
					Name: "_",
					Type: paramType,
				})
			} else {
				for _, name := range param.Names {
					funcDef.Parameters = append(funcDef.Parameters, ctxpkg.Param{
						Name: name.Name,
						Type: paramType,
					})
				}
			}
		}
	}

	// Extract returns
	if decl.Type.Results != nil {
		for _, result := range decl.Type.Results.List {
			resultType := formatNode(result.Type)
			funcDef.Returns = append(funcDef.Returns, resultType)
		}
	}

	// Build signature
	funcDef.Signature = buildSignature(funcDef)

	// Extract description from doc comment
	if decl.Doc != nil {
		funcDef.Description = strings.TrimSpace(decl.Doc.Text())
	}

	// Calculate simple complexity (count of branches)
	funcDef.Complexity = a.calculateComplexity(decl.Body)

	// === DEEP CONTEXT EXTRACTION ===

	// Extract function behavior (what the function does)
	behaviorExtractor := NewBehaviorExtractor(fset)
	funcDef.Behavior = behaviorExtractor.Extract(decl, fileCtx.Imports)

	// Extract function calls
	callGraphBuilder := NewCallGraphBuilder()
	funcDef.Calls = callGraphBuilder.ExtractFunctionCalls(fset, decl, fileCtx.Path, fileCtx.Imports)

	// Extract error handling patterns
	errorExtractor := NewErrorHandlingExtractor(fset)
	funcDef.ErrorHandling = errorExtractor.Extract(decl)

	// Extract side effects
	funcDef.SideEffects = ExtractSideEffects(decl, fileCtx.Imports)

	// Extract used imports
	funcDef.UsesImports = a.extractUsedImports(decl, fileCtx.Imports)

	// Extract API flow (requests, responses, DB queries, external calls)
	apiFlowExtractor := NewAPIFlowExtractor(fset)
	apiFlowExtractor.SetFileDecls(fileDecls)
	funcDef.APIFlow = apiFlowExtractor.ExtractAPIFlow(decl, fileCtx.Imports)

	// Extract Kafka/async calls
	kafkaExtractor := NewKafkaExtractor(fset, fileDecls)
	funcDef.AsyncCalls = kafkaExtractor.Extract(decl, fileCtx.Imports)
	// Add kafka side effects
	for _, ac := range funcDef.AsyncCalls {
		effect := "kafka_" + ac.Direction
		if !containsString(funcDef.SideEffects, effect) {
			funcDef.SideEffects = append(funcDef.SideEffects, effect)
		}
	}

	// === END DEEP CONTEXT ===

	fileCtx.Functions = append(fileCtx.Functions, funcDef)

	if funcDef.IsPublic {
		fileCtx.Exports = append(fileCtx.Exports, ctxpkg.Export{
			Name:        funcDef.Name,
			Kind:        "function",
			Signature:   funcDef.Signature,
			Description: funcDef.Description,
			LineStart:   funcDef.LineStart,
			LineEnd:     funcDef.LineEnd,
		})
	}
}

// extractUsedImports finds which imports are actually used in a function.
func (a *goAnalyzer) extractUsedImports(fn *ast.FuncDecl, imports []ctxpkg.Import) []string {
	if fn.Body == nil {
		return nil
	}

	// Build import map
	importMap := make(map[string]string) // alias -> path
	for _, imp := range imports {
		alias := imp.Alias
		if alias == "" {
			parts := strings.Split(imp.Path, "/")
			alias = parts[len(parts)-1]
		}
		importMap[alias] = imp.Path
	}

	usedImports := make(map[string]bool)

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if path, ok := importMap[ident.Name]; ok {
					usedImports[path] = true
				}
			}
		}
		return true
	})

	result := make([]string, 0, len(usedImports))
	for imp := range usedImports {
		result = append(result, imp)
	}
	return result
}

func (a *goAnalyzer) calculateComplexity(body *ast.BlockStmt) int {
	if body == nil {
		return 1
	}

	complexity := 1 // Base complexity
	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt,
			*ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt,
			*ast.CaseClause, *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if be, ok := n.(*ast.BinaryExpr); ok {
				if be.Op == token.LAND || be.Op == token.LOR {
					complexity++
				}
			}
		}
		return true
	})
	return complexity
}

func (a *goAnalyzer) extractPurpose(f *ast.File) string {
	// Use package doc if available
	if f.Doc != nil {
		text := strings.TrimSpace(f.Doc.Text())
		// Take first sentence or first line
		if idx := strings.Index(text, "."); idx > 0 && idx < 200 {
			return text[:idx+1]
		}
		if idx := strings.Index(text, "\n"); idx > 0 {
			return text[:idx]
		}
		if len(text) > 200 {
			return text[:200] + "..."
		}
		return text
	}

	return "Package " + f.Name.Name
}

func (a *goAnalyzer) extractConcepts(fileCtx *ctxpkg.FileContext) []string {
	concepts := make(map[string]bool)

	// Add type names
	for _, t := range fileCtx.Types {
		concepts[strings.ToLower(t.Name)] = true
	}

	// Add function names (split camelCase)
	for _, fn := range fileCtx.Functions {
		for _, word := range splitCamelCase(fn.Name) {
			if len(word) > 2 {
				concepts[strings.ToLower(word)] = true
			}
		}
	}

	// Convert to slice
	result := make([]string, 0, len(concepts))
	for c := range concepts {
		result = append(result, c)
	}
	return result
}

// Helper functions

func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}

func formatNode(node ast.Node) string {
	switch n := node.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		return formatNode(n.X) + "." + n.Sel.Name
	case *ast.StarExpr:
		return "*" + formatNode(n.X)
	case *ast.ArrayType:
		if n.Len == nil {
			return "[]" + formatNode(n.Elt)
		}
		return "[" + formatNode(n.Len) + "]" + formatNode(n.Elt)
	case *ast.MapType:
		return "map[" + formatNode(n.Key) + "]" + formatNode(n.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + formatNode(n.Value)
	case *ast.Ellipsis:
		return "..." + formatNode(n.Elt)
	case *ast.BasicLit:
		return n.Value
	default:
		return "..."
	}
}

func buildSignature(fn ctxpkg.FunctionDef) string {
	var sb strings.Builder

	if fn.Receiver != "" {
		sb.WriteString("(" + fn.Receiver + ") ")
	}

	sb.WriteString(fn.Name + "(")

	for i, p := range fn.Parameters {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(p.Name + " " + p.Type)
	}
	sb.WriteString(")")

	if len(fn.Returns) > 0 {
		sb.WriteString(" ")
		if len(fn.Returns) > 1 {
			sb.WriteString("(")
		}
		sb.WriteString(strings.Join(fn.Returns, ", "))
		if len(fn.Returns) > 1 {
			sb.WriteString(")")
		}
	}

	return sb.String()
}

func countLines(content []byte) int {
	count := 1
	for _, b := range content {
		if b == '\n' {
			count++
		}
	}
	return count
}

func splitCamelCase(s string) []string {
	var words []string
	var word strings.Builder

	for _, r := range s {
		if unicode.IsUpper(r) && word.Len() > 0 {
			words = append(words, word.String())
			word.Reset()
		}
		word.WriteRune(r)
	}
	if word.Len() > 0 {
		words = append(words, word.String())
	}

	return words
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
