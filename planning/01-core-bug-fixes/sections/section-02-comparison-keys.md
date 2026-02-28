# Section 02: Receiver-Aware Comparison Keys

## Overview

This section fixes the core comparison key bug in `internal/comparison/comparer.go`. The functions `normalizeFunctionKey()` and `normalizeTypeKey()` currently return only the bare name (e.g., `fn.Name`, `td.Name`), which causes methods with common names like `ServeHTTP`, `Write`, or `Get` on different receiver types to collide. This produces false duplicates, false conflicts, and incorrect gaps across three tools: `compare_repos`, `find_duplicates`, and `find_conflicts`.

Additionally, `FindConflicts` and `FindGaps` bypass the normalize functions entirely, using raw `fn.Name` for map keys. Both must be refactored to call the normalize functions.

Finally, an idempotent lazy migration is needed to handle stored context data that was keyed without receivers.

**Depends on:** section-01-shared-infra (schema versioning concept using `Version` field on `RepoContext`)

---

## Current State of the Code

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go`

The normalize functions at lines 481-488 are trivial stubs:

```go
func (c *comparer) normalizeFunctionKey(fn *ctxpkg.FunctionDef) string {
	// Create a normalized key based on function name and parameter types
	return fn.Name
}

func (c *comparer) normalizeTypeKey(td *ctxpkg.TypeDef) string {
	return td.Name
}
```

`FindDuplicates` (line 128) correctly calls `c.normalizeFunctionKey(&fn)` and `c.normalizeTypeKey(&td)`, so fixing the normalize functions will fix duplicate detection automatically.

`FindConflicts` (line 195) does **not** use normalize functions. It builds its target map using `key := fn.Name` (line 208) and looks up source functions using `fn.Name` (line 218).

`FindGaps` (line 288) does **not** use normalize functions. It builds its target inventory using `targetFuncs[fn.Name] = true` (line 304) and checks source functions using `fn.Name` (line 317).

### File: `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`

`FunctionDef` (line 172) has a `Receiver` field:

```go
type FunctionDef struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Receiver  string `json:"receiver,omitempty"`
	// ... other fields
}
```

The `Receiver` field is populated during analysis (e.g., `"*Router"`, `"Router"`) but is currently ignored by comparison logic.

`TypeDef` (line 150) does **not** have a Package field. However, `FileContext` (line 106) has `Package string`, so the package name for a type must be obtained from the enclosing `FileContext`.

`RepoContext` (line 6) has a `Version int` field at line 15 that is serialized as `json:"version"`. Pre-fix data has `Version == 0`.

---

## Tests

Write these tests FIRST in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer_test.go`. All tests use the standard Go `testing` package with inline fixtures.

### normalizeFunctionKey tests

```go
// Test: normalizeFunctionKey with receiver "*Router" and name "ServeHTTP" returns "Router.ServeHTTP"
// Test: normalizeFunctionKey with receiver "Router" (no pointer) returns "Router.ServeHTTP"
// Test: normalizeFunctionKey with empty receiver returns "ServeHTTP" (package-level function)
// Test: normalizeFunctionKey strips only leading "*" from receiver
```

### normalizeTypeKey tests

```go
// Test: normalizeTypeKey with different packages produces different keys
// Test: normalizeTypeKey for "Handler" in package "mux" vs "Handler" in package "cors" are distinct
```

Note: Since `TypeDef` does not have a `Package` field, the normalize function signature will need to accept additional context (the package name from the enclosing `FileContext`). See the implementation approach below.

### FindDuplicates tests

```go
// Test: FindDuplicates does NOT flag Router.ServeHTTP and cors.ServeHTTP as duplicates
// Test: FindDuplicates DOES flag two Router.ServeHTTP from different repos as duplicates
// Test: FindDuplicates with package-level functions (no receiver) still works correctly
```

Build test fixtures using inline `RepoContext` structs. Create two repos, each containing a `ServeHTTP` method but on different receiver types (`*Router` vs `*cors`). Assert they are NOT grouped as duplicates. Then create two repos with the same receiver type and assert they ARE grouped.

### FindConflicts tests

```go
// Test: FindConflicts does NOT flag methods on different receiver types as conflicts
// Test: FindConflicts DOES flag methods on same receiver type with different signatures as conflicts
// Test: FindConflicts uses normalizeFunctionKey (not raw fn.Name)
```

Build fixtures with two repos. Repo A has `(*Router).ServeHTTP(w, r)` and Repo B has `(*cors).ServeHTTP(w, r, opts)`. These should NOT be conflicts (different receiver types). Then build a case where both have `(*Router).ServeHTTP` with different signatures -- that IS a conflict.

### FindGaps tests

```go
// Test: FindGaps does NOT flag Router.ServeHTTP as filling gap for cors.ServeHTTP
// Test: FindGaps uses normalizeFunctionKey for target function map (not raw fn.Name)
```

Build fixtures with a source repo containing `(*cors).ServeHTTP` and a target repo containing `(*Router).ServeHTTP`. Since the receiver-qualified keys differ, the target's `Router.ServeHTTP` should NOT satisfy the gap check for source's `cors.ServeHTTP`.

### Lazy migration tests

```go
// Test: Lazy migration triggers when Version == 0 and functions have receivers
// Test: Lazy migration sets Version to 1 after re-keying
// Test: Lazy migration is idempotent (running twice does not corrupt data)
// Test: Version 1 contexts skip migration entirely
```

