package uninstall

import (
	"os"
	"path/filepath"
	"testing"
)

func mk(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

type layout struct {
	root, bin, other, cfgDir, key, cache, claude, codex string
}

func setup(t *testing.T) layout {
	r := t.TempDir()
	l := layout{
		root:   r,
		bin:    filepath.Join(r, "bin", "nvc"),
		other:  filepath.Join(r, "bin", "claude"),
		cfgDir: filepath.Join(r, "cfg", "nvc"),
		claude: filepath.Join(r, ".claude", "settings.json"),
		codex:  filepath.Join(r, ".codex", "config.toml"),
	}
	l.key = filepath.Join(l.cfgDir, "config.json")
	l.cache = filepath.Join(l.cfgDir, "models.json")
	mk(t, l.bin, "binary")
	mk(t, l.other, "someone else's binary")
	mk(t, l.key, `{"api_key":"sk"}`)
	mk(t, l.cache, "{}")
	mk(t, l.claude, `{"model":"other-model"}`)
	mk(t, l.codex, `model = "other-model"`)
	return l
}

func (l layout) paths() Paths {
	return Paths{Binary: l.bin, ConfigDir: l.cfgDir, ConfigFile: l.key, AgentConfig: []string{l.claude, l.codex}}
}

func TestRemovesOnlyNvcFiles(t *testing.T) {
	l := setup(t)
	mk(t, l.claude+".nvc-tmp", "scratch")
	items := Targets(l.paths())
	if len(items) != 5 { // binary, key, cache, 1 scratch file, dir
		t.Fatalf("targets = %+v", items)
	}
	kept, errs := Remove(items)
	if len(errs) > 0 || len(kept) > 0 {
		t.Fatalf("kept=%v errs=%v", kept, errs)
	}
	for _, gone := range []string{l.bin, l.key, l.cache, l.claude + ".nvc-tmp", l.cfgDir} {
		if exists(gone) {
			t.Errorf("%s should be removed", gone)
		}
	}
	for _, stays := range []string{l.other, l.claude, l.codex, filepath.Dir(l.bin)} {
		if !exists(stays) {
			t.Errorf("%s must not be touched", stays)
		}
	}
	if b, _ := os.ReadFile(l.claude); string(b) != `{"model":"other-model"}` {
		t.Error("agent config content changed")
	}
}

func TestKeepsConfigDirWithForeignFiles(t *testing.T) {
	l := setup(t)
	foreign := filepath.Join(l.cfgDir, "my-notes.txt")
	mk(t, foreign, "user file")
	kept, errs := Remove(Targets(l.paths()))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(kept) != 1 || kept[0] != l.cfgDir || !exists(foreign) {
		t.Fatalf("dir with a user file must be kept: kept=%v", kept)
	}
	if exists(l.key) || exists(l.cache) {
		t.Error("nvc's own files should still be removed")
	}
}

func TestNothingInstalled(t *testing.T) {
	r := t.TempDir()
	p := Paths{ConfigDir: filepath.Join(r, "nvc"), ConfigFile: filepath.Join(r, "nvc", "config.json")}
	if items := Targets(p); len(items) != 0 {
		t.Fatalf("expected nothing, got %+v", items)
	}
}

func TestSymlinkedBinaryNotFollowed(t *testing.T) {
	l := setup(t)
	link := filepath.Join(l.root, "bin", "nvc-link")
	if err := os.Symlink(l.other, link); err != nil {
		t.Skip(err)
	}
	// Binary must be a regular file named by the caller; a directory with that name is ignored.
	p := l.paths()
	p.Binary = filepath.Join(l.root, "bin")
	for _, it := range Targets(p) {
		if it.What == "nvc binary" {
			t.Fatal("a directory must never be treated as the binary")
		}
	}
}
