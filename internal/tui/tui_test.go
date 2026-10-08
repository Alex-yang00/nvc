package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeAuth struct {
	key       string
	validErr  error
	validated []string
}

func (f *fakeAuth) opts() Options {
	return Options{
		Version: "t",
		Agents: []Agent{
			{Name: "claude", Title: "Claude Code", Detail: "kimi-k3", Installed: true},
			{Name: "codex", Title: "Codex", Detail: "glm-5.3", Installed: false, InstallHint: "npm i -g @openai/codex"},
		},
		Key:      func() (string, bool) { return "sk_…" + f.key, f.key != "" },
		Validate: func(_ context.Context, k string) error { f.validated = append(f.validated, k); return f.validErr },
		Save:     func(k string) error { f.key = k; return nil },
		SavePath: "~/.config/nvc/config.json",
		NoMotion: true,
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func send(t *testing.T, m model, msgs ...tea.Msg) (model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(model)
	}
	return m, cmd
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// runCmd executes an async command (e.g. key validation) and feeds its result back.
func runCmd(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	m, _ = send(t, m, cmd())
	return m
}

func TestMenuNavigationWraps(t *testing.T) {
	m := newModel((&fakeAuth{key: "x"}).opts())
	if m.cursor != 0 {
		t.Fatalf("with a key the cursor starts on the first agent, got %d", m.cursor)
	}
	m, _ = send(t, m, key("up"))
	if m.items[m.cursor].kind != kindQuit {
		t.Fatal("up from top should wrap to Quit")
	}
	m, _ = send(t, m, key("down"), key("j"))
	if m.cursor != 1 {
		t.Fatalf("cursor = %d", m.cursor)
	}
}

func TestStartsOnLoginWithoutKey(t *testing.T) {
	m := newModel((&fakeAuth{}).opts())
	if m.items[m.cursor].kind != kindLogin {
		t.Fatal("without a key the cursor should start on Login")
	}
	if !strings.Contains(m.View(), "no API key yet") {
		t.Fatal("view should explain the missing key")
	}
}

func TestLaunchAgent(t *testing.T) {
	m := newModel((&fakeAuth{key: "x"}).opts())
	m, _ = send(t, m, key("enter"))
	if m.screen != screenLaunching || m.launch != "claude" {
		t.Fatalf("screen=%v launch=%q", m.screen, m.launch)
	}
	if !strings.Contains(m.View(), "Starting Claude Code") {
		t.Fatal("launch view missing")
	}
	m, cmd := send(t, m, tickMsg(time.Now()))
	if !isQuit(cmd) || m.launch != "claude" {
		t.Fatal("should quit with launch=claude after the launch animation")
	}
}

func TestNumberShortcutAndUninstalledAgent(t *testing.T) {
	m := newModel((&fakeAuth{key: "x"}).opts())
	m, _ = send(t, m, key("2"))
	if m.screen != screenMenu || m.launch != "" {
		t.Fatal("an uninstalled agent must not launch")
	}
	if !strings.Contains(m.notice, "npm i -g @openai/codex") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c", "4"} {
		m := newModel((&fakeAuth{key: "x"}).opts())
		m, cmd := send(t, m, key(k))
		if !isQuit(cmd) || m.launch != "" {
			t.Errorf("%q should quit without launching", k)
		}
	}
}

func TestLoginSuccessWithPastedKey(t *testing.T) {
	f := &fakeAuth{}
	m := newModel(f.opts())
	m, _ = send(t, m, key("enter")) // cursor is on Login
	if m.screen != screenLogin {
		t.Fatal("expected login screen")
	}
	// paste with surrounding whitespace / newline, then a typo fixed with backspace
	m, _ = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" sk_abc123\n"), Paste: true}, key("x"), key("backspace"))
	if m.input != "sk_abc123" {
		t.Fatalf("input = %q", m.input)
	}
	if strings.Contains(m.View(), "sk_abc123") {
		t.Fatal("key must be masked on screen")
	}
	m, cmd := send(t, m, key("enter"))
	if m.login != loginVerifying {
		t.Fatal("expected verifying state")
	}
	m = runCmd(t, m, cmd)
	if m.login != loginSaved || f.key != "sk_abc123" || !m.keyOK {
		t.Fatalf("login=%v saved=%q keyOK=%v", m.login, f.key, m.keyOK)
	}
	m, _ = send(t, m, key("enter"))
	if m.screen != screenMenu {
		t.Fatal("enter after save should return to the menu")
	}
}

func TestLoginFailureThenEsc(t *testing.T) {
	f := &fakeAuth{validErr: errors.New("key rejected by Novita (401)")}
	m := newModel(f.opts())
	m, _ = send(t, m, key("enter"), key("bad"))
	m, cmd := send(t, m, key("enter"))
	m = runCmd(t, m, cmd)
	if m.login != loginFailed || f.key != "" {
		t.Fatal("failed validation must not save")
	}
	if !strings.Contains(m.View(), "401") {
		t.Fatal("error should be shown")
	}
	m, _ = send(t, m, key("q")) // typing q in the field must not quit
	if m.screen != screenLogin || m.input != "badq" {
		t.Fatalf("q should be typed, input=%q", m.input)
	}
	m, _ = send(t, m, key("esc"))
	if m.screen != screenMenu {
		t.Fatal("esc should go back to the menu")
	}
}

func TestPickAgentWithoutKeyLogsInThenLaunches(t *testing.T) {
	f := &fakeAuth{}
	m := newModel(f.opts())
	m, _ = send(t, m, key("1"))
	if m.screen != screenLogin || m.pendingAgent != "claude" {
		t.Fatal("picking an agent without a key should open login")
	}
	if !strings.Contains(m.View(), "Login to start Claude Code") {
		t.Fatal("login title should mention the pending agent")
	}
	m, _ = send(t, m, key("sk_good"))
	m, cmd := send(t, m, key("enter"))
	m = runCmd(t, m, cmd)
	m, _ = send(t, m, key("enter"))
	if m.screen != screenLaunching || m.launch != "claude" {
		t.Fatalf("should launch claude after login, screen=%v launch=%q", m.screen, m.launch)
	}
}

func TestSavedLoginAutoContinues(t *testing.T) {
	f := &fakeAuth{}
	m := newModel(f.opts())
	m, _ = send(t, m, key("enter"), key("sk_good"))
	m, cmd := send(t, m, key("enter"))
	m = runCmd(t, m, cmd)
	m, _ = send(t, m, tickMsg(time.Now().Add(2*savedPause)))
	if m.screen != screenMenu || m.cursor != 0 {
		t.Fatalf("saved state should auto-return to the menu on the first agent, cursor=%d", m.cursor)
	}
}
