package flow

import (
	"context"
	"fmt"
	"strings"

	"github.com/yashpalc/mcp-repo-context/internal/storage"
)

// CrossServiceImpact describes the cross-service impact of changes to a repo.
type CrossServiceImpact struct {
	UpstreamCallers  []ServiceCallRef // services calling modified endpoints
	DownstreamDeps   []ServiceCallRef // services called by modified functions
	KafkaConsumers   []ServiceCallRef // consumers of topics produced by modified functions
	RiskLevel        string           // "none", "low", "medium", "high"
	AffectedServices int
}

// ServiceCallRef is a reference to a service call or endpoint.
type ServiceCallRef struct {
	ServiceName  string
	RepoID       string
	FunctionName string
	CallType     string // "http", "grpc", "kafka"
	Path         string // URL or topic
}

// ImpactStore is the subset of storage methods needed for impact analysis.
type ImpactStore interface {
	GetEndpoints(ctx context.Context, repoID string) ([]storage.Endpoint, error)
	GetServiceCalls(ctx context.Context, repoID string) ([]storage.ServiceCall, error)
	FindMatchingEndpoints(ctx context.Context, method, pathPattern string) ([]storage.Endpoint, error)
	FindMatchingConsumers(ctx context.Context, topic string) ([]storage.ServiceCall, error)
}

// AnalyzeCrossServiceImpact analyzes the cross-service impact of function changes.
func AnalyzeCrossServiceImpact(ctx context.Context, repoID string, changedFunctions []string, store ImpactStore) (*CrossServiceImpact, error) {
	impact := &CrossServiceImpact{RiskLevel: "none"}

	if len(changedFunctions) == 0 {
		return impact, nil
	}

	changedSet := make(map[string]bool)
	for _, fn := range changedFunctions {
		changedSet[fn] = true
	}

	// Get endpoints and service calls for this repo
	endpoints, err := store.GetEndpoints(ctx, repoID)
	if err != nil {
		return impact, nil // non-fatal
	}

	serviceCalls, err := store.GetServiceCalls(ctx, repoID)
	if err != nil {
		return impact, nil // non-fatal
	}

	affectedServiceSet := make(map[string]bool)

	// Check if changed functions are registered handlers (upstream callers)
	for _, ep := range endpoints {
		if !changedSet[ep.HandlerName] {
			continue
		}
		// This endpoint is modified - find who calls it
		// Search all service_calls across all repos that target this path
		callers, err := findUpstreamCallers(ctx, store, ep)
		if err != nil {
			continue
		}
		for _, caller := range callers {
			if caller.RepoID == repoID {
				continue // skip self-references
			}
			svcName := DeriveServiceName(caller.RepoID)
			impact.UpstreamCallers = append(impact.UpstreamCallers, ServiceCallRef{
				ServiceName:  svcName,
				RepoID:       caller.RepoID,
				FunctionName: caller.FunctionName,
				CallType:     caller.CallType,
				Path:         ep.Path,
			})
			affectedServiceSet[svcName] = true
		}
	}

	// Check if changed functions have outgoing calls (downstream deps)
	for _, sc := range serviceCalls {
		if !changedSet[sc.FunctionName] {
			continue
		}
		switch sc.CallType {
		case "http", "grpc":
			if sc.Target == "" || sc.Target == "<dynamic>" {
				continue
			}
			method := sc.Method
			normalized := NormalizePath(sc.Target)
			if sc.CallType == "grpc" {
				normalized = NormalizePathPreserveCase(sc.Target)
			}
			targets, err := store.FindMatchingEndpoints(ctx, method, normalized)
			if err != nil {
				continue
			}
			for _, target := range targets {
				if target.RepoID == repoID {
					continue
				}
				svcName := DeriveServiceName(target.RepoID)
				impact.DownstreamDeps = append(impact.DownstreamDeps, ServiceCallRef{
					ServiceName:  svcName,
					RepoID:       target.RepoID,
					FunctionName: target.HandlerName,
					CallType:     sc.CallType,
					Path:         sc.Target,
				})
				affectedServiceSet[svcName] = true
			}
		case "kafka_produce":
			if sc.Target == "" || sc.Target == "<dynamic>" {
				continue
			}
			consumers, err := store.FindMatchingConsumers(ctx, sc.Target)
			if err != nil {
				continue
			}
			for _, c := range consumers {
				if c.RepoID == repoID {
					continue
				}
				svcName := DeriveServiceName(c.RepoID)
				impact.KafkaConsumers = append(impact.KafkaConsumers, ServiceCallRef{
					ServiceName:  svcName,
					RepoID:       c.RepoID,
					FunctionName: c.FunctionName,
					CallType:     "kafka",
					Path:         sc.Target,
				})
				affectedServiceSet[svcName] = true
			}
		}
	}

	impact.AffectedServices = len(affectedServiceSet)
	switch {
	case impact.AffectedServices == 0:
		impact.RiskLevel = "none"
	case impact.AffectedServices == 1:
		impact.RiskLevel = "low"
	case impact.AffectedServices == 2:
		impact.RiskLevel = "medium"
	default:
		impact.RiskLevel = "high"
	}

	return impact, nil
}

