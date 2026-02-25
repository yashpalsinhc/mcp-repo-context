package graph

import (
	"strings"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestBuildGraph_TwoReposOneDependency(t *testing.T) {
	infoA := &ctxpkg.ModuleInfo{ModulePath: "github.com/org/a"}
	infoB := &ctxpkg.ModuleInfo{
		ModulePath: "github.com/org/b",
		Dependencies: []ctxpkg.ModuleDependency{
			{Path: "github.com/org/a", Version: "v1.0.0", IsDirect: true},
		},
	}

	builder := NewGraphBuilder()
	graph := builder.BuildGraph(map[string]*ctxpkg.ModuleInfo{
		"repoA": infoA,
		"repoB": infoB,
	})

	if len(graph.Nodes) != 2 {
		t.Fatalf("Nodes = %d, want 2", len(graph.Nodes))
	}
	for _, node := range graph.Nodes {
		if !node.IsAnalyzed {
			t.Errorf("Node %q should be analyzed", node.ModulePath)
		}
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("Edges = %d, want 1", len(graph.Edges))
	}
	edge := graph.Edges[0]
	if edge.From != "github.com/org/b" || edge.To != "github.com/org/a" {
		t.Errorf("Edge = %s -> %s, want org/b -> org/a", edge.From, edge.To)
	}
	if edge.Version != "v1.0.0" {
		t.Errorf("Version = %q, want v1.0.0", edge.Version)
	}
	if !edge.Direct {
		t.Error("Edge should be direct")
	}
}

func TestBuildGraph_ExternalDependency(t *testing.T) {
	infoA := &ctxpkg.ModuleInfo{
		ModulePath: "github.com/org/a",
		Dependencies: []ctxpkg.ModuleDependency{
			{Path: "github.com/external/lib", Version: "v0.3.0", IsDirect: true},
		},
	}

	builder := NewGraphBuilder()
	graph := builder.BuildGraph(map[string]*ctxpkg.ModuleInfo{"repoA": infoA})

	if len(graph.Nodes) != 2 {
		t.Fatalf("Nodes = %d, want 2", len(graph.Nodes))
	}

	var externalNode *ctxpkg.DependencyNode
	for i, node := range graph.Nodes {
		if node.ModulePath == "github.com/external/lib" {
			externalNode = &graph.Nodes[i]
		}
	}
	if externalNode == nil {
		t.Fatal("external node not found")
	}
	if externalNode.IsAnalyzed {
		t.Error("external node should not be analyzed")
	}

	if len(graph.Edges) != 1 {
		t.Fatalf("Edges = %d, want 1", len(graph.Edges))
	}
}

func TestBuildGraph_NoDependencies(t *testing.T) {
	infoA := &ctxpkg.ModuleInfo{ModulePath: "github.com/org/a"}

	builder := NewGraphBuilder()
	graph := builder.BuildGraph(map[string]*ctxpkg.ModuleInfo{"repoA": infoA})

	if len(graph.Nodes) != 1 {
		t.Fatalf("Nodes = %d, want 1", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("Edges = %d, want 0", len(graph.Edges))
	}
}

func TestBuildGraph_ExternalNodesUnanalyzed(t *testing.T) {
	info := &ctxpkg.ModuleInfo{
		ModulePath: "github.com/org/main",
		Dependencies: []ctxpkg.ModuleDependency{
			{Path: "github.com/ext/one", Version: "v1.0.0", IsDirect: true},
			{Path: "github.com/ext/two", Version: "v2.0.0", IsDirect: true},
			{Path: "github.com/ext/three", Version: "v3.0.0", IsDirect: true},
		},
	}

	builder := NewGraphBuilder()
	graph := builder.BuildGraph(map[string]*ctxpkg.ModuleInfo{"main": info})

	externalCount := 0
	for _, node := range graph.Nodes {
		if !node.IsAnalyzed {
			externalCount++
		}
	}
	if externalCount != 3 {
		t.Errorf("external nodes = %d, want 3", externalCount)
	}
}

func TestBuildGraphFiltered_ExcludeExternal(t *testing.T) {
	infoA := &ctxpkg.ModuleInfo{ModulePath: "github.com/org/a"}
	infoB := &ctxpkg.ModuleInfo{
		ModulePath: "github.com/org/b",
		Dependencies: []ctxpkg.ModuleDependency{
			{Path: "github.com/org/a", Version: "v1.0.0", IsDirect: true},
			{Path: "github.com/external/lib", Version: "v0.5.0", IsDirect: true},
		},
	}

	builder := NewGraphBuilder()
	graph := builder.BuildGraphFiltered(map[string]*ctxpkg.ModuleInfo{
		"repoA": infoA,
		"repoB": infoB,
	}, false)

	// Only analyzed nodes should appear
	for _, node := range graph.Nodes {
		if !node.IsAnalyzed {
			t.Errorf("unexpected external node: %s", node.ModulePath)
		}
	}
	// Only edge to analyzed repo
	if len(graph.Edges) != 1 {
		t.Fatalf("Edges = %d, want 1", len(graph.Edges))
	}
	if graph.Edges[0].To != "github.com/org/a" {
		t.Errorf("Edge To = %q, want org/a", graph.Edges[0].To)
	}
}

func TestGenerateDependencyMermaid(t *testing.T) {
	graph := &ctxpkg.DependencyGraph{
		Nodes: []ctxpkg.DependencyNode{
			{ModulePath: "github.com/org/a", IsAnalyzed: true},
			{ModulePath: "github.com/org/b", IsAnalyzed: true},
			{ModulePath: "github.com/ext/lib", IsAnalyzed: false},
		},
		Edges: []ctxpkg.DependencyEdge{
			{From: "github.com/org/b", To: "github.com/org/a", Version: "v1.0.0"},
			{From: "github.com/org/b", To: "github.com/ext/lib", Version: "v0.3.0"},
		},
	}

	output := GenerateDependencyMermaid(graph)

	if !strings.HasPrefix(output, "graph TD") {
		t.Error("Mermaid output should start with 'graph TD'")
	}
	// Analyzed nodes use box notation
	if !strings.Contains(output, `["github.com/org/a"]`) {
		t.Error("analyzed node should use box notation")
	}
	// External nodes use rounded notation
	if !strings.Contains(output, `("github.com/ext/lib")`) {
		t.Error("external node should use rounded notation")
	}
	// Edge with version label
	if !strings.Contains(output, `"v1.0.0"`) {
		t.Error("edge should contain version label")
	}
}

func TestGenerateDependencyDOT(t *testing.T) {
	graph := &ctxpkg.DependencyGraph{
		Nodes: []ctxpkg.DependencyNode{
			{ModulePath: "github.com/org/a", IsAnalyzed: true},
			{ModulePath: "github.com/ext/lib", IsAnalyzed: false},
		},
		Edges: []ctxpkg.DependencyEdge{
			{From: "github.com/org/a", To: "github.com/ext/lib", Version: "v1.0.0"},
		},
	}

	output := GenerateDependencyDOT(graph)

	if !strings.HasPrefix(output, "digraph") {
		t.Error("DOT output should start with 'digraph'")
	}
	if !strings.Contains(output, "shape=box") {
		t.Error("analyzed node should use box shape")
	}
	if !strings.Contains(output, "style=dashed") {
		t.Error("external node should use dashed style")
	}
	if !strings.Contains(output, `label="v1.0.0"`) {
		t.Error("edge should have version label")
	}
}

func TestGenerateTextSummary(t *testing.T) {
	graph := &ctxpkg.DependencyGraph{
		Nodes: []ctxpkg.DependencyNode{
			{ModulePath: "github.com/org/a", IsAnalyzed: true},
			{ModulePath: "github.com/org/b", IsAnalyzed: true},
			{ModulePath: "github.com/ext/lib", IsAnalyzed: false},
		},
		Edges: []ctxpkg.DependencyEdge{
			{From: "github.com/org/b", To: "github.com/org/a", Version: "v1.0.0"},
			{From: "github.com/org/b", To: "github.com/ext/lib", Version: "v0.3.0"},
		},
	}

	output := GenerateTextSummary(graph)

	if !strings.Contains(output, "github.com/org/b depends on") {
		t.Error("summary should mention repo-b depends on")
	}
	if !strings.Contains(output, "[analyzed]") {
		t.Error("summary should mark analyzed deps")
	}
	if !strings.Contains(output, "[external]") {
		t.Error("summary should mark external deps")
	}
	if !strings.Contains(output, "github.com/org/a: no dependencies") {
		t.Error("summary should note repo-a has no deps")
	}
}
