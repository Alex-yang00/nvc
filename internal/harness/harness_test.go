package harness

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/novitalabs/nvc/internal/lineup"
)

const testKey = "sk_test_0123456789abcdef"

func ctx(model string) Context {
	return Context{Key: testKey, BaseURL: "https://api.novita.ai", Model: model, Version: "t", Lineup: lineup.Default}
}

func noKeyInArgs(t *testing.T, p Plan) {
	t.Helper()
	for _, a := range p.Args {
		if strings.Contains(a, testKey) {
			t.Fatalf("API key leaked into argv: %q", a)
		}
	}
	if p.Env["NOVITA_API_KEY"] != testKey {
		t.Fatal("key must be passed via env")
	}
	if len(p.Guards) == 0 {
		t.Fatal("expected config guards")
	}
}

func claudeSettings(t *testing.T, p Plan) map[string]any {
	t.Helper()
	if p.Args[0] != "--settings" {
		t.Fatalf("args = %v", p.Args)
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(p.Args[1]), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestClaudePlan(t *testing.T) {
	p, err := Claude{}.Plan(ctx(""), []string{"-p", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	noKeyInArgs(t, p)
	s := claudeSettings(t, p)
	env := s["env"].(map[string]any)
	for k, want := range map[string]string{
		"ANTHROPIC_BASE_URL":            "https://api.novita.ai/anthropic",
		"ANTHROPIC_AUTH_TOKEN":          "",
		"ANTHROPIC_API_KEY":             "",
		"ANTHROPIC_MODEL":               lineup.Default.Opus,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL": lineup.Default.Haiku,
	} {
		if env[k] != want {
			t.Errorf("%s = %v, want %q", k, env[k], want)
		}
	}
	if _, ok := env["CLAUDE_CODE_SUBAGENT_MODEL"]; ok {
		t.Error("CLAUDE_CODE_SUBAGENT_MODEL must not be set (would force every subagent onto haiku tier)")
	}
	if s["apiKeyHelper"] != "printenv NOVITA_API_KEY" {
		t.Errorf("apiKeyHelper = %v", s["apiKeyHelper"])
	}
	deny := s["permissions"].(map[string]any)["deny"].([]any)
	if len(deny) != 1 || deny[0] != "WebSearch" {
		t.Errorf("deny = %v", deny)
	}
	if got := p.Args[2:]; strings.Join(got, " ") != "-p hi" {
		t.Errorf("user args not passed through: %v", got)
	}
}

func TestClaudeModelOverride(t *testing.T) {
	p, _ := Claude{}.Plan(ctx("zai-org/glm-5.3"), nil)
	env := claudeSettings(t, p)["env"].(map[string]any)
	if env["ANTHROPIC_MODEL"] != "zai-org/glm-5.3" || env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "zai-org/glm-5.3" {
		t.Errorf("override not applied: %v / %v", env["ANTHROPIC_MODEL"], env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != lineup.Default.Haiku {
		t.Error("haiku tier should keep the cheap model")
	}
}

func TestClaudeRejectsUserSettingsFlag(t *testing.T) {
	if _, err := (Claude{}).Plan(ctx(""), []string{"--settings", "x.json"}); err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestCodexPlan(t *testing.T) {
	p, err := Codex{}.Plan(ctx(""), []string{"exec", "do it"})
	if err != nil {
		t.Fatal(err)
	}
	noKeyInArgs(t, p)
	joined := strings.Join(p.Args, "\n")
	for _, want := range []string{
		`model_provider="novita"`,
		`model_providers.novita.wire_api="responses"`,
		`model_providers.novita.env_key="NOVITA_API_KEY"`,
		`model="` + lineup.Default.Default + `"`,
		`web_search="disabled"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing -c %s", want)
		}
	}
	// -c overrides must precede the subcommand
	if n := len(p.Args); p.Args[n-2] != "exec" || p.Args[n-1] != "do it" {
		t.Errorf("user args must come last: %v", p.Args)
	}
}

func TestCodexConflicts(t *testing.T) {
	for _, args := range [][]string{{"--profile", "x"}, {"-p", "x"}, {"--oss"}, {"exec", "--profile=x"}} {
		if _, err := (Codex{}).Plan(ctx(""), args); err == nil {
			t.Errorf("expected conflict for %v", args)
		}
	}
	if _, err := (Codex{}).Plan(ctx(""), []string{"exec", "--", "--profile"}); err != nil {
		t.Errorf("args after -- are not flags: %v", err)
	}
}

func TestRenderMasksKey(t *testing.T) {
	p, _ := Codex{}.Plan(ctx(""), nil)
	out := Render(p, func(string) string { return "MASKED" })
	if strings.Contains(out, testKey) || !strings.Contains(out, "NOVITA_API_KEY=MASKED") {
		t.Fatalf("render did not mask key:\n%s", out)
	}
	if !strings.Contains(out, "# after exit nvc restores:") {
		t.Error("render should disclose guarded keys")
	}
}
