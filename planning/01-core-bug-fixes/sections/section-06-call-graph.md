# Section 06: Call Graph Callee Extraction

## Overview

This section fixes the call graph builder so that method calls are properly resolved and included in callee lists. Currently, method calls (`x.Method()`) are extracted with `Type == "method"` but are then excluded from both call resolution and `CalledBy` population. This results in incomplete call graphs -- for example, `ServeHTTP` in gorilla/mux shows 32 callers but only 2 callees because 13 internal method calls are invisible.

Additionally, `funcFile` and `makeNodeID` use plain function names as keys, causing collisions when multiple functions share the same name but have different receivers (e.g., `Router.ServeHTTP` vs `cors.ServeHTTP`).

This section has no dependencies on other sections and can be implemented in parallel.

## Background

### Current Data Model

The `CallRef` struct (defined in `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`, line 204) currently looks like:

```go
type CallRef struct {
    Function string `json:"function"`
    Package  string `json:"package,omitempty"`
    File     string `json:"file,omitempty"`
    Line     int    `json:"line,omitempty"`
    Type     string `json:"type,omitempty"` // internal, stdlib, external
}
```

When a method call like `obj.Method()` is extracted, `Package` is set to the variable name (e.g., `"r"`) and `Type` is set to `"method"`. This overloads the `Package` field and makes it impossible to distinguish the package from the receiver variable.

### Current Call Extraction

In `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/callgraph.go`:

- `extractCall()` (line 164) classifies `SelectorExpr` calls: if the left-hand identifier matches an import alias, it is classified by import type; otherwise it is classified as `"method"`.
- `resolveCall()` (line 263) only handles `call.Type == "internal"` -- method calls are never resolved to a node.
- `PopulateCalledBy()` (line 316) filters on `call.Type == "internal" || call.Type == "" || call.Package == ""` which excludes method calls (they have `Type == "method"` and `Package` set to the variable name).
- `funcFile` map (line 17) uses `fn.Name` as the key, so `Router.ServeHTTP` and `cors.ServeHTTP` collide.
- `makeNodeID()` (line 246) produces `"file:function"` without receiver info, also causing node map collisions.

### CallGraphNode

Defined in the same types file (line 30):

```go
type CallGraphNode struct {
    ID        string   `json:"id"`
    File      string   `json:"file"`
    Function  string   `json:"function"`
    Signature string   `json:"signature"`
    Package   string   `json:"package"`
    IsPublic  bool     `json:"is_public"`
    Calls     []string `json:"calls"`
    CalledBy  []string `json:"called_by"`
}
```

### FunctionDef

`FunctionDef` (line 172 in types.go) has a `Receiver` field (string, e.g., `"*Router"`) that is populated during analysis but currently ignored by the call graph builder.

---

## Tests First

All tests use the standard Go `testing` package with inline fixtures. No external test frameworks.

### `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/callgraph_test.go`

```go
package analyzer

import "testing"

// --- Heuristic receiver type inference ---

// Test: Method call x.Method() where x is a function parameter with known type resolves to Type.Method
func TestMethodCallResolution_FunctionParameter(t *testing.T) {
    // Build fixture with a function that takes a typed parameter and calls a method on it.
    // Verify the resolved callee ID includes the receiver type.
}

// Test: Method call x.Method() where x is declared as "var x SomeType" resolves to SomeType.Method
func TestMethodCallResolution_VarDeclaration(t *testing.T) {}

// Test: Method call x.Method() where x is composite literal "x := SomeType{}" resolves to SomeType.Method
func TestMethodCallResolution_CompositeLiteral(t *testing.T) {}

// Test: Method call x.Method() where x type is unknown is recorded as "unresolved method" (not dropped)
func TestMethodCallResolution_UnknownType(t *testing.T) {}

// --- CalledBy includes method calls ---

// Test: CalledBy includes method calls (Type == "method" no longer filtered out)
func TestPopulateCalledBy_IncludesMethodCalls(t *testing.T) {
    // Build fixture with a function that makes method calls on same-package types.
    // Verify those calls appear in the target function's CalledBy list.
}

// Test: ServeHTTP in test fixture shows both callers AND callees
func TestServeHTTP_CallersAndCallees(t *testing.T) {}

// --- makeNodeID with receiver ---

// Test: makeNodeID with receiver "Router" and function "ServeHTTP" returns "file:Router.ServeHTTP"
func TestMakeNodeID_WithReceiver(t *testing.T) {}

// Test: makeNodeID without receiver returns "file:ServeHTTP"
func TestMakeNodeID_WithoutReceiver(t *testing.T) {}

// Test: Two methods with same name, different receivers, same file produce different node IDs
func TestMakeNodeID_NoDuplicateIDs(t *testing.T) {}

// --- funcFile map collision fix ---

// Test: funcFile map with receiver-qualified keys has no collision for same-name methods
func TestFuncFileMap_NoCollision(t *testing.T) {}

// Test: funcFile["Router.ServeHTTP"] != funcFile["cors.ServeHTTP"]
func TestFuncFileMap_DifferentReceivers(t *testing.T) {}

// --- CallRef Receiver field ---

// Test: CallRef Receiver field populated for method calls
func TestCallRef_ReceiverPopulatedForMethods(t *testing.T) {}

// Test: CallRef Receiver field empty for package-level calls
func TestCallRef_ReceiverEmptyForPackageLevel(t *testing.T) {}

// Test: Old CallRef data (no Receiver field) deserializes with empty Receiver (omitempty)
func TestCallRef_BackwardsCompatibility(t *testing.T) {}
```

