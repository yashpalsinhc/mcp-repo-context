package analyzer

import (
	"encoding/json"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ServiceInfo describes a non-Go service detected from config files.
type ServiceInfo struct {
	Name               string
	Language           string   // "go", "typescript", "python", "java", "unknown"
	Topics             []string // Kafka topic names from env vars
	ServiceURLs        []string // service URLs from env vars
	IsKafkaParticipant bool
	HasHTTPClient      bool
	Source             string // "docker-compose", "package.json"
}

// MultiServiceDetector detects non-Go services from docker-compose.yml and package.json.
type MultiServiceDetector struct{}

// NewMultiServiceDetector creates a new MultiServiceDetector.
func NewMultiServiceDetector() *MultiServiceDetector {
	return &MultiServiceDetector{}
}

// dockerCompose is the minimal structure we need from docker-compose.yml.
type dockerCompose struct {
	Services map[string]dockerService `yaml:"services"`
}

type dockerService struct {
	Image       string            `yaml:"image"`
	Build       interface{}       `yaml:"build"` // string or struct
	Environment interface{}       `yaml:"environment"`
	DependsOn   interface{}       `yaml:"depends_on"`
}

var (
	topicPattern = regexp.MustCompile(`(?i)(kafka[_.]?topic|topic[_.]?name|event[_.]?topic|consume[_.]?topic|produce[_.]?topic)`)
	urlPattern   = regexp.MustCompile(`(?i)(url|endpoint|host)`)
	httpURLRe    = regexp.MustCompile(`^https?://`)
)

// ParseDockerCompose extracts service info from docker-compose.yml content.
func (d *MultiServiceDetector) ParseDockerCompose(content []byte) ([]ServiceInfo, error) {
	var dc dockerCompose
	if err := yaml.Unmarshal(content, &dc); err != nil {
		return nil, err
	}

	var results []ServiceInfo
	for name, svc := range dc.Services {
		info := ServiceInfo{
			Name:   name,
			Source: "docker-compose",
		}

		// Detect language from image
		info.Language = detectLanguageFromImage(svc.Image)

		// Parse environment variables
		envVars := extractEnvVars(svc.Environment)
		for key, val := range envVars {
			if topicPattern.MatchString(key) && val != "" && !isBoolOrNumber(val) {
				info.Topics = append(info.Topics, val)
			}
			if urlPattern.MatchString(key) && httpURLRe.MatchString(val) {
				info.ServiceURLs = append(info.ServiceURLs, val)
			}
		}

		results = append(results, info)
	}

	return results, nil
}

// detectLanguageFromImage guesses language from Docker image name.
func detectLanguageFromImage(image string) string {
	lower := strings.ToLower(image)
	switch {
	case strings.Contains(lower, "node") || strings.Contains(lower, "bun") || strings.Contains(lower, "deno"):
		return "typescript"
	case strings.Contains(lower, "python"):
		return "python"
	case strings.Contains(lower, "openjdk") || strings.Contains(lower, "eclipse-temurin") || strings.Contains(lower, "java"):
		return "java"
	case strings.Contains(lower, "golang") || strings.Contains(lower, "go:"):
		return "go"
	default:
		return "unknown"
	}
}

// extractEnvVars extracts environment variables from docker-compose format.
// Supports both list format (KEY=VALUE) and map format ({KEY: VALUE}).
func extractEnvVars(env interface{}) map[string]string {
	result := make(map[string]string)
	if env == nil {
		return result
	}

	switch v := env.(type) {
	case map[string]interface{}:
		for key, val := range v {
			if s, ok := val.(string); ok {
				result[key] = s
			}
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts := strings.SplitN(s, "=", 2)
				if len(parts) == 2 {
					result[parts[0]] = parts[1]
				}
			}
		}
	}
	return result
}

func isBoolOrNumber(s string) bool {
	lower := strings.ToLower(s)
	return lower == "true" || lower == "false" || lower == "0" || lower == "1"
}

// packageJSON is the minimal structure we need from package.json.
type packageJSON struct {
	Name            string            `json:"name"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

var kafkaDeps = map[string]bool{
	"kafkajs": true, "node-rdkafka": true, "kafka-node": true, "@nestjs/microservices": true,
}

var httpClientDeps = map[string]bool{
	"axios": true, "node-fetch": true, "got": true, "superagent": true,
}

// ParsePackageJSON extracts service info from package.json content.
func (d *MultiServiceDetector) ParsePackageJSON(content []byte) (*ServiceInfo, error) {
	var pkg packageJSON
	if err := json.Unmarshal(content, &pkg); err != nil {
		return nil, err
	}

	info := &ServiceInfo{
		Name:     pkg.Name,
		Language: "typescript",
		Source:   "package.json",
	}

	allDeps := make(map[string]string)
	for k, v := range pkg.Dependencies {
		allDeps[k] = v
	}
	for k, v := range pkg.DevDependencies {
		allDeps[k] = v
	}

	for dep := range allDeps {
		if kafkaDeps[dep] {
			info.IsKafkaParticipant = true
		}
		if httpClientDeps[dep] {
			info.HasHTTPClient = true
		}
	}

	return info, nil
}

// DetectNonGoServices finds non-Go services from repo config files.
// repoFiles maps filenames to contents, goRepos lists known Go repo names.
func (d *MultiServiceDetector) DetectNonGoServices(repoFiles map[string][]byte, goRepos map[string]bool) ([]ServiceInfo, error) {
	var results []ServiceInfo

	// Check docker-compose files
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml"} {
		if content, ok := repoFiles[name]; ok {
			services, err := d.ParseDockerCompose(content)
			if err != nil {
				continue // skip invalid YAML
			}
			for _, svc := range services {
				if !goRepos[svc.Name] {
					results = append(results, svc)
				}
			}
			break // only parse one docker-compose file
		}
	}

	// Check package.json
	if content, ok := repoFiles["package.json"]; ok {
		info, err := d.ParsePackageJSON(content)
		if err == nil && info != nil {
			if !goRepos[info.Name] {
				results = append(results, *info)
			}
		}
	}

	return results, nil
}
