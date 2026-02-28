# Section 08: Basic Multi-Language Service Detection

## Overview

This section detects non-Go services in an org by parsing docker-compose.yml and package.json files. Detected services appear as metadata-only nodes in the service map, with hints about Kafka topics and service URLs extracted from environment variables and dependencies.

## Dependencies

- None (can be implemented in parallel with Sections 1-3)

## Tests First

### File: `internal/analyzer/multiservice_detector_test.go` (new)

```go
// Test: ParseDockerCompose extracts service names
// Setup: docker-compose.yml with services: auth, users, frontend
// Assert: 3 ServiceInfo entries with names "auth", "users", "frontend"

// Test: ParseDockerCompose extracts Kafka topics from env vars
// Setup: environment: KAFKA_TOPIC=user.events, TOPIC_NAME=orders
// Assert: ServiceInfo.Topics contains ["user.events", "orders"]

// Test: ParseDockerCompose extracts service URLs from env vars
// Setup: environment: AUTH_SERVICE_URL=http://auth:8080, USER_API_URL=http://users:9090
// Assert: ServiceInfo.ServiceURLs contains URLs with service hints

// Test: ParseDockerCompose detects language from image
// Setup: image: node:18-alpine
// Assert: ServiceInfo.Language == "typescript" (node → typescript)

// Test: ParseDockerCompose detects language from build context
// Setup: build: ./services/auth (contains go.mod)
// Assert: ServiceInfo.Language == "go"

// Test: ParseDockerCompose handles missing environment block
// Setup: service with no environment
// Assert: Empty topics and URLs, no error

// Test: ParsePackageJSON detects Kafka dependency
// Setup: package.json with "kafkajs" in dependencies
// Assert: ServiceInfo.IsKafkaParticipant == true

// Test: ParsePackageJSON detects HTTP client dependency
// Setup: package.json with "axios" in dependencies
// Assert: ServiceInfo.HasHTTPClient == true

// Test: ParsePackageJSON handles missing file gracefully
// Assert: No error, nil result

// Test: DetectNonGoServices combines docker-compose and package.json
// Setup: Repo with docker-compose.yml (3 services) and Go files for 2 of them
// Call: DetectNonGoServices
// Assert: Returns only the non-Go service

// Test: Environment variable patterns for topic detection
// Setup: Various env var names: KAFKA_TOPIC, TOPIC_NAME, EVENT_TOPIC, CONSUME_TOPIC, PRODUCE_TOPIC
// Assert: All detected as topic patterns

// Test: Environment variable patterns for URL detection
// Setup: Various: AUTH_URL, SERVICE_URL, API_ENDPOINT, BASE_URL
// Assert: All detected as service URL patterns
```

## Implementation Details

### 1. Types

**File:** `internal/analyzer/multiservice_detector.go` (new)

```go
type ServiceInfo struct {
    Name              string
    Language          string   // "go", "typescript", "python", "java", "unknown"
    Topics            []string // Kafka topic names from env vars
    ServiceURLs       []string // service URLs from env vars
    IsKafkaParticipant bool
    HasHTTPClient     bool
    Source            string   // "docker-compose", "package.json", "kubernetes"
}

type MultiServiceDetector struct{}

func NewMultiServiceDetector() *MultiServiceDetector
```

### 2. docker-compose.yml Parsing

```go
func (d *MultiServiceDetector) ParseDockerCompose(content []byte) ([]ServiceInfo, error)
```

Uses `gopkg.in/yaml.v3` to parse:

1. Unmarshal into a generic `map[string]interface{}` for the top-level structure
2. Extract `services` block
3. For each service:
   - Name from the key
   - Language detection: check `image` field for language hints (node → typescript, python → python, openjdk/eclipse-temurin → java). If `build` context exists, check for go.mod, package.json, requirements.txt
   - Environment scanning: iterate env vars, match against topic patterns and URL patterns

**Topic name patterns** (env var name matching, case-insensitive):
- Contains `TOPIC` and value doesn't look like a number/boolean
- Contains `KAFKA` and `TOPIC`
- Contains `EVENT` and (`TOPIC` or `NAME`)
- Value starts with common topic prefixes

**URL patterns** (env var name matching, case-insensitive):
- Contains `URL` or `ENDPOINT` or `HOST`
- Value starts with `http://` or `https://`
- Extract service hint from URL hostname

### 3. package.json Parsing

```go
func (d *MultiServiceDetector) ParsePackageJSON(content []byte) (*ServiceInfo, error)
```

1. Parse JSON
2. Scan `dependencies` and `devDependencies` for:
   - Kafka: `kafkajs`, `node-rdkafka`, `kafka-node`, `@nestjs/microservices`
   - HTTP client: `axios`, `node-fetch`, `got`, `superagent`
3. Set flags accordingly

### 4. Combined Detection

```go
func (d *MultiServiceDetector) DetectNonGoServices(repoFiles map[string][]byte, goRepos map[string]bool) ([]ServiceInfo, error)
```

1. Look for docker-compose.yml, docker-compose.yaml in repo files
2. If found: parse and get service list
3. Look for package.json files in repo files
4. If found: parse and merge with docker-compose results
5. Filter out services that are already known Go repos (by name matching against goRepos)
6. Return remaining as non-Go services

### 5. Integration with TopologyBuilder

The TopologyBuilder (Section 6) calls `DetectNonGoServices()` during `BuildTopology`. It:
1. Loads docker-compose.yml and package.json from each repo's context (via ContextStore)
2. Passes to detector
3. Creates ServiceNode entries with `IsDeepAnalyzed=false` for detected non-Go services

## Error Handling

- Missing docker-compose.yml: skip, no error
- Invalid YAML: log warning, continue without docker-compose data
- Missing package.json: skip, no error
- Invalid JSON: log warning, continue without package.json data
- No non-Go services detected: return empty slice, no error

## File Summary

| File | Action |
|------|--------|
| `internal/analyzer/multiservice_detector.go` | New: MultiServiceDetector, ParseDockerCompose, ParsePackageJSON, DetectNonGoServices |
| `internal/analyzer/multiservice_detector_test.go` | New: Tests for parsing and detection |

## Implementation Order

1. Write tests
2. Define ServiceInfo type
3. Implement ParseDockerCompose with env var scanning
4. Implement ParsePackageJSON
5. Implement DetectNonGoServices combining both
6. Run all tests
