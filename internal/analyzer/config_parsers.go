package analyzer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxConfigSize = 102400 // 100KB

// DockerfileInfo holds structured data extracted from a Dockerfile.
type DockerfileInfo struct {
	BaseImages []string `json:"base_images"`
	Stages     []string `json:"stages"`
	Ports      []string `json:"ports"`
	Entrypoint string   `json:"entrypoint"`
}

// ComposeInfo holds structured data from docker-compose.yml.
type ComposeInfo struct {
	Services []ComposeService `json:"services"`
}

// ComposeService describes one service in a docker-compose file.
type ComposeService struct {
	Name      string   `json:"name"`
	Image     string   `json:"image,omitempty"`
	Build     string   `json:"build,omitempty"`
	Ports     []string `json:"ports,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// MakefileInfo holds structured data from a Makefile.
type MakefileInfo struct {
	Targets []MakeTarget `json:"targets"`
}

// MakeTarget describes a Makefile target.
type MakeTarget struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CIInfo holds structured data from CI config files.
type CIInfo struct {
	Platform string   `json:"platform"`
	Jobs     []CIJob  `json:"jobs"`
	Triggers []string `json:"triggers,omitempty"`
}

// CIJob describes a CI job.
type CIJob struct {
	Name  string `json:"name"`
	Stage string `json:"stage,omitempty"`
}

// ParseConfigFile determines the config type and calls the appropriate parser.
// Returns ("", nil, nil) if the file is not a recognized config file.
func ParseConfigFile(path string, content []byte) (configType string, structured json.RawMessage, err error) {
	if len(content) > maxConfigSize {
		content = content[:maxConfigSize]
	}

	lower := strings.ToLower(path)

	switch {
	case strings.Contains(lower, "dockerfile"):
		info, err := ParseDockerfile(content)
		if err != nil {
			return "dockerfile", nil, err
		}
		data, _ := json.Marshal(info)
		return "dockerfile", data, nil

	case strings.HasSuffix(lower, "docker-compose.yml") || strings.HasSuffix(lower, "docker-compose.yaml"):
		info, err := ParseDockerCompose(content)
		if err != nil {
			return "docker-compose", nil, err
		}
		data, _ := json.Marshal(info)
		return "docker-compose", data, nil

	case strings.Contains(lower, "makefile"):
		info, err := ParseMakefile(content)
		if err != nil {
			return "makefile", nil, err
		}
		data, _ := json.Marshal(info)
		return "makefile", data, nil

	case strings.Contains(lower, ".github/workflows/") && (strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")):
		info, err := ParseCIConfig(content, "github-actions")
		if err != nil {
			return "ci-config", nil, err
		}
		data, _ := json.Marshal(info)
		return "ci-config", data, nil

	case strings.HasSuffix(lower, ".gitlab-ci.yml"):
		info, err := ParseCIConfig(content, "gitlab-ci")
		if err != nil {
			return "ci-config", nil, err
		}
		data, _ := json.Marshal(info)
		return "ci-config", data, nil
	}

	return "", nil, nil
}

// ParseDockerfile extracts base images, stages, ports, and entrypoint from a Dockerfile.
func ParseDockerfile(content []byte) (*DockerfileInfo, error) {
	info := &DockerfileInfo{
		BaseImages: []string{},
		Stages:     []string{},
		Ports:      []string{},
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		upper := strings.ToUpper(line)

		if strings.HasPrefix(upper, "FROM ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				image := parts[1]
				info.BaseImages = append(info.BaseImages, image)
				// Check for AS clause
				for i, p := range parts {
					if strings.EqualFold(p, "AS") && i+1 < len(parts) {
						info.Stages = append(info.Stages, parts[i+1])
						break
					}
				}
			}
		} else if strings.HasPrefix(upper, "EXPOSE ") {
			parts := strings.Fields(line)
			for _, p := range parts[1:] {
				info.Ports = append(info.Ports, p)
			}
		} else if strings.HasPrefix(upper, "ENTRYPOINT ") {
			info.Entrypoint = strings.TrimPrefix(line, line[:len("ENTRYPOINT ")])
		} else if strings.HasPrefix(upper, "CMD ") && info.Entrypoint == "" {
			info.Entrypoint = strings.TrimPrefix(line, line[:len("CMD ")])
		}
	}

	return info, nil
}

// ParseDockerCompose extracts services from docker-compose.yml.
func ParseDockerCompose(content []byte) (*ComposeInfo, error) {
	var raw struct {
		Services map[string]struct {
			Image     string      `yaml:"image"`
			Build     interface{} `yaml:"build"`
			Ports     []string    `yaml:"ports"`
			DependsOn interface{} `yaml:"depends_on"`
		} `yaml:"services"`
	}

	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse docker-compose: %w", err)
	}

	info := &ComposeInfo{Services: make([]ComposeService, 0, len(raw.Services))}
	for name, svc := range raw.Services {
		cs := ComposeService{
			Name:  name,
			Image: svc.Image,
			Ports: svc.Ports,
		}

		// Handle build field (string or object)
		switch b := svc.Build.(type) {
		case string:
			cs.Build = b
		case map[string]interface{}:
			if ctx, ok := b["context"].(string); ok {
				cs.Build = ctx
			}
		}

		// Handle depends_on (list or map)
		switch d := svc.DependsOn.(type) {
		case []interface{}:
			for _, item := range d {
				if s, ok := item.(string); ok {
					cs.DependsOn = append(cs.DependsOn, s)
				}
			}
		case map[string]interface{}:
			for key := range d {
				cs.DependsOn = append(cs.DependsOn, key)
			}
		}

		info.Services = append(info.Services, cs)
	}

	return info, nil
}

var makeTargetRegex = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*)\s*:`)

