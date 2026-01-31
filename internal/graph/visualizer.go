package graph

import (
	"context"
	"fmt"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
	"github.com/yashpalc/mcp-repo-context/internal/orchestrator"
)

// CallNode represents a node in the call tree structure.
type CallNode struct {
	Name     string      `json:"name"`
	File     string      `json:"file"`
	Line     int         `json:"line"`
	Callers  []*CallNode `json:"callers,omitempty"`
	Callees  []*CallNode `json:"callees,omitempty"`
}

// GraphVisualizer generates visual representations of call graphs.
type GraphVisualizer struct {
	manager orchestrator.Manager
}

// NewGraphVisualizer creates a new GraphVisualizer.
func NewGraphVisualizer(manager orchestrator.Manager) *GraphVisualizer {
	return &GraphVisualizer{
		manager: manager,
	}
}

// GenerateMermaid generates a Mermaid flowchart of call relationships.
// It traverses both callers and callees up to the specified depth.
func (v *GraphVisualizer) GenerateMermaid(ctx context.Context, repoID, functionName string, depth int) (string, error) {
	if depth <= 0 {
		depth = 2
	}
	if depth > 5 {
		depth = 5 // Limit max depth to avoid overly complex graphs
	}

	repoCtx, err := v.manager.GetContext(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("failed to get repository context: %w", err)
	}

	// Build the call tree
	callTree := v.buildCallTree(repoCtx, functionName, depth)
	if callTree == nil {
		return "", fmt.Errorf("function '%s' not found in repository", functionName)
	}

	// Generate Mermaid diagram
	var sb strings.Builder
	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart TD\n")

	// Track visited nodes to avoid duplicates
	visited := make(map[string]bool)
	nodeIDs := make(map[string]string) // node name -> sanitized ID

	// Helper to create a safe node ID
	getNodeID := func(name, file string) string {
		key := name + ":" + file
		if id, ok := nodeIDs[key]; ok {
			return id
		}
		id := fmt.Sprintf("n%d", len(nodeIDs))
		nodeIDs[key] = id
		return id
	}

	// Write the root node (highlighted)
	rootID := getNodeID(callTree.Name, callTree.File)
	sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", rootID, escapeForMermaid(callTree.Name)))
	sb.WriteString(fmt.Sprintf("    style %s fill:#f9f,stroke:#333,stroke-width:2px\n", rootID))
	visited[callTree.Name+":"+callTree.File] = true

	// Traverse and write callers (functions that call this one)
	v.writeMermaidCallers(&sb, callTree, rootID, visited, getNodeID)

	// Traverse and write callees (functions called by this one)
	v.writeMermaidCallees(&sb, callTree, rootID, visited, getNodeID)

	sb.WriteString("```\n")

	return sb.String(), nil
}

// writeMermaidCallers recursively writes caller relationships.
func (v *GraphVisualizer) writeMermaidCallers(sb *strings.Builder, node *CallNode, nodeID string, visited map[string]bool, getNodeID func(string, string) string) {
	for _, caller := range node.Callers {
		key := caller.Name + ":" + caller.File
		callerID := getNodeID(caller.Name, caller.File)

		if !visited[key] {
			// Define the caller node
			sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", callerID, escapeForMermaid(caller.Name)))
			visited[key] = true
		}

		// Draw edge: caller --> current node (caller calls current)
		sb.WriteString(fmt.Sprintf("    %s --> %s\n", callerID, nodeID))

		// Recurse for caller's callers
		v.writeMermaidCallers(sb, caller, callerID, visited, getNodeID)
	}
}

// writeMermaidCallees recursively writes callee relationships.
func (v *GraphVisualizer) writeMermaidCallees(sb *strings.Builder, node *CallNode, nodeID string, visited map[string]bool, getNodeID func(string, string) string) {
	for _, callee := range node.Callees {
		key := callee.Name + ":" + callee.File
		calleeID := getNodeID(callee.Name, callee.File)

		if !visited[key] {
			// Define the callee node
			sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", calleeID, escapeForMermaid(callee.Name)))
			visited[key] = true
		}

		// Draw edge: current node --> callee (current calls callee)
		sb.WriteString(fmt.Sprintf("    %s --> %s\n", nodeID, calleeID))

		// Recurse for callee's callees
		v.writeMermaidCallees(sb, callee, calleeID, visited, getNodeID)
	}
}

