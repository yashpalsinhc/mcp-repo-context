//go:build integration

package comparison

import (
	"context"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// buildMuxFixture creates a RepoContext mimicking gorilla/mux.
func buildMuxFixture() *ctxpkg.RepoContext {
	return &ctxpkg.RepoContext{
		ID:      "github.com/gorilla/mux",
		Version: 0, // Pre-migration, should trigger lazy migration
		Files: map[string]*ctxpkg.FileContext{
			"mux.go": {
				Path:    "mux.go",
				Package: "mux",
				Functions: []ctxpkg.FunctionDef{
					{Name: "NewRouter", Signature: "func NewRouter() *Router", IsPublic: true, LineStart: 10, LineEnd: 20},
					{Name: "Vars", Signature: "func Vars(r *http.Request) map[string]string", IsPublic: true, LineStart: 25, LineEnd: 30},
					{Name: "SetURLVars", Signature: "func SetURLVars(r *http.Request, val map[string]string) *http.Request", IsPublic: true, LineStart: 35, LineEnd: 40},
				},
				Types: []ctxpkg.TypeDef{
					{Name: "Router", Kind: "struct", IsPublic: true, LineStart: 5},
					{Name: "RouteMatch", Kind: "struct", IsPublic: true, LineStart: 45},
				},
			},
			"route.go": {
				Path:    "route.go",
				Package: "mux",
				Functions: []ctxpkg.FunctionDef{
					{Name: "Handler", Receiver: "*Route", Signature: "func (r *Route) Handler(handler http.Handler) *Route", IsPublic: true, LineStart: 10, LineEnd: 15},
					{Name: "Path", Receiver: "*Route", Signature: "func (r *Route) Path(tpl string) *Route", IsPublic: true, LineStart: 20, LineEnd: 25},
				},
				Types: []ctxpkg.TypeDef{
					{Name: "Route", Kind: "struct", IsPublic: true, LineStart: 5},
				},
			},
			"router.go": {
				Path:    "router.go",
				Package: "mux",
				Functions: []ctxpkg.FunctionDef{
					{Name: "ServeHTTP", Receiver: "*Router", Signature: "func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request)", IsPublic: true, LineStart: 10, LineEnd: 50},
					{Name: "HandleFunc", Receiver: "*Router", Signature: "func (r *Router) HandleFunc(path string, f func(http.ResponseWriter, *http.Request)) *Route", IsPublic: true, LineStart: 55, LineEnd: 70},
					{Name: "Use", Receiver: "*Router", Signature: "func (r *Router) Use(mwf ...MiddlewareFunc)", IsPublic: true, LineStart: 75, LineEnd: 85},
				},
			},
		},
		Statistics: ctxpkg.RepoStatistics{
			FunctionCount: 8,
			TypeCount:     3,
			TotalLines:    500,
		},
	}
}

// buildHandlersFixture creates a RepoContext mimicking gorilla/handlers.
func buildHandlersFixture() *ctxpkg.RepoContext {
	return &ctxpkg.RepoContext{
		ID:      "github.com/gorilla/handlers",
		Version: 0, // Pre-migration
		Files: map[string]*ctxpkg.FileContext{
			"cors.go": {
				Path:    "cors.go",
				Package: "handlers",
				Functions: []ctxpkg.FunctionDef{
					{Name: "CORS", Signature: "func CORS(opts ...CORSOption) func(http.Handler) http.Handler", IsPublic: true, LineStart: 10, LineEnd: 30},
					{Name: "ServeHTTP", Receiver: "*cors", Signature: "func (ch *cors) ServeHTTP(w http.ResponseWriter, r *http.Request)", IsPublic: true, LineStart: 35, LineEnd: 80},
				},
				Types: []ctxpkg.TypeDef{
					{Name: "cors", Kind: "struct", IsPublic: false, LineStart: 5},
				},
			},
			"compress.go": {
				Path:    "compress.go",
				Package: "handlers",
				Functions: []ctxpkg.FunctionDef{
					{Name: "CompressHandler", Signature: "func CompressHandler(h http.Handler) http.Handler", IsPublic: true, LineStart: 10, LineEnd: 25},
					{Name: "Write", Receiver: "*CompressResponseWriter", Signature: "func (w *CompressResponseWriter) Write(b []byte) (int, error)", IsPublic: true, LineStart: 30, LineEnd: 50},
				},
				Types: []ctxpkg.TypeDef{
					{Name: "CompressResponseWriter", Kind: "struct", IsPublic: true, LineStart: 5},
				},
			},
			"logging.go": {
				Path:    "logging.go",
				Package: "handlers",
				Functions: []ctxpkg.FunctionDef{
					{Name: "LoggingHandler", Signature: "func LoggingHandler(out io.Writer, h http.Handler) http.Handler", IsPublic: true, LineStart: 10, LineEnd: 30},
				},
			},
			"recovery.go": {
				Path:    "recovery.go",
				Package: "handlers",
				Functions: []ctxpkg.FunctionDef{
					{Name: "RecoveryHandler", Signature: "func RecoveryHandler(opts ...RecoveryOption) func(h http.Handler) http.Handler", IsPublic: true, LineStart: 10, LineEnd: 30},
				},
			},
			"proxy_headers.go": {
				Path:    "proxy_headers.go",
				Package: "handlers",
				Functions: []ctxpkg.FunctionDef{
					{Name: "ProxyHeaders", Signature: "func ProxyHeaders(h http.Handler) http.Handler", IsPublic: true, LineStart: 10, LineEnd: 25},
				},
			},
			"canonical.go": {
				Path:    "canonical.go",
				Package: "handlers",
				Functions: []ctxpkg.FunctionDef{
					{Name: "CanonicalHost", Signature: "func CanonicalHost(domain string, code int) func(h http.Handler) http.Handler", IsPublic: true, LineStart: 10, LineEnd: 25},
				},
			},
			"content_type.go": {
				Path:    "content_type.go",
				Package: "handlers",
				Functions: []ctxpkg.FunctionDef{
					{Name: "ContentTypeHandler", Signature: "func ContentTypeHandler(h http.Handler, contentTypes ...string) http.Handler", IsPublic: true, LineStart: 10, LineEnd: 25},
				},
			},
		},
		Statistics: ctxpkg.RepoStatistics{
			FunctionCount: 9,
			TypeCount:     2,
			TotalLines:    800,
		},
	}
}

func TestIntegration_FixturesAreValid(t *testing.T) {
	mux := buildMuxFixture()
	handlers := buildHandlersFixture()

	if len(mux.Files) == 0 {
		t.Fatal("mux fixture has no files")
	}
	if len(handlers.Files) == 0 {
		t.Fatal("handlers fixture has no files")
	}

	// Verify mux has Router.ServeHTTP
	found := false
	for _, fc := range mux.Files {
		for _, fn := range fc.Functions {
			if fn.Name == "ServeHTTP" && fn.Receiver == "*Router" {
				found = true
			}
		}
	}
	if !found {
		t.Error("mux fixture missing Router.ServeHTTP")
	}

	// Verify handlers has cors.ServeHTTP
	found = false
	for _, fc := range handlers.Files {
		for _, fn := range fc.Functions {
			if fn.Name == "ServeHTTP" && fn.Receiver == "*cors" {
				found = true
			}
		}
	}
	if !found {
		t.Error("handlers fixture missing cors.ServeHTTP")
	}
}

func TestIntegration_GapCountReduced(t *testing.T) {
	mux := buildMuxFixture()
	handlers := buildHandlersFixture()

	comp := NewComparer()
	gaps, err := comp.FindGaps(context.Background(), []*ctxpkg.RepoContext{handlers}, mux)
	if err != nil {
		t.Fatalf("FindGaps: %v", err)
	}

	// Total handler functions = 9. With domain-aware gap analysis,
	// most middleware-specific functions (CORS, CompressHandler, LoggingHandler, etc.)
	// should be filtered out because they have low similarity to the routing domain.
	// A 60% reduction (keeping at most 6) is a reasonable threshold.
	totalHandlerFuncs := 9
	maxGaps := totalHandlerFuncs * 2 / 3 // 6
	if len(gaps) > maxGaps {
		t.Errorf("gap count = %d, want at most %d (60%% of handler functions)", len(gaps), maxGaps)
	}
}

func TestIntegration_NoFalseDuplicates(t *testing.T) {
	mux := buildMuxFixture()
	handlers := buildHandlersFixture()

	comp := NewComparer()
	duplicates, err := comp.FindDuplicates(context.Background(), []*ctxpkg.RepoContext{mux, handlers})
	if err != nil {
		t.Fatalf("FindDuplicates: %v", err)
	}

	for _, dg := range duplicates {
		if containsCrossRepoInstances(dg) {
			// With receiver-aware keys, Router.ServeHTTP and cors.ServeHTTP
			// should never be grouped together
			if dg.Name == "ServeHTTP" {
				t.Errorf("false duplicate: ServeHTTP grouped across repos without receiver qualification")
			}
			if dg.Name == "Write" {
				t.Errorf("false duplicate: Write grouped across repos without receiver qualification")
			}
		}
	}
}

func TestIntegration_NoFalseConflicts(t *testing.T) {
	mux := buildMuxFixture()
	handlers := buildHandlersFixture()

	comp := NewComparer()
	conflicts, err := comp.FindConflicts(context.Background(), []*ctxpkg.RepoContext{handlers}, mux)
	if err != nil {
		t.Fatalf("FindConflicts: %v", err)
	}

	for _, c := range conflicts {
		if c.Name == "ServeHTTP" {
			t.Errorf("false conflict for ServeHTTP: different receivers should not conflict")
		}
		if c.Name == "Write" {
			t.Errorf("false conflict for Write: different receivers should not conflict")
		}
	}
}

func TestIntegration_DifferentReceiversAreUnrelated(t *testing.T) {
	mux := buildMuxFixture()
	handlers := buildHandlersFixture()

	comp := NewComparer()
	result, err := comp.Compare(context.Background(), []*ctxpkg.RepoContext{mux, handlers}, CompareOptions{
		TargetRepoID:      mux.ID,
		IncludeDuplicates: true,
		IncludeConflicts:  true,
		IncludeGaps:       true,
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	// Check duplicates
	for _, dg := range result.Duplicates {
		if containsCrossRepoInstances(dg) {
			for _, inst := range dg.Instances {
				if inst.RepoID == mux.ID {
					for _, other := range dg.Instances {
						if other.RepoID == handlers.ID {
							t.Errorf("cross-repo duplicate found: %s", dg.Name)
						}
					}
				}
			}
		}
	}

	// Check conflicts - no plain "ServeHTTP" conflict
	for _, c := range result.Conflicts {
		if c.Name == "ServeHTTP" || c.Name == "Write" {
			t.Errorf("false conflict for %s", c.Name)
		}
	}
}

func TestIntegration_EndToEnd(t *testing.T) {
	mux := buildMuxFixture()
	handlers := buildHandlersFixture()

	comp := NewComparer()
	result, err := comp.Compare(context.Background(), []*ctxpkg.RepoContext{mux, handlers}, CompareOptions{
		TargetRepoID:        mux.ID,
		IncludeDuplicates:   true,
		IncludeConflicts:    true,
		IncludeGaps:         true,
		IncludeConsistency:  true,
		SimilarityThreshold: 0.8,
	})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	// Basic sanity
	if len(result.Repos) != 2 {
		t.Errorf("Repos = %d, want 2", len(result.Repos))
	}

	// No false duplicates from same-name-different-receiver methods
	for _, dg := range result.Duplicates {
		if containsCrossRepoInstances(dg) && (dg.Name == "ServeHTTP" || dg.Name == "Write") {
			t.Errorf("false duplicate: %s", dg.Name)
		}
	}

	// No false conflicts
	for _, c := range result.Conflicts {
		if c.Name == "ServeHTTP" || c.Name == "Write" {
			t.Errorf("false conflict: %s", c.Name)
		}
	}

	// Gap count should be reasonable
	totalHandlerFuncs := 9
	if len(result.Gaps) > totalHandlerFuncs {
		t.Errorf("gap count %d exceeds total handler functions %d", len(result.Gaps), totalHandlerFuncs)
	}
}

// containsCrossRepoInstances returns true if a duplicate group has instances from multiple repos.
func containsCrossRepoInstances(group DuplicateGroup) bool {
	repos := make(map[string]bool)
	for _, inst := range group.Instances {
		repos[inst.RepoID] = true
	}
	return len(repos) > 1
}
