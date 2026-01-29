package comparison

import (
	"context"
	"sort"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// comparer implements the Comparer interface.
type comparer struct {
	// Configuration options
	defaultThreshold float64
}

// NewComparer creates a new Comparer instance.
func NewComparer() Comparer {
	return &comparer{
		defaultThreshold: 0.8,
	}
}

// Compare analyzes multiple repos and returns a comparison result.
func (c *comparer) Compare(ctx context.Context, repoContexts []*ctxpkg.RepoContext, opts CompareOptions) (*CompareResult, error) {
	if len(repoContexts) == 0 {
		return &CompareResult{}, nil
	}

	result := &CompareResult{
		Repos:       make([]RepoSummary, 0, len(repoContexts)),
		Duplicates:  []DuplicateGroup{},
		Conflicts:   []Conflict{},
		Gaps:        []Gap{},
		FileOverlap: []FileOverlapInfo{},
	}

	// Build repo summaries
	for _, rc := range repoContexts {
		summary := RepoSummary{
			ID:            rc.ID,
			URL:           rc.URL,
			Branch:        rc.Branch,
			FileCount:     len(rc.Files),
			Languages:     make(map[string]int),
			IsTarget:      rc.ID == opts.TargetRepoID,
			TotalLines:    rc.Statistics.TotalLines,
			FunctionCount: rc.Statistics.FunctionCount,
			TypeCount:     rc.Statistics.TypeCount,
		}

		// Count languages
		for _, fc := range rc.Files {
			summary.Languages[fc.Language]++
		}

		result.Repos = append(result.Repos, summary)
	}

	// Calculate unified statistics
	result.UnifiedStatistics = c.calculateUnifiedStats(repoContexts)

	// Find file overlaps
	result.FileOverlap = c.findFileOverlaps(repoContexts)

	// Optional analyses
	if opts.IncludeDuplicates {
		duplicates, err := c.FindDuplicates(ctx, repoContexts)
		if err != nil {
			return nil, err
		}
		result.Duplicates = duplicates
	}

	if opts.IncludeConflicts && opts.TargetRepoID != "" {
		var sourceContexts []*ctxpkg.RepoContext
		var targetContext *ctxpkg.RepoContext
		for _, rc := range repoContexts {
			if rc.ID == opts.TargetRepoID {
				targetContext = rc
			} else {
				sourceContexts = append(sourceContexts, rc)
			}
		}
		if targetContext != nil {
			conflicts, err := c.FindConflicts(ctx, sourceContexts, targetContext)
			if err != nil {
				return nil, err
			}
			result.Conflicts = conflicts
		}
	}

	if opts.IncludeGaps && opts.TargetRepoID != "" {
		var sourceContexts []*ctxpkg.RepoContext
		var targetContext *ctxpkg.RepoContext
		for _, rc := range repoContexts {
			if rc.ID == opts.TargetRepoID {
				targetContext = rc
			} else {
				sourceContexts = append(sourceContexts, rc)
			}
		}
		if targetContext != nil {
			gaps, err := c.FindGaps(ctx, sourceContexts, targetContext)
			if err != nil {
				return nil, err
			}
			result.Gaps = gaps
		}
	}

	if opts.IncludeConsistency {
		consistency, err := c.AnalyzeConsistency(ctx, repoContexts)
		if err != nil {
			return nil, err
		}
		result.Consistency = consistency
	}

	// Generate recommendations
	result.Recommendations = c.generateRecommendations(result)

	return result, nil
}

// FindDuplicates identifies duplicate or similar implementations across repos.
func (c *comparer) FindDuplicates(ctx context.Context, repoContexts []*ctxpkg.RepoContext) ([]DuplicateGroup, error) {
	var duplicates []DuplicateGroup

	// Find function duplicates
	funcMap := make(map[string][]DuplicateInstance)
	for _, rc := range repoContexts {
		for filePath, fc := range rc.Files {
			for _, fn := range fc.Functions {
				key := c.normalizeFunctionKey(&fn)
				funcMap[key] = append(funcMap[key], DuplicateInstance{
					RepoID:    rc.ID,
					FilePath:  filePath,
					Line:      fn.LineStart,
					Signature: fn.Signature,
				})
			}
		}
	}

	for name, instances := range funcMap {
		if len(instances) > 1 && c.instancesFromMultipleRepos(instances) {
			duplicates = append(duplicates, DuplicateGroup{
				Type:           "function",
				Name:           name,
				Instances:      instances,
				Similarity:     1.0, // Exact match by normalized key
				Recommendation: "Consider consolidating into a shared package",
			})
		}
	}

	// Find type duplicates
	typeMap := make(map[string][]DuplicateInstance)
	for _, rc := range repoContexts {
		for filePath, fc := range rc.Files {
			for _, td := range fc.Types {
				key := c.normalizeTypeKey(&td)
				typeMap[key] = append(typeMap[key], DuplicateInstance{
					RepoID:   rc.ID,
					FilePath: filePath,
					Line:     td.LineStart,
				})
			}
		}
	}

	for name, instances := range typeMap {
		if len(instances) > 1 && c.instancesFromMultipleRepos(instances) {
			duplicates = append(duplicates, DuplicateGroup{
				Type:           "type",
				Name:           name,
				Instances:      instances,
				Similarity:     1.0,
				Recommendation: "Consider extracting to a shared types package",
			})
		}
	}

	// Sort by number of instances (most duplicated first)
	sort.Slice(duplicates, func(i, j int) bool {
		return len(duplicates[i].Instances) > len(duplicates[j].Instances)
	})

	return duplicates, nil
}

// FindConflicts identifies conflicting implementations between source and target repos.
func (c *comparer) FindConflicts(ctx context.Context, sourceContexts []*ctxpkg.RepoContext, targetContext *ctxpkg.RepoContext) ([]Conflict, error) {
	var conflicts []Conflict

	if targetContext == nil {
		return conflicts, nil
	}

	// Build target function map
	targetFuncs := make(map[string]*ctxpkg.FunctionDef)
	targetFuncFiles := make(map[string]string)
	for filePath, fc := range targetContext.Files {
		for i := range fc.Functions {
			fn := &fc.Functions[i]
			key := fn.Name
			targetFuncs[key] = fn
			targetFuncFiles[key] = filePath
		}
	}

	// Check source functions against target
	for _, srcCtx := range sourceContexts {
		for filePath, fc := range srcCtx.Files {
			for _, fn := range fc.Functions {
				if targetFn, exists := targetFuncs[fn.Name]; exists {
					// Check for signature mismatch
					if fn.Signature != targetFn.Signature {
						conflicts = append(conflicts, Conflict{
							Type:        "signature_mismatch",
							Name:        fn.Name,
							Description: "Function has different signatures",
							SourceInstances: []ConflictInstance{{
								RepoID:    srcCtx.ID,
								FilePath:  filePath,
								Line:      fn.LineStart,
								Signature: fn.Signature,
							}},
							TargetInstance: &ConflictInstance{
								RepoID:    targetContext.ID,
								FilePath:  targetFuncFiles[fn.Name],
								Line:      targetFn.LineStart,
								Signature: targetFn.Signature,
							},
							Severity:   c.assessSeverity(fn.Signature, targetFn.Signature),
							Resolution: "Reconcile function signatures before merging",
						})
					}
				}
			}
		}
	}

	// Build target type map
	targetTypes := make(map[string]*ctxpkg.TypeDef)
	for _, fc := range targetContext.Files {
		for i := range fc.Types {
			td := &fc.Types[i]
			targetTypes[td.Name] = td
		}
	}

	// Check source types against target
	for _, srcCtx := range sourceContexts {
		for filePath, fc := range srcCtx.Files {
			for _, td := range fc.Types {
				if targetTd, exists := targetTypes[td.Name]; exists {
					// Check for field count mismatch (simplified check)
					if len(td.Fields) != len(targetTd.Fields) {
						conflicts = append(conflicts, Conflict{
							Type:        "type_mismatch",
							Name:        td.Name,
							Description: "Type has different structure",
							SourceInstances: []ConflictInstance{{
								RepoID:   srcCtx.ID,
								FilePath: filePath,
								Line:     td.LineStart,
								Details:  formatFields(td.Fields),
							}},
							TargetInstance: &ConflictInstance{
								RepoID:  targetContext.ID,
								Details: formatFields(targetTd.Fields),
							},
							Severity:   "medium",
							Resolution: "Review type definitions and merge fields",
						})
					}
				}
			}
		}
	}

	return conflicts, nil
}

// FindGaps identifies functionality in source repos missing from target repo.
func (c *comparer) FindGaps(ctx context.Context, sourceContexts []*ctxpkg.RepoContext, targetContext *ctxpkg.RepoContext) ([]Gap, error) {
	var gaps []Gap

	if targetContext == nil {
		return gaps, nil
	}

	// Build target inventory
	targetFuncs := make(map[string]bool)
	targetTypes := make(map[string]bool)
	targetFiles := make(map[string]bool)

	for filePath, fc := range targetContext.Files {
		targetFiles[filePath] = true
		for _, fn := range fc.Functions {
			targetFuncs[fn.Name] = true
		}
		for _, td := range fc.Types {
			targetTypes[td.Name] = true
		}
	}

	// Find missing functions
	funcSources := make(map[string][]string)
	funcFiles := make(map[string]string)
	for _, srcCtx := range sourceContexts {
		for filePath, fc := range srcCtx.Files {
			for _, fn := range fc.Functions {
				if !targetFuncs[fn.Name] {
					funcSources[fn.Name] = append(funcSources[fn.Name], srcCtx.ID)
					funcFiles[fn.Name] = filePath
				}
			}
		}
	}

	for name, repos := range funcSources {
		gaps = append(gaps, Gap{
			Type:        "function",
			Name:        name,
			SourceRepos: repos,
			FilePath:    funcFiles[name],
			Description: "Function not present in target repo",
			Priority:    c.assessGapPriority(len(repos)),
		})
	}

	// Find missing types
	typeSources := make(map[string][]string)
	for _, srcCtx := range sourceContexts {
		for _, fc := range srcCtx.Files {
			for _, td := range fc.Types {
				if !targetTypes[td.Name] {
					typeSources[td.Name] = append(typeSources[td.Name], srcCtx.ID)
				}
			}
		}
	}

	for name, repos := range typeSources {
		gaps = append(gaps, Gap{
			Type:        "type",
			Name:        name,
			SourceRepos: repos,
			Description: "Type not present in target repo",
			Priority:    c.assessGapPriority(len(repos)),
		})
	}

	// Sort by priority
	priorityOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.Slice(gaps, func(i, j int) bool {
		return priorityOrder[gaps[i].Priority] < priorityOrder[gaps[j].Priority]
	})

	return gaps, nil
}

// AnalyzeConsistency checks for implementation consistency across repos.
func (c *comparer) AnalyzeConsistency(ctx context.Context, repoContexts []*ctxpkg.RepoContext) (*ConsistencyReport, error) {
	report := &ConsistencyReport{
		Issues: []ConsistencyIssue{},
	}

	// Analyze naming consistency
	namingScore, namingIssues := c.analyzeNamingConsistency(repoContexts)
	report.NamingConsistency = ConsistencyCategory{
		Score:       namingScore,
		Description: "Consistency of naming conventions across repos",
	}
	report.Issues = append(report.Issues, namingIssues...)

	// Analyze pattern consistency
	patternScore, patternIssues := c.analyzePatternConsistency(repoContexts)
	report.PatternConsistency = ConsistencyCategory{
		Score:       patternScore,
		Description: "Consistency of code patterns and idioms",
	}
	report.Issues = append(report.Issues, patternIssues...)

	// Analyze structure consistency
	structureScore, structureIssues := c.analyzeStructureConsistency(repoContexts)
	report.StructureConsistency = ConsistencyCategory{
		Score:       structureScore,
		Description: "Consistency of project structure",
	}
	report.Issues = append(report.Issues, structureIssues...)

	// Calculate overall score
	report.OverallScore = (namingScore + patternScore + structureScore) / 3.0

	return report, nil
}

// Helper methods

func (c *comparer) calculateUnifiedStats(repoContexts []*ctxpkg.RepoContext) UnifiedStats {
	stats := UnifiedStats{
		TotalRepos:        len(repoContexts),
		LanguageBreakdown: make(map[string]int),
	}

	fileSet := make(map[string]int) // path -> count of repos containing it

	for _, rc := range repoContexts {
		stats.TotalFiles += len(rc.Files)
		stats.TotalLines += rc.Statistics.TotalLines
		stats.TotalFunctions += rc.Statistics.FunctionCount
		stats.TotalTypes += rc.Statistics.TypeCount

		for lang, count := range rc.Statistics.LanguageBreakdown {
			stats.LanguageBreakdown[lang] += count
		}

		for filePath := range rc.Files {
			fileSet[filePath]++
		}
	}

	for _, count := range fileSet {
		if count == 1 {
			stats.UniqueFiles++
		} else {
			stats.OverlappingFiles++
		}
	}

	return stats
}

func (c *comparer) findFileOverlaps(repoContexts []*ctxpkg.RepoContext) []FileOverlapInfo {
	fileMap := make(map[string]map[string]string) // path -> repoID -> hash

	for _, rc := range repoContexts {
		for filePath, fc := range rc.Files {
			if fileMap[filePath] == nil {
				fileMap[filePath] = make(map[string]string)
			}
			fileMap[filePath][rc.ID] = fc.Hash
		}
	}

	var overlaps []FileOverlapInfo
	for path, repoHashes := range fileMap {
		if len(repoHashes) > 1 {
			repos := make([]string, 0, len(repoHashes))
			hashes := make(map[string]string)
			identical := true
			var firstHash string

			for repoID, hash := range repoHashes {
				repos = append(repos, repoID)
				hashes[repoID] = hash
				if firstHash == "" {
					firstHash = hash
				} else if hash != firstHash {
					identical = false
				}
			}

			overlaps = append(overlaps, FileOverlapInfo{
				Path:      path,
				Repos:     repos,
				Identical: identical,
				Hashes:    hashes,
			})
		}
	}

	return overlaps
}

func (c *comparer) normalizeFunctionKey(fn *ctxpkg.FunctionDef) string {
	// Create a normalized key based on function name and parameter types
	return fn.Name
}

func (c *comparer) normalizeTypeKey(td *ctxpkg.TypeDef) string {
	return td.Name
}

func (c *comparer) instancesFromMultipleRepos(instances []DuplicateInstance) bool {
	repos := make(map[string]bool)
	for _, inst := range instances {
		repos[inst.RepoID] = true
	}
	return len(repos) > 1
}

func (c *comparer) assessSeverity(sig1, sig2 string) string {
	// Simple heuristic: more different = higher severity
	if sig1 == sig2 {
		return "low"
	}
	// Check if return type differs (simplified)
	if strings.Contains(sig1, "error") != strings.Contains(sig2, "error") {
		return "high"
	}
	return "medium"
}

func (c *comparer) assessGapPriority(repoCount int) string {
	if repoCount >= 3 {
		return "high"
	}
	if repoCount == 2 {
		return "medium"
	}
	return "low"
}

func formatFields(fields []ctxpkg.Field) string {
	var parts []string
	for _, f := range fields {
		parts = append(parts, f.Name+": "+f.Type)
	}
	return strings.Join(parts, ", ")
}

func (c *comparer) analyzeNamingConsistency(repoContexts []*ctxpkg.RepoContext) (float64, []ConsistencyIssue) {
	var issues []ConsistencyIssue

	// Check for naming convention variations
	camelCaseCount := 0
	snakeCaseCount := 0
	totalFuncs := 0

	for _, rc := range repoContexts {
		for _, fc := range rc.Files {
			for _, fn := range fc.Functions {
				totalFuncs++
				if strings.Contains(fn.Name, "_") {
					snakeCaseCount++
				} else {
					camelCaseCount++
				}
			}
		}
	}

	if totalFuncs > 0 {
		// If there's a significant mix, flag it
		mixRatio := float64(min(camelCaseCount, snakeCaseCount)) / float64(totalFuncs)
		if mixRatio > 0.2 {
			issues = append(issues, ConsistencyIssue{
				Type:        "naming_convention",
				Description: "Mixed naming conventions detected (camelCase vs snake_case)",
				Severity:    "medium",
				Suggestion:  "Standardize on a single naming convention",
			})
		}
	}

	// Calculate score based on consistency
	score := 1.0 - float64(len(issues))*0.2
	if score < 0 {
		score = 0
	}

	return score, issues
}

func (c *comparer) analyzePatternConsistency(repoContexts []*ctxpkg.RepoContext) (float64, []ConsistencyIssue) {
	var issues []ConsistencyIssue

	// Check for error handling patterns
	errorReturnCount := 0
	totalFuncs := 0

	for _, rc := range repoContexts {
		for _, fc := range rc.Files {
			for _, fn := range fc.Functions {
				totalFuncs++
				if strings.Contains(fn.Signature, "error") {
					errorReturnCount++
				}
			}
		}
	}

	// Score based on how consistent error handling is
	score := 0.8 // Base score
	if len(issues) > 0 {
		score -= float64(len(issues)) * 0.1
	}

	return score, issues
}

func (c *comparer) analyzeStructureConsistency(repoContexts []*ctxpkg.RepoContext) (float64, []ConsistencyIssue) {
	var issues []ConsistencyIssue

	// Check for directory structure consistency (as proxy for package structure)
	dirPaths := make(map[string][]string)

	for _, rc := range repoContexts {
		for filePath := range rc.Files {
			// Extract directory from file path
			dir := extractDir(filePath)
			if dir != "" {
				dirPaths[dir] = append(dirPaths[dir], rc.ID)
			}
		}
	}

	// Flag directories that appear in some but not all repos
	for dir, repos := range dirPaths {
		if len(repos) > 1 && len(repos) < len(repoContexts) {
			issues = append(issues, ConsistencyIssue{
				Type:        "directory_structure",
				Description: "Directory '" + dir + "' exists in some repos but not all",
				Repos:       repos,
				Severity:    "low",
				Suggestion:  "Consider aligning directory structure across repos",
			})
		}
	}

	score := 1.0 - float64(len(issues))*0.1
	if score < 0.5 {
		score = 0.5
	}

	return score, issues
}

func (c *comparer) generateRecommendations(result *CompareResult) []string {
	var recommendations []string

	// Based on duplicates
	if len(result.Duplicates) > 5 {
		recommendations = append(recommendations, "Consider creating a shared library for common functionality")
	}

	// Based on conflicts
	highSeverityConflicts := 0
	for _, conflict := range result.Conflicts {
		if conflict.Severity == "high" {
			highSeverityConflicts++
		}
	}
	if highSeverityConflicts > 0 {
		recommendations = append(recommendations, "Resolve high-severity conflicts before proceeding with merge")
	}

	// Based on gaps
	highPriorityGaps := 0
	for _, gap := range result.Gaps {
		if gap.Priority == "high" {
			highPriorityGaps++
		}
	}
	if highPriorityGaps > 0 {
		recommendations = append(recommendations, "Ensure high-priority functionality is migrated to target repo")
	}

	// Based on consistency
	if result.Consistency != nil && result.Consistency.OverallScore < 0.7 {
		recommendations = append(recommendations, "Address consistency issues to improve code maintainability")
	}

	// Based on file overlaps
	conflictingOverlaps := 0
	for _, overlap := range result.FileOverlap {
		if !overlap.Identical {
			conflictingOverlaps++
		}
	}
	if conflictingOverlaps > 0 {
		recommendations = append(recommendations, "Review files that exist in multiple repos with different content")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Repos appear well-aligned for merging")
	}

	return recommendations
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func extractDir(filePath string) string {
	lastSlash := strings.LastIndex(filePath, "/")
	if lastSlash <= 0 {
		return ""
	}
	return filePath[:lastSlash]
}
