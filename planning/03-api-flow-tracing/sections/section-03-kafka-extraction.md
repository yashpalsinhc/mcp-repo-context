Now I have all the context I need. Let me generate the section content for `section-03-kafka-extraction`.

# Section 03: Kafka Producer/Consumer Detection

## Overview

This section introduces a new `KafkaExtractor` that performs AST-level static analysis to detect Kafka producer and consumer patterns in Go source files. It covers three major Kafka client libraries: `segmentio/kafka-go`, `Sarama`, and `confluent-kafka-go`. The extractor produces a new `AsyncCall` type that is integrated into the existing analyzer pipeline and stored on `FunctionDef`.

This section is **parallelizable** — it has no dependencies on sections 01, 02, or 08. It is itself a dependency for section 04 (endpoint storage).

## Background

The existing analyzer detects Kafka calls only as the generic `"kafka_call"` string in side effects. No topic name, direction (produce vs consume), or library information is captured. This section replaces that with structured extraction.

The analyzer pipeline in `internal/analyzer/` calls five sub-extractors per function inside `AnalyzeFile()`. This section adds a sixth: `KafkaExtractor.Extract()`. The new extractor follows the same pattern as the existing side effect and API flow extractors.

## Files to Create / Modify

- **Create:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/kafka_extractor.go`
- **Create:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/kafka_extractor_test.go`
- **Modify:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/types.go` — add `AsyncCall` and `AsyncCalls []AsyncCall` to `FunctionDef`
- **Modify:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/analyzer.go` (or whichever file hosts `AnalyzeFile`) — call `KafkaExtractor.Extract()` and attach results

## Tests First

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/kafka_extractor_test.go`

Tests use the Go standard `testing` package and real in-process AST parsing (no mocking required). The pattern should match existing tests in `internal/analyzer/`.

```go
// Test: detect segmentio/kafka-go writer with topic in struct literal
// Setup: writer := &kafka.Writer{Topic: "user.events"}; writer.WriteMessages(ctx, msg)
// Assert: AsyncCall{Protocol:"kafka", Direction:"produce", Topic:"user.events", Library:"segmentio/kafka-go"}

// Test: detect segmentio/kafka-go reader with topic
// Setup: reader := kafka.NewReader(kafka.ReaderConfig{Topic: "user.events"})
// Assert: AsyncCall{Direction:"consume", Topic:"user.events", Library:"segmentio/kafka-go"}

// Test: detect Sarama producer (SendMessage)
// Setup: producer.SendMessage(&sarama.ProducerMessage{Topic: "orders"})
// Assert: AsyncCall{Direction:"produce", Topic:"orders", Library:"sarama"}

// Test: detect Sarama consumer group (Consume with string slice)
// Setup: consumerGroup.Consume(ctx, []string{"orders", "payments"}, handler)
// Assert: Two AsyncCalls, one per topic, both Direction="consume", Library="sarama"

// Test: detect confluent-kafka-go producer with variable topic
// Setup: producer.Produce(&kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topicVar}}, nil)
// Assert: AsyncCall{Direction:"produce", Topic:"<dynamic>", TopicExpr contains "topicVar"}

// Test: dynamic topic from function call marked correctly
// Setup: writer := &kafka.Writer{Topic: getTopicName()}
// Assert: Topic == "<dynamic>", TopicExpr contains "getTopicName()"

// Test: kafka_produce and kafka_consume added to side effects (replacing kafka_call)
// Setup: function containing a segmentio/kafka-go WriteMessages call
// Assert: SideEffects slice contains "kafka_produce", does NOT contain "kafka_call"

// Test: kafka consumer side effect is kafka_consume
// Setup: function containing a kafka.NewReader + ReadMessage call
// Assert: SideEffects contains "kafka_consume"

// Test: AsyncCalls field populated on FunctionDef after analysis
// Setup: source file with one producer and one consumer call
// Assert: FunctionDef.AsyncCalls has len == 2, one produce, one consume
```

## New Type: AsyncCall

Add to `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/types.go`:

```go
// AsyncCall represents a Kafka (or future async messaging) interaction found
// during static analysis of a single function.
type AsyncCall struct {
    Protocol    string // Always "kafka" in this section
    Direction   string // "produce" or "consume"
    Topic       string // topic name literal, or "<dynamic>" if not statically determinable
    TopicExpr   string // original AST expression text when Topic == "<dynamic>"
    Library     string // "segmentio/kafka-go", "sarama", or "confluent"
    MessageType string // Go type name of the message payload if determinable; empty otherwise
    Line        int
    File        string
    Function    string // name of the function containing this call
}
```

Also extend `FunctionDef` (already in `types.go`) with:

```go
AsyncCalls []AsyncCall
```

## Implementation: KafkaExtractor

**File:** `/Users/yashpalc/yashpalc-mcp/mcp-repo-context/internal/analyzer/kafka_extractor.go`

### Struct Definition

```go
// KafkaExtractor detects Kafka producer and consumer patterns during AST analysis.
// It supports segmentio/kafka-go, Sarama, and confluent-kafka-go.
type KafkaExtractor struct{}

