package flow

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// Store is the subset of storage.SQLiteStore methods needed by EndpointMatcher.
type Store interface {
	FindMatchingEndpoints(ctx context.Context, method, pathPattern string) ([]storage.Endpoint, error)
	FindMatchingConsumers(ctx context.Context, topic string) ([]storage.ServiceCall, error)
	FindMatchingProducers(ctx context.Context, topic string) ([]storage.ServiceCall, error)
	GetServiceCalls(ctx context.Context, repoID string) ([]storage.ServiceCall, error)
	GetEndpoints(ctx context.Context, repoID string) ([]storage.Endpoint, error)
}

// MatchResult represents a single endpoint match for a service call.
type MatchResult struct {
	Endpoint   storage.Endpoint
	Confidence string // "exact", "parameterized", "hint-matched", "ambiguous"
}

// CallMatch associates a service call with zero or more matched endpoints.
type CallMatch struct {
	Call    storage.ServiceCall
	Matches []MatchResult
}

// EndpointMatcher connects client service_calls to server endpoints.
type EndpointMatcher struct {
	store Store
}

// NewEndpointMatcher creates an EndpointMatcher backed by the given store.
func NewEndpointMatcher(store Store) *EndpointMatcher {
	return &EndpointMatcher{store: store}
}

// NormalizePath normalizes a raw URL or path for matching.
// Extracts path from URLs, lowercases, decodes percent-encoding,
// resolves dot-segments, collapses slashes, strips trailing slash.
func NormalizePath(raw string) string {
	return normalizePath(raw, true)
}

// NormalizePathPreserveCase normalizes without lowercasing (for case-sensitive protocols like gRPC).
func NormalizePathPreserveCase(raw string) string {
	return normalizePath(raw, false)
}

func normalizePath(raw string, lowercase bool) string {
	if raw == "" {
		return "/"
	}

	// Extract path from URL if it has a scheme
	p := raw
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil {
			p = u.Path
		}
	}

	// Lowercase (skip for case-sensitive protocols)
	if lowercase {
		p = strings.ToLower(p)
	}

	// Decode percent-encoded characters
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}

	// Resolve dot-segments
	p = path.Clean(p)

	// Collapse consecutive slashes (path.Clean handles most, but ensure leading /)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	// Strip trailing slash (unless root)
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimRight(p, "/")
	}

	return p
}

// matchSegments returns true if the endpoint path template matches the
// normalized client path. A template segment of {param} matches any single
// literal segment. Returns (matched, isParameterized).
func matchSegments(template, clientPath string) (bool, bool) {
	tParts := strings.Split(strings.Trim(template, "/"), "/")
	cParts := strings.Split(strings.Trim(clientPath, "/"), "/")

	if len(tParts) != len(cParts) {
		return false, false
	}

	parameterized := false
	for i, tp := range tParts {
		if strings.HasPrefix(tp, "{") && strings.HasSuffix(tp, "}") {
			parameterized = true
			continue // wildcard matches any segment
		}
		if tp != cParts[i] {
			return false, false
		}
	}

	return true, parameterized
}

// MatchHTTP finds server-side endpoints that match the given service call.
func (m *EndpointMatcher) MatchHTTP(ctx context.Context, call storage.ServiceCall) ([]MatchResult, error) {
	if call.Target == "<dynamic>" || call.Target == "" {
		return nil, nil
	}

	normalized := NormalizePath(call.Target)

	// Try exact match first
	endpoints, err := m.store.FindMatchingEndpoints(ctx, call.Method, normalized)
	if err != nil {
		return nil, err
	}

	// Also try matching all endpoints by loading candidates
	// For parameterized matching, we need to check segment-by-segment
	var results []MatchResult

	for _, ep := range endpoints {
		epNorm := NormalizePath(ep.Path)
		matched, param := matchSegments(epNorm, normalized)
		if matched {
			confidence := "exact"
			if param {
				confidence = "parameterized"
			}
			results = append(results, MatchResult{Endpoint: ep, Confidence: confidence})
		}
	}

	// Apply service hint ranking
	if call.ServiceHint != "" && len(results) > 1 {
		results = applyServiceHint(results, call.ServiceHint)
	} else if len(results) > 1 {
		// Multiple matches without hint → ambiguous
		for i := range results {
			results[i].Confidence = "ambiguous"
		}
	}

	return results, nil
}

// applyServiceHint ranks matches by service hint.
func applyServiceHint(results []MatchResult, hint string) []MatchResult {
	hintLower := strings.ToLower(hint)
	var hinted, other []MatchResult

	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Endpoint.RepoID), hintLower) {
			r.Confidence = "hint-matched"
			hinted = append(hinted, r)
		} else {
			r.Confidence = "ambiguous"
			other = append(other, r)
		}
	}

	// If hint narrowed to exactly one match, keep "hint-matched" to show
	// that the hint was the disambiguating factor


	return append(hinted, other...)
}

// MatchKafkaConsumers finds all Kafka consumers matching the given topic.
func (m *EndpointMatcher) MatchKafkaConsumers(ctx context.Context, topic string) ([]storage.ServiceCall, error) {
	if topic == "<dynamic>" || topic == "" {
		return nil, nil
	}
	return m.store.FindMatchingConsumers(ctx, topic)
}

// MatchKafkaProducers finds all Kafka producers for the given topic.
func (m *EndpointMatcher) MatchKafkaProducers(ctx context.Context, topic string) ([]storage.ServiceCall, error) {
	if topic == "<dynamic>" || topic == "" {
		return nil, nil
	}
	return m.store.FindMatchingProducers(ctx, topic)
}