// findUpstreamCallers finds service calls from other repos that target this endpoint.
// This is a simplified search - in production you'd want a reverse index.
func findUpstreamCallers(ctx context.Context, store ImpactStore, ep storage.Endpoint) ([]storage.ServiceCall, error) {
	// We can't easily query "all service calls targeting this path" without scanning all repos.
	// For now, use FindMatchingEndpoints in reverse: find all service_calls that match this endpoint.
	// This is a limitation - the current store only indexes endpoints, not service calls by target.
	// We'll return an empty set and note this as a future optimization.
	// TODO: Add reverse index for service_calls by target path
	return nil, nil
}

// FormatCrossServiceImpact formats the impact analysis as markdown.
func FormatCrossServiceImpact(impact *CrossServiceImpact) string {
	if impact == nil || impact.AffectedServices == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n## Cross-Service Impact\n\n")
	riskDisplay := impact.RiskLevel
	if len(riskDisplay) > 0 {
		riskDisplay = strings.ToUpper(riskDisplay[:1]) + riskDisplay[1:]
	}
	fmt.Fprintf(&sb, "**Risk Level: %s** (%d service(s) affected)\n\n",
		riskDisplay, impact.AffectedServices)

	if len(impact.UpstreamCallers) > 0 {
		sb.WriteString("### Upstream (services calling modified endpoints)\n")
		for _, ref := range impact.UpstreamCallers {
			fmt.Fprintf(&sb, "- **%s** calls `%s` via %s\n", ref.ServiceName, ref.Path, ref.CallType)
			fmt.Fprintf(&sb, "  - Function: `%s` (%s)\n", ref.FunctionName, ref.RepoID)
		}
		sb.WriteString("\n")
	}

	if len(impact.DownstreamDeps) > 0 {
		sb.WriteString("### Downstream (services called by modified functions)\n")
		for _, ref := range impact.DownstreamDeps {
			fmt.Fprintf(&sb, "- **%s** handles `%s` via %s\n", ref.ServiceName, ref.Path, ref.CallType)
			fmt.Fprintf(&sb, "  - Handler: `%s` (%s)\n", ref.FunctionName, ref.RepoID)
		}
		sb.WriteString("\n")
	}

	if len(impact.KafkaConsumers) > 0 {
		sb.WriteString("### Kafka Consumers (affected by modified producers)\n")
		for _, ref := range impact.KafkaConsumers {
			fmt.Fprintf(&sb, "- **%s** consumes topic `%s`\n", ref.ServiceName, ref.Path)
			fmt.Fprintf(&sb, "  - Handler: `%s` (%s)\n", ref.FunctionName, ref.RepoID)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