// GenerateDOT generates a DOT format graph for Graphviz.
func (v *GraphVisualizer) GenerateDOT(ctx context.Context, repoID, functionName string, depth int) (string, error) {
	if depth <= 0 {
		depth = 2
	}
	if depth > 5 {
		depth = 5
	}

	repoCtx, err := v.manager.GetContext(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("failed to get repository context: %w", err)
	}

	// Build the call tree
	callTree := v.buildCallTree(repoCtx, functionName, depth)
	if callTree == nil {
		return "", fmt.Errorf("function '%s' not found in repository", functionName)
	}

	// Generate DOT diagram
	var sb strings.Builder
	sb.WriteString("digraph CallGraph {\n")
	sb.WriteString("    rankdir=TB;\n")
	sb.WriteString("    node [shape=box, style=rounded];\n")
	sb.WriteString("    edge [color=\"#666666\"];\n\n")

	// Track visited nodes
	visited := make(map[string]bool)
	nodeIDs := make(map[string]string)

	getNodeID := func(name, file string) string {
		key := name + ":" + file
		if id, ok := nodeIDs[key]; ok {
			return id
		}
		id := fmt.Sprintf("n%d", len(nodeIDs))
		nodeIDs[key] = id
		return id
	}

	// Write the root node (highlighted)
	rootID := getNodeID(callTree.Name, callTree.File)
	sb.WriteString(fmt.Sprintf("    %s [label=\"%s\", style=\"rounded,filled\", fillcolor=\"#ffccff\"];\n",
		rootID, escapeForDOT(callTree.Name)))
	visited[callTree.Name+":"+callTree.File] = true

	// Write callers
	v.writeDOTCallers(&sb, callTree, rootID, visited, getNodeID)

	// Write callees
	v.writeDOTCallees(&sb, callTree, rootID, visited, getNodeID)

	sb.WriteString("}\n")

	return sb.String(), nil
}

// writeDOTCallers recursively writes caller relationships in DOT format.
func (v *GraphVisualizer) writeDOTCallers(sb *strings.Builder, node *CallNode, nodeID string, visited map[string]bool, getNodeID func(string, string) string) {
	for _, caller := range node.Callers {
		key := caller.Name + ":" + caller.File
		callerID := getNodeID(caller.Name, caller.File)

		if !visited[key] {
			sb.WriteString(fmt.Sprintf("    %s [label=\"%s\"];\n", callerID, escapeForDOT(caller.Name)))
			visited[key] = true
		}

		// Edge: caller -> current
		sb.WriteString(fmt.Sprintf("    %s -> %s;\n", callerID, nodeID))

		v.writeDOTCallers(sb, caller, callerID, visited, getNodeID)
	}
}

// writeDOTCallees recursively writes callee relationships in DOT format.
func (v *GraphVisualizer) writeDOTCallees(sb *strings.Builder, node *CallNode, nodeID string, visited map[string]bool, getNodeID func(string, string) string) {
	for _, callee := range node.Callees {
		key := callee.Name + ":" + callee.File
		calleeID := getNodeID(callee.Name, callee.File)

		if !visited[key] {
			sb.WriteString(fmt.Sprintf("    %s [label=\"%s\"];\n", calleeID, escapeForDOT(callee.Name)))
			visited[key] = true
		}

		// Edge: current -> callee
		sb.WriteString(fmt.Sprintf("    %s -> %s;\n", nodeID, calleeID))

		v.writeDOTCallees(sb, callee, calleeID, visited, getNodeID)
	}
}

