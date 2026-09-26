package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
)

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

	case "enter":
		switch m.cursor {
		case 0:
			m.mods, m.modsErr = content.ScanMods(m.modsDir)
			m.screen = screenMods
		case 1:
			m.games, m.gamesErr = content.ScanGames(m.gamesDir)
			m.screen = screenGames
		case 2:
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) viewMenu() string {
	s := "luanti-contentdb-cli\n\n"

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %s\n", cursor, choice)
	}

	s += fmt.Sprintf("\nmods dir:  %s\n", m.modsDir)
	s += fmt.Sprintf("games dir: %s\n", m.gamesDir)
	s += "\n(up/down to move, enter to select, q to quit)\n"

	return s
}