// Extract walks the given function declaration and its file-level imports,
// returning all detected AsyncCall entries.
func (e *KafkaExtractor) Extract(decl *ast.FuncDecl, imports []*ast.ImportSpec, fset *token.FileSet, fileName string) []AsyncCall
```

### Detection Logic

The extractor walks function bodies looking for `*ast.CallExpr` nodes. For each call expression it checks:

**segmentio/kafka-go producers** — `writer.WriteMessages(...)`:
- Match calls where the function selector is `WriteMessages`
- The topic is NOT in the call itself but in the `kafka.Writer{Topic: "..."}` struct literal assigned to the writer variable
- Walk the enclosing function body for `kafka.Writer{...}` composite literals and extract the `Topic` field value
- Also handle `kafka.NewWriter(kafka.WriterConfig{Topic: "..."})` if present

**segmentio/kafka-go consumers** — `kafka.NewReader(kafka.ReaderConfig{Topic: "..."})`:
- Match `NewReader` calls with a `ReaderConfig` composite literal argument
- Walk composite literal keys for the `Topic` field

**Sarama producers** — `producer.SendMessage(&sarama.ProducerMessage{Topic: "..."})`:
- Match `SendMessage` calls
- First argument is expected to be an address-of composite literal `&sarama.ProducerMessage{...}`
- Walk composite literal keys for `Topic`

**Sarama consumer groups** — `consumerGroup.Consume(ctx, []string{"topic1", "topic2"}, handler)`:
- Match `Consume` calls with three arguments
- Second argument is `[]string{...}` composite literal
- Each string element is a separate `AsyncCall` with `Direction="consume"`

**confluent-kafka-go producers** — `producer.Produce(&kafka.Message{TopicPartition: ...}, nil)`:
- Match `Produce` calls
- Walk the `TopicPartition` struct for a `Topic` field (which is a `*string` pointer)
- If topic is a variable/expression, mark as `"<dynamic>"` with `TopicExpr` set

**confluent-kafka-go consumers** — `consumer.SubscribeTopics([]string{"topic"}, nil)`:
- Match `SubscribeTopics` calls
- First argument is `[]string{...}` — same extraction as Sarama consumer

### Topic Extraction Helper

```go
// extractTopicFromCompositeLit attempts to extract a static topic name from a
// composite literal's key-value pairs, looking for a key named "Topic".
// Returns the topic string and a boolean indicating whether it was resolved.
// If the value is not a string literal, returns "<dynamic>" and false.
func extractTopicFromCompositeLit(lit *ast.CompositeLit) (topic string, topicExpr string, resolved bool)
```

Use the same constant-resolution approach as section 01's `extractExternalCall()` for file-scoped constants: when the `Topic` field value is an `*ast.Ident`, look up the identifier in the file's `*ast.GenDecl` constant blocks.

### Library Detection via Imports

```go
// detectKafkaLibraries inspects the import specs to determine which Kafka
// libraries are present in this file. Returns a set of library identifiers.
func detectKafkaLibraries(imports []*ast.ImportSpec) map[string]bool
```

Library keys:
- `"segmentio/kafka-go"` — import path `"github.com/segmentio/kafka-go"`
- `"sarama"` — import path `"github.com/Shopify/sarama"` or `"github.com/IBM/sarama"`
- `"confluent"` — import path `"github.com/confluentinc/confluent-kafka-go/kafka"`

If none of these are imported, `Extract()` can return immediately with an empty slice.

### Side Effects Integration

After `KafkaExtractor.Extract()` runs, the caller in `AnalyzeFile()` must update the `SideEffects` slice:
- Each `AsyncCall` with `Direction="produce"` adds `"kafka_produce"` to side effects
- Each `AsyncCall` with `Direction="consume"` adds `"kafka_consume"` to side effects
- The old generic `"kafka_call"` should NOT be added if the new extractor produces results

Deduplication is required: if the same side effect string would be added twice, add it only once.

### Integration in AnalyzeFile

In the file that calls the sub-extractors (likely `internal/analyzer/analyzer.go` or `internal/analyzer/goAnalyzer.go`), add:

```go
kafkaExtractor := &KafkaExtractor{}
asyncCalls := kafkaExtractor.Extract(decl, fileImports, fset, filePath)
funcDef.AsyncCalls = asyncCalls
// Update side effects
for _, ac := range asyncCalls {
    sideEffect := "kafka_" + ac.Direction // "kafka_produce" or "kafka_consume"
    // add to funcDef.SideEffects if not already present
}
```

## Dynamic Topic Handling

When a topic value cannot be resolved to a string literal:

1. Set `AsyncCall.Topic = "<dynamic>"`
2. Set `AsyncCall.TopicExpr` to the text representation of the AST expression (use `go/printer` or `go/format` to render it, or fall back to the identifier name)
3. Do NOT skip or ignore the call — it still produces a side effect entry

This mirrors the `"<dynamic>"` convention used for HTTP URLs in section 01.

## Scope and Known Limitations

- **File-scoped constants only:** Topic constants defined in other files (e.g., `topics.go`) will not be resolved. Per-package constant indexing is a follow-up outside this section.
- **Sarama ConsumerGroupHandler:** When the topic list is passed in the caller and the handler implements a separate interface, the topics are only captured if the `Consume(...)` call is in the analyzed file.
- **confluent Topic pointer:** The `*string` pointer to topic in confluent is common in dynamic patterns; these will produce `"<dynamic>"` entries.
- **Writer reuse across functions:** If a `kafka.Writer` is initialized in one function and used in another, the `WriteMessages` call in the second function may not have a matching struct literal in scope. This is a known limitation — mark as `"<dynamic>"` in that case.

## Dependencies This Section Provides

Once this section is merged:
- Section 04 (endpoint storage) can read `FunctionDef.AsyncCalls` to populate the `service_calls` SQLite table with `call_type="kafka_produce"` or `call_type="kafka_consume"` entries
- Section 05 (endpoint matching) depends on those table entries for Kafka topic matching
- The `"kafka_produce"` / `"kafka_consume"` side effect subtypes are more specific than the existing `"kafka_call"` and allow targeted side-effect queries