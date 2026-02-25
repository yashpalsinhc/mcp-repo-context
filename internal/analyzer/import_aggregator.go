package analyzer

import (
	"sort"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// AggregateImports collects all imports from analyzed files and classifies them
// as stdlib, internal, or external relative to the given module info.
// If moduleInfo is nil (no go.mod found), returns an empty ImportSummary.
func AggregateImports(files map[string]*ctxpkg.FileContext, moduleInfo *ctxpkg.ModuleInfo) *ctxpkg.ImportSummary {
	summary := &ctxpkg.ImportSummary{}

	if moduleInfo == nil {
		return summary
	}

	// Collect unique import paths
	unique := make(map[string]struct{})
	for _, fc := range files {
		if fc == nil {
			continue
		}
		for _, imp := range fc.Imports {
			if imp.Path != "" {
				unique[imp.Path] = struct{}{}
			}
		}
	}

	for path := range unique {
		// Skip C pseudo-package
		if path == "C" {
			continue
		}

		if isStdlib(path) {
			summary.Stdlib = append(summary.Stdlib, path)
		} else if path == moduleInfo.ModulePath || strings.HasPrefix(path, moduleInfo.ModulePath+"/") {
			summary.Internal = append(summary.Internal, path)
		} else {
			ext := resolveExternal(path, moduleInfo)
			summary.External = append(summary.External, ext)
		}
	}

	// Sort for deterministic output
	sort.Strings(summary.Stdlib)
	sort.Strings(summary.Internal)
	sort.Slice(summary.External, func(i, j int) bool {
		return summary.External[i].ImportPath < summary.External[j].ImportPath
	})

	return summary
}

// resolveExternal resolves an external import path to a module path using go.mod data.
func resolveExternal(importPath string, moduleInfo *ctxpkg.ModuleInfo) ctxpkg.ExternalImport {
	// Apply replace directives first
	resolvedPath, replaceVersion := applyReplaces(importPath, moduleInfo.Replaces)

	// Find longest prefix match among dependencies
	var bestMatch string
	var bestVersion string
	for _, dep := range moduleInfo.Dependencies {
		if resolvedPath == dep.Path || strings.HasPrefix(resolvedPath, dep.Path+"/") {
			if len(dep.Path) > len(bestMatch) {
				bestMatch = dep.Path
				bestVersion = dep.Version
			}
		}
	}

	if bestMatch != "" {
		modulePath := bestMatch
		version := bestVersion
		if replaceVersion != "" {
			version = replaceVersion
		}
		return ctxpkg.ExternalImport{
			ImportPath: importPath,
			ModulePath: modulePath,
			Version:    version,
		}
	}

	// No matching require entry — best-effort
	return ctxpkg.ExternalImport{
		ImportPath: importPath,
		ModulePath: importPath,
	}
}

// applyReplaces checks if the import path matches any replace directive
// and returns the (possibly modified) path and version.
func applyReplaces(importPath string, replaces []ctxpkg.ModuleReplace) (string, string) {
	for _, rep := range replaces {
		if importPath == rep.Old || strings.HasPrefix(importPath, rep.Old+"/") {
			suffix := strings.TrimPrefix(importPath, rep.Old)
			return rep.New + suffix, rep.Version
		}
	}
	return importPath, ""
}

// isStdlib returns true if the import path is a Go standard library package.
func isStdlib(path string) bool {
	if stdlibPackages[path] {
		return true
	}
	// Fallback: no-dots heuristic. Stdlib first segment never contains a dot.
	firstSeg := path
	if idx := strings.Index(path, "/"); idx >= 0 {
		firstSeg = path[:idx]
	}
	return !strings.Contains(firstSeg, ".")
}

// stdlibPackages is a static set of known Go standard library packages.
var stdlibPackages = map[string]bool{
	"archive/tar":            true,
	"archive/zip":            true,
	"bufio":                  true,
	"bytes":                  true,
	"cmp":                    true,
	"compress/bzip2":         true,
	"compress/flate":         true,
	"compress/gzip":          true,
	"compress/lzw":           true,
	"compress/zlib":          true,
	"container/heap":         true,
	"container/list":         true,
	"container/ring":         true,
	"context":                true,
	"crypto":                 true,
	"crypto/aes":             true,
	"crypto/cipher":          true,
	"crypto/des":             true,
	"crypto/dsa":             true,
	"crypto/ecdsa":           true,
	"crypto/ed25519":         true,
	"crypto/elliptic":        true,
	"crypto/hmac":            true,
	"crypto/md5":             true,
	"crypto/rand":            true,
	"crypto/rc4":             true,
	"crypto/rsa":             true,
	"crypto/sha1":            true,
	"crypto/sha256":          true,
	"crypto/sha512":          true,
	"crypto/subtle":          true,
	"crypto/tls":             true,
	"crypto/x509":            true,
	"crypto/x509/pkix":       true,
	"database/sql":           true,
	"database/sql/driver":    true,
	"debug":                  true,
	"debug/buildinfo":        true,
	"debug/dwarf":            true,
	"debug/elf":              true,
	"debug/gosym":            true,
	"debug/macho":            true,
	"debug/pe":               true,
	"debug/plan9obj":         true,
	"embed":                  true,
	"encoding":               true,
	"encoding/ascii85":       true,
	"encoding/asn1":          true,
	"encoding/base32":        true,
	"encoding/base64":        true,
	"encoding/binary":        true,
	"encoding/csv":           true,
	"encoding/gob":           true,
	"encoding/hex":           true,
	"encoding/json":          true,
	"encoding/pem":           true,
	"encoding/xml":           true,
	"errors":                 true,
	"expvar":                 true,
	"flag":                   true,
	"fmt":                    true,
	"go/ast":                 true,
	"go/build":               true,
	"go/constant":            true,
	"go/doc":                 true,
	"go/format":              true,
	"go/importer":            true,
	"go/parser":              true,
	"go/printer":             true,
	"go/scanner":             true,
	"go/token":               true,
	"go/types":               true,
	"hash":                   true,
	"hash/adler32":           true,
	"hash/crc32":             true,
	"hash/crc64":             true,
	"hash/fnv":               true,
	"hash/maphash":           true,
	"html":                   true,
	"html/template":          true,
	"image":                  true,
	"image/color":            true,
	"image/draw":             true,
	"image/gif":              true,
	"image/jpeg":             true,
	"image/png":              true,
	"io":                     true,
	"io/fs":                  true,
	"io/ioutil":              true,
	"iter":                   true,
	"log":                    true,
	"log/slog":               true,
	"log/syslog":             true,
	"maps":                   true,
	"math":                   true,
	"math/big":               true,
	"math/bits":              true,
	"math/cmplx":             true,
	"math/rand":              true,
	"math/rand/v2":           true,
	"mime":                   true,
	"mime/multipart":         true,
	"mime/quotedprintable":   true,
	"net":                    true,
	"net/http":               true,
	"net/http/cgi":           true,
	"net/http/cookiejar":     true,
	"net/http/fcgi":          true,
	"net/http/httptest":      true,
	"net/http/httptrace":     true,
	"net/http/httputil":      true,
	"net/http/pprof":         true,
	"net/mail":               true,
	"net/netip":              true,
	"net/rpc":                true,
	"net/rpc/jsonrpc":        true,
	"net/smtp":               true,
	"net/textproto":          true,
	"net/url":                true,
	"os":                     true,
	"os/exec":                true,
	"os/signal":              true,
	"os/user":                true,
	"path":                   true,
	"path/filepath":          true,
	"plugin":                 true,
	"reflect":                true,
	"regexp":                 true,
	"regexp/syntax":          true,
	"runtime":                true,
	"runtime/cgo":            true,
	"runtime/debug":          true,
	"runtime/metrics":        true,
	"runtime/pprof":          true,
	"runtime/race":           true,
	"runtime/trace":          true,
	"slices":                 true,
	"sort":                   true,
	"strconv":                true,
	"strings":                true,
	"sync":                   true,
	"sync/atomic":            true,
	"syscall":                true,
	"testing":                true,
	"testing/fstest":         true,
	"testing/iotest":         true,
	"testing/quick":          true,
	"text/scanner":           true,
	"text/tabwriter":         true,
	"text/template":          true,
	"text/template/parse":    true,
	"time":                   true,
	"time/tzdata":            true,
	"unicode":                true,
	"unicode/utf16":          true,
	"unicode/utf8":           true,
	"unsafe":                 true,
}
