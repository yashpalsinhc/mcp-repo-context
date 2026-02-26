package analyzer

import (
	"testing"
)

func TestParseDockerComposeExtractsServices(t *testing.T) {
	content := []byte(`
services:
  auth:
    image: golang:1.21
  users:
    image: node:18-alpine
  frontend:
    image: nginx:latest
`)
	d := NewMultiServiceDetector()
	services, err := d.ParseDockerCompose(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 3 {
		t.Fatalf("Expected 3 services, got %d", len(services))
	}
	names := map[string]bool{}
	for _, s := range services {
		names[s.Name] = true
	}
	for _, name := range []string{"auth", "users", "frontend"} {
		if !names[name] {
			t.Errorf("Missing service %q", name)
		}
	}
}

func TestParseDockerComposeExtractsKafkaTopicsFromEnv(t *testing.T) {
	content := []byte(`
services:
  worker:
    image: node:18
    environment:
      KAFKA_TOPIC: user.events
      TOPIC_NAME: orders
`)
	d := NewMultiServiceDetector()
	services, err := d.ParseDockerCompose(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(services))
	}
	if len(services[0].Topics) != 2 {
		t.Errorf("Expected 2 topics, got %d: %v", len(services[0].Topics), services[0].Topics)
	}
}

func TestParseDockerComposeExtractsServiceURLs(t *testing.T) {
	content := []byte(`
services:
  gateway:
    image: node:18
    environment:
      AUTH_SERVICE_URL: http://auth:8080
      USER_API_URL: http://users:9090
`)
	d := NewMultiServiceDetector()
	services, err := d.ParseDockerCompose(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(services[0].ServiceURLs) != 2 {
		t.Errorf("Expected 2 service URLs, got %d: %v", len(services[0].ServiceURLs), services[0].ServiceURLs)
	}
}

func TestParseDockerComposeDetectsLanguageFromImage(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{"node:18-alpine", "typescript"},
		{"python:3.11", "python"},
		{"openjdk:17", "java"},
		{"golang:1.21", "go"},
		{"nginx:latest", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := detectLanguageFromImage(tt.image)
			if got != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestParseDockerComposeHandlesMissingEnvironment(t *testing.T) {
	content := []byte(`
services:
  worker:
    image: node:18
`)
	d := NewMultiServiceDetector()
	services, err := d.ParseDockerCompose(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(services[0].Topics) != 0 {
		t.Errorf("Expected 0 topics, got %d", len(services[0].Topics))
	}
	if len(services[0].ServiceURLs) != 0 {
		t.Errorf("Expected 0 URLs, got %d", len(services[0].ServiceURLs))
	}
}

func TestParseDockerComposeListFormatEnv(t *testing.T) {
	content := []byte(`
services:
  worker:
    image: node:18
    environment:
      - KAFKA_TOPIC=user.events
      - API_URL=http://auth:8080
`)
	d := NewMultiServiceDetector()
	services, err := d.ParseDockerCompose(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(services[0].Topics) != 1 {
		t.Errorf("Expected 1 topic, got %d", len(services[0].Topics))
	}
	if len(services[0].ServiceURLs) != 1 {
		t.Errorf("Expected 1 URL, got %d", len(services[0].ServiceURLs))
	}
}

func TestParsePackageJSONDetectsKafka(t *testing.T) {
	content := []byte(`{
		"name": "my-service",
		"dependencies": {
			"kafkajs": "^2.0.0",
			"express": "^4.18.0"
		}
	}`)
	d := NewMultiServiceDetector()
	info, err := d.ParsePackageJSON(content)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsKafkaParticipant {
		t.Error("Expected IsKafkaParticipant to be true")
	}
}

func TestParsePackageJSONDetectsHTTPClient(t *testing.T) {
	content := []byte(`{
		"name": "my-service",
		"dependencies": {
			"axios": "^1.0.0"
		}
	}`)
	d := NewMultiServiceDetector()
	info, err := d.ParsePackageJSON(content)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasHTTPClient {
		t.Error("Expected HasHTTPClient to be true")
	}
}

func TestParsePackageJSONHandlesInvalidJSON(t *testing.T) {
	d := NewMultiServiceDetector()
	_, err := d.ParsePackageJSON([]byte(`invalid`))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestDetectNonGoServicesFilterOutGoRepos(t *testing.T) {
	files := map[string][]byte{
		"docker-compose.yml": []byte(`
services:
  auth:
    image: golang:1.21
  frontend:
    image: node:18
  users:
    image: golang:1.21
`),
	}
	goRepos := map[string]bool{"auth": true, "users": true}

	d := NewMultiServiceDetector()
	services, err := d.DetectNonGoServices(files, goRepos)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 {
		t.Fatalf("Expected 1 non-Go service, got %d", len(services))
	}
	if services[0].Name != "frontend" {
		t.Errorf("Expected 'frontend', got %q", services[0].Name)
	}
}

func TestDetectNonGoServicesCombinesSources(t *testing.T) {
	files := map[string][]byte{
		"docker-compose.yml": []byte(`
services:
  api:
    image: node:18
    environment:
      KAFKA_TOPIC: events
`),
		"package.json": []byte(`{
			"name": "standalone-service",
			"dependencies": {"axios": "^1.0.0"}
		}`),
	}
	goRepos := map[string]bool{}

	d := NewMultiServiceDetector()
	services, err := d.DetectNonGoServices(files, goRepos)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 2 {
		t.Fatalf("Expected 2 services, got %d", len(services))
	}
}
