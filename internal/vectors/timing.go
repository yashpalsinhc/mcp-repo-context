package vectors

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// logTiming logs the duration of a vector operation.
func logTiming(operation string, start time.Time, details ...string) {
	duration := time.Since(start)
	msg := fmt.Sprintf("[vectors] %s completed in %s", operation, duration.Round(time.Millisecond))
	if len(details) > 0 {
		msg += " (" + strings.Join(details, ", ") + ")"
	}
	log.Print(msg)
}

// truncateQuery truncates a query string for logging.
func truncateQuery(query string, maxLen int) string {
	if len(query) <= maxLen {
		return query
	}
	return query[:maxLen] + "..."
}
