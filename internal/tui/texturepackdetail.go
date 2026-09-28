package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateTexturepackDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenTexturepacks
	}

	return m, nil
}

func (m model) viewTexturepackDetail() string {
	texturepack := m.selectedTexturepack

	s := fmt.Sprintf("%s\n\n", texturepack.Name)

	if texturepack.Title != "" {
		s += fmt.Sprintf("title:       %s\n", texturepack.Title)
	}
	if texturepack.Author != "" {
		s += fmt.Sprintf("author:      %s\n", texturepack.Author)
	}
	if texturepack.Description != "" {
		s += fmt.Sprintf("description: %s\n", texturepack.Description)
	}

	s += fmt.Sprintf("folder:      %s\n", texturepack.Dir)
	s += fmt.Sprintf("path:        %s\n", texturepack.Path)

	s += "\n(esc/backspace to go back, q to quit)\n"

	return s
}