### `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/go_analyzer_test.go` (type-checker mode)

```go
package analyzer

import "testing"

// Test: Type-checker mode resolves method calls via go/types
func TestTypeCheckerMode_ResolvesMethodCalls(t *testing.T) {}

// Test: Type-checker mode falls back to heuristic when module resolution fails
func TestTypeCheckerMode_FallbackOnFailure(t *testing.T) {}

// Test: Type-checker mode produces warning log on fallback
func TestTypeCheckerMode_FallbackWarning(t *testing.T) {}

// Test: Flag --use-type-checker=false uses heuristic mode (default)
func TestTypeCheckerMode_DefaultOff(t *testing.T) {}
```

---

## Implementation Details

### Step 1: Add `Receiver` Field to `CallRef`

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go`

Add a `Receiver` field to `CallRef` with `omitempty` so old serialized data (without the field) deserializes cleanly:

```go
type CallRef struct {
    Function string `json:"function"`
    Package  string `json:"package,omitempty"`
    Receiver string `json:"receiver,omitempty"` // Receiver type for method calls (new field)
    File     string `json:"file,omitempty"`
    Line     int    `json:"line,omitempty"`
    Type     string `json:"type,omitempty"` // internal, stdlib, external, method
}
```

This is a backwards-compatible change. Old JSON without `receiver` will deserialize with an empty string.

### Step 2: Fix `makeNodeID` to Include Receiver

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/callgraph.go`

Update `makeNodeID` to accept a receiver parameter and produce `"file:ReceiverType.function"` when a receiver is present, or `"file:function"` for package-level functions. Strip the `*` prefix from receivers (so `*Router` and `Router` produce the same key).

Update all call sites of `makeNodeID` to pass the receiver from `FunctionDef.Receiver`.

### Step 3: Fix `funcFile` Map to Use Receiver-Qualified Keys

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/callgraph.go`

In `BuildFromFiles` (line 33), change the `funcFile` map key from `fn.Name` to a receiver-qualified key (same format as the function part of `makeNodeID`, e.g., `"Router.ServeHTTP"` or just `"ServeHTTP"` for package-level functions).

Create a helper function `qualifiedFuncName(receiver, name string) string` to produce this key consistently. This function should strip the `*` prefix from the receiver and return `Receiver.Name` if receiver is non-empty, or plain `Name` otherwise.

### Step 4: Add Heuristic Receiver Type Inference

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/callgraph.go`

Add a `localVarTypes` tracking mechanism to `ExtractFunctionCalls`. Before walking the AST for calls, do a pre-pass over the function body to build a `map[string]string` of variable name to type name. Handle these patterns:

1. **Function parameters:** Walk `fn.Type.Params` to extract `paramName -> typeName` from typed parameters.
2. **Var declarations:** `var x SomeType` -- look for `*ast.GenDecl` with `token.VAR` spec.
3. **Short declarations with composite literals:** `x := SomeType{}` -- look for `*ast.AssignStmt` where the RHS is `*ast.CompositeLit`.

**NOT handled (document this):** Return types from function calls (`x := SomeFunc()`), type assertions (`x.(Type)`), shadowed variables in nested scopes. These require `go/types` for correct resolution.

