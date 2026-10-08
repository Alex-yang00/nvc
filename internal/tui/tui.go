// Package tui is the interactive menu shown by a bare `nvc` in a terminal:
// pick Claude Code / Codex, log in, or quit. It only decides what to do; the
// caller launches the agent after the TUI has restored the terminal.
package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type Agent struct {
	Name, Title string
	Detail      string // e.g. "kimi-k3 · glm-5.3 · deepseek-v4.1-flash"
	Installed   bool
	InstallHint string
}

type Options struct {
	Version  string
	Agents   []Agent
	Key      func() (masked string, ok bool) // current key status
	Validate func(ctx context.Context, key string) error
	Save     func(key string) error
	SavePath string // shown after login
	NoMotion bool   // NO_COLOR / TERM=dumb / NVC_NO_ANIMATION: static rendering, no intro
}

// Result is the user's choice; Launch == "" means quit.
type Result struct{ Launch string }

func Run(o Options) (Result, error) {
	final, err := tea.NewProgram(newModel(o), tea.WithAltScreen()).Run()
	if err != nil {
		return Result{}, err
	}
	return Result{Launch: final.(model).launch}, nil
}

// ---------------------------------------------------------------- model

type itemKind int

const (
	kindAgent itemKind = iota
	kindLogin
	kindQuit
)

type item struct {
	kind  itemKind
	agent Agent
}

type screen int

const (
	screenMenu screen = iota
	screenLogin
	screenLaunching
)

type loginState int

const (
	loginTyping loginState = iota
	loginVerifying
	loginFailed
	loginSaved
)

const (
	fps            = 60 * time.Millisecond
	launchDuration = 360 * time.Millisecond
	savedPause     = 900 * time.Millisecond
	maxKeyLen      = 256
)

type (
	tickMsg     time.Time
	verifiedMsg struct{ err error }
)

type model struct {
	o      Options
	items  []item
	cursor int
	frame  int // animation frames since start
	width  int

	keyMasked string
	keyOK     bool
	notice    string // one-line message under the menu

	screen       screen
	input        string
	login        loginState
	loginErr     string
	savedAt      time.Time
	pendingAgent string // agent to start after a successful login
	launchStart  time.Time

	launch string // result
}

func newModel(o Options) model {
	m := model{o: o}
	for _, a := range o.Agents {
		m.items = append(m.items, item{kind: kindAgent, agent: a})
	}
	m.items = append(m.items, item{kind: kindLogin}, item{kind: kindQuit})
	m.refreshKey()
	if !m.keyOK {
		m.cursor = m.index(kindLogin)
	}
	if o.NoMotion {
		m.frame = 1 << 20 // skip the intro reveal
	}
	return m
}

func (m *model) refreshKey() { m.keyMasked, m.keyOK = m.o.Key() }

func (m model) index(k itemKind) int {
	for i, it := range m.items {
		if it.kind == k {
			return i
		}
	}
	return 0
}

func tick() tea.Cmd { return tea.Tick(fps, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func (m model) Init() tea.Cmd { return tick() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tickMsg:
		m.frame++
		now := time.Time(msg)
		switch {
		case m.screen == screenLaunching && now.Sub(m.launchStart) >= launchDuration:
			return m, tea.Quit
		case m.screen == screenLogin && m.login == loginSaved && now.Sub(m.savedAt) >= savedPause:
			m = m.afterLogin()
			if m.screen == screenLaunching {
				m.launchStart = now
			}
		}
		return m, tick()

	case verifiedMsg:
		if msg.err != nil {
			m.login, m.loginErr = loginFailed, msg.err.Error()
			return m, nil
		}
		m.login, m.savedAt = loginSaved, time.Now()
		m.refreshKey()
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.launch = ""
			return m, tea.Quit
		}
		switch m.screen {
		case screenLogin:
			return m.updateLogin(msg)
		case screenMenu:
			return m.updateMenu(msg)
		}
	}
	return m, nil
}

func (m model) updateMenu(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch k.String() {
	case "up", "k", "shift+tab":
		m.cursor = (m.cursor - 1 + len(m.items)) % len(m.items)
	case "down", "j", "tab":
		m.cursor = (m.cursor + 1) % len(m.items)
	case "q", "esc":
		return m, tea.Quit
	case "enter", " ":
		return m.choose(m.cursor)
	default:
		if s := k.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			if i := int(s[0] - '1'); i < len(m.items) {
				m.cursor = i
				return m.choose(i)
			}
		}
	}
	return m, nil
}

func (m model) choose(i int) (tea.Model, tea.Cmd) {
	it := m.items[i]
	switch it.kind {
	case kindQuit:
		return m, tea.Quit
	case kindLogin:
		m.openLogin("")
	case kindAgent:
		switch {
		case !it.agent.Installed:
			m.notice = it.agent.Title + " is not installed · " + it.agent.InstallHint
		case !m.keyOK:
			m.openLogin(it.agent.Name)
		default:
			m.startLaunch(it.agent.Name)
		}
	}
	return m, nil
}

func (m *model) openLogin(pending string) {
	m.screen, m.login, m.input, m.loginErr, m.pendingAgent = screenLogin, loginTyping, "", "", pending
}

func (m *model) startLaunch(name string) {
	m.screen, m.launch, m.launchStart = screenLaunching, name, time.Now()
	if m.o.NoMotion {
		m.launchStart = time.Time{} // quit on the next tick
	}
}

func (m model) afterLogin() model {
	m.input, m.login = "", loginTyping
	if m.pendingAgent != "" {
		m.startLaunch(m.pendingAgent)
		return m
	}
	m.screen, m.cursor = screenMenu, 0 // logged in: jump to the first agent
	return m
}

func (m model) updateLogin(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.login {
	case loginVerifying:
		return m, nil // wait for the result
	case loginSaved:
		if k.Type == tea.KeyEnter || k.Type == tea.KeyEsc {
			m = m.afterLogin()
		}
		return m, nil
	}
	switch k.Type {
	case tea.KeyEsc:
		m.screen, m.pendingAgent = screenMenu, ""
	case tea.KeyEnter:
		if m.input == "" {
			return m, nil
		}
		m.login, m.loginErr = loginVerifying, ""
		key, validate, save := m.input, m.o.Validate, m.o.Save
		return m, func() tea.Msg {
			if err := validate(context.Background(), key); err != nil {
				return verifiedMsg{err}
			}
			return verifiedMsg{save(key)}
		}
	case tea.KeyBackspace:
		if m.input != "" {
			m.input = m.input[:len(m.input)-1]
		}
		m.login = loginTyping
	case tea.KeyCtrlU:
		m.input, m.login = "", loginTyping
	case tea.KeyRunes, tea.KeySpace:
		// pasted keys often carry spaces / newlines
		add := strings.Map(func(r rune) rune {
			if r <= ' ' || r == 0x7f {
				return -1
			}
			return r
		}, string(k.Runes))
		if len(m.input)+len(add) <= maxKeyLen {
			m.input += add
		}
		m.login = loginTyping
	}
	return m, nil
}
