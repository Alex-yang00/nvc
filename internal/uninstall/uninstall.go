// Package uninstall removes exactly what nvc added to the machine and nothing else.
//
// nvc creates: its own binary, ~/.config/nvc/{config.json, models.json[.tmp]}, and (only
// mid-restore, normally gone a moment later) <agent config>.nvc-tmp scratch files.
// It never writes the agents' config files except to put back keys the agents changed,
// so there is nothing in them to undo. Any other file — including ones a user may have
// dropped into ~/.config/nvc — is left alone.
package uninstall

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/novitalabs/nvc/internal/catalog"
	"github.com/novitalabs/nvc/internal/guard"
)

type Item struct {
	Path  string
	What  string // shown to the user
	isDir bool   // removed only if empty
}

type Paths struct {
	Binary      string   // resolved path of the running nvc ("" = don't remove)
	ConfigDir   string   // ~/.config/nvc
	ConfigFile  string   // ~/.config/nvc/config.json
	AgentConfig []string // agent config files guarded during sessions
}

// Targets lists what exists now and would be removed.
func Targets(p Paths) []Item {
	var out []Item
	add := func(path, what string, dir bool) {
		if path == "" {
			return
		}
		if st, err := os.Lstat(path); err == nil && st.IsDir() == dir {
			out = append(out, Item{Path: path, What: what, isDir: dir})
		}
	}
	add(p.Binary, "nvc binary", false)
	add(p.ConfigFile, "saved Novita API key", false)
	cache := catalog.CachePath(p.ConfigDir)
	add(cache, "model list cache", false)
	add(cache+".tmp", "model list cache (partial)", false)
	for _, c := range p.AgentConfig {
		add(c+guard.TmpSuffix, "leftover restore scratch file", false)
	}
	add(p.ConfigDir, "nvc config directory (only if empty)", true)
	return out
}

// Remove deletes items in order. The config directory is removed only once empty;
// if it still holds files nvc didn't create, it's kept and reported in kept.
func Remove(items []Item) (kept []string, errs []error) {
	for _, it := range items {
		var err error
		if it.isDir {
			err = os.Remove(it.Path) // fails if not empty
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				if entries, rerr := os.ReadDir(it.Path); rerr == nil && len(entries) > 0 {
					kept = append(kept, it.Path)
					continue
				}
			}
		} else {
			err = os.Remove(it.Path)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("%s: %w", it.Path, err))
		}
	}
	return kept, errs
}

// SelfBinary returns the real path of the running executable if it looks like an
// installed nvc (file named "nvc"), else "" — e.g. under `go run` / `go test`.
func SelfBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	name := filepath.Base(exe)
	if name != "nvc" && name != "nvc.exe" {
		return ""
	}
	return exe
}