// GetCallTree returns a structured call tree for a function.
func (v *GraphVisualizer) GetCallTree(ctx context.Context, repoID, functionName string, depth int) (*CallNode, error) {
	if depth <= 0 {
		depth = 2
	}
	if depth > 5 {
		depth = 5
	}

	repoCtx, err := v.manager.GetContext(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository context: %w", err)
	}

	callTree := v.buildCallTree(repoCtx, functionName, depth)
	if callTree == nil {
		return nil, fmt.Errorf("function '%s' not found in repository", functionName)
	}

	return callTree, nil
}

// buildCallTree constructs a call tree by traversing both callers and callees.
func (v *GraphVisualizer) buildCallTree(repoCtx *ctxpkg.RepoContext, functionName string, depth int) *CallNode {
	// First, find the function in the repo
	var rootNode *CallNode
	var rootFile string
	var rootFn *ctxpkg.FunctionDef

	for path, fileCtx := range repoCtx.Files {
		for i := range fileCtx.Functions {
			fn := &fileCtx.Functions[i]
			if fn.Name == functionName {
				rootNode = &CallNode{
					Name: fn.Name,
					File: path,
					Line: fn.LineStart,
				}
				rootFile = path
				rootFn = fn
				break
			}
		}
		if rootNode != nil {
			break
		}
	}

	if rootNode == nil {
		return nil
	}

	// Build caller chain
	visitedCallers := make(map[string]bool)
	visitedCallers[functionName+":"+rootFile] = true
	v.populateCallers(repoCtx, rootNode, depth, visitedCallers)

	// Build callee chain
	visitedCallees := make(map[string]bool)
	visitedCallees[functionName+":"+rootFile] = true
	v.populateCallees(repoCtx, rootNode, rootFn, depth, visitedCallees)

	return rootNode
}

// populateCallers recursively finds and adds callers to a node.
func (v *GraphVisualizer) populateCallers(repoCtx *ctxpkg.RepoContext, node *CallNode, depth int, visited map[string]bool) {
	if depth <= 0 {
		return
	}

	// Find all functions that call this function
	for path, fileCtx := range repoCtx.Files {
		for i := range fileCtx.Functions {
			fn := &fileCtx.Functions[i]
			for _, call := range fn.Calls {
				if call.Function == node.Name {
					key := fn.Name + ":" + path
					if visited[key] {
						continue
					}
					visited[key] = true

					callerNode := &CallNode{
						Name: fn.Name,
						File: path,
						Line: fn.LineStart,
					}
					node.Callers = append(node.Callers, callerNode)

					// Recursively find callers of this caller
					v.populateCallers(repoCtx, callerNode, depth-1, visited)
				}
			}
		}
	}
}

// populateCallees recursively finds and adds callees to a node.
func (v *GraphVisualizer) populateCallees(repoCtx *ctxpkg.RepoContext, node *CallNode, fn *ctxpkg.FunctionDef, depth int, visited map[string]bool) {
	if depth <= 0 || fn == nil {
		return
	}

	// Add callees from the function's calls
	for _, call := range fn.Calls {
		// Only include internal calls (same package or repo)
		if call.Type != "internal" && call.Type != "" && call.Package != "" {
			// Skip external/stdlib calls
			continue
		}

		// Find the called function in the repo
		for path, fileCtx := range repoCtx.Files {
			for i := range fileCtx.Functions {
				calledFn := &fileCtx.Functions[i]
				if calledFn.Name == call.Function {
					key := calledFn.Name + ":" + path
					if visited[key] {
						continue
					}
					visited[key] = true

					calleeNode := &CallNode{
						Name: calledFn.Name,
						File: path,
						Line: calledFn.LineStart,
					}
					node.Callees = append(node.Callees, calleeNode)

					// Recursively find callees of this callee
					v.populateCallees(repoCtx, calleeNode, calledFn, depth-1, visited)
					break
				}
			}
		}
	}
}

// escapeForMermaid escapes special characters for Mermaid diagrams.
func escapeForMermaid(s string) string {
	s = strings.ReplaceAll(s, "\"", "#quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// escapeForDOT escapes special characters for DOT format.
func escapeForDOT(s string) string {
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "<", "\\<")
	s = strings.ReplaceAll(s, ">", "\\>")
	return s
}
