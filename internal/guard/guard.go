// Package guard keeps nvc from leaving traces in the user's agent config.
//
// nvc injects everything per-process, but the agents themselves persist some in-session
// choices to their user config — e.g. `/model` in Claude Code writes "model" to
// ~/.claude/settings.json and in Codex writes model/model_reasoning_effort to
// ~/.codex/config.toml. A Guard snapshots exactly those keys before launch and puts them
// back after the agent exits. Every other key and every other edit is left untouched.
package guard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"regexp"
	"strings"
)

type Guard interface {
	Snapshot() error
	// Restore reverts the guarded keys; changed reports whether the file was rewritten.
	Restore() (changed bool, err error)
	Describe() string // "model in ~/.claude/settings.json"
}

func readOptional(path string) ([]byte, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return b, err == nil, err
}

// TmpSuffix marks the scratch file used for atomic restores (path+TmpSuffix).
const TmpSuffix = ".nvc-tmp"

// writeKeepMode replaces path atomically, preserving its permission bits.
func writeKeepMode(path string, data []byte) error {
	mode := fs.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp := path + TmpSuffix
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Tildify shortens $HOME to ~ for display.
func Tildify(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}

// ---------------------------------------------------------------- JSON (top-level keys)

type JSONKeys struct {
	Path string
	Keys []string

	existed bool
	orig    map[string]json.RawMessage // only guarded keys that were present
}

func (g *JSONKeys) Describe() string {
	return strings.Join(g.Keys, ", ") + " in " + Tildify(g.Path)
}

func (g *JSONKeys) Snapshot() error {
	b, ok, err := readOptional(g.Path)
	if err != nil {
		return err
	}
	g.existed, g.orig = ok, map[string]json.RawMessage{}
	if !ok {
		return nil
	}
	obj, err := parseObject(b)
	if err != nil {
		return fmt.Errorf("%s: %w", g.Path, err)
	}
	for _, k := range g.Keys {
		if v, ok := obj.get(k); ok {
			g.orig[k] = v
		}
	}
	return nil
}

func (g *JSONKeys) Restore() (bool, error) {
	if g.orig == nil {
		return false, nil // Snapshot failed or wasn't called
	}
	b, ok, err := readOptional(g.Path)
	if err != nil || !ok {
		return false, err // file gone: nothing of ours to undo
	}
	obj, err := parseObject(b)
	if err != nil {
		return false, nil // user left it mid-edit / invalid; don't touch
	}
	changed := false
	for _, k := range g.Keys {
		cur, hasCur := obj.get(k)
		orig, hasOrig := g.orig[k]
		switch {
		case hasOrig && (!hasCur || !jsonEqual(cur, orig)):
			obj.set(k, orig)
			changed = true
		case !hasOrig && hasCur:
			obj.del(k)
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if !g.existed && len(obj.keys) == 0 {
		return true, os.Remove(g.Path)
	}
	return true, writeKeepMode(g.Path, obj.marshal())
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(a, b)
	}
	return reflect.DeepEqual(x, y)
}

// object is a top-level JSON object that keeps key order and raw member values.
type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func parseObject(b []byte) (*object, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	o := &object{vals: map[string]json.RawMessage{}}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		k, _ := t.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		if _, dup := o.vals[k]; !dup {
			o.keys = append(o.keys, k)
		}
		o.vals[k] = v
	}
	return o, nil
}

func (o *object) get(k string) (json.RawMessage, bool) { v, ok := o.vals[k]; return v, ok }

func (o *object) set(k string, v json.RawMessage) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *object) del(k string) {
	delete(o.vals, k)
	for i, x := range o.keys {
		if x == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

// marshal writes 2-space-indented JSON with a trailing newline, matching Claude Code's own output.
func (o *object) marshal() []byte {
	var b bytes.Buffer
	b.WriteString("{")
	for i, k := range o.keys {
		if i > 0 {
			b.WriteString(",")
		}
		kb, _ := json.Marshal(k)
		var v bytes.Buffer
		if json.Indent(&v, o.vals[k], "  ", "  ") != nil {
			v.Write(o.vals[k])
		}
		fmt.Fprintf(&b, "\n  %s: %s", kb, v.Bytes())
	}
	if len(o.keys) > 0 {
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

// ---------------------------------------------------------------- TOML (keys in any table)

// TOMLKeys guards `key = value` lines for the given bare key names, in the root table and in
// any [table] (Codex writes into [profiles.<name>] when a profile is active). It edits lines
// in place instead of re-serializing, so comments and formatting survive.
type TOMLKeys struct {
	Path string
	Keys []string

	existed bool
	orig    map[[2]string]string // {table, key} -> original line
	snapped bool
}

func (g *TOMLKeys) Describe() string {
	return strings.Join(g.Keys, ", ") + " in " + Tildify(g.Path)
}

var (
	tomlHeader = regexp.MustCompile(`^\s*\[\[?\s*([^\]]+?)\s*\]\]?\s*(#.*)?$`)
	tomlKV     = regexp.MustCompile(`^\s*([A-Za-z0-9_-]+)\s*=`)
)

type tomlLine struct {
	table, key string // key == "" for non-guarded lines
}

func (g *TOMLKeys) scan(lines []string) []tomlLine {
	out := make([]tomlLine, len(lines))
	table := ""
	for i, l := range lines {
		if m := tomlHeader.FindStringSubmatch(l); m != nil {
			table = m[1]
			out[i] = tomlLine{table: table}
			continue
		}
		out[i] = tomlLine{table: table}
		if m := tomlKV.FindStringSubmatch(l); m != nil {
			for _, k := range g.Keys {
				if m[1] == k {
					out[i].key = k
				}
			}
		}
	}
	return out
}

func splitLines(b []byte) []string {
	s := strings.TrimSuffix(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (g *TOMLKeys) Snapshot() error {
	b, ok, err := readOptional(g.Path)
	if err != nil {
		return err
	}
	g.existed, g.orig, g.snapped = ok, map[[2]string]string{}, true
	lines := splitLines(b)
	for i, info := range g.scan(lines) {
		if info.key != "" {
			g.orig[[2]string{info.table, info.key}] = lines[i]
		}
	}
	return nil
}

func (g *TOMLKeys) Restore() (bool, error) {
	if !g.snapped {
		return false, nil
	}
	b, ok, err := readOptional(g.Path)
	if err != nil || !ok {
		return false, err
	}
	lines := splitLines(b)
	info := g.scan(lines)
	seen := map[[2]string]bool{}
	changed := false
	var out []string
	for i, l := range lines {
		if info[i].key == "" {
			out = append(out, l)
			continue
		}
		id := [2]string{info[i].table, info[i].key}
		seen[id] = true
		orig, had := g.orig[id]
		switch {
		case !had: // added during the session → drop
			changed = true
		case orig != l:
			out = append(out, orig)
			changed = true
		default:
			out = append(out, l)
		}
	}
	// Keys removed during the session: put them back at the end of their table's key block.
	for id, line := range g.orig {
		if seen[id] {
			continue
		}
		out = insertInTable(out, id[0], line)
		changed = true
	}
	if !changed {
		return false, nil
	}
	if !g.existed && strings.TrimSpace(strings.Join(out, "")) == "" {
		return true, os.Remove(g.Path)
	}
	return true, writeKeepMode(g.Path, []byte(strings.Join(out, "\n")+"\n"))
}

// insertInTable puts line right after the header of table ("" = root, i.e. top of file).
func insertInTable(lines []string, table, line string) []string {
	at := 0
	if table != "" {
		at = -1
		for i, l := range lines {
			if m := tomlHeader.FindStringSubmatch(l); m != nil && m[1] == table {
				at = i + 1
				break
			}
		}
		if at < 0 { // table vanished: recreate it
			return append(lines, "", "["+table+"]", line)
		}
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, line)
	return append(out, lines[at:]...)
}