Then in `extractCall`, when the call is classified as `"method"`, look up the variable name in `localVarTypes`. If found, set `CallRef.Receiver` to the resolved type and change `CallRef.Package` back to empty (since it was incorrectly set to the variable name). If not found, still record the call but mark it distinctly (leave `Receiver` empty, keep `Type` as `"method"`) so it appears as an "unresolved method" rather than being silently dropped.

The `localVarTypes` map needs to be passed through the call chain. Add it as a parameter to `extractCall` or store it on the `CallGraphBuilder` struct temporarily during function analysis.

### Step 5: Fix `resolveCall` to Handle Method Calls

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/callgraph.go`

Expand `resolveCall` (line 263) to handle `call.Type == "method"` in addition to `"internal"`:

- When `call.Receiver` is non-empty, construct the qualified function name using `qualifiedFuncName(call.Receiver, call.Function)` and look it up in `funcFile`.
- When `call.Receiver` is empty (unresolved method), attempt a best-effort lookup by function name alone. If there is exactly one match in `funcFile`, resolve it. If multiple matches or zero matches, leave unresolved.

### Step 6: Fix `PopulateCalledBy` to Include Method Calls

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/callgraph.go`

In `PopulateCalledBy` (line 324), expand the filter condition to also include `call.Type == "method"`:

```go
if call.Type == "internal" || call.Type == "" || call.Package == "" || call.Type == "method" {
```

When recording the CalledBy reference for method calls with a known receiver, use `qualifiedFuncName(call.Receiver, call.Function)` as the lookup key into the calledBy map. Update the second pass (line 342) to also use receiver-qualified names (`qualifiedFuncName(fn.Receiver, fn.Name)`) for the function lookup.

### Step 7: Implement Opt-In `go/types` Mode

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/go_analyzer.go`

Add a `UseTypeChecker` field to the `goAnalyzer` struct (or introduce a config struct). When enabled (via `--use-type-checker` flag):

1. Use `golang.org/x/tools/go/packages` to load the package with full type information.
2. Use `types.Info.Uses` to resolve every `*ast.SelectorExpr` to its declaration, giving exact receiver types for all method calls.
3. For interface method calls, record the interface type as the receiver.
4. If module resolution fails (e.g., private dependencies, missing network), fall back to heuristic mode with a warning log message.

**New dependency:** `golang.org/x/tools/go/packages` -- Go ecosystem quasi-standard tooling, not a third-party library.

**File:** Server config or `cmd/` entry point -- add the `--use-type-checker` flag defaulting to `false`.

### Step 8: Update `BuildFromFiles` Node Registration

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/callgraph.go`

In `BuildFromFiles` (line 31), pass `fn.Receiver` to `makeNodeID` and use receiver-qualified keys for both `funcFile` and `fileFunc`:

```go
for _, fn := range fileCtx.Functions {
    nodeID := b.makeNodeID(path, fn.Receiver, fn.Name)
    // ... register node with nodeID ...
    qualName := qualifiedFuncName(fn.Receiver, fn.Name)
    b.funcFile[qualName] = path
    b.fileFunc[path] = append(b.fileFunc[path], qualName)
}
```

---

## Files to Modify

| File | Change |
|------|--------|
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/context/types.go` | Add `Receiver` field to `CallRef` |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/callgraph.go` | Fix `makeNodeID`, `funcFile`, `resolveCall`, `PopulateCalledBy`, add heuristic receiver inference, add `qualifiedFuncName` helper |
| `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/go_analyzer.go` | Add type-checker mode with `go/types` integration and fallback |
| Server config or `cmd/` entry point | Add `--use-type-checker` flag (default: false) |

## New Dependencies

- `golang.org/x/tools/go/packages` -- for loading typed packages in type-checker mode. This is Go ecosystem tooling, not a third-party library.

## Risk Notes

- **CallRef schema change:** The `Receiver` field uses `omitempty` JSON tag, so old data without the field deserializes cleanly with an empty string. No migration needed.
- **Node ID format change:** Only affects in-memory representation; call graphs are regenerated on next analysis. No stored data is broken.
- **go/types mode complexity:** It is opt-in (flag defaults to OFF). Heuristic mode works without it and covers the most common cases (typed parameters, var declarations, composite literals). The type-checker mode is a best-effort enhancement for codebases that need full resolution.
- **Unresolved method calls:** Instead of silently dropping them (current behavior), they are now recorded as unresolved. This is strictly more information than before.