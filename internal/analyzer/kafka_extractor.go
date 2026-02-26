package analyzer

import (
	"go/ast"
	"go/token"
	"strings"

	ctxpkg "github.com/yashpalc/mcp-repo-context/internal/context"
)

// KafkaExtractor detects Kafka producer/consumer patterns.
type KafkaExtractor struct {
	fset      *token.FileSet
	fileDecls []ast.Decl
}

// NewKafkaExtractor creates a new Kafka extractor.
func NewKafkaExtractor(fset *token.FileSet, fileDecls []ast.Decl) *KafkaExtractor {
	return &KafkaExtractor{
		fset:      fset,
		fileDecls: fileDecls,
	}
}

// Extract detects Kafka producer/consumer calls in a function.
func (ke *KafkaExtractor) Extract(fn *ast.FuncDecl, imports []ctxpkg.Import) []ctxpkg.AsyncCall {
	if fn.Body == nil {
		return nil
	}

	var asyncCalls []ctxpkg.AsyncCall

	// Build import map for library detection
	importMap := ke.buildImportMap(imports)

	// Inspect the function body for Kafka patterns
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Try to extract async call from this call expression
		asyncCall := ke.extractAsyncCall(call, importMap)
		if asyncCall != nil {
			asyncCalls = append(asyncCalls, *asyncCall)
		}

		return true
	})

	return asyncCalls
}

// extractAsyncCall tries to extract an async call from a call expression.
func (ke *KafkaExtractor) extractAsyncCall(call *ast.CallExpr, importMap map[string]string) *ctxpkg.AsyncCall {
	// Get method name and receiver
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	methodName := sel.Sel.Name
	pos := ke.fset.Position(call.Pos())

	// Check for segmentio/kafka-go patterns
	if asyncCall := ke.checkSegmentioKafka(call, sel, methodName, importMap, pos.Line); asyncCall != nil {
		return asyncCall
	}

	// Check for Sarama patterns
	if asyncCall := ke.checkSarama(call, sel, methodName, importMap, pos.Line); asyncCall != nil {
		return asyncCall
	}

	// Check for Confluent patterns
	if asyncCall := ke.checkConfluent(call, sel, methodName, importMap, pos.Line); asyncCall != nil {
		return asyncCall
	}

	return nil
}

// checkSegmentioKafka checks for segmentio/kafka-go patterns.
func (ke *KafkaExtractor) checkSegmentioKafka(call *ast.CallExpr, sel *ast.SelectorExpr, methodName string,
	importMap map[string]string, line int) *ctxpkg.AsyncCall {

	// Check for NewReader: kafka.NewReader(kafka.ReaderConfig{Topic: "..."})
	if methodName == "NewReader" && len(call.Args) > 0 {
		if ke.isQualifiedCallFromImport(sel.X, "kafka", "segmentio/kafka-go", importMap) {
			// Look for ReaderConfig argument
			if compLit, ok := call.Args[0].(*ast.CompositeLit); ok {
				topic := ke.extractTopicFromConfig(compLit)
				if topic == "" && ke.hasTopicField(compLit) {
					topic = "<dynamic>"
				}
				if topic != "" {
					return &ctxpkg.AsyncCall{
						Protocol:    "kafka",
						Direction:   "consume",
						Topic:       topic,
						Library:     "segmentio/kafka-go",
						Line:        line,
					}
				}
			}
		}
	}

	// Check for NewWriter: kafka.NewWriter(kafka.WriterConfig{Topic: "..."})
	if methodName == "NewWriter" && len(call.Args) > 0 {
		if ke.isQualifiedCallFromImport(sel.X, "kafka", "segmentio/kafka-go", importMap) {
			// Look for WriterConfig argument
			if compLit, ok := call.Args[0].(*ast.CompositeLit); ok {
				topic := ke.extractTopicFromConfig(compLit)
				if topic == "" && ke.hasTopicField(compLit) {
					topic = "<dynamic>"
				}
				if topic != "" {
					return &ctxpkg.AsyncCall{
						Protocol:    "kafka",
						Direction:   "produce",
						Topic:       topic,
						Library:     "segmentio/kafka-go",
						Line:        line,
					}
				}
			}
		}
	}

	// Check WriteMessages on a writer variable (also segmentio/kafka-go)
	if methodName == "WriteMessages" && len(call.Args) > 0 {
		// This is a writer.WriteMessages(...) call
		// For segmentio, we detect it's a kafka writer if the topic was set on NewWriter
		// But we also mark it here since it's clearly a produce operation
		if ke.looksLikeKafkaWriter(sel.X) {
			return &ctxpkg.AsyncCall{
				Protocol:    "kafka",
				Direction:   "produce",
				Topic:       "<dynamic>", // Topic is set on Writer config, not in WriteMessages
				Library:     "segmentio/kafka-go",
				Line:        line,
			}
		}
	}

	// Check ReadMessage on a reader variable
	if methodName == "ReadMessage" && len(call.Args) > 0 {
		if ke.looksLikeKafkaReader(sel.X) {
			return &ctxpkg.AsyncCall{
				Protocol:    "kafka",
				Direction:   "consume",
				Topic:       "<dynamic>", // Topic is set on Reader config, not in ReadMessage
				Library:     "segmentio/kafka-go",
				Line:        line,
			}
		}
	}

	return nil
}

