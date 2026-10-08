// Package lineup holds the default model lineup and per-model tuning.
package lineup

import "github.com/novitalabs/nvc/internal/catalog"

type Lineup struct {
	Opus, Sonnet, Haiku string
	Default             string // codex / opencode / hermes
}

var Default = Lineup{
	Opus:    "moonshotai/kimi-k3",
	Sonnet:  "zai-org/glm-5.3",
	Haiku:   "deepseek/deepseek-v4.1-flash",
	Default: "zai-org/glm-5.3",
}

// Limits used when the catalog is unavailable. Kept in sync with /v1/models by hand.
var fallback = map[string]Spec{
	"moonshotai/kimi-k3":           {Name: "Kimi K3", Context: 1 << 20, MaxOutput: 1 << 20},
	"zai-org/glm-5.3":              {Name: "GLM 5.3", Context: 1 << 20, MaxOutput: 131072},
	"deepseek/deepseek-v4.1-flash": {Name: "DeepSeek V4.1 Flash", Context: 1 << 20, MaxOutput: 393216},
}

const (
	// Claude Code sends max_tokens = CLAUDE_CODE_MAX_OUTPUT_TOKENS on every request; a 1M value is pointless.
	OutputCap = 64000
	// Auto-compact well before 1M so long sessions don't resend huge contexts each turn.
	CompactWindow = 200000
)

type Spec struct {
	ID        string
	Name      string
	Context   int
	MaxOutput int
}

// Resolve returns limits for a model from the catalog, then the fallback table, then conservative defaults.
func Resolve(c *catalog.Catalog, id string) Spec {
	if m, ok := c.Get(id); ok {
		name := m.DisplayName
		if name == "" {
			name = id
		}
		return Spec{ID: id, Name: name, Context: m.ContextSize, MaxOutput: m.MaxOutput}
	}
	if s, ok := fallback[id]; ok {
		s.ID = id
		return s
	}
	return Spec{ID: id, Name: id, Context: 128000, MaxOutput: 32000}
}

func (s Spec) CappedOutput() int {
	if s.MaxOutput <= 0 || s.MaxOutput > OutputCap {
		return OutputCap
	}
	return s.MaxOutput
}

func (s Spec) CompactWindow() int {
	if s.Context <= 0 || s.Context > CompactWindow {
		return CompactWindow
	}
	return s.Context
}