// MatchGRPC finds endpoints with method="gRPC" matching the call target exactly.
func (m *EndpointMatcher) MatchGRPC(ctx context.Context, call storage.ServiceCall) ([]MatchResult, error) {
	if call.Target == "<dynamic>" || call.Target == "" {
		return nil, nil
	}

	endpoints, err := m.store.FindMatchingEndpoints(ctx, "gRPC", NormalizePathPreserveCase(call.Target))
	if err != nil {
		return nil, err
	}

	var results []MatchResult
	for _, ep := range endpoints {
		results = append(results, MatchResult{Endpoint: ep, Confidence: "exact"})
	}
	return results, nil
}

// PathTrie is an in-memory trie over path segments for O(depth) lookup.
type PathTrie struct {
	root *trieNode
}

type trieNode struct {
	children map[string]*trieNode // literal segment → child
	wildcard *trieNode            // matches any single segment
	endpoints []storage.Endpoint  // endpoints at this path
}

func newTrieNode() *trieNode {
	return &trieNode{children: make(map[string]*trieNode)}
}

// NewPathTrie creates an empty path trie.
func NewPathTrie() *PathTrie {
	return &PathTrie{root: newTrieNode()}
}

// Insert adds an endpoint to the trie.
func (t *PathTrie) Insert(ep storage.Endpoint) {
	normalized := NormalizePath(ep.Path)
	segments := strings.Split(strings.Trim(normalized, "/"), "/")

	node := t.root
	for _, seg := range segments {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			if node.wildcard == nil {
				node.wildcard = newTrieNode()
			}
			node = node.wildcard
		} else {
			if _, ok := node.children[seg]; !ok {
				node.children[seg] = newTrieNode()
			}
			node = node.children[seg]
		}
	}
	node.endpoints = append(node.endpoints, ep)
}

// Match returns all endpoints whose path template matches the given normalized path.
func (t *PathTrie) Match(normalizedPath string) []MatchResult {
	segments := strings.Split(strings.Trim(normalizedPath, "/"), "/")
	return t.matchRecursive(t.root, segments, 0, false)
}

func (t *PathTrie) matchRecursive(node *trieNode, segments []string, idx int, paramUsed bool) []MatchResult {
	if idx == len(segments) {
		var results []MatchResult
		for _, ep := range node.endpoints {
			confidence := "exact"
			if paramUsed {
				confidence = "parameterized"
			}
			results = append(results, MatchResult{Endpoint: ep, Confidence: confidence})
		}
		return results
	}

	seg := segments[idx]
	var results []MatchResult

	// Try literal match
	if child, ok := node.children[seg]; ok {
		results = append(results, t.matchRecursive(child, segments, idx+1, paramUsed)...)
	}

	// Try wildcard match
	if node.wildcard != nil {
		results = append(results, t.matchRecursive(node.wildcard, segments, idx+1, true)...)
	}

	return results
}

// BuildTrie loads all endpoints from the given repos and builds a path trie.
func (m *EndpointMatcher) BuildTrie(ctx context.Context, repoIDs []string) (*PathTrie, error) {
	trie := NewPathTrie()

	for _, repoID := range repoIDs {
		endpoints, err := m.store.GetEndpoints(ctx, repoID)
		if err != nil {
			return nil, err
		}
		for _, ep := range endpoints {
			trie.Insert(ep)
		}
	}

	return trie, nil
}

// MatchAll runs all service_calls from the given repos against a trie
// and returns CallMatch for every call (including unmatched).
func (m *EndpointMatcher) MatchAll(ctx context.Context, repoIDs []string) ([]CallMatch, error) {
	trie, err := m.BuildTrie(ctx, repoIDs)
	if err != nil {
		return nil, err
	}

	var allMatches []CallMatch

	for _, repoID := range repoIDs {
		calls, err := m.store.GetServiceCalls(ctx, repoID)
		if err != nil {
			return nil, err
		}

		for _, call := range calls {
			cm := CallMatch{Call: call}

			switch {
			case call.CallType == "kafka_produce" || call.CallType == "kafka_consume":
				// Kafka matching is exact topic
				if call.CallType == "kafka_produce" {
					consumers, _ := m.store.FindMatchingConsumers(ctx, call.Target)
					for _, c := range consumers {
						cm.Matches = append(cm.Matches, MatchResult{
							Endpoint: storage.Endpoint{
								RepoID:      c.RepoID,
								FilePath:    c.FilePath,
								HandlerName: c.FunctionName,
								Method:      "kafka_consume",
								Path:        c.Target,
							},
							Confidence: "exact",
						})
					}
				}
			case call.CallType == "grpc":
				normalized := NormalizePath(call.Target)
				matches := trie.Match(normalized)
				cm.Matches = matches
			default:
				// HTTP matching via trie
				if call.Target != "<dynamic>" && call.Target != "" {
					normalized := NormalizePath(call.Target)
					matches := trie.Match(normalized)

					// Filter by method
					var filtered []MatchResult
					for _, mr := range matches {
						if mr.Endpoint.Method == call.Method || mr.Endpoint.Method == "ANY" {
							filtered = append(filtered, mr)
						}
					}

					// Apply service hint
					if call.ServiceHint != "" && len(filtered) > 1 {
						filtered = applyServiceHint(filtered, call.ServiceHint)
					} else if len(filtered) > 1 {
						for i := range filtered {
							filtered[i].Confidence = "ambiguous"
						}
					}

					cm.Matches = filtered
				}
			}

			allMatches = append(allMatches, cm)
		}
	}

	return allMatches, nil
}
