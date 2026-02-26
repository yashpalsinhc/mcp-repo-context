package flow

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// ServiceNode represents a service in the topology graph.
type ServiceNode struct {
	RepoID         string
	ServiceName    string
	Language       string
	EndpointCount  int
	IsDeepAnalyzed bool
}

// ServiceEdge represents a connection between services.
type ServiceEdge struct {
	Source     string // source service name
	Target     string // target service name, topic name, or "<unknown>"
	EdgeType   string // "http", "grpc", "kafka_produce", "kafka_consume"
	Method     string
	Path       string // URL path or topic name
	SourceFunc string
	TargetFunc string
	Confidence string // "exact", "parameterized", "hint-matched", "ambiguous", "unmatched"
}

// ServiceTopology is the complete topology for an org.
type ServiceTopology struct {
	OrgID   string
	Nodes   []ServiceNode
	Edges   []ServiceEdge
	BuiltAt time.Time
}

// TopologyStore is the subset of storage methods needed by TopologyBuilder.
type TopologyStore interface {
	GetEndpoints(ctx context.Context, repoID string) ([]storage.Endpoint, error)
	GetServiceCalls(ctx context.Context, repoID string) ([]storage.ServiceCall, error)
	FindMatchingConsumers(ctx context.Context, topic string) ([]storage.ServiceCall, error)
	FindMatchingEndpoints(ctx context.Context, method, pathPattern string) ([]storage.Endpoint, error)
}

// OrgStore is the subset of org store methods needed by TopologyBuilder.
type OrgStore interface {
	GetOrgRepos(ctx context.Context, orgID string) ([]string, error)
}

// TopologyBuilder builds service topology graphs.
type TopologyBuilder struct {
	store    TopologyStore
	orgStore OrgStore
}

// NewTopologyBuilder creates a new TopologyBuilder.
func NewTopologyBuilder(store TopologyStore, orgStore OrgStore) *TopologyBuilder {
	return &TopologyBuilder{store: store, orgStore: orgStore}
}

// deriveServiceName extracts a service name from a repo ID.
func deriveServiceName(repoID string) string {
	// For local repos: "local:/path/to/auth-service" → "auth-service"
	if strings.HasPrefix(repoID, "local:") {
		return path.Base(strings.TrimPrefix(repoID, "local:"))
	}
	// For GitHub repos: "github.com/org/auth-service" → "auth-service"
	return path.Base(repoID)
}

// BuildTopology builds a service topology for an org.
func (b *TopologyBuilder) BuildTopology(ctx context.Context, orgID string) (*ServiceTopology, error) {
	repoIDs, err := b.orgStore.GetOrgRepos(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get org repos: %w", err)
	}

	topology := &ServiceTopology{
		OrgID:   orgID,
		BuiltAt: time.Now(),
	}

	if len(repoIDs) == 0 {
		return topology, nil
	}

	// Build service nodes and collect endpoints
	repoToService := make(map[string]string) // repoID → serviceName
	trie := NewPathTrie()

	for _, repoID := range repoIDs {
		serviceName := deriveServiceName(repoID)
		repoToService[repoID] = serviceName

		endpoints, err := b.store.GetEndpoints(ctx, repoID)
		if err != nil {
			return nil, fmt.Errorf("failed to get endpoints for %s: %w", repoID, err)
		}

		node := ServiceNode{
			RepoID:         repoID,
			ServiceName:    serviceName,
			Language:       "go",
			EndpointCount:  len(endpoints),
			IsDeepAnalyzed: true,
		}
		topology.Nodes = append(topology.Nodes, node)

		// Add endpoints to trie for matching
		for _, ep := range endpoints {
			ep.RepoID = repoID // ensure RepoID is set
			trie.Insert(ep)
		}
	}

	// Match all service calls against the trie
	for _, repoID := range repoIDs {
		calls, err := b.store.GetServiceCalls(ctx, repoID)
		if err != nil {
			return nil, fmt.Errorf("failed to get service calls for %s: %w", repoID, err)
		}

		sourceName := repoToService[repoID]

		for _, call := range calls {
			switch {
			case call.CallType == "kafka_produce":
				if call.Target == "" || call.Target == "<dynamic>" {
					continue
				}
				// Create produce edge
				topology.Edges = append(topology.Edges, ServiceEdge{
					Source:     sourceName,
					Target:     call.Target,
					EdgeType:   "kafka_produce",
					Method:     "produce",
					Path:       call.Target,
					SourceFunc: call.FunctionName,
					Confidence: "exact",
				})
				// Find consumers
				consumers, _ := b.store.FindMatchingConsumers(ctx, call.Target)
				for _, c := range consumers {
					targetName := repoToService[c.RepoID]
					if targetName == "" {
						targetName = deriveServiceName(c.RepoID)
					}
					topology.Edges = append(topology.Edges, ServiceEdge{
						Source:     call.Target,
						Target:     targetName,
						EdgeType:   "kafka_consume",
						Method:     "consume",
						Path:       call.Target,
						TargetFunc: c.FunctionName,
						Confidence: "exact",
					})
				}

			case call.CallType == "kafka_consume":
				// Consumer edges are created from the producer side
				continue

			default:
				// HTTP/gRPC matching via trie
				if call.Target == "<dynamic>" || call.Target == "" {
					continue
				}
				normalized := NormalizePath(call.Target)
				if call.CallType == "grpc" {
					normalized = NormalizePathPreserveCase(call.Target)
				}

				matches := trie.Match(normalized)

				// Filter by method for HTTP
				if call.CallType != "grpc" {
					var filtered []MatchResult
					for _, mr := range matches {
						if mr.Endpoint.Method == call.Method || mr.Endpoint.Method == "ANY" {
							filtered = append(filtered, mr)
						}
					}
					matches = filtered
				}

				// Apply service hint
				if call.ServiceHint != "" && len(matches) > 1 {
					matches = applyServiceHint(matches, call.ServiceHint)
				} else if len(matches) > 1 {
					for i := range matches {
						matches[i].Confidence = "ambiguous"
					}
				}

				if len(matches) == 0 {
					// Unmatched call
					topology.Edges = append(topology.Edges, ServiceEdge{
						Source:     sourceName,
						Target:     "<unknown>",
						EdgeType:   call.CallType,
						Method:     call.Method,
						Path:       call.Target,
						SourceFunc: call.FunctionName,
						Confidence: "unmatched",
					})
				} else {
					for _, mr := range matches {
						targetName := repoToService[mr.Endpoint.RepoID]
						if targetName == "" {
							targetName = deriveServiceName(mr.Endpoint.RepoID)
						}
						topology.Edges = append(topology.Edges, ServiceEdge{
							Source:     sourceName,
							Target:     targetName,
							EdgeType:   call.CallType,
							Method:     call.Method,
							Path:       NormalizePath(call.Target),
							SourceFunc: call.FunctionName,
							TargetFunc: mr.Endpoint.HandlerName,
							Confidence: mr.Confidence,
						})
					}
				}
			}
		}
	}

	return topology, nil
}

