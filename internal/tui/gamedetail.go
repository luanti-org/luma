package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateGameDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenGames
	}

	return m, nil
}

func (m model) viewGameDetail() string {
	game := m.selectedGame

	s := fmt.Sprintf("%s\n\n", game.Title)

	if game.Author != "" {
		s += fmt.Sprintf("author:      %s\n", game.Author)
	}
	if game.Description != "" {
		s += fmt.Sprintf("description: %s\n", game.Description)
	}

	s += fmt.Sprintf("folder:      %s\n", game.Dir)
	s += fmt.Sprintf("path:        %s\n", game.Path)

	s += "\n(esc/backspace to go back, q to quit)\n"

	return s
}
