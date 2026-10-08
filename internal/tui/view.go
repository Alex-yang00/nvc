package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	green = [3]float64{0x23, 0xd1, 0x8b}
	blue  = [3]float64{0x3b, 0x82, 0xf6}

	cGreen = lipgloss.Color("#23d18b")
	cBlue  = lipgloss.Color("#3b82f6")
	cAmber = lipgloss.Color("#f5a524")
	cRed   = lipgloss.Color("#f05252")
	cDim   = lipgloss.Color("#7a7f87")
	cText  = lipgloss.Color("#e6e6e6")

	sDim   = lipgloss.NewStyle().Foreground(cDim)
	sText  = lipgloss.NewStyle().Foreground(cText)
	sBold  = lipgloss.NewStyle().Foreground(cText).Bold(true)
	sGreen = lipgloss.NewStyle().Foreground(cGreen)
	sAmber = lipgloss.NewStyle().Foreground(cAmber)
	sRed   = lipgloss.NewStyle().Foreground(cRed)
	sPad   = lipgloss.NewStyle().Padding(1, 3)
)

var logo = []string{
	"███╗   ██╗██╗   ██╗ ██████╗",
	"████╗  ██║██║   ██║██╔════╝",
	"██╔██╗ ██║██║   ██║██║     ",
	"██║╚██╗██║╚██╗ ██╔╝██║     ",
	"██║ ╚████║ ╚████╔╝ ╚██████╗",
	"╚═╝  ╚═══╝  ╚═══╝   ╚═════╝",
}

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func mix(a, b [3]float64, t float64) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
		int(a[0]+(b[0]-a[0])*t), int(a[1]+(b[1]-a[1])*t), int(a[2]+(b[2]-a[2])*t)))
}

// wave maps a column + time to [0,1] for a gradient that slowly drifts.
func wave(col int, phase float64) float64 {
	return 0.5 + 0.5*math.Sin(float64(col)*0.18-phase)
}

func gradient(s string, phase float64) string {
	var b strings.Builder
	for i, r := range []rune(s) {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(mix(green, blue, wave(i, phase))).Render(string(r)))
	}
	return b.String()
}

func (m model) phase() float64 {
	if m.o.NoMotion {
		return 0
	}
	return float64(m.frame) * 0.12
}

func (m model) renderLogo() string {
	if m.width > 0 && m.width < 40 {
		return gradient("nvc", m.phase())
	}
	reveal := m.frame * 2 // columns revealed so far (intro sweep)
	lines := make([]string, len(logo))
	for i, l := range logo {
		rs := []rune(l)
		if reveal < len(rs) {
			for j := reveal; j < len(rs); j++ {
				rs[j] = ' '
			}
		}
		lines[i] = gradient(string(rs), m.phase())
	}
	return strings.Join(lines, "\n")
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(m.renderLogo())
	b.WriteString("\n\n")
	sub := "  ·  v" + m.o.Version
	if m.width == 0 || m.width >= 96 {
		sub += "  ·  your agents, Novita models, zero config changes"
	}
	b.WriteString(sText.Render("Novita Connect") + sDim.Render(sub))
	b.WriteString("\n\n")

	switch m.screen {
	case screenLogin:
		b.WriteString(m.viewLogin())
	case screenLaunching:
		b.WriteString(m.viewLaunching())
	default:
		b.WriteString(m.viewMenu())
	}
	return sPad.Render(b.String())
}

func (m model) pulse() lipgloss.Color {
	if m.o.NoMotion {
		return cGreen
	}
	return mix(green, blue, 0.5+0.5*math.Sin(float64(m.frame)*0.15))
}

func (m model) viewMenu() string {
	var b strings.Builder
	if m.keyOK {
		b.WriteString(sGreen.Render("●") + sDim.Render(" API key ") + sText.Render(m.keyMasked))
	} else {
		b.WriteString(sAmber.Render("●") + sDim.Render(" no API key yet — choose ") + sText.Render("Login") + sDim.Render(" first"))
	}
	b.WriteString("\n\n")

	for i, it := range m.items {
		label, detail := "", ""
		switch it.kind {
		case kindAgent:
			label, detail = it.agent.Title, it.agent.Detail
			if !it.agent.Installed {
				detail = sAmber.Render("not installed")
			}
		case kindLogin:
			label = "Login"
			if m.keyOK {
				detail = "replace saved key"
			} else {
				detail = "paste your Novita API key"
			}
		case kindQuit:
			label = "Quit"
		}
		num := fmt.Sprintf("%d", i+1)
		if i == m.cursor {
			cur := lipgloss.NewStyle().Foreground(m.pulse()).Bold(true)
			b.WriteString(cur.Render(" ▸ ") + cur.Render(num+"  "+fmt.Sprintf("%-12s", label)) + "  " + sDim.Render(detail))
		} else {
			b.WriteString("   " + sDim.Render(num+"  ") + sText.Render(fmt.Sprintf("%-12s", label)) + "  " + sDim.Render(detail))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.notice != "" {
		b.WriteString(sAmber.Render("! "+m.notice) + "\n\n")
	}
	b.WriteString(sDim.Render("↑/↓ move · enter select · 1-9 jump · q quit"))
	return b.String()
}

func (m model) viewLaunching() string {
	title := m.launch
	for _, it := range m.items {
		if it.kind == kindAgent && it.agent.Name == m.launch {
			title = it.agent.Title
		}
	}
	const width = 28
	p := 1.0
	if !m.o.NoMotion && !m.launchStart.IsZero() {
		p = math.Min(1, float64(time.Since(m.launchStart))/float64(launchDuration))
	}
	n := int(p * width)
	bar := gradient(strings.Repeat("━", n), m.phase()) + sDim.Render(strings.Repeat("━", width-n))
	return sBold.Render("Starting "+title) + sDim.Render(" on Novita") + "\n\n" + bar
}

func (m model) viewLogin() string {
	var b strings.Builder
	title := "Login"
	if m.pendingAgent != "" {
		title = "Login to start " + m.pendingTitle()
	}
	b.WriteString(sBold.Render(title) + "\n")
	b.WriteString(sDim.Render("Paste your Novita API key · https://novita.ai/settings/key-management") + "\n\n")

	masked := strings.Repeat("•", min(len(m.input), 40))
	cursor := " "
	if m.login == loginTyping && (m.o.NoMotion || (m.frame/8)%2 == 0) {
		cursor = lipgloss.NewStyle().Foreground(m.pulse()).Render("▌")
	}
	count := ""
	if m.input != "" {
		count = sDim.Render(fmt.Sprintf("  %d chars", len(m.input)))
	}
	b.WriteString(sDim.Render("  key  ") + sText.Render(masked) + cursor + count + "\n\n")

	switch m.login {
	case loginVerifying:
		s := spinner[m.frame%len(spinner)]
		b.WriteString(lipgloss.NewStyle().Foreground(m.pulse()).Render(s) + sDim.Render(" verifying with Novita…"))
	case loginFailed:
		b.WriteString(sRed.Render("✗ "+m.loginErr) + "\n\n" + sDim.Render("edit and press enter to retry · ctrl+u clear · esc back"))
	case loginSaved:
		b.WriteString(sGreen.Render("✓ key verified") + sDim.Render(" · saved to "+m.o.SavePath))
	default:
		b.WriteString(sDim.Render("enter verify · ctrl+u clear · esc back"))
	}
	return b.String()
}

func (m model) pendingTitle() string {
	for _, it := range m.items {
		if it.kind == kindAgent && it.agent.Name == m.pendingAgent {
			return it.agent.Title
		}
	}
	return m.pendingAgent
}
