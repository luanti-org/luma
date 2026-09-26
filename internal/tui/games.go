package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateGames(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "backspace":
		m.screen = screenMenu

	case "up", "k":
		if m.gamesCursor > 0 {
			m.gamesCursor--
		}

	case "down", "j":
		if m.gamesCursor < len(m.games)-1 {
			m.gamesCursor++
		}

	case "enter", " ":
		if m.gamesErr == nil && len(m.games) > 0 {
			m.selectedGame = m.games[m.gamesCursor]
			m.screen = screenGameDetail
		}
	}

	return m, nil
}

func (m model) viewGames() string {
	s := fmt.Sprintf("games in %s\n\n", m.gamesDir)

	if m.gamesErr != nil {
		s += fmt.Sprintf("error: %v\n", m.gamesErr)
		s += "\n(backspace to go back)\n"
		return s
	}

	if len(m.games) == 0 {
		s += "(no games found)\n"
	}

	for i, game := range m.games {
		cursor := " "
		if m.gamesCursor == i {
			cursor = ">"
		}

		tag := "   " // regular, well-formed game: no flag needed
		if !game.ConfOK {
			tag = "[!]" // missing/malformed game.conf, title guessed from folder
		}

		s += fmt.Sprintf("%s %s %s\n", cursor, tag, game.Title)
	}

	s += "\n(! = no valid game.conf, title guessed from folder)\n"
	s += "(up/down to move, enter for details, backspace to go back)\n"

	return s
}
