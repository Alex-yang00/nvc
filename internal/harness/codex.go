package harness

import (
	"fmt"
	"strconv"

	"github.com/novitalabs/nvc/internal/guard"
)

// Codex → Novita Responses endpoint, via `-c key=value` overrides (no ~/.codex/config.toml edits).
type Codex struct{}

func (Codex) Name() string        { return "codex" }
func (Codex) Title() string       { return "Codex" }
func (Codex) Binary() string      { return "codex" }
func (Codex) InstallHint() string { return "npm i -g @openai/codex" }

func (Codex) Plan(c Context, args []string) (Plan, error) {
	if hasFlag(args, "--profile", "-p") {
		return Plan{}, fmt.Errorf("--profile conflicts with nvc's provider override; run `codex` directly to use a profile")
	}
	if hasFlag(args, "--oss", "--local-provider") {
		return Plan{}, fmt.Errorf("--oss/--local-provider conflicts with nvc's provider override")
	}
	model := c.Model
	if model == "" {
		model = c.Lineup.Default
	}
	spec := c.Spec(model)
	cfg := []string{
		`model_provider="novita"`,
		`model_providers.novita.name="Novita AI"`,
		fmt.Sprintf(`model_providers.novita.base_url=%q`, c.BaseURL+"/openai/v1"),
		`model_providers.novita.wire_api="responses"`,
		`model_providers.novita.env_key="NOVITA_API_KEY"`,
		fmt.Sprintf(`model_providers.novita.http_headers={%q=%q}`, SourceHeader, c.Source("codex")),
		fmt.Sprintf(`model=%q`, model),
		"model_context_window=" + strconv.Itoa(spec.Context),
		// Hosted web_search: Novita returns 200 without searching and the model improvises.
		`web_search="disabled"`,
	}
	var out []string
	for _, kv := range cfg {
		out = append(out, "-c", kv)
	}
	return Plan{
		Binary:  "codex",
		Args:    append(out, args...),
		Env:     map[string]string{"NOVITA_API_KEY": c.Key},
		Secrets: []string{c.Key},
		Summary: "Codex → Novita · model=" + model,
		// `/model` writes model + model_reasoning_effort to config.toml (root or [profiles.x]).
		Guards: []guard.Guard{&guard.TOMLKeys{Path: CodexConfigPath(), Keys: []string{"model", "model_reasoning_effort"}}},
	}, nil
}
