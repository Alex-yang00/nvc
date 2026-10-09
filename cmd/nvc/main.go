package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/x/term"

	"github.com/novitalabs/nvc/internal/auth"
	"github.com/novitalabs/nvc/internal/catalog"
	"github.com/novitalabs/nvc/internal/guard"
	"github.com/novitalabs/nvc/internal/harness"
	"github.com/novitalabs/nvc/internal/lineup"
	"github.com/novitalabs/nvc/internal/tui"
	"github.com/novitalabs/nvc/internal/uninstall"
)

var version = "0.0.0-dev" // set by -ldflags "-X main.version=..."

func baseURL() string {
	if u := os.Getenv("NOVITA_BASE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "https://api.novita.ai"
}

const usage = `nvc — run coding agents on Novita models, without touching your config

  nvc                               interactive menu (in a terminal)
  nvc login                         save your Novita API key
  nvc claude   [--model M] [args…]  Claude Code
  nvc codex    [--model M] [args…]  Codex
  nvc models                        live model list with prices
  nvc doctor                        check key, connectivity, installed agents
  nvc uninstall [--yes]             remove nvc and its saved key; agent configs are not touched
  nvc version

  --print-env   print what would be injected instead of launching
  Everything after the agent name (other than --model / --print-env) is passed through.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "nvc:", err)
		os.Exit(1)
	}
}

type opts struct {
	model    string
	printEnv bool
}

// takeFlags consumes nvc's own flags from the front of args.
func takeFlags(args []string, o *opts) ([]string, error) {
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--print-env":
			o.printEnv = true
			args = args[1:]
		case a == "--model" || a == "-m":
			if len(args) < 2 {
				return nil, errors.New("--model needs a value")
			}
			o.model, args = args[1], args[2:]
		case strings.HasPrefix(a, "--model="):
			o.model, args = strings.TrimPrefix(a, "--model="), args[1:]
		default:
			return args, nil
		}
	}
	return args, nil
}

func run(args []string) error {
	var o opts
	args, err := takeFlags(args, &o)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		if o.model == "" && !o.printEnv && isTerminal(os.Stdin) && isTerminal(os.Stdout) {
			return menu()
		}
		fmt.Print(usage)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	case "version", "--version":
		fmt.Println("nvc", version)
		return nil
	case "login":
		return login()
	case "models":
		return models()
	case "doctor":
		return doctor()
	case "uninstall":
		return uninstallCmd(rest)
	}
	h, ok := harness.Get(cmd)
	if !ok {
		return fmt.Errorf("unknown command %q (try `nvc help`)", cmd)
	}
	rest, err = takeFlags(rest, &o)
	if err != nil {
		return err
	}
	return launch(h, o, rest)
}

func launch(h harness.Harness, o opts, args []string) error {
	key, err := auth.Key()
	if err != nil {
		return err
	}
	// Refresh a stale cache synchronously (bounded by catalog.FetchTimeout); no background work.
	cat := catalog.Load(baseURL(), auth.ConfigDir(), true)
	if o.model != "" && cat != nil {
		if _, ok := cat.Get(o.model); !ok {
			return fmt.Errorf("model %q not found on Novita (see `nvc models`)", o.model)
		}
	}
	ctx := harness.Context{Key: key, BaseURL: baseURL(), Model: o.model, Version: version, Lineup: lineup.Default, Catalog: cat}
	p, err := h.Plan(ctx, args)
	if err != nil {
		return err
	}
	if o.printEnv {
		fmt.Print(harness.Render(p, auth.Mask))
		return nil
	}
	if harness.Detect(h) == "" {
		return fmt.Errorf("%s is not installed. Install it with:\n  %s", h.Title(), h.InstallHint())
	}
	fmt.Fprintln(os.Stderr, "✓", p.Summary)
	code, err := harness.Launch(p)
	if err != nil {
		return err
	}
	os.Exit(code)
	return nil
}

func login() error {
	fmt.Fprint(os.Stderr, "Paste your Novita API key (https://novita.ai/settings/key-management): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	key := strings.TrimSpace(line)
	if key == "" {
		return errors.New("empty key")
	}
	if err := auth.Validate(context.Background(), baseURL(), key, lineup.Default.Haiku); err != nil {
		return err
	}
	if err := auth.Save(key); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "✓ key saved to", auth.ConfigPath())
	return nil
}

func models() error {
	c, err := catalog.Refresh(context.Background(), baseURL(), auth.ConfigDir())
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tCONTEXT\t$/M IN\t$/M OUT\tAGENTS")
	for _, m := range c.Models {
		if m.ModelType != "chat" || !m.Supports("chat/completions") {
			continue
		}
		var agents []string
		if m.Supports("anthropic") {
			agents = append(agents, "claude")
		}
		if m.Supports("responses") {
			agents = append(agents, "codex")
		}
		fmt.Fprintf(w, "%s\t%dK\t%.2f\t%.2f\t%s\n", m.ID, m.ContextSize/1024, m.InputPrice(), m.OutputPrice(), strings.Join(agents, ","))
	}
	return w.Flush()
}

func doctor() error {
	ok := func(b bool) string {
		if b {
			return "✓"
		}
		return "✗"
	}
	key, err := auth.Key()
	fmt.Printf("%s API key %s\n", ok(err == nil), map[bool]string{true: auth.Mask(key), false: "missing — run `nvc login`"}[err == nil])
	if err == nil {
		verr := auth.Validate(context.Background(), baseURL(), key, lineup.Default.Haiku)
		fmt.Printf("%s Novita API %s %v\n", ok(verr == nil), baseURL(), orEmpty(verr))
	}
	for _, h := range harness.All() {
		p := harness.Detect(h)
		fmt.Printf("%s %-12s %s\n", ok(p != ""), h.Title(), map[bool]string{true: p, false: "not installed: " + h.InstallHint()}[p != ""])
	}
	if self, err := os.Executable(); err == nil {
		if p := harness.LookPathOther("nvc", self); p != "" {
			fmt.Printf("! another `nvc` on PATH: %s (e.g. the VHDL compiler) — check PATH order\n", p)
		}
	}
	return nil
}

func orEmpty(err error) string {
	if err == nil {
		return ""
	}
	return "— " + err.Error()
}

// isTerminal is a real isatty check (a char-device test would also accept /dev/null).
func isTerminal(f *os.File) bool { return term.IsTerminal(f.Fd()) }

func shortID(id string) string {
	if _, after, ok := strings.Cut(id, "/"); ok {
		return after
	}
	return id
}

// menu runs the interactive TUI, then launches whatever the user picked.
func menu() error {
	l := lineup.Default
	details := map[string]string{
		"claude": shortID(l.Opus) + " · " + shortID(l.Sonnet) + " · " + shortID(l.Haiku),
		"codex":  shortID(l.Default),
	}
	var agents []tui.Agent
	for _, h := range harness.All() {
		agents = append(agents, tui.Agent{
			Name: h.Name(), Title: h.Title(), Detail: details[h.Name()],
			Installed: harness.Detect(h) != "", InstallHint: h.InstallHint(),
		})
	}
	savePath := guard.Tildify(auth.ConfigPath())
	res, err := tui.Run(tui.Options{
		Version: version,
		Agents:  agents,
		Key: func() (string, bool) {
			k, err := auth.Key()
			return auth.Mask(k), err == nil
		},
		Validate: func(ctx context.Context, key string) error {
			return auth.Validate(ctx, baseURL(), key, lineup.Default.Haiku)
		},
		Save:     auth.Save,
		SavePath: savePath,
		NoMotion: os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || os.Getenv("NVC_NO_ANIMATION") != "",
	})
	if err != nil || res.Launch == "" {
		return err
	}
	h, _ := harness.Get(res.Launch)
	return launch(h, opts{}, nil)
}

func uninstallCmd(args []string) error {
	yes := false
	for _, a := range args {
		switch a {
		case "-y", "--yes":
			yes = true
		default:
			return fmt.Errorf("unknown flag %q (usage: nvc uninstall [--yes])", a)
		}
	}
	items := uninstall.Targets(uninstall.Paths{
		Binary:      uninstall.SelfBinary(),
		ConfigDir:   auth.ConfigDir(),
		ConfigFile:  auth.ConfigPath(),
		AgentConfig: []string{harness.ClaudeSettingsPath(), harness.CodexConfigPath()},
	})
	if len(items) == 0 {
		fmt.Println("Nothing to remove: nvc is not installed here.")
		return nil
	}

	fmt.Println("This removes:")
	for _, it := range items {
		fmt.Printf("  %-44s %s\n", guard.Tildify(it.Path), it.What)
	}
	fmt.Println("\nNot touched:")
	fmt.Printf("  %-44s your Claude Code config\n", guard.Tildify(harness.ClaudeSettingsPath()))
	fmt.Printf("  %-44s your Codex config\n", guard.Tildify(harness.CodexConfigPath()))
	fmt.Println("  claude / codex themselves, shell profile, session history")

	if !yes {
		if !isTerminal(os.Stdin) {
			return errors.New("not a terminal: re-run with --yes to confirm")
		}
		fmt.Print("\nProceed? [y/N] ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Println("Aborted, nothing removed.")
			return nil
		}
	}

	kept, errs := uninstall.Remove(items)
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "✗", e)
		if errors.Is(e, os.ErrPermission) {
			fmt.Fprintln(os.Stderr, "  (no permission — re-run with sudo, or remove it by hand)")
		}
	}
	for _, k := range kept {
		fmt.Printf("· kept %s (contains files nvc didn't create)\n", guard.Tildify(k))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d item(s) could not be removed", len(errs))
	}
	fmt.Println("\n✓ nvc removed. claude / codex work exactly as they did before nvc.")
	if os.Getenv(auth.EnvKey) != "" {
		fmt.Println("  NOVITA_API_KEY is still set in your environment (nvc never sets it) — remove it from your shell profile if you added it.")
	}
	fmt.Println("  Codex conversations started through nvc can't be resumed with plain `codex resume` (it starts a new one); history files are kept.")
	return nil
}
