package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/yashpalc/mcp-repo-context/internal/flow"
	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// toolGetServiceMap handles the get_service_map tool call.
func (s *server) toolGetServiceMap(ctx context.Context, args map[string]any) callToolResult {
	orgID, ok := args["org_id"].(string)
	if !ok || orgID == "" {
		return errorResult("org_id is required")
	}

	if s.config.TopologyBuilder == nil {
		return errorResult("Service topology not configured. Ensure TopologyBuilder is initialized.")
	}

	topology, err := s.config.TopologyBuilder.BuildTopology(ctx, orgID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to build topology: %v", err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Service Map: %s\n\n", orgID)

	if len(topology.Nodes) == 0 {
		sb.WriteString("No services found in organization.\n")
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: sb.String()}},
		}
	}

	// Service table
	sb.WriteString("| Service | Language | Endpoints | Deep Analyzed |\n")
	sb.WriteString("|---------|----------|-----------|---------------|\n")
	for _, node := range topology.Nodes {
		analyzed := "Yes"
		if !node.IsDeepAnalyzed {
			analyzed = "No"
		}
		fmt.Fprintf(&sb, "| %s | %s | %d | %s |\n",
			node.ServiceName, node.Language, node.EndpointCount, analyzed)
	}

	// Edge summary
	httpEdges, grpcEdges, kafkaEdges, unmatchedEdges := 0, 0, 0, 0
	for _, edge := range topology.Edges {
		switch edge.EdgeType {
		case "http":
			httpEdges++
		case "grpc":
			grpcEdges++
		case "kafka_produce", "kafka_consume":
			kafkaEdges++
		}
		if edge.Confidence == "unmatched" {
			unmatchedEdges++
		}
	}

	fmt.Fprintf(&sb, "\n**Connections:** %d HTTP, %d gRPC, %d Kafka", httpEdges, grpcEdges, kafkaEdges)
	if unmatchedEdges > 0 {
		fmt.Fprintf(&sb, ", %d unmatched", unmatchedEdges)
	}
	sb.WriteString("\n\n")

	// Mermaid diagram
	sb.WriteString("```mermaid\n")
	sb.WriteString(topology.GenerateMermaid())
	sb.WriteString("```\n")

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// toolTraceAPIFlow handles the trace_api_flow tool call.
func (s *server) toolTraceAPIFlow(ctx context.Context, args map[string]any) callToolResult {
	orgID, ok := args["org_id"].(string)
	if !ok || orgID == "" {
		return errorResult("org_id is required")
	}

	entryPoint, ok := args["entry_point"].(string)
	if !ok || entryPoint == "" {
		return errorResult("entry_point is required")
	}

	repoID, _ := args["repo_id"].(string)

	maxDepth := 5
	if md, ok := args["max_depth"].(float64); ok && md > 0 {
		maxDepth = int(md)
	}

	if s.config.TopologyBuilder == nil {
		return errorResult("Service topology not configured. Ensure TopologyBuilder is initialized.")
	}

	// Parse entry point "METHOD /path"
	parts := strings.SplitN(entryPoint, " ", 2)
	if len(parts) != 2 {
		return errorResult("entry_point must be 'METHOD /path' format (e.g., 'POST /login')")
	}
	method := strings.ToUpper(parts[0])
	path := parts[1]

	// Build topology to get all the data
	topology, err := s.config.TopologyBuilder.BuildTopology(ctx, orgID)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to build topology: %v", err))
	}

	// Find matching entry point endpoint
	var entryEndpoints []storage.Endpoint
	if s.config.EndpointMatcher != nil {
		normalized := flow.NormalizePath(path)
		eps, err := s.config.EndpointMatcher.MatchHTTP(ctx, storage.ServiceCall{
			CallType: "http",
			Method:   method,
			Target:   normalized,
		})
		if err == nil {
			for _, mr := range eps {
				if repoID == "" || mr.Endpoint.RepoID == repoID {
					entryEndpoints = append(entryEndpoints, mr.Endpoint)
				}
			}
		}
	}

	if len(entryEndpoints) == 0 {
		return errorResult(fmt.Sprintf("No matching endpoint found for '%s %s'. Ensure the org has been analyzed.", method, path))
	}

	if len(entryEndpoints) > 1 && repoID == "" {
		var sb strings.Builder
		sb.WriteString("Multiple entry points found. Please specify repo_id to disambiguate:\n\n")
		for _, ep := range entryEndpoints {
			fmt.Fprintf(&sb, "- **%s** → `%s` (%s:%d)\n",
				ep.RepoID, ep.HandlerName, ep.FilePath, ep.Line)
		}
		return callToolResult{
			Content: []contentItem{{Type: "text", Text: sb.String()}},
		}
	}

	// Use first match
	entry := entryEndpoints[0]

	// Build trace output
	var sb strings.Builder
	fmt.Fprintf(&sb, "## API Flow Trace: %s %s\n\n", method, path)
	fmt.Fprintf(&sb, "**Entry:** %s → `%s` (%s)\n\n", flow.DeriveServiceName(entry.RepoID), entry.HandlerName, entry.FilePath)

	// Trace through topology edges
	sb.WriteString("### Call Chain\n\n")
	visited := make(map[string]bool)
	traceEdges(&sb, topology, flow.DeriveServiceName(entry.RepoID), visited, 0, maxDepth)

	// Add Mermaid sequence diagram
	sb.WriteString("\n### Sequence Diagram\n\n")
	sb.WriteString("```mermaid\n")
	sb.WriteString("sequenceDiagram\n")
	generateSequenceDiagram(&sb, topology, flow.DeriveServiceName(entry.RepoID), make(map[string]bool), maxDepth)
	sb.WriteString("```\n")

	return callToolResult{
		Content: []contentItem{{Type: "text", Text: sb.String()}},
	}
}

