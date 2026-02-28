package recipes

import (
	"fmt"
	"strings"
)

// buildMermaidDiagram generates a Mermaid sequence diagram from flow steps.
func buildMermaidDiagram(entryFunc string, steps []FlowStep) string {
	if len(steps) == 0 {
		return fmt.Sprintf("sequenceDiagram\n    participant Client\n    participant Handler as %s\n    Client->>Handler: request\n    Handler-->>Client: response\n", entryFunc)
	}

	var sb strings.Builder
	sb.WriteString("sequenceDiagram\n")

	// Collect unique participants
	participants := collectParticipants(entryFunc, steps)
	sb.WriteString("    participant Client\n")
	for _, p := range participants {
		if p.alias != "" {
			fmt.Fprintf(&sb, "    participant %s as %s\n", sanitizeMermaidID(p.alias), p.label)
		} else {
			fmt.Fprintf(&sb, "    participant %s\n", sanitizeMermaidID(p.label))
		}
	}

	// Add Database participant if there are DB side effects
	hasDB := false
	for _, step := range steps {
		for _, effect := range step.SideEffects {
			if strings.Contains(effect, "db_query") || strings.Contains(effect, "db_write") || strings.Contains(strings.ToLower(effect), "select") || strings.Contains(strings.ToLower(effect), "insert") {
				hasDB = true
				break
			}
		}
	}
	if hasDB {
		sb.WriteString("    participant DB as Database\n")
	}

	sb.WriteString("\n")

	// Entry: Client calls first function
	firstID := sanitizeMermaidID(steps[0].Function)
	fmt.Fprintf(&sb, "    Client->>%s: request\n", firstID)

	// Truncate at 15 steps
	maxSteps := 15
	truncated := false
	displaySteps := steps
	if len(steps) > maxSteps {
		displaySteps = steps[:maxSteps]
		truncated = true
	}

	// Add interactions
	for i := 0; i < len(displaySteps)-1; i++ {
		from := sanitizeMermaidID(displaySteps[i].Function)
		to := sanitizeMermaidID(displaySteps[i+1].Function)

		// Show side effects as interactions with DB
		for _, effect := range displaySteps[i].SideEffects {
			if strings.Contains(effect, "db_query") || strings.Contains(strings.ToLower(effect), "select") {
				fmt.Fprintf(&sb, "    %s->>DB: %s\n", from, truncateLabel(effect, 50))
				fmt.Fprintf(&sb, "    DB->>%s: result\n", from)
			} else if strings.Contains(effect, "db_write") || strings.Contains(strings.ToLower(effect), "insert") || strings.Contains(strings.ToLower(effect), "update") {
				fmt.Fprintf(&sb, "    %s->>DB: %s\n", from, truncateLabel(effect, 50))
				fmt.Fprintf(&sb, "    DB-->>%s: ok\n", from)
			}
		}

		// Show call to next function
		label := displaySteps[i+1].Action
		if label == "" {
			label = "call"
		}
		fmt.Fprintf(&sb, "    %s->>%s: %s\n", from, to, truncateLabel(label, 50))
	}

	// Last step side effects
	if len(displaySteps) > 0 {
		last := displaySteps[len(displaySteps)-1]
		lastID := sanitizeMermaidID(last.Function)
		for _, effect := range last.SideEffects {
			if strings.Contains(effect, "db_query") || strings.Contains(strings.ToLower(effect), "select") {
				fmt.Fprintf(&sb, "    %s->>DB: %s\n", lastID, truncateLabel(effect, 50))
				fmt.Fprintf(&sb, "    DB->>%s: result\n", lastID)
			}
		}
	}

	if truncated {
		sb.WriteString("    Note over Client: ... (truncated, more steps follow)\n")
	}

	// Response back to client
	fmt.Fprintf(&sb, "    %s-->>Client: response\n", firstID)

	return sb.String()
}

type mermaidParticipant struct {
	alias string
	label string
}

// collectParticipants identifies unique participants from flow steps.
func collectParticipants(entryFunc string, steps []FlowStep) []mermaidParticipant {
	seen := make(map[string]bool)
	var participants []mermaidParticipant

	for _, step := range steps {
		id := step.Function
		if seen[id] {
			continue
		}
		seen[id] = true

		label := id
		if step.Service != "" && step.Service != "same" {
			label = step.Service + ":" + id
		}

		participants = append(participants, mermaidParticipant{
			alias: id,
			label: label,
		})
	}

	return participants
}

// sanitizeMermaidID makes a string safe for use as a Mermaid participant ID.
func sanitizeMermaidID(s string) string {
	// Replace non-alphanumeric chars with underscores
	var sb strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			sb.WriteRune(c)
		} else {
			sb.WriteRune('_')
		}
	}
	result := sb.String()
	if result == "" {
		return "unknown"
	}
	return result
}

// truncateLabel limits a label string to maxLen characters.
func truncateLabel(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
