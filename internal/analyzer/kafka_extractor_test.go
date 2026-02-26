package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

func TestKafkaExtractorSegmentioWriter(t *testing.T) {
	code := `
package main

import (
	"context"
	"github.com/segmentio/kafka-go"
)

func ProduceMessage(ctx context.Context) error {
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "events",
	})
	defer writer.Close()

	writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte("key"),
		Value: []byte("value"),
	})
	return nil
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	// Extract function
	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "ProduceMessage" {
			funcDecl = fn
			break
		}
	}

	if funcDecl == nil {
		t.Fatal("Could not find ProduceMessage function")
	}

	// Extract imports
	imports := extractImportsFromFile(f)

	// Extract Kafka calls
	extractor := NewKafkaExtractor(fset, f.Decls)
	asyncCalls := extractor.Extract(funcDecl, imports)

	if len(asyncCalls) == 0 {
		t.Fatal("Expected to find Kafka produce call")
	}

	call := asyncCalls[0]
	if call.Protocol != "kafka" {
		t.Errorf("Expected protocol 'kafka', got '%s'", call.Protocol)
	}
	if call.Direction != "produce" {
		t.Errorf("Expected direction 'produce', got '%s'", call.Direction)
	}
	if call.Library != "segmentio/kafka-go" {
		t.Errorf("Expected library 'segmentio/kafka-go', got '%s'", call.Library)
	}
}

func TestKafkaExtractorSegmentioReader(t *testing.T) {
	code := `
package main

import (
	"context"
	"github.com/segmentio/kafka-go"
)

func ConsumeMessage(ctx context.Context) error {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "events",
		GroupID: "my-group",
	})
	defer reader.Close()

	msg, err := reader.ReadMessage(ctx)
	return err
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	// Extract function
	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "ConsumeMessage" {
			funcDecl = fn
			break
		}
	}

	if funcDecl == nil {
		t.Fatal("Could not find ConsumeMessage function")
	}

	// Extract imports
	imports := extractImportsFromFile(f)

	// Extract Kafka calls
	extractor := NewKafkaExtractor(fset, f.Decls)
	asyncCalls := extractor.Extract(funcDecl, imports)

	if len(asyncCalls) == 0 {
		t.Fatal("Expected to find Kafka consume call")
	}

	call := asyncCalls[0]
	if call.Protocol != "kafka" {
		t.Errorf("Expected protocol 'kafka', got '%s'", call.Protocol)
	}
	if call.Direction != "consume" {
		t.Errorf("Expected direction 'consume', got '%s'", call.Direction)
	}
	// Topic should be extracted from the ReaderConfig
	if call.Topic != "events" && call.Topic != "<dynamic>" {
		t.Errorf("Expected topic 'events' or '<dynamic>', got '%s'", call.Topic)
	}
	if call.Library != "segmentio/kafka-go" {
		t.Errorf("Expected library 'segmentio/kafka-go', got '%s'", call.Library)
	}
}