// traceEdges recursively traces outgoing edges from a service.
func traceEdges(sb *strings.Builder, topology *flow.ServiceTopology, serviceName string, visited map[string]bool, depth, maxDepth int) {
	if depth >= maxDepth {
		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(sb, "%s- [max depth reached]\n", indent)
		return
	}
	if visited[serviceName] {
		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(sb, "%s- [cycle detected: %s]\n", indent, serviceName)
		return
	}
	visited[serviceName] = true

	for _, edge := range topology.Edges {
		if edge.Source != serviceName {
			continue
		}
		indent := strings.Repeat("  ", depth)
		switch edge.EdgeType {
		case "kafka_produce":
			fmt.Fprintf(sb, "%s- → Kafka: `%s` (produce)\n", indent, edge.Path)
			// Find consume edges from this topic
			for _, consumeEdge := range topology.Edges {
				if consumeEdge.EdgeType == "kafka_consume" && consumeEdge.Path == edge.Path {
					fmt.Fprintf(sb, "%s  - → **%s** `%s` (consume) [%s]\n",
						indent, consumeEdge.Target, consumeEdge.TargetFunc, consumeEdge.Confidence)
					traceEdges(sb, topology, consumeEdge.Target, visited, depth+2, maxDepth)
				}
			}
		case "http", "grpc":
			confidence := edge.Confidence
			fmt.Fprintf(sb, "%s- → **%s** `%s %s` → `%s` [%s]\n",
				indent, edge.Target, edge.Method, edge.Path, edge.TargetFunc, confidence)
			if edge.Target != "<unknown>" {
				traceEdges(sb, topology, edge.Target, visited, depth+1, maxDepth)
			}
		}
	}

	delete(visited, serviceName) // allow re-entry from different paths
}

// generateSequenceDiagram generates Mermaid sequence diagram entries.
func generateSequenceDiagram(sb *strings.Builder, topology *flow.ServiceTopology, serviceName string, visited map[string]bool, maxDepth int) {
	if maxDepth <= 0 || visited[serviceName] {
		return
	}
	visited[serviceName] = true

	for _, edge := range topology.Edges {
		if edge.Source != serviceName {
			continue
		}
		switch edge.EdgeType {
		case "kafka_produce":
			fmt.Fprintf(sb, "    %s->>Kafka_%s: produce %s\n",
				sanitizeMermaidID(serviceName), sanitizeMermaidID(edge.Path), edge.Path)
			for _, consumeEdge := range topology.Edges {
				if consumeEdge.EdgeType == "kafka_consume" && consumeEdge.Path == edge.Path {
					fmt.Fprintf(sb, "    Kafka_%s-->>%s: consume\n",
						sanitizeMermaidID(edge.Path), sanitizeMermaidID(consumeEdge.Target))
					generateSequenceDiagram(sb, topology, consumeEdge.Target, visited, maxDepth-1)
				}
			}
		case "http", "grpc":
			if edge.Target == "<unknown>" {
				fmt.Fprintf(sb, "    %s->>Unknown: %s %s\n",
					sanitizeMermaidID(serviceName), edge.Method, edge.Path)
			} else {
				fmt.Fprintf(sb, "    %s->>%s: %s %s\n",
					sanitizeMermaidID(serviceName), sanitizeMermaidID(edge.Target), edge.Method, edge.Path)
				generateSequenceDiagram(sb, topology, edge.Target, visited, maxDepth-1)
			}
		}
	}
}

func sanitizeMermaidID(s string) string {
	r := strings.NewReplacer(".", "_", "-", "_", "/", "_", ":", "_", " ", "_")
	return r.Replace(s)
}
