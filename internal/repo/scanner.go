package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Ensure scanner implements FileScanner interface.
var _ FileScanner = (*scanner)(nil)

// scanner walks repository files (private struct).
type scanner struct {
	excludePatterns []string
	maxFileSize     int64
}

// NewScanner creates a new file scanner.
func NewScanner(excludePatterns []string, maxFileSize int64) FileScanner {
	if maxFileSize == 0 {
		maxFileSize = 1024 * 1024 // 1MB default
	}
	return &scanner{
		excludePatterns: excludePatterns,
		maxFileSize:     maxFileSize,
	}
}

// Scan walks all files in repository, respecting ignore patterns.
func (s *scanner) Scan(ctx context.Context, rootPath string, fn func(FileInfo) error) error {
	return filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil
		}

		// Skip root
		if relPath == "." {
			return nil
		}

		// Check exclusions
		if s.shouldExclude(relPath, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories (we only process files)
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Skip large files
		if info.Size() > s.maxFileSize {
			return nil
		}

		// Compute hash
		hash, err := computeFileHash(path)
		if err != nil {
			hash = ""
		}

		fileInfo := FileInfo{
			Path:        relPath,
			AbsPath:     path,
			Size:        info.Size(),
			Hash:        hash,
			Language:    detectLanguage(relPath),
			IsDirectory: false,
			ModTime:     info.ModTime(),
		}

		return fn(fileInfo)
	})
}

func (s *scanner) shouldExclude(path string, isDir bool) bool {
	// Always exclude .git
	if strings.HasPrefix(path, ".git") || strings.Contains(path, "/.git") {
		return true
	}

	for _, pattern := range s.excludePatterns {
		// Simple pattern matching
		if strings.HasPrefix(pattern, "**/") {
			// Match anywhere in path
			suffix := strings.TrimPrefix(pattern, "**/")
			if strings.Contains(path, suffix) || strings.HasSuffix(path, strings.TrimSuffix(suffix, "/**")) {
				return true
			}
		} else if strings.HasSuffix(pattern, "/**") {
			// Match directory prefix
			prefix := strings.TrimSuffix(pattern, "/**")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		} else if strings.HasPrefix(pattern, "*.") {
			// Match extension
			ext := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(path, ext) {
				return true
			}
		} else {
			// Exact match
			if path == pattern || strings.HasPrefix(path, pattern+"/") {
				return true
			}
		}
	}

	return false
}

func computeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil // Short hash
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	name := strings.ToLower(filepath.Base(path))

	// Check by filename first
	switch name {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	case "go.mod", "go.sum":
		return "go-mod"
	case "package.json":
		return "json"
	case ".gitignore":
		return "gitignore"
	}

	// Check by extension
	switch ext {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescript"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".cs":
		return "csharp"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".scss", ".sass":
		return "scss"
	case ".sql":
		return "sql"
	case ".sh", ".bash":
		return "shell"
	case ".proto":
		return "protobuf"
	default:
		return "unknown"
	}
}
