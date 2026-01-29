package skills

// Skill represents a built-in skill that can be used by AI agents.
type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`    // "code-review", "development", "analysis", etc.
	Tags        []string `json:"tags"`        // For filtering
	Prompt      string   `json:"prompt"`      // The actual skill instructions
	UsesTools   []string `json:"uses_tools"`  // MCP tools this skill uses
	Examples    []Example `json:"examples,omitempty"`
}

// Example shows how to use a skill.
type Example struct {
	Input       string `json:"input"`
	Description string `json:"description"`
}

// SkillMetadata is a lightweight version for listing.
type SkillMetadata struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	UsesTools   []string `json:"uses_tools"`
}

// Registry manages available skills.
type Registry struct {
	skills map[string]*Skill
}

// NewRegistry creates a new skill registry with built-in skills.
func NewRegistry() *Registry {
	r := &Registry{
		skills: make(map[string]*Skill),
	}
	r.loadBuiltinSkills()
	return r
}

// List returns metadata for all skills.
func (r *Registry) List() []SkillMetadata {
	result := make([]SkillMetadata, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, SkillMetadata{
			Name:        s.Name,
			Description: s.Description,
			Category:    s.Category,
			Tags:        s.Tags,
			UsesTools:   s.UsesTools,
		})
	}
	return result
}

// ListByCategory returns skills filtered by category.
func (r *Registry) ListByCategory(category string) []SkillMetadata {
	var result []SkillMetadata
	for _, s := range r.skills {
		if s.Category == category {
			result = append(result, SkillMetadata{
				Name:        s.Name,
				Description: s.Description,
				Category:    s.Category,
				Tags:        s.Tags,
				UsesTools:   s.UsesTools,
			})
		}
	}
	return result
}

// Get returns a skill by name.
func (r *Registry) Get(name string) (*Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

// Register adds a skill to the registry.
func (r *Registry) Register(skill *Skill) {
	r.skills[skill.Name] = skill
}

// loadBuiltinSkills loads all embedded skills.
func (r *Registry) loadBuiltinSkills() {
	// Register all built-in skills
	r.Register(skillGoExpert())
	r.Register(skillPRReview())
	r.Register(skillCodeAnalysis())
	r.Register(skillSecurityReview())
	r.Register(skillPerformanceReview())
	r.Register(skillTestGeneration())
	r.Register(skillRefactoring())
	r.Register(skillDocumentation())
}
