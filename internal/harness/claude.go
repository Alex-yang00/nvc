package harness

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/novitalabs/nvc/internal/guard"
)

// Claude Code → Novita Anthropic endpoint.
//
// Config goes through `--settings <json>` because flag settings outrank ~/.claude/settings.json,
// whose `env` block would otherwise override our process env. The key itself never appears in
// argv: ANTHROPIC_AUTH_TOKEN/ANTHROPIC_API_KEY are blanked in settings and apiKeyHelper reads it
// from NOVITA_API_KEY in the process env.
type Claude struct{}

func (Claude) Name() string        { return "claude" }
func (Claude) Title() string       { return "Claude Code" }
func (Claude) Binary() string      { return "claude" }
func (Claude) InstallHint() string { return "curl -fsSL https://claude.ai/install.sh | bash" }

func (Claude) Plan(c Context, args []string) (Plan, error) {
	if hasFlag(args, "--settings") {
		return Plan{}, fmt.Errorf("nvc injects its own --settings; remove yours or run `claude` directly")
	}
	opus, sonnet, haiku := c.Spec(c.Lineup.Opus), c.Spec(c.Lineup.Sonnet), c.Spec(c.Lineup.Haiku)
	main := opus
	if c.Model != "" {
		main = c.Spec(c.Model)
		opus, sonnet = main, main
	}
	outCap := min(opus.CappedOutput(), sonnet.CappedOutput(), haiku.CappedOutput())

	env := map[string]string{
		"ANTHROPIC_BASE_URL":   c.BaseURL + "/anthropic",
		"ANTHROPIC_AUTH_TOKEN": "",
		"ANTHROPIC_API_KEY":    "",

		"ANTHROPIC_MODEL":                main.ID, // also overrides a user-level ANTHROPIC_MODEL
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   opus.ID,
		"ANTHROPIC_DEFAULT_SONNET_MODEL": sonnet.ID,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  haiku.ID,
		"ANTHROPIC_DEFAULT_FABLE_MODEL":  opus.ID,
		"ANTHROPIC_SMALL_FAST_MODEL":     haiku.ID, // deprecated, still read by older versions

		"ANTHROPIC_DEFAULT_OPUS_MODEL_NAME":          opus.Name + " · Novita",
		"ANTHROPIC_DEFAULT_SONNET_MODEL_NAME":        sonnet.Name + " · Novita",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME":         haiku.Name + " · Novita",
		"ANTHROPIC_DEFAULT_OPUS_MODEL_DESCRIPTION":   opus.ID,
		"ANTHROPIC_DEFAULT_SONNET_MODEL_DESCRIPTION": sonnet.ID,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL_DESCRIPTION":  haiku.ID,

		"CLAUDE_CODE_MAX_CONTEXT_TOKENS":  strconv.Itoa(main.Context),
		"CLAUDE_CODE_AUTO_COMPACT_WINDOW": strconv.Itoa(main.CompactWindow()),
		"CLAUDE_CODE_MAX_OUTPUT_TOKENS":   strconv.Itoa(outCap),
		"API_TIMEOUT_MS":                  "3000000",

		"CLAUDE_CODE_ATTRIBUTION_HEADER":           "0",
		"CLAUDE_CODE_DISABLE_EXPLORE_INHERIT_CAP":  "1",
		"DISABLE_TELEMETRY":                        "1",
		"DO_NOT_TRACK":                             "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",

		"ANTHROPIC_CUSTOM_HEADERS": SourceHeader + ": " + c.Source("claude"),
	}
	settings := map[string]any{
		"env":          env,
		"apiKeyHelper": "printenv NOVITA_API_KEY",
		// WebSearch is an Anthropic server-side tool. Novita doesn't execute it, yet Claude Code
		// reports success with an empty result and the model then improvises an answer.
		"permissions": map[string]any{"deny": []string{"WebSearch"}},
	}
	js, _ := json.Marshal(settings)

	return Plan{
		Binary:  "claude",
		Args:    append([]string{"--settings", string(js)}, args...),
		Env:     map[string]string{"NOVITA_API_KEY": c.Key},
		Secrets: []string{c.Key},
		Summary: fmt.Sprintf("Claude Code → Novita · opus=%s sonnet=%s haiku=%s", opus.ID, sonnet.ID, haiku.ID),
		// `/model` → "set as default" writes "model" to the user settings file.
		Guards: []guard.Guard{&guard.JSONKeys{Path: ClaudeSettingsPath(), Keys: []string{"model"}}},
	}, nil
}