// checkSarama checks for Sarama patterns.
func (ke *KafkaExtractor) checkSarama(call *ast.CallExpr, sel *ast.SelectorExpr, methodName string,
	importMap map[string]string, line int) *ctxpkg.AsyncCall {

	// Check SendMessage (produce): producer.SendMessage(...)
	if methodName == "SendMessage" && len(call.Args) > 0 {
		if ke.looksLikeSaramaProducer(sel.X) || ke.hasImportPath(importMap, "Shopify/sarama") || ke.hasImportPath(importMap, "IBM/sarama") {
			// Look for topic in ProducerMessage
			topic := ""
			if compLit, ok := call.Args[0].(*ast.CompositeLit); ok {
				topic = ke.extractTopicFromConfig(compLit)
			}
			if topic == "" {
				topic = "<dynamic>"
			}
			return &ctxpkg.AsyncCall{
				Protocol:    "kafka",
				Direction:   "produce",
				Topic:       topic,
				Library:     "sarama",
				Line:        line,
			}
		}
	}

	// Check Consume pattern: consumerGroup.Consume(ctx, topics, handler)
	if methodName == "Consume" && len(call.Args) >= 2 {
		if ke.looksLikeSaramaConsumer(sel.X) || ke.hasImportPath(importMap, "Shopify/sarama") || ke.hasImportPath(importMap, "IBM/sarama") {
			// Second argument should be []string of topics
			if arrayLit, ok := call.Args[1].(*ast.CompositeLit); ok {
				// Extract topics from array
				topics := ke.extractTopicsFromArray(arrayLit)
				if len(topics) > 0 {
					// Return first topic (we could return multiple calls for multiple topics)
					return &ctxpkg.AsyncCall{
						Protocol:    "kafka",
						Direction:   "consume",
						Topic:       topics[0],
						Library:     "sarama",
						Line:        line,
					}
				}
			}
			// If we can't extract topics, mark as dynamic
			return &ctxpkg.AsyncCall{
				Protocol:    "kafka",
				Direction:   "consume",
				Topic:       "<dynamic>",
				Library:     "sarama",
				Line:        line,
			}
		}
	}

	return nil
}

// checkConfluent checks for Confluent patterns.
func (ke *KafkaExtractor) checkConfluent(call *ast.CallExpr, sel *ast.SelectorExpr, methodName string,
	importMap map[string]string, line int) *ctxpkg.AsyncCall {

	// Check Produce (produce): producer.Produce(...)
	if methodName == "Produce" && len(call.Args) > 0 {
		if ke.looksLikeConfluentProducer(sel.X) || ke.hasImportPath(importMap, "confluentinc/confluent-kafka-go") {
			// Look for topic in Message
			topic := ""
			if compLit, ok := call.Args[0].(*ast.CompositeLit); ok {
				topic = ke.extractTopicFromConfig(compLit)
			}
			if topic == "" {
				topic = "<dynamic>"
			}
			return &ctxpkg.AsyncCall{
				Protocol:    "kafka",
				Direction:   "produce",
				Topic:       topic,
				Library:     "confluent",
				Line:        line,
			}
		}
	}

	// Check SubscribeTopics (consume): consumer.SubscribeTopics([]string{...}, handler)
	if methodName == "SubscribeTopics" && len(call.Args) > 0 {
		if ke.looksLikeConfluentConsumer(sel.X) || ke.hasImportPath(importMap, "confluentinc/confluent-kafka-go") {
			if arrayLit, ok := call.Args[0].(*ast.CompositeLit); ok {
				topics := ke.extractTopicsFromArray(arrayLit)
				if len(topics) > 0 {
					return &ctxpkg.AsyncCall{
						Protocol:    "kafka",
						Direction:   "consume",
						Topic:       topics[0],
						Library:     "confluent",
						Line:        line,
					}
				}
			}
			return &ctxpkg.AsyncCall{
				Protocol:    "kafka",
				Direction:   "consume",
				Topic:       "<dynamic>",
				Library:     "confluent",
				Line:        line,
			}
		}
	}

	return nil
}

