package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewScanner(t *testing.T) {
	s := NewScanner(nil, 0)
	if s == nil {
		t.Fatal("expected non-nil scanner")
	}
}

func TestScanner_Scan(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create test files
	files := map[string]string{
		"main.go":          "package main",
		"pkg/util.go":      "package pkg",
		"pkg/helper.go":    "package pkg",
		"internal/core.go": "package internal",
		"README.md":        "# Project",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	// Scan
	scanner := NewScanner(nil, 1024*1024)
	ctx := context.Background()

	var scannedFiles []string
	err := scanner.Scan(ctx, tmpDir, func(file FileInfo) error {
		scannedFiles = append(scannedFiles, file.Path)
		return nil
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(scannedFiles) != len(files) {
		t.Errorf("expected %d files, got %d: %v", len(files), len(scannedFiles), scannedFiles)
	}

	// Verify all files were scanned
	scannedMap := make(map[string]bool)
	for _, f := range scannedFiles {
		scannedMap[f] = true
	}
	for path := range files {
		if !scannedMap[path] {
			t.Errorf("file %s was not scanned", path)
		}
	}
}

func TestScanner_Scan_ExcludePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	files := []string{
		"main.go",
		"vendor/pkg/lib.go",
		"node_modules/pkg/index.js",
		"dist/bundle.js",
		"src/app.go",
	}

	for _, path := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte("content"), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	// Scan with exclusions
	excludePatterns := []string{
		"vendor/**",
		"node_modules/**",
		"dist/**",
	}
	scanner := NewScanner(excludePatterns, 1024*1024)
	ctx := context.Background()

	var scannedFiles []string
	err := scanner.Scan(ctx, tmpDir, func(file FileInfo) error {
		scannedFiles = append(scannedFiles, file.Path)
		return nil
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Should only have main.go and src/app.go
	if len(scannedFiles) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(scannedFiles), scannedFiles)
	}

	scannedMap := make(map[string]bool)
	for _, f := range scannedFiles {
		scannedMap[f] = true
	}

	if !scannedMap["main.go"] {
		t.Error("expected main.go to be scanned")
	}
	if !scannedMap["src/app.go"] {
		t.Error("expected src/app.go to be scanned")
	}
}

func TestScanner_Scan_ExcludeGit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .git directory (always excluded)
	gitDir := filepath.Join(tmpDir, ".git", "objects")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "pack"), []byte("git data"), 0644); err != nil {
		t.Fatalf("failed to write git file: %v", err)
	}

	// Create regular file
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	scanner := NewScanner(nil, 1024*1024)
	ctx := context.Background()

	var scannedFiles []string
	err := scanner.Scan(ctx, tmpDir, func(file FileInfo) error {
		scannedFiles = append(scannedFiles, file.Path)
		return nil
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Should only have main.go
	if len(scannedFiles) != 1 || scannedFiles[0] != "main.go" {
		t.Errorf("expected only main.go, got %v", scannedFiles)
	}
}

func TestScanner_Scan_MaxFileSize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create small file
	if err := os.WriteFile(filepath.Join(tmpDir, "small.go"), []byte("small"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create large file (100 bytes, exceeds our 50 byte limit)
	largeContent := make([]byte, 100)
	if err := os.WriteFile(filepath.Join(tmpDir, "large.go"), largeContent, 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner(nil, 50) // 50 byte limit
	ctx := context.Background()

	var scannedFiles []string
	err := scanner.Scan(ctx, tmpDir, func(file FileInfo) error {
		scannedFiles = append(scannedFiles, file.Path)
		return nil
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Should only have small.go
	if len(scannedFiles) != 1 || scannedFiles[0] != "small.go" {
		t.Errorf("expected only small.go, got %v", scannedFiles)
	}
}

func TestScanner_Scan_LanguageDetection(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"main.go":       "go",
		"app.ts":        "typescript",
		"script.js":     "javascript",
		"lib.py":        "python",
		"Makefile":      "makefile",
		"Dockerfile":    "dockerfile",
		"config.yaml":   "yaml",
		"data.json":     "json",
		"unknown.xyz":   "unknown",
	}

	for path := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, path), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	scanner := NewScanner(nil, 1024*1024)
	ctx := context.Background()

	detectedLangs := make(map[string]string)
	err := scanner.Scan(ctx, tmpDir, func(file FileInfo) error {
		detectedLangs[file.Path] = file.Language
		return nil
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	for path, expectedLang := range files {
		if detectedLangs[path] != expectedLang {
			t.Errorf("expected %s for %s, got %s", expectedLang, path, detectedLangs[path])
		}
	}
}

func TestScanner_Scan_FileInfo(t *testing.T) {
	tmpDir := t.TempDir()

	content := []byte("package main\n\nfunc main() {}\n")
	filePath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner(nil, 1024*1024)
	ctx := context.Background()

	var fileInfo FileInfo
	err := scanner.Scan(ctx, tmpDir, func(file FileInfo) error {
		fileInfo = file
		return nil
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Verify FileInfo fields
	if fileInfo.Path != "main.go" {
		t.Errorf("expected path main.go, got %s", fileInfo.Path)
	}
	if fileInfo.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), fileInfo.Size)
	}
	if fileInfo.Language != "go" {
		t.Errorf("expected language go, got %s", fileInfo.Language)
	}
	if fileInfo.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if fileInfo.IsDirectory {
		t.Error("expected IsDirectory to be false")
	}
	if fileInfo.ModTime.IsZero() {
		t.Error("expected non-zero mod time")
	}
}

func TestScanner_Scan_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some files
	for i := 0; i < 10; i++ {
		path := filepath.Join(tmpDir, "file"+string(rune('0'+i))+".go")
		if err := os.WriteFile(path, []byte("package main"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	scanner := NewScanner(nil, 1024*1024)
	ctx, cancel := context.WithCancel(context.Background())

	scannedCount := 0
	err := scanner.Scan(ctx, tmpDir, func(file FileInfo) error {
		scannedCount++
		if scannedCount >= 3 {
			cancel() // Cancel after scanning 3 files
		}
		return nil
	})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
