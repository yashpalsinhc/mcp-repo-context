# Deep Project Interview Transcript

## Context
- **Project:** MCP Repo-Context Server — improvements and new features
- **Input:** planning/mcp-server-gaps-requirements.md (12 issues found via testing with gorilla/mux, gorilla/handlers, gorilla/sessions)
- **Additional scope:** Codebase indexing as a service, agent efficiency, API flow tracing

## Interview Summary

### Round 1: Vision & Scope

**Q: Deployment model?**
A: **Both local + hosted** — Local MCP server for individual devs, but also deployable as org-wide service (self-hosted or cloud).

**Q: Agent use cases?**
A: **All of the above** — Code review (PR context), feature implementation, cross-repo API tracing. All equally important.

**Q: Multi-language support priority?**
A: **Go-only for now** — Fix Go analyzer bugs first, add other languages later.

**Q: Bug fixes vs features priority?**
A: **Interleave both** — Fix critical bugs as part of feature work (e.g., fix comparison when building dependency graph).

### Round 2: Architecture & Scale

**Q: Repo connection method for hosted service?**
A: **Both** — GitHub/GitLab webhooks for auto-analysis + manual API trigger.

**Q: Cross-repo API tracing depth?**
A: **Full API flow maps** — End-to-end: "when user hits /login, it goes through service A → B → C with these payloads."

**Q: Agent API design?**
A: **Both** — Raw tools for flexibility + pre-built recipes for common workflows (analyze_pr_impact, explain_api_flow).

**Q: Scale expectation?**
A: **Medium (50-200 repos)** — Mid-size org, mix of small and large repos.

### Round 3: Technical Decisions

**Q: Authentication model for hosted service?**
A: **Decide later** — Focus on core indexing engine first, add auth later.

**Q: Agent-optimized context approach?**
A: **Hybrid** — Pre-computed structural context (call graphs, types) + real-time RAG for content retrieval.

**Q: MVP scope?**
A: **Fix bugs + dependency graph** AND **API flow tracing MVP**. Both are priority. The killer feature is understanding how services talk to each other.

## Research Findings (Incorporated)

### Industry State of the Art
- **Greptile** ($25M Series A): Language-agnostic function/class dependency graphs, MCP server
- **Sourcegraph Cody**: RAG with code search index, enterprise-focused
- **Code Property Graphs (CPG)**: Merge AST + Control Flow + Data Dependencies — most comprehensive
- **Voyage Code-3**: Best code embedding model (16% better than OpenAI on code retrieval)
- **OpenTelemetry**: Industry standard for cross-service tracing

### Plugin Pattern Insights (piercelamb)
- **File-based recovery**: Infer state from file existence, not in-memory state
- **Parallel subagent dispatch**: Batch work into parallel Task calls
- **Pre-computed context**: Analyze once, agents query pre-built context
- **Progressive loading**: 2K-8K tokens covers 95% of agent queries
- **Hook automation**: Reduce JSON parsing overhead via SubagentStop hooks

### Agent Efficiency Key Insight
The research consensus: **pre-computed structural context + explicit project files > larger token windows**. Agents should focus on reasoning, not exploring. This validates the MCP server's approach — but the current implementation needs better pre-computation (dependency graphs, API flows, richer summaries).

## Key Constraints & Decisions

1. **Go-only for now** — All improvements target the Go analyzer
2. **Interleave approach** — Fix comparison bugs while building dependency graph
3. **Hybrid context** — Pre-computed structure + vector-based content retrieval
4. **Auth deferred** — Service layer first, auth later
5. **Medium scale target** — 50-200 repos per org, both small and large
6. **MVP = reliability + cross-repo intelligence** — Fix what's broken, add dependency/flow analysis
