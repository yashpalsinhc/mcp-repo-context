package analyzer

import (
	"strings"
	"testing"
)

// --- Dockerfile Tests ---

func TestParseDockerfile_SingleStage(t *testing.T) {
	content := []byte("FROM golang:1.21-alpine\nRUN go build\nEXPOSE 8080\nCMD [\"./app\"]")
	info, err := ParseDockerfile(content)
	if err != nil {
		t.Fatalf("ParseDockerfile: %v", err)
	}
	if len(info.BaseImages) != 1 || info.BaseImages[0] != "golang:1.21-alpine" {
		t.Errorf("BaseImages = %v, want [golang:1.21-alpine]", info.BaseImages)
	}
	if len(info.Ports) != 1 || info.Ports[0] != "8080" {
		t.Errorf("Ports = %v, want [8080]", info.Ports)
	}
	if info.Entrypoint == "" {
		t.Error("Entrypoint should be set")
	}
}

func TestParseDockerfile_MultiStage(t *testing.T) {
	content := []byte("FROM golang:1.21 AS builder\nRUN go build\nFROM alpine:3.18\nCOPY --from=builder /app /app")
	info, err := ParseDockerfile(content)
	if err != nil {
		t.Fatalf("ParseDockerfile: %v", err)
	}
	if len(info.BaseImages) != 2 {
		t.Fatalf("BaseImages = %v, want 2 entries", info.BaseImages)
	}
	if info.BaseImages[0] != "golang:1.21" || info.BaseImages[1] != "alpine:3.18" {
		t.Errorf("BaseImages = %v", info.BaseImages)
	}
	if len(info.Stages) != 1 || info.Stages[0] != "builder" {
		t.Errorf("Stages = %v, want [builder]", info.Stages)
	}
}

func TestParseDockerfile_MultiplePorts(t *testing.T) {
	content := []byte("FROM alpine\nEXPOSE 8080 9090")
	info, err := ParseDockerfile(content)
	if err != nil {
		t.Fatalf("ParseDockerfile: %v", err)
	}
	if len(info.Ports) != 2 {
		t.Errorf("Ports = %v, want 2 entries", info.Ports)
	}
}

func TestParseDockerfile_Empty(t *testing.T) {
	info, err := ParseDockerfile([]byte{})
	if err != nil {
		t.Fatalf("ParseDockerfile: %v", err)
	}
	if len(info.BaseImages) != 0 {
		t.Error("expected empty BaseImages")
	}
}

// --- docker-compose.yml Tests ---

func TestParseDockerCompose_Services(t *testing.T) {
	content := []byte(`services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
  db:
    image: postgres:15
`)
	info, err := ParseDockerCompose(content)
	if err != nil {
		t.Fatalf("ParseDockerCompose: %v", err)
	}
	if len(info.Services) != 2 {
		t.Fatalf("Services count = %d, want 2", len(info.Services))
	}
}

func TestParseDockerCompose_DependsOnList(t *testing.T) {
	content := []byte(`services:
  web:
    image: nginx
    depends_on:
      - db
      - redis
  db:
    image: postgres
  redis:
    image: redis
`)
	info, err := ParseDockerCompose(content)
	if err != nil {
		t.Fatalf("ParseDockerCompose: %v", err)
	}
	for _, svc := range info.Services {
		if svc.Name == "web" {
			if len(svc.DependsOn) != 2 {
				t.Errorf("web DependsOn = %v, want 2 entries", svc.DependsOn)
			}
		}
	}
}