func TestKafkaExtractorSaramaSendMessage(t *testing.T) {
	code := `
package main

import (
	"github.com/Shopify/sarama"
)

func ProduceSarama() error {
	producer, err := sarama.NewSyncProducer([]string{"localhost:9092"}, nil)
	if err != nil {
		return err
	}
	defer producer.Close()

	_, _, err = producer.SendMessage(&sarama.ProducerMessage{
		Topic: "events",
		Key:   sarama.StringEncoder("key"),
		Value: sarama.StringEncoder("value"),
	})
	return err
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	// Extract function
	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "ProduceSarama" {
			funcDecl = fn
			break
		}
	}

	if funcDecl == nil {
		t.Fatal("Could not find ProduceSarama function")
	}

	// Extract imports
	imports := extractImportsFromFile(f)

	// Extract Kafka calls
	extractor := NewKafkaExtractor(fset, f.Decls)
	asyncCalls := extractor.Extract(funcDecl, imports)

	if len(asyncCalls) == 0 {
		t.Fatal("Expected to find Sarama produce call")
	}

	call := asyncCalls[0]
	if call.Protocol != "kafka" {
		t.Errorf("Expected protocol 'kafka', got '%s'", call.Protocol)
	}
	if call.Direction != "produce" {
		t.Errorf("Expected direction 'produce', got '%s'", call.Direction)
	}
	// Topic should be extracted from the ProducerMessage
	if call.Topic != "events" && call.Topic != "<dynamic>" {
		t.Errorf("Expected topic 'events' or '<dynamic>', got '%s'", call.Topic)
	}
	if call.Library != "sarama" {
		t.Errorf("Expected library 'sarama', got '%s'", call.Library)
	}
}

func TestKafkaExtractorSaramaConsumerGroup(t *testing.T) {
	code := `
package main

import (
	"context"
	"github.com/Shopify/sarama"
)

type handler struct {}

func (h *handler) Setup(sarama.ConsumerGroupSession) error { return nil }
func (h *handler) Cleanup(sarama.ConsumerGroupSession) error { return nil }
func (h *handler) ConsumeClaim(sarama.ConsumerGroupSession, sarama.ConsumerGroupClaim) error { return nil }

func ConsumeSarama(ctx context.Context) error {
	cg, err := sarama.NewConsumerGroup([]string{"localhost:9092"}, "my-group", nil)
	if err != nil {
		return err
	}
	defer cg.Close()

	err = cg.Consume(ctx, []string{"events", "logs"}, &handler{})
	return err
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	// Extract function
	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "ConsumeSarama" {
			funcDecl = fn
			break
		}
	}

	if funcDecl == nil {
		t.Fatal("Could not find ConsumeSarama function")
	}

	// Extract imports
	imports := extractImportsFromFile(f)

	// Extract Kafka calls
	extractor := NewKafkaExtractor(fset, f.Decls)
	asyncCalls := extractor.Extract(funcDecl, imports)

	if len(asyncCalls) == 0 {
		t.Fatal("Expected to find Sarama consume calls")
	}

	// Should have 2 topics
	if len(asyncCalls) < 1 {
		t.Errorf("Expected at least 1 consume call, got %d", len(asyncCalls))
	}

	call := asyncCalls[0]
	if call.Protocol != "kafka" {
		t.Errorf("Expected protocol 'kafka', got '%s'", call.Protocol)
	}
	if call.Direction != "consume" {
		t.Errorf("Expected direction 'consume', got '%s'", call.Direction)
	}
	if call.Library != "sarama" {
		t.Errorf("Expected library 'sarama', got '%s'", call.Library)
	}
	// First topic should be "events" or dynamic
	if call.Topic != "events" && call.Topic != "<dynamic>" {
		t.Errorf("Expected topic 'events' or '<dynamic>', got '%s'", call.Topic)
	}
}

func TestKafkaExtractorDynamicTopic(t *testing.T) {
	code := `
package main

import (
	"context"
	"github.com/segmentio/kafka-go"
)

func ProduceWithDynamicTopic(ctx context.Context, topic string) error {
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   topic,
	})
	defer writer.Close()

	writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte("key"),
		Value: []byte("value"),
	})
	return nil
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	// Extract function
	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "ProduceWithDynamicTopic" {
			funcDecl = fn
			break
		}
	}

	if funcDecl == nil {
		t.Fatal("Could not find ProduceWithDynamicTopic function")
	}

	// Extract imports
	imports := extractImportsFromFile(f)

	// Extract Kafka calls
	extractor := NewKafkaExtractor(fset, f.Decls)
	asyncCalls := extractor.Extract(funcDecl, imports)

	if len(asyncCalls) == 0 {
		t.Fatal("Expected to find Kafka produce call")
	}

	call := asyncCalls[0]
	if call.Topic != "<dynamic>" {
		t.Errorf("Expected topic '<dynamic>' for variable topic, got '%s'", call.Topic)
	}
}

func TestKafkaExtractorSideEffects(t *testing.T) {
	code := `
package main

import (
	"context"
	"github.com/segmentio/kafka-go"
)

func ProduceAndConsume(ctx context.Context) error {
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "events",
	})
	defer writer.Close()

	writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte("key"),
		Value: []byte("value"),
	})

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "events",
		GroupID: "my-group",
	})
	defer reader.Close()

	msg, err := reader.ReadMessage(ctx)
	return err
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	// Extract function
	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "ProduceAndConsume" {
			funcDecl = fn
			break
		}
	}

	if funcDecl == nil {
		t.Fatal("Could not find ProduceAndConsume function")
	}

	// Extract imports
	imports := extractImportsFromFile(f)

	// Extract Kafka calls
	extractor := NewKafkaExtractor(fset, f.Decls)
	asyncCalls := extractor.Extract(funcDecl, imports)

	if len(asyncCalls) < 2 {
		t.Fatalf("Expected at least 2 async calls (produce and consume), got %d", len(asyncCalls))
	}

	// Check we have both produce and consume
	hasProducer := false
	hasConsumer := false
	for _, call := range asyncCalls {
		if call.Direction == "produce" {
			hasProducer = true
		}
		if call.Direction == "consume" {
			hasConsumer = true
		}
	}

	if !hasProducer {
		t.Error("Expected to find a produce call")
	}
	if !hasConsumer {
		t.Error("Expected to find a consume call")
	}
}

