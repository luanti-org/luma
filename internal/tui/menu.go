package tui

import (
	"fmt"
	"math/rand/v2"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/engine"
	"github.com/luanti-org/luma/internal/version"
)

var asciiBorder = lipgloss.Border{
	Top: "-", Bottom: "-", Left: "|", Right: "|",
	TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
}

var menuHeaderStyle = lipgloss.NewStyle().Border(asciiBorder).Padding(0, 1)

var (
	bulbLowStyle = lipgloss.NewStyle().Width(13).
			Foreground(lipgloss.AdaptiveColor{Light: "#8A7A2E", Dark: "#C8B145"})
	bulbOnStyle = lipgloss.NewStyle().Width(13).
			Foreground(lipgloss.AdaptiveColor{Light: "#B8860B", Dark: "#FFF27A"})
	bulbBaseStyle = lipgloss.NewStyle().Width(13).
			Foreground(lipgloss.AdaptiveColor{Light: "#5A5A5A", Dark: "#B0B0B0"})
)

const bulbLowGlass = `    .---.
   /     \
  |  ~~~  |
   \     /
    |   |`

const bulbOnGlass = ` /  .---.  \
   /     \
| |  ~~~  | |
   \     /
 \  |   |  /`

const bulbBase = `    |===|
    '---'`

type bulbTickMsg struct{}

func bulbTick(lo, hi int) tea.Cmd {
	d := time.Duration(lo+rand.IntN(hi-lo)) * time.Millisecond
	return tea.Tick(d, func(time.Time) tea.Msg { return bulbTickMsg{} })
}

// flickerBulb keeps the bulb mostly on, each dip to low stays under 0.5s even with a stutter.
func (m *model) flickerBulb() tea.Cmd {
	m.bulbOn = !m.bulbOn
	switch {
	case !m.bulbOn:
		return bulbTick(40, 160)
	case !m.bulbStuttered && rand.IntN(3) == 0:
		m.bulbStuttered = true
		return bulbTick(60, 150)
	}
	m.bulbStuttered = false
	return bulbTick(1500, 5000)
}

func bulb(on bool) string {
	glass := bulbLowStyle.Render(bulbLowGlass)
	if on {
		glass = bulbOnStyle.Render(bulbOnGlass)
	}
	return glass + "\n" + bulbBaseStyle.Render(bulbBase)
}

func (m model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.choices)-1 {
			m.cursor++
		}

	case "enter", " ":
		switch m.cursor {
		case 0:
			return m.openBrowse()
		case 1:
			if !m.modActionInProgress { // a folder may be mid-replace; the run rescans when done
				m.mods, m.modsErr = content.ScanMods(m.engInfo.ModsDir)
				// folders may have been removed since the last visit
				m.modsCursor = min(m.modsCursor, max(modsRowCount+len(m.mods)-1, 0))
			}
			m.screen = screenMods
		case 2:
			if !m.gameActionInProgress {
				m.games, m.gamesErr = content.ScanGames(m.engInfo.GamesDir)
				m.gamesCursor = min(m.gamesCursor, max(gamesRowCount+len(m.games)-1, 0))
			}
			m.screen = screenGames
		case 3:
			if !m.texturepackActionInProgress {
				m.texturepacks, m.texturepacksErr = content.ScanTextures(m.engInfo.TexturesDir)
				m.texturepacksCursor = min(m.texturepacksCursor, max(texturepacksRowCount+len(m.texturepacks)-1, 0))
			}
			m.screen = screenTexturepacks
		case 4:
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) viewMenu() string {
	title := menuHeaderStyle.Render(fmt.Sprintf("Luma : Luanti Package Manager\nversion: %s", version.String()))
	s := lipgloss.JoinHorizontal(lipgloss.Center, lipgloss.NewStyle().Padding(1, 4, 0).Render(bulb(m.bulbOn)), title) + "\n\n"

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %s\n", cursor, choice)
	}

	s += fmt.Sprintf("\nmods dir:  %s\n", m.engInfo.ModsDir)
	s += fmt.Sprintf("games dir: %s\n", m.engInfo.GamesDir)
	s += fmt.Sprintf("textures dir: %s\n", m.engInfo.TexturesDir)
	s += fmt.Sprintf("engine:    %s\n", formatEngineInfo(m.engInfo))
	if m.engWarn != nil {
		s += wrapLabeled("warning:   ", []string{m.engWarn.Error()}, m.width)
	}
	s += "\n" + helpLine(m.width, "up/down", "to move", "enter", "to select", "q", "to quit") + "\n"

	return s
}

func formatEngineInfo(info engine.Info) string {
	if info.Version == "" {
		return "unknown"
	}

	s := info.Version
	if info.Protocol > 0 {
		s += fmt.Sprintf(" (protocol %d)", info.Protocol)
	}

	if info.ViaFlatpak {
		s += " [via flatpak]"
	}

	return s
}