func TestParseDockerCompose_InvalidYAML(t *testing.T) {
	_, err := ParseDockerCompose([]byte("not: valid: yaml: [["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// --- Makefile Tests ---

func TestParseMakefile_Targets(t *testing.T) {
	content := []byte("build:\n\tgo build\ntest:\n\tgo test")
	info, err := ParseMakefile(content)
	if err != nil {
		t.Fatalf("ParseMakefile: %v", err)
	}
	if len(info.Targets) != 2 {
		t.Fatalf("Targets = %d, want 2", len(info.Targets))
	}
	names := map[string]bool{}
	for _, tgt := range info.Targets {
		names[tgt.Name] = true
	}
	if !names["build"] || !names["test"] {
		t.Errorf("Targets = %v, want build and test", info.Targets)
	}
}

func TestParseMakefile_Description(t *testing.T) {
	content := []byte("# Build the binary\nbuild:\n\tgo build")
	info, err := ParseMakefile(content)
	if err != nil {
		t.Fatalf("ParseMakefile: %v", err)
	}
	if len(info.Targets) != 1 {
		t.Fatalf("Targets = %d, want 1", len(info.Targets))
	}
	if info.Targets[0].Description != "Build the binary" {
		t.Errorf("Description = %q, want %q", info.Targets[0].Description, "Build the binary")
	}
}

func TestParseMakefile_NoTargets(t *testing.T) {
	content := []byte("# Just a comment\nVAR = value")
	info, err := ParseMakefile(content)
	if err != nil {
		t.Fatalf("ParseMakefile: %v", err)
	}
	if len(info.Targets) != 0 {
		t.Errorf("Targets = %d, want 0", len(info.Targets))
	}
}

// --- CI Config Tests ---

func TestParseCIConfig_GitHubActions(t *testing.T) {
	content := []byte(`on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
  test:
    runs-on: ubuntu-latest
`)
	info, err := ParseCIConfig(content, "github-actions")
	if err != nil {
		t.Fatalf("ParseCIConfig: %v", err)
	}
	if len(info.Triggers) != 2 {
		t.Errorf("Triggers = %v, want 2", info.Triggers)
	}
	if len(info.Jobs) != 2 {
		t.Errorf("Jobs = %d, want 2", len(info.Jobs))
	}
}

func TestParseCIConfig_GitLabCI(t *testing.T) {
	content := []byte(`stages:
  - build
  - test
build_job:
  stage: build
  script: make build
test_job:
  stage: test
  script: make test
`)
	info, err := ParseCIConfig(content, "gitlab-ci")
	if err != nil {
		t.Fatalf("ParseCIConfig: %v", err)
	}
	if len(info.Jobs) != 2 {
		t.Errorf("Jobs = %d, want 2", len(info.Jobs))
	}
	for _, job := range info.Jobs {
		if job.Stage == "" {
			t.Errorf("Job %q has no stage", job.Name)
		}
	}
}

func TestParseCIConfig_Unknown(t *testing.T) {
	info, err := ParseCIConfig([]byte("foo: bar"), "unknown")
	if err != nil {
		t.Fatalf("ParseCIConfig: %v", err)
	}
	if len(info.Jobs) != 0 {
		t.Error("expected empty jobs for unknown platform")
	}
}

// --- Dispatch Tests ---

func TestParseConfigFile_Dockerfile(t *testing.T) {
	ct, structured, err := ParseConfigFile("Dockerfile", []byte("FROM alpine\nEXPOSE 80"))
	if err != nil {
		t.Fatalf("ParseConfigFile: %v", err)
	}
	if ct != "dockerfile" {
		t.Errorf("configType = %q, want dockerfile", ct)
	}
	if structured == nil {
		t.Error("expected non-nil structured")
	}
}

func TestParseConfigFile_Unrecognized(t *testing.T) {
	ct, structured, err := ParseConfigFile("main.go", []byte("package main"))
	if err != nil {
		t.Fatalf("ParseConfigFile: %v", err)
	}
	if ct != "" || structured != nil {
		t.Errorf("expected empty for unrecognized file, got ct=%q", ct)
	}
}

func TestParseConfigFile_Truncation(t *testing.T) {
	// Content larger than 100KB should be truncated without error
	bigContent := []byte(strings.Repeat("FROM alpine\n", 10000))
	ct, _, err := ParseConfigFile("Dockerfile", bigContent)
	if err != nil {
		t.Fatalf("ParseConfigFile: %v", err)
	}
	if ct != "dockerfile" {
		t.Errorf("configType = %q, want dockerfile", ct)
	}
}