// ParseMakefile extracts target names and descriptions from a Makefile.
func ParseMakefile(content []byte) (*MakefileInfo, error) {
	info := &MakefileInfo{Targets: []MakeTarget{}}

	lines := strings.Split(string(content), "\n")
	var lastComment string

	for _, line := range lines {
		// Skip recipe lines (tab-prefixed)
		if strings.HasPrefix(line, "\t") {
			lastComment = ""
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Track comments for descriptions
		if strings.HasPrefix(trimmed, "#") {
			lastComment = strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			continue
		}

		// Skip variable assignments
		if strings.Contains(line, "=") && !strings.Contains(line, ":") {
			lastComment = ""
			continue
		}

		match := makeTargetRegex.FindStringSubmatch(line)
		if match != nil {
			name := match[1]
			if name == ".PHONY" {
				lastComment = ""
				continue
			}
			target := MakeTarget{Name: name, Description: lastComment}
			info.Targets = append(info.Targets, target)
			lastComment = ""
		} else {
			lastComment = ""
		}
	}

	return info, nil
}

// ParseCIConfig extracts jobs and triggers from CI config files.
func ParseCIConfig(content []byte, platform string) (*CIInfo, error) {
	info := &CIInfo{
		Platform: platform,
		Jobs:     []CIJob{},
	}

	switch platform {
	case "github-actions":
		return parseGitHubActions(content, info)
	case "gitlab-ci":
		return parseGitLabCI(content, info)
	default:
		return info, nil
	}
}

func parseGitHubActions(content []byte, info *CIInfo) (*CIInfo, error) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub Actions: %w", err)
	}

	// Extract triggers from "on"
	if on, ok := raw["on"]; ok {
		switch v := on.(type) {
		case string:
			info.Triggers = []string{v}
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					info.Triggers = append(info.Triggers, s)
				}
			}
		case map[string]interface{}:
			for key := range v {
				info.Triggers = append(info.Triggers, key)
			}
		}
	}
	if onTrue, ok := raw["true"]; ok && len(info.Triggers) == 0 {
		// yaml parses "on:" as true sometimes
		_ = onTrue
	}

	// Extract jobs
	if jobs, ok := raw["jobs"].(map[string]interface{}); ok {
		for name := range jobs {
			info.Jobs = append(info.Jobs, CIJob{Name: name})
		}
	}

	return info, nil
}

var gitlabReservedKeys = map[string]bool{
	"stages": true, "variables": true, "include": true, "default": true,
	"workflow": true, "image": true, "before_script": true, "after_script": true,
	"cache": true, "services": true,
}

func parseGitLabCI(content []byte, info *CIInfo) (*CIInfo, error) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse GitLab CI: %w", err)
	}

	for key, val := range raw {
		if gitlabReservedKeys[key] {
			continue
		}
		// Treat as a job
		job := CIJob{Name: key}
		if m, ok := val.(map[string]interface{}); ok {
			if stage, ok := m["stage"].(string); ok {
				job.Stage = stage
			}
		}
		info.Jobs = append(info.Jobs, job)
	}

	return info, nil
}
