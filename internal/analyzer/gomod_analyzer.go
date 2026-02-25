package analyzer

import (
	"fmt"

	"golang.org/x/mod/modfile"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// GoModAnalyzer parses go.mod files into ModuleInfo structs.
type GoModAnalyzer struct{}

// NewGoModAnalyzer creates a new GoModAnalyzer.
func NewGoModAnalyzer() *GoModAnalyzer {
	return &GoModAnalyzer{}
}

// ParseGoMod takes the raw bytes of a go.mod file and returns structured ModuleInfo.
func ParseGoMod(content []byte) (*ctxpkg.ModuleInfo, error) {
	file, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ctxpkg.ErrGoModMalformed, err)
	}

	if file.Module == nil {
		return nil, fmt.Errorf("%w: missing module directive", ctxpkg.ErrGoModMalformed)
	}

	info := &ctxpkg.ModuleInfo{
		ModulePath:   file.Module.Mod.Path,
		Dependencies: make([]ctxpkg.ModuleDependency, 0),
	}

	if file.Go != nil {
		info.GoVersion = file.Go.Version
	}

	// Build set of replaced modules
	replacedModules := make(map[string]bool)
	for _, rep := range file.Replace {
		replacedModules[rep.Old.Path] = true
	}

	// Process require directives
	for _, req := range file.Require {
		dep := ctxpkg.ModuleDependency{
			Path:       req.Mod.Path,
			Version:    req.Mod.Version,
			IsDirect:   !req.Indirect,
			IsReplaced: replacedModules[req.Mod.Path],
		}
		info.Dependencies = append(info.Dependencies, dep)
	}

	// Process replace directives
	for _, rep := range file.Replace {
		mr := ctxpkg.ModuleReplace{
			Old:     rep.Old.Path,
			New:     rep.New.Path,
			Version: rep.New.Version,
		}
		info.Replaces = append(info.Replaces, mr)
	}

	return info, nil
}