Create a `RepoContext` with `Version: 0` containing functions with receivers. Run the migration function. Assert `Version` becomes `1`. Run it again and assert data is unchanged. Create a context with `Version: 1` and assert migration is skipped (no changes).

---

## Implementation Details

### 1. Fix `normalizeFunctionKey()` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go`

Change the function to produce receiver-qualified keys:

- If `fn.Receiver` is non-empty, strip the leading `*` (pointer prefix) and return `ReceiverType.FunctionName`.
- If `fn.Receiver` is empty (package-level function), return just `FunctionName`.

The key format is: `strings.TrimPrefix(fn.Receiver, "*") + "." + fn.Name` when receiver is present, otherwise `fn.Name`.

Example outputs:
- Receiver `"*Router"`, Name `"ServeHTTP"` produces `"Router.ServeHTTP"`
- Receiver `"Router"`, Name `"ServeHTTP"` produces `"Router.ServeHTTP"`
- Receiver `""`, Name `"NewRouter"` produces `"NewRouter"`

### 2. Fix `normalizeTypeKey()` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go`

Since `TypeDef` does not have a `Package` field, but `FileContext` does, the normalize function needs the package context. Two approaches:

**Option A (recommended):** Change the signature to accept the package name:

```go
func (c *comparer) normalizeTypeKey(td *ctxpkg.TypeDef, packageName string) string
```

This returns `packageName + "." + td.Name` when `packageName` is non-empty, otherwise just `td.Name`.

**Option B:** Pass the full `FileContext` and extract `fc.Package`.

Update all callers of `normalizeTypeKey` in `FindDuplicates` and `FindConflicts` to pass the package name from the enclosing `FileContext.Package` field.

### 3. Refactor `FindConflicts` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go`

Currently at lines 202-212, the target function map is built using `fn.Name`:

```go
key := fn.Name
targetFuncs[key] = fn
targetFuncFiles[key] = filePath
```

Change to:

```go
key := c.normalizeFunctionKey(fn)
targetFuncs[key] = fn
targetFuncFiles[key] = filePath
```

And at line 218, change the lookup:

```go
// Before:
if targetFn, exists := targetFuncs[fn.Name]; exists {
// After:
srcKey := c.normalizeFunctionKey(&fn)
if targetFn, exists := targetFuncs[srcKey]; exists {
```

Also update the type conflict section (lines 247-253) to use `normalizeTypeKey` with the package name instead of `td.Name`.

### 4. Refactor `FindGaps` in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go`

Currently at lines 303-304, the target function inventory is built using `fn.Name`:

```go
targetFuncs[fn.Name] = true
```

Change to:

```go
targetFuncs[c.normalizeFunctionKey(&fn)] = true
```

At line 317, change the gap check:

```go
// Before:
if !targetFuncs[fn.Name] {
// After:
fnKey := c.normalizeFunctionKey(&fn)
if !targetFuncs[fnKey] {
```

Also update `funcSources` and `funcFiles` maps to use the receiver-qualified key instead of `fn.Name`, and update the type gap check similarly with `normalizeTypeKey`.

### 5. Implement Lazy Migration

Add a migration function that re-keys function data in memory when `Version < 1`. This should be called at the entry point of the comparison tools before any key-based operations.

**Migration function location:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go` (or a new file `migration.go` in the same package).

The migration function should:

1. Accept a `*ctxpkg.RepoContext`.
2. Check `rc.Version`. If `>= 1`, return immediately (no-op).
3. The migration does not need to physically re-key stored data since the normalize functions now produce correct keys at query time. The migration simply bumps the version.
4. Set `rc.Version = 1`.
5. The version bump signals that this context has been "migrated" (even though the actual fix is in the normalize functions, the version prevents any future 0-to-1 migration logic from running).

**Where to call migration:** Add a helper method like `ensureMigrated(contexts []*ctxpkg.RepoContext)` that iterates all provided contexts and calls the migration function on each. Call this at the top of `FindDuplicates`, `FindConflicts`, and `FindGaps`.

**Idempotency:** The version check (`if rc.Version >= 1, return`) makes migration idempotent. Running it twice on the same context is a no-op. Under concurrent access, the worst case is two goroutines both bump from 0 to 1, which is harmless since the operation is setting the same value.

**Persistence:** If the storage layer supports saving updated contexts, the bumped version should be persisted so future loads skip migration. If the comparison layer does not have direct access to storage, the version bump happens in-memory only and is re-applied on next load. This is acceptable since the migration is cheap (just a version check and set).

### Files Summary

| File | Action | Changes |
|------|--------|---------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer.go` | Modify | Fix `normalizeFunctionKey`, `normalizeTypeKey`, refactor `FindConflicts` and `FindGaps` to use normalize functions, add migration helper |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/comparison/comparer_test.go` | Create/Modify | Add all tests listed above |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` | No change needed | `Version` field and `Receiver` field already exist |

---

## Acceptance Criteria

1. `normalizeFunctionKey` returns `"Router.ServeHTTP"` for both `*Router` and `Router` receivers with method `ServeHTTP`.
2. `normalizeFunctionKey` returns `"ServeHTTP"` for package-level functions with no receiver.
3. `normalizeTypeKey` distinguishes same-named types in different packages.
4. `FindDuplicates` no longer flags methods on different receiver types as duplicates.
5. `FindConflicts` no longer flags methods on different receiver types as conflicts.
6. `FindGaps` no longer treats `Router.ServeHTTP` as satisfying a gap for `cors.ServeHTTP`.
7. Lazy migration sets `Version` to 1 and is idempotent.
8. All tests pass with `go test ./internal/comparison/...`.