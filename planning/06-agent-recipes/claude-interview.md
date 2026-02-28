# Interview: Agent Recipes & Pre-Built Workflows

## Q1: Should recipes replace the existing execute_pattern system or extend it?

**Answer:** Replace execute_pattern (Recommended)

Deprecate the existing Pattern/Chain system. Recipes become the new abstraction. Simpler, avoids maintaining two systems.

## Q2: How should recipes handle missing dependency data?

**Answer:** Partial results with gaps noted (Recommended)

Return what's available, clearly mark sections as "not available — requires dependency-graph split." Recipes work progressively as more splits are implemented.

## Q3: Should recipe output be AI-generated or structured data?

**Answer:** Structured + AI summary (Recommended)

Return structured JSON data with an optional AI-generated natural language summary. Agents can use either format.
