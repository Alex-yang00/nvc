package guard

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o640); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func run(t *testing.T, g Guard, during func()) bool {
	t.Helper()
	if err := g.Snapshot(); err != nil {
		t.Fatal(err)
	}
	during()
	changed, err := g.Restore()
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

func TestJSONRestoresChangedModelKeepsOtherEdits(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	write(t, p, `{"env":{"A":"1"},"model":"other-model","theme":"dark"}`)
	g := &JSONKeys{Path: p, Keys: []string{"model"}}
	changed := run(t, g, func() {
		// what Claude Code's /model does, plus an unrelated edit the user made meanwhile
		write(t, p, "{\n  \"env\": {\n    \"A\": \"1\"\n  },\n  \"model\": \"sonnet\",\n  \"theme\": \"light\"\n}\n")
	})
	if !changed {
		t.Fatal("expected restore")
	}
	want := "{\n  \"env\": {\n    \"A\": \"1\"\n  },\n  \"model\": \"other-model\",\n  \"theme\": \"light\"\n}\n"
	if got := read(t, p); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o640 {
		t.Fatalf("mode changed: %v", st.Mode().Perm())
	}
}

func TestJSONRemovesModelAddedDuringSession(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	write(t, p, `{"theme":"dark"}`)
	g := &JSONKeys{Path: p, Keys: []string{"model"}}
	run(t, g, func() { write(t, p, `{"theme":"dark","model":"sonnet"}`) })
	if got := read(t, p); got != "{\n  \"theme\": \"dark\"\n}\n" {
		t.Fatalf("got %q", got)
	}
}

func TestJSONDeletesFileCreatedOnlyForModel(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	g := &JSONKeys{Path: p, Keys: []string{"model"}}
	run(t, g, func() { write(t, p, `{"model":"sonnet"}`) })
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file should be removed, err=%v", err)
	}
}

func TestJSONUntouchedWhenUnchanged(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	orig := `{"model":"opus",  "x":1}`
	write(t, p, orig)
	g := &JSONKeys{Path: p, Keys: []string{"model"}}
	if run(t, g, func() {}) {
		t.Fatal("no change expected")
	}
	if read(t, p) != orig {
		t.Fatal("file was rewritten")
	}
}

func TestJSONLeavesInvalidFileAlone(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	write(t, p, `{"model":"opus"}`)
	g := &JSONKeys{Path: p, Keys: []string{"model"}}
	run(t, g, func() { write(t, p, `{"model":`) })
	if read(t, p) != `{"model":` {
		t.Fatal("invalid file must not be touched")
	}
}

const codexOrig = `# my config
model_provider = "custom"
model = "other-model"
model_reasoning_effort = "medium"

[model_providers.custom]
name = "custom"

[profiles.work]
model = "work-model"
`

func TestTOMLRestoresRootKeysPreservesRest(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	write(t, p, codexOrig)
	g := &TOMLKeys{Path: p, Keys: []string{"model", "model_reasoning_effort"}}
	changed := run(t, g, func() {
		// what Codex's /model does, plus Codex recording a trusted project
		write(t, p, `# my config
model_provider = "custom"
model = "gpt-6-astra"
model_reasoning_effort = "low"

[model_providers.custom]
name = "custom"

[profiles.work]
model = "work-model"

[projects."/tmp/x"]
trust_level = "trusted"
`)
	})
	if !changed {
		t.Fatal("expected restore")
	}
	want := codexOrig + "\n[projects.\"/tmp/x\"]\ntrust_level = \"trusted\"\n"
	if got := read(t, p); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTOMLProfileTableAndAddedKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	write(t, p, codexOrig)
	g := &TOMLKeys{Path: p, Keys: []string{"model", "model_reasoning_effort"}}
	run(t, g, func() {
		write(t, p, `# my config
model_provider = "custom"
model = "other-model"
model_reasoning_effort = "medium"

[model_providers.custom]
name = "custom"

[profiles.work]
model = "gpt-6-astra"
model_reasoning_effort = "high"
`)
	})
	if got := read(t, p); got != codexOrig {
		t.Fatalf("got\n%s\nwant\n%s", got, codexOrig)
	}
}

func TestTOMLReinsertsRemovedKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	write(t, p, codexOrig)
	g := &TOMLKeys{Path: p, Keys: []string{"model_reasoning_effort"}}
	run(t, g, func() {
		write(t, p, `# my config
model_provider = "custom"
model = "other-model"

[model_providers.custom]
name = "custom"

[profiles.work]
model = "work-model"
`)
	})
	got := read(t, p)
	if want := "model_reasoning_effort = \"medium\"\n# my config\n"; got[:len(want)] != want {
		t.Fatalf("removed key not reinserted at root:\n%s", got)
	}
}

func TestTOMLMissingFileCreatedDuringSession(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	g := &TOMLKeys{Path: p, Keys: []string{"model", "model_reasoning_effort"}}
	run(t, g, func() { write(t, p, "model = \"gpt-6-astra\"\nmodel_reasoning_effort = \"low\"\n") })
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file should be removed, err=%v", err)
	}
}
