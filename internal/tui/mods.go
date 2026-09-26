package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateMods(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "backspace":
		m.screen = screenMenu

	case "up", "k":
		if m.modsCursor > 0 {
			m.modsCursor--
		}

	case "down", "j":
		if m.modsCursor < len(m.mods)-1 {
			m.modsCursor++
		}

	case "enter":
		if m.modsErr == nil && len(m.mods) > 0 {
			m.selectedMod = m.mods[m.modsCursor]
			m.screen = screenModDetail
		}
	}

	return m, nil
}

func (m model) viewMods() string {
	s := fmt.Sprintf("mods in %s\n\n", m.modsDir)

	if m.modsErr != nil {
		s += fmt.Sprintf("error: %v\n", m.modsErr)
		s += "\n(backspace to go back)\n"
		return s
	}

	if len(m.mods) == 0 {
		s += "(no mods found)\n"
	}

	for i, mod := range m.mods {
		cursor := " "
		if m.modsCursor == i {
			cursor = ">"
		}

		tag := "   " // regular, well-formed mod: no flag needed
		if mod.IsModpack {
			tag = "[P]" // modpack, not scanned further yet
		} else if !mod.ConfOK {
			tag = "[!]" // missing/malformed mod.conf, name is a folder-name guess
		}

		s += fmt.Sprintf("%s %s %s\n", cursor, tag, mod.Name)
	}

	s += "\n(P = modpack, ! = no valid mod.conf, name guessed from folder)\n"
	s += "(up/down to move, enter for details, backspace to go back)\n"

	return s
}
