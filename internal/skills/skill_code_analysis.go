package skills

func skillCodeAnalysis() *Skill {
	return &Skill{
		Name:        "code-analysis",
		Description: "Analyze codebase structure, architecture, patterns, and dependencies. Understand how code works and how components interact.",
		Category:    "analysis",
		Tags:        []string{"analysis", "architecture", "patterns", "dependencies"},
		UsesTools:   []string{"ask", "get_context", "search_context", "compare_repos"},
		Examples: []Example{
			{Input: "How does authentication work in this codebase?", Description: "Traces auth flow through the codebase"},
			{Input: "What patterns are used in the handlers?", Description: "Identifies common patterns"},
			{Input: "What depends on the User service?", Description: "Analyzes dependencies"},
		},
		Prompt: `# Code Analysis Skill

You are an expert at understanding and explaining codebases. Use MCP tools to analyze code structure, patterns, and dependencies.

## Analysis Types

### 1. Architecture Analysis
` + "```" + `
# Get overall architecture
mcp__repo-context__get_context: repo_id="github.com/<owner>/<repo>" scope="architecture"

# Get AI-generated architecture analysis
mcp__repo-context__generate_ai_arch_analysis: repo_id="github.com/<owner>/<repo>"
` + "```" + `

### 2. Pattern Discovery
` + "```" + `
# Ask about patterns
mcp__repo-context__ask:
  query="What patterns are used in the handlers/services?"
  repo_ids=["github.com/<owner>/<repo>"]

# Search for specific patterns
mcp__repo-context__search_context:
  query="Handler"
  search_type="type"
` + "```" + `

### 3. Dependency Analysis
` + "```" + `
# Find what uses a function/type
mcp__repo-context__ask:
  query="What functions call {function_name}? What uses {type_name}?"
  repo_ids=["github.com/<owner>/<repo>"]

# Compare multiple repos for dependencies
mcp__repo-context__compare_repos:
  repo_ids=["repo1", "repo2"]
  include_duplicates=true
  include_gaps=true
` + "```" + `

### 4. Flow Tracing
` + "```" + `
# Trace a feature flow
mcp__repo-context__ask:
  query="How does a request flow from API to database for {feature}?"
  repo_ids=["github.com/<owner>/<repo>"]
` + "```" + `

## Analysis Framework

### When Analyzing Architecture
1. **Entry Points**: Where does code start? (main, handlers, etc.)
2. **Layers**: What layers exist? (API, service, repository, etc.)
3. **Data Flow**: How does data move through the system?
4. **External Dependencies**: What external services/databases?
5. **Configuration**: How is the app configured?

### When Analyzing Patterns
1. **Naming Conventions**: How are things named?
2. **Error Handling**: How are errors propagated?
3. **Dependency Injection**: How are dependencies managed?
4. **Testing Patterns**: How is code tested?
5. **Logging/Monitoring**: How is observability handled?

### When Analyzing Dependencies
1. **Direct Dependencies**: What does X directly call?
2. **Reverse Dependencies**: What calls X?
3. **Transitive Dependencies**: Full dependency chain
4. **Circular Dependencies**: Any cycles?
5. **External Dependencies**: Third-party packages

## Output Format

When providing analysis, structure it as:

` + "```markdown" + `
## [Analysis Topic]

### Overview
[High-level summary]

### Key Findings
1. [Finding 1]
2. [Finding 2]

### Details
[Detailed explanation with code references]

### Diagram (if applicable)
` + "```" + `
[Component A] --> [Component B] --> [Component C]
` + "```" + `

### Recommendations
- [Recommendation 1]
- [Recommendation 2]
` + "```" + `
`,
	}
}

