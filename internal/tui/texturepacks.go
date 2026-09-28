package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateTexturepacks(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenMenu

	case "up", "k":
		if m.texturepacksCursor > 0 {
			m.texturepacksCursor--
		}

	case "down", "j":
		if m.texturepacksCursor < len(m.texturepacks)-1 {
			m.texturepacksCursor++
		}

	case "enter", " ":
		if m.texturepacksErr == nil && len(m.texturepacks) > 0 {
			m.selectedTexturepack = m.texturepacks[m.texturepacksCursor]
			m.screen = screenTexturepackDetail
		}
	}

	return m, nil
}

func (m model) viewTexturepacks() string {
	s := fmt.Sprintf("texture packs in %s\n\n", m.engInfo.TexturesDir)

	if m.texturepacksErr != nil {
		s += fmt.Sprintf("error: %v\n", m.texturepacksErr)
		s += "\n(backspace to go back)\n"
		return s
	}

	if len(m.texturepacks) == 0 {
		s += "(no texture packs found)\n"
	}

	rows := m.listRows(6)
	start, end := listWindow(m.texturepacksCursor, len(m.texturepacks), rows)

	for i := start; i < end; i++ {
		texturepack := m.texturepacks[i]
		cursor := " "
		if m.texturepacksCursor == i {
			cursor = ">"
		}

		tag := "   " // regular, well-formed texture pack: no flag needed
		if !texturepack.ConfOK {
			tag = "[!]" // missing/malformed texture_pack.conf, name is a folder-name guess
		}

		s += truncate(fmt.Sprintf("%s %s %s", cursor, tag, texturepack.Name), m.width) + "\n"
	}

	pad := rows - (end - start)
	if len(m.texturepacks) == 0 {
		pad-- // the "no ... found" line
	}

	s += strings.Repeat("\n", pad)
	if end-start < len(m.texturepacks) {
		s += fmt.Sprintf("(%d-%d of %d)", start+1, end, len(m.texturepacks))
	}

	s += "\n\n(! = no valid texture_pack.conf, name guessed from folder)\n"
	s += "(up/down to move, enter for details, esc/backspace to go back, q to quit)"

	return s
}