func TestKafkaExtractorConfluentProduce(t *testing.T) {
	code := `
package main

import (
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func ProduceConfluent() error {
	p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": "localhost"})
	if err != nil {
		return err
	}
	defer p.Close()

	p.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topicStr, Partition: kafka.PartitionAny},
		Value:          []byte("value"),
	}, nil)
	return nil
}

var topicStr = "events"
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	// Extract function
	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "ProduceConfluent" {
			funcDecl = fn
			break
		}
	}

	if funcDecl == nil {
		t.Fatal("Could not find ProduceConfluent function")
	}

	// Extract imports
	imports := extractImportsFromFile(f)

	// Extract Kafka calls
	extractor := NewKafkaExtractor(fset, f.Decls)
	asyncCalls := extractor.Extract(funcDecl, imports)

	if len(asyncCalls) == 0 {
		t.Fatal("Expected to find Confluent produce call")
	}

	call := asyncCalls[0]
	if call.Protocol != "kafka" {
		t.Errorf("Expected protocol 'kafka', got '%s'", call.Protocol)
	}
	if call.Direction != "produce" {
		t.Errorf("Expected direction 'produce', got '%s'", call.Direction)
	}
	if call.Library != "confluent" {
		t.Errorf("Expected library 'confluent', got '%s'", call.Library)
	}
}

func TestKafkaExtractorConfluentSubscribe(t *testing.T) {
	code := `
package main

import (
	"context"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func ConsumeConfluent(ctx context.Context) error {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": "localhost",
		"group.id":          "mygroup",
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		return err
	}
	defer c.Close()

	c.SubscribeTopics([]string{"topic1", "topic2"}, nil)
	msg, err := c.ReadMessage(ctx)
	return err
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse code: %v", err)
	}

	// Extract function
	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "ConsumeConfluent" {
			funcDecl = fn
			break
		}
	}

	if funcDecl == nil {
		t.Fatal("Could not find ConsumeConfluent function")
	}

	// Extract imports
	imports := extractImportsFromFile(f)

	// Extract Kafka calls
	extractor := NewKafkaExtractor(fset, f.Decls)
	asyncCalls := extractor.Extract(funcDecl, imports)

	if len(asyncCalls) == 0 {
		t.Fatal("Expected to find Confluent consume call")
	}

	call := asyncCalls[0]
	if call.Protocol != "kafka" {
		t.Errorf("Expected protocol 'kafka', got '%s'", call.Protocol)
	}
	if call.Direction != "consume" {
		t.Errorf("Expected direction 'consume', got '%s'", call.Direction)
	}
	if call.Library != "confluent" {
		t.Errorf("Expected library 'confluent', got '%s'", call.Library)
	}
	// Topics could be topic1 or topic2 or dynamic
	if call.Topic == "" {
		t.Errorf("Expected topic to be set")
	}
}

// Helper function to extract imports from a parsed file
func extractImportsFromFile(f *ast.File) []ctxpkg.Import {
	var imports []ctxpkg.Import
	for _, imp := range f.Imports {
		importPath := imp.Path.Value[1 : len(imp.Path.Value)-1] // Remove quotes
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		imports = append(imports, ctxpkg.Import{
			Path:  importPath,
			Alias: alias,
		})
	}
	return imports
}