func skillSecurityReview() *Skill {
	return &Skill{
		Name:        "security-review",
		Description: "Security-focused code review identifying vulnerabilities, authentication issues, injection attacks, and security best practices.",
		Category:    "security",
		Tags:        []string{"security", "vulnerabilities", "owasp", "authentication"},
		UsesTools:   []string{"review_pr", "ask", "search_context"},
		Examples: []Example{
			{Input: "Review this code for security issues", Description: "Comprehensive security review"},
			{Input: "Check for SQL injection vulnerabilities", Description: "Specific vulnerability check"},
		},
		Prompt: `# Security Review Skill

You are a security expert. Review code for vulnerabilities following OWASP guidelines and security best practices.

## Security Checklist

### 1. Injection Attacks
- **SQL Injection**: Are queries parameterized?
- **Command Injection**: Is user input sanitized before shell commands?
- **XSS**: Is output encoded/escaped?
- **LDAP Injection**: Are LDAP queries safe?

` + "```go" + `
// BAD: SQL Injection
query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", userID)

// GOOD: Parameterized query
query := "SELECT * FROM users WHERE id = $1"
db.Query(query, userID)
` + "```" + `

### 2. Authentication & Authorization
- Are credentials handled securely?
- Is session management secure?
- Are authorization checks in place?
- Is password hashing using bcrypt/argon2?

` + "```go" + `
// BAD: Plain text password comparison
if password == user.Password { ... }

// GOOD: Secure password comparison
if err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)); err == nil { ... }
` + "```" + `

### 3. Sensitive Data Exposure
- Are secrets in code/config files?
- Is sensitive data logged?
- Is data encrypted in transit and at rest?
- Are API keys/tokens exposed?

` + "```go" + `
// BAD: Logging sensitive data
log.Printf("User login: %s password: %s", user, password)

// GOOD: Don't log sensitive data
log.Printf("User login attempt: %s", user)
` + "```" + `

### 4. Input Validation
- Is all input validated?
- Are there size limits on input?
- Is file upload validated?
- Are redirects validated?

### 5. Error Handling
- Do errors expose internal details?
- Are stack traces hidden from users?
- Is error logging secure?

` + "```go" + `
// BAD: Exposing internal error
return fmt.Errorf("database error: %v", err)

// GOOD: Generic error to user, log details internally
log.Errorf("database error: %v", err)
return ErrInternalServer
` + "```" + `

### 6. Cryptography
- Are deprecated algorithms used (MD5, SHA1 for hashing)?
- Is random number generation secure?
- Are keys properly managed?

### 7. Rate Limiting & DoS
- Is rate limiting in place?
- Are there resource limits?
- Can requests cause expensive operations?

## Using MCP Tools for Security Review

` + "```" + `
# Security-focused PR review
mcp__repo-context__review_pr:
  pr_url="..."
  focus_areas=["security", "authentication"]

# Search for potential issues
mcp__repo-context__search_context:
  query="password"
  search_type="function"

# Ask about auth implementation
mcp__repo-context__ask:
  query="How is authentication implemented? Are there any security concerns?"
` + "```" + `

## Severity Levels

- **🔴 Critical**: Exploitable vulnerability (injection, auth bypass, data leak)
- **🟡 High**: Potential vulnerability needing investigation
- **🟠 Medium**: Security weakness that should be fixed
- **🟢 Low**: Best practice improvement
`,
	}
}

func skillPerformanceReview() *Skill {
	return &Skill{
		Name:        "performance-review",
		Description: "Performance-focused code review identifying bottlenecks, inefficient algorithms, memory issues, and optimization opportunities.",
		Category:    "performance",
		Tags:        []string{"performance", "optimization", "memory", "latency"},
		UsesTools:   []string{"review_pr", "ask", "search_context"},
		Examples: []Example{
			{Input: "Review this code for performance issues", Description: "Comprehensive performance review"},
			{Input: "Are there any N+1 query problems?", Description: "Database performance check"},
		},
		Prompt: `# Performance Review Skill

You are a performance expert. Review code for efficiency, scalability, and resource usage.

## Performance Checklist

### 1. Database Performance
- **N+1 Queries**: Are related records fetched in loops?
- **Missing Indexes**: Are queries using indexes?
- **Large Result Sets**: Are results paginated?
- **Connection Pooling**: Is the pool properly sized?

` + "```go" + `
// BAD: N+1 Query
for _, user := range users {
    orders, _ := db.GetOrdersByUserID(user.ID) // Query per user!
}

// GOOD: Batch query
userIDs := extractIDs(users)
ordersByUser, _ := db.GetOrdersByUserIDs(userIDs) // Single query
` + "```" + `

### 2. Memory Usage
- **Unnecessary Allocations**: Are objects reused?
- **Large Slices**: Are slices pre-allocated?
- **Memory Leaks**: Are resources properly closed?
- **Goroutine Leaks**: Are goroutines properly managed?

` + "```go" + `
// BAD: Growing slice
var results []Result
for _, item := range items {
    results = append(results, process(item))
}

// GOOD: Pre-allocated slice
results := make([]Result, 0, len(items))
for _, item := range items {
    results = append(results, process(item))
}
` + "```" + `

### 3. Concurrency Issues
- **Lock Contention**: Are locks held too long?
- **Race Conditions**: Is shared state protected?
- **Goroutine Overhead**: Too many goroutines?
- **Channel Blocking**: Are channels properly buffered?

### 4. Algorithm Complexity
- **O(n²) or worse**: Can algorithms be improved?
- **Unnecessary Iterations**: Are loops optimized?
- **Caching**: Are expensive computations cached?

### 5. I/O Operations
- **Unbuffered I/O**: Is I/O buffered?
- **Serial I/O**: Can operations be parallelized?
- **Large Payloads**: Is data streamed?

` + "```go" + `
// BAD: Reading entire file into memory
data, _ := ioutil.ReadFile(largefile)

// GOOD: Streaming with bufio
file, _ := os.Open(largefile)
scanner := bufio.NewScanner(file)
for scanner.Scan() {
    process(scanner.Text())
}
` + "```" + `

### 6. Serialization
- **JSON Parsing**: Using efficient libraries?
- **Reflection**: Minimizing reflection usage?
- **Reusable Encoders**: Reusing encoder instances?

## Using MCP Tools for Performance Review

` + "```" + `
# Performance-focused PR review
mcp__repo-context__review_pr:
  pr_url="..."
  focus_areas=["performance"]

# Search for potential issues
mcp__repo-context__search_context:
  query="for range"
  search_type="function"

# Ask about performance patterns
mcp__repo-context__ask:
  query="Are there any N+1 queries or performance bottlenecks?"
` + "```" + `

## Severity Levels

- **🔴 Critical**: Causes outages or severe degradation
- **🟡 High**: Noticeable user impact
- **🟠 Medium**: Suboptimal but acceptable
- **🟢 Low**: Micro-optimization opportunity
`,
	}
}