// buildImportMap creates a map of import aliases/names to their paths.
func (ke *KafkaExtractor) buildImportMap(imports []ctxpkg.Import) map[string]string {
	importMap := make(map[string]string)
	for _, imp := range imports {
		alias := imp.Alias
		if alias == "" {
			// Use last component of path as alias
			parts := strings.Split(imp.Path, "/")
			alias = parts[len(parts)-1]
		}
		importMap[alias] = imp.Path
	}
	return importMap
}

// isQualifiedCallFromImport checks if this is a qualified call from an import (e.g., kafka.NewReader).
func (ke *KafkaExtractor) isQualifiedCallFromImport(n ast.Node, expectedAlias, expectedPath string, importMap map[string]string) bool {
	if ident, ok := n.(*ast.Ident); ok {
		if ident.Name == expectedAlias {
			if path, exists := importMap[expectedAlias]; exists {
				return strings.Contains(path, expectedPath)
			}
		}
	}
	return false
}

// hasImportPath checks if a specific import path is in the import map.
func (ke *KafkaExtractor) hasImportPath(importMap map[string]string, pathPart string) bool {
	for _, path := range importMap {
		if strings.Contains(path, pathPart) {
			return true
		}
	}
	return false
}

// looksLikeKafkaWriter checks if an identifier looks like a Kafka writer.
func (ke *KafkaExtractor) looksLikeKafkaWriter(n ast.Node) bool {
	if ident, ok := n.(*ast.Ident); ok {
		name := ident.Name
		return strings.Contains(name, "writer") || strings.Contains(name, "Writer") ||
			strings.Contains(name, "producer") || strings.Contains(name, "Producer")
	}
	return false
}

// looksLikeKafkaReader checks if an identifier looks like a Kafka reader.
func (ke *KafkaExtractor) looksLikeKafkaReader(n ast.Node) bool {
	if ident, ok := n.(*ast.Ident); ok {
		name := ident.Name
		return strings.Contains(name, "reader") || strings.Contains(name, "Reader") ||
			strings.Contains(name, "consumer") || strings.Contains(name, "Consumer")
	}
	return false
}

// looksLikeSaramaProducer checks if an identifier looks like a Sarama producer.
func (ke *KafkaExtractor) looksLikeSaramaProducer(n ast.Node) bool {
	if ident, ok := n.(*ast.Ident); ok {
		name := ident.Name
		return strings.Contains(name, "producer") || strings.Contains(name, "Producer") ||
			strings.Contains(name, "producer") || strings.Contains(name, "p")
	}
	return false
}

// looksLikeSaramaConsumer checks if an identifier looks like a Sarama consumer.
func (ke *KafkaExtractor) looksLikeSaramaConsumer(n ast.Node) bool {
	if ident, ok := n.(*ast.Ident); ok {
		name := ident.Name
		return strings.Contains(name, "consumer") || strings.Contains(name, "Consumer") ||
			strings.Contains(name, "group") || strings.Contains(name, "cg")
	}
	return false
}

// looksLikeConfluentProducer checks if an identifier looks like a Confluent producer.
func (ke *KafkaExtractor) looksLikeConfluentProducer(n ast.Node) bool {
	if ident, ok := n.(*ast.Ident); ok {
		name := ident.Name
		return strings.Contains(name, "producer") || strings.Contains(name, "Producer") || name == "p"
	}
	return false
}

// looksLikeConfluentConsumer checks if an identifier looks like a Confluent consumer.
func (ke *KafkaExtractor) looksLikeConfluentConsumer(n ast.Node) bool {
	if ident, ok := n.(*ast.Ident); ok {
		name := ident.Name
		return strings.Contains(name, "consumer") || strings.Contains(name, "Consumer") || name == "c"
	}
	return false
}

// extractTopicFromConfig extracts topic from a composite literal (e.g., ReaderConfig{Topic: "..."}).
func (ke *KafkaExtractor) extractTopicFromConfig(compLit *ast.CompositeLit) string {
	for _, elt := range compLit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "Topic" {
				if basicLit, ok := kv.Value.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
					// Remove quotes
					topic := strings.Trim(basicLit.Value, `"`)
					return topic
				}
			}
		}
	}
	return ""
}

// hasTopicField checks if a composite literal has a Topic field.
func (ke *KafkaExtractor) hasTopicField(compLit *ast.CompositeLit) bool {
	for _, elt := range compLit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "Topic" {
				return true
			}
		}
	}
	return false
}

// extractTopicsFromArray extracts string values from an array composite literal.
func (ke *KafkaExtractor) extractTopicsFromArray(arrayLit *ast.CompositeLit) []string {
	var topics []string
	for _, elt := range arrayLit.Elts {
		if basicLit, ok := elt.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
			topic := strings.Trim(basicLit.Value, `"`)
			topics = append(topics, topic)
		}
	}
	return topics
}