// GenerateMermaid produces a Mermaid flowchart string from the topology.
func (t *ServiceTopology) GenerateMermaid() string {
	var sb strings.Builder
	sb.WriteString("graph LR\n")

	// Create node IDs
	nodeIDs := make(map[string]string)
	counter := 0
	getID := func(name string) string {
		if id, ok := nodeIDs[name]; ok {
			return id
		}
		counter++
		id := fmt.Sprintf("N%d", counter)
		nodeIDs[name] = id
		return id
	}

	// Add service nodes
	seen := make(map[string]bool)
	for _, node := range t.Nodes {
		id := getID(node.ServiceName)
		if !seen[node.ServiceName] {
			sb.WriteString(fmt.Sprintf("    %s[%s]\n", id, node.ServiceName))
			seen[node.ServiceName] = true
			if !node.IsDeepAnalyzed {
				sb.WriteString(fmt.Sprintf("    style %s fill:#ddd,stroke:#999\n", id))
			}
		}
	}

	// Collect unique Kafka topics
	kafkaTopics := make(map[string]bool)
	for _, edge := range t.Edges {
		if edge.EdgeType == "kafka_produce" || edge.EdgeType == "kafka_consume" {
			kafkaTopics[edge.Path] = true
		}
	}
	for topic := range kafkaTopics {
		id := getID("kafka:" + topic)
		sb.WriteString(fmt.Sprintf("    %s[Kafka: %s]\n", id, topic))
	}

	// Add unknown node if needed
	for _, edge := range t.Edges {
		if edge.Target == "<unknown>" {
			id := getID("<unknown>")
			sb.WriteString(fmt.Sprintf("    %s[???]\n", id))
			break
		}
	}

	// Add edges
	for _, edge := range t.Edges {
		sourceID := getID(edge.Source)
		switch edge.EdgeType {
		case "kafka_produce":
			targetID := getID("kafka:" + edge.Path)
			sb.WriteString(fmt.Sprintf("    %s -.->|produce| %s\n", sourceID, targetID))
		case "kafka_consume":
			sourceID := getID("kafka:" + edge.Path)
			targetID := getID(edge.Target)
			sb.WriteString(fmt.Sprintf("    %s -.->|consume| %s\n", sourceID, targetID))
		default:
			targetID := getID(edge.Target)
			label := fmt.Sprintf("%s %s", edge.Method, edge.Path)
			sb.WriteString(fmt.Sprintf("    %s -->|%s| %s\n", sourceID, label, targetID))
		}
	}

	return sb.String()
}

// SortedEdges returns edges sorted for deterministic output.
func (t *ServiceTopology) SortedEdges() []ServiceEdge {
	edges := make([]ServiceEdge, len(t.Edges))
	copy(edges, t.Edges)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		if edges[i].Target != edges[j].Target {
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Path < edges[j].Path
	})
	return edges
}
