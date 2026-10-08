// Package harness adapts each coding agent to Novita without touching user config files.
// Each adapter only builds argv + env for a single process launch.
package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/novitalabs/nvc/internal/catalog"
	"github.com/novitalabs/nvc/internal/guard"
	"github.com/novitalabs/nvc/internal/lineup"
)

const SourceHeader = "X-Novita-Source"

type Context struct {
	Key     string
	BaseURL string // https://api.novita.ai
	Model   string // --model override, may be empty
	Version string
	Lineup  lineup.Lineup
	Catalog *catalog.Catalog // may be nil (offline)
}

func (c Context) Source(h string) string { return fmt.Sprintf("nvc/%s/%s", h, c.Version) }

// Spec resolves model limits for id.
func (c Context) Spec(id string) lineup.Spec { return lineup.Resolve(c.Catalog, id) }

// Plan is everything needed to start the agent.
type Plan struct {
	Binary  string
	Args    []string          // excluding argv[0]
	Env     map[string]string // added to / overriding the current environment
	Secrets []string          // values to mask in --print-env
	Summary string            // one-line banner
	Guards  []guard.Guard     // user-config keys the agent may persist; reverted after exit
}

type Harness interface {
	Name() string
	Title() string
	Binary() string
	InstallHint() string
	Plan(ctx Context, userArgs []string) (Plan, error)
}

var all = []Harness{Claude{}, Codex{}}

func All() []Harness { return all }

func Get(name string) (Harness, bool) {
	for _, h := range all {
		if h.Name() == name {
			return h, true
		}
	}
	return nil, false
}

// Detect returns the agent's resolved path, or "" if it is not installed.
func Detect(h Harness) string {
	p, err := exec.LookPath(h.Binary())
	if err != nil {
		return ""
	}
	return p
}

// MergedEnv returns os.Environ() with plan.Env applied (overrides replace, not duplicate).
func MergedEnv(p Plan) []string {
	out := make([]string, 0, len(os.Environ())+len(p.Env))
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := p.Env[k]; !ok {
			out = append(out, kv)
		}
	}
	for _, k := range sortedKeys(p.Env) {
		out = append(out, k+"="+p.Env[k])
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Render prints the plan as a copy-pastable shell snippet with secrets masked.
func Render(p Plan, mask func(string) string) string {
	var b strings.Builder
	hide := func(s string) string {
		for _, sec := range p.Secrets {
			if sec != "" {
				s = strings.ReplaceAll(s, sec, mask(sec))
			}
		}
		return s
	}
	for _, k := range sortedKeys(p.Env) {
		fmt.Fprintf(&b, "export %s=%s\n", k, shellQuote(hide(p.Env[k])))
	}
	b.WriteString(p.Binary)
	for _, a := range p.Args {
		b.WriteString(" " + shellQuote(hide(a)))
	}
	b.WriteString("\n")
	for _, g := range p.Guards {
		fmt.Fprintf(&b, "# after exit nvc restores: %s\n", g.Describe())
	}
	return b.String()
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`!*?[]{}()<>|&;#~=,") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// homeFile resolves <$envDir or ~/defaultDir>/name.
func homeFile(envDir, defaultDir, name string) string {
	if d := os.Getenv(envDir); d != "" {
		return filepath.Join(d, name)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, defaultDir, name)
}

func hasFlag(args []string, names ...string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		for _, n := range names {
			if a == n || strings.HasPrefix(a, n+"=") {
				return true
			}
		}
	}
	return false
}

// LookPathOther returns the first `name` on PATH that is not self, or "".
func LookPathOther(name, self string) string {
	selfReal, _ := filepath.EvalSymlinks(self)
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			continue
		}
		if real, _ := filepath.EvalSymlinks(p); real != selfReal {
			return p
		}
	}
	return ""
}
