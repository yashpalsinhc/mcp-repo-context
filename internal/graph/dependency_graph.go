package graph

import (
	"fmt"
	"sort"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// GraphBuilder computes cross-repo dependency graphs from ModuleInfo data.
type GraphBuilder struct{}

// NewGraphBuilder creates a new GraphBuilder.
func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{}
}

// BuildGraph builds a dependency graph from the given module infos.
// All external dependencies are included.
func (b *GraphBuilder) BuildGraph(moduleInfos map[string]*ctxpkg.ModuleInfo) *ctxpkg.DependencyGraph {
	return b.BuildGraphFiltered(moduleInfos, true)
}

// BuildGraphFiltered builds a dependency graph, optionally excluding external deps.
func (b *GraphBuilder) BuildGraphFiltered(moduleInfos map[string]*ctxpkg.ModuleInfo, includeExternal bool) *ctxpkg.DependencyGraph {
	graph := &ctxpkg.DependencyGraph{
		Nodes: []ctxpkg.DependencyNode{},
		Edges: []ctxpkg.DependencyEdge{},
	}

	// Build lookup: module_path -> repo_id
	moduleToRepo := make(map[string]string)
	for repoID, info := range moduleInfos {
		if info != nil {
			moduleToRepo[info.ModulePath] = repoID
		}
	}

	// Track added nodes
	addedNodes := make(map[string]bool) // module_path -> added

	// Add analyzed repo nodes
	for repoID, info := range moduleInfos {
		if info == nil {
			continue
		}
		graph.Nodes = append(graph.Nodes, ctxpkg.DependencyNode{
			ModulePath:  info.ModulePath,
			RepoID:      repoID,
			IsAnalyzed:  true,
			PackageType: "module",
		})
		addedNodes[info.ModulePath] = true
	}

	// Build edges and add external nodes
	for _, info := range moduleInfos {
		if info == nil {
			continue
		}
		for _, dep := range info.Dependencies {
			if _, isAnalyzed := moduleToRepo[dep.Path]; isAnalyzed {
				// Internal dependency (between analyzed repos)
				graph.Edges = append(graph.Edges, ctxpkg.DependencyEdge{
					From:    info.ModulePath,
					To:      dep.Path,
					Version: dep.Version,
					Direct:  dep.IsDirect,
				})
			} else if includeExternal {
				// External dependency
				if !addedNodes[dep.Path] {
					graph.Nodes = append(graph.Nodes, ctxpkg.DependencyNode{
						ModulePath: dep.Path,
						IsAnalyzed: false,
					})
					addedNodes[dep.Path] = true
				}
				graph.Edges = append(graph.Edges, ctxpkg.DependencyEdge{
					From:    info.ModulePath,
					To:      dep.Path,
					Version: dep.Version,
					Direct:  dep.IsDirect,
				})
			}
		}
	}

	// Sort for deterministic output
	sort.Slice(graph.Nodes, func(i, j int) bool {
		return graph.Nodes[i].ModulePath < graph.Nodes[j].ModulePath
	})
	sort.Slice(graph.Edges, func(i, j int) bool {
		if graph.Edges[i].From != graph.Edges[j].From {
			return graph.Edges[i].From < graph.Edges[j].From
		}
		return graph.Edges[i].To < graph.Edges[j].To
	})

	return graph
}

// sanitizeMermaidID creates a safe Mermaid node ID from a module path.
func sanitizeMermaidID(modulePath string) string {
	r := strings.NewReplacer("/", "_", ".", "_", "-", "_")
	return r.Replace(modulePath)
}

// GenerateDependencyMermaid generates a Mermaid diagram from a dependency graph.
func GenerateDependencyMermaid(graph *ctxpkg.DependencyGraph) string {
	var sb strings.Builder
	sb.WriteString("graph TD\n")

	for _, node := range graph.Nodes {
		id := sanitizeMermaidID(node.ModulePath)
		if node.IsAnalyzed {
			sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", id, node.ModulePath))
		} else {
			sb.WriteString(fmt.Sprintf("    %s(\"%s\")\n", id, node.ModulePath))
		}
	}

	for _, edge := range graph.Edges {
		fromID := sanitizeMermaidID(edge.From)
		toID := sanitizeMermaidID(edge.To)
		if edge.Version != "" {
			sb.WriteString(fmt.Sprintf("    %s -->|\"%s\"| %s\n", fromID, edge.Version, toID))
		} else {
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", fromID, toID))
		}
	}

	return sb.String()
}

// GenerateDependencyDOT generates a DOT/Graphviz diagram from a dependency graph.
func GenerateDependencyDOT(graph *ctxpkg.DependencyGraph) string {
	var sb strings.Builder
	sb.WriteString("digraph dependencies {\n")
	sb.WriteString("    rankdir=LR;\n")

	for _, node := range graph.Nodes {
		id := sanitizeMermaidID(node.ModulePath)
		if node.IsAnalyzed {
			sb.WriteString(fmt.Sprintf("    \"%s\" [shape=box, label=\"%s\"];\n", id, node.ModulePath))
		} else {
			sb.WriteString(fmt.Sprintf("    \"%s\" [shape=ellipse, style=dashed, label=\"%s\"];\n", id, node.ModulePath))
		}
	}

	for _, edge := range graph.Edges {
		fromID := sanitizeMermaidID(edge.From)
		toID := sanitizeMermaidID(edge.To)
		if edge.Version != "" {
			sb.WriteString(fmt.Sprintf("    \"%s\" -> \"%s\" [label=\"%s\"];\n", fromID, toID, edge.Version))
		} else {
			sb.WriteString(fmt.Sprintf("    \"%s\" -> \"%s\";\n", fromID, toID))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// GenerateTextSummary generates a human-readable text summary of the dependency graph.
func GenerateTextSummary(graph *ctxpkg.DependencyGraph) string {
	var sb strings.Builder
	sb.WriteString("Dependency Summary:\n")

	// Group edges by source
	edgesByFrom := make(map[string][]ctxpkg.DependencyEdge)
	for _, edge := range graph.Edges {
		edgesByFrom[edge.From] = append(edgesByFrom[edge.From], edge)
	}

	// Build node lookup for analyzed status
	nodeStatus := make(map[string]bool)
	for _, node := range graph.Nodes {
		nodeStatus[node.ModulePath] = node.IsAnalyzed
	}

	// Report each analyzed node's dependencies
	for _, node := range graph.Nodes {
		if !node.IsAnalyzed {
			continue
		}
		edges := edgesByFrom[node.ModulePath]
		if len(edges) == 0 {
			sb.WriteString(fmt.Sprintf("- %s: no dependencies on analyzed repos\n", node.ModulePath))
			continue
		}
		sb.WriteString(fmt.Sprintf("- %s depends on:\n", node.ModulePath))
		for _, edge := range edges {
			status := "external"
			if nodeStatus[edge.To] {
				status = "analyzed"
			}
			if edge.Version != "" {
				sb.WriteString(fmt.Sprintf("  - %s (%s) [%s]\n", edge.To, edge.Version, status))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s [%s]\n", edge.To, status))
			}
		}
	}

	return sb.String()
}
