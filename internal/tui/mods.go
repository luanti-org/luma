package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) updateMods(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenMenu

	case "up", "k":
		if m.modsCursor > 0 {
			m.modsCursor--
		}

	case "down", "j":
		if m.modsCursor < len(m.mods)-1 {
			m.modsCursor++
		}

	case "enter", " ":
		if m.modsErr == nil && len(m.mods) > 0 {
			m.selectedMod = m.mods[m.modsCursor]
			m.screen = screenModDetail
		}
	}

	return m, nil
}

func (m model) viewMods() string {
	s := fmt.Sprintf("mods in %s\n\n", m.engInfo.ModsDir)

	if m.modsErr != nil {
		s += fmt.Sprintf("error: %v\n", m.modsErr)
		s += "\n(backspace to go back)\n"
		return s
	}

	if len(m.mods) == 0 {
		s += "(no mods found)\n"
	}

	rows := m.listRows(6)
	start, end := listWindow(m.modsCursor, len(m.mods), rows)

	for i := start; i < end; i++ {
		mod := m.mods[i]
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

		s += truncate(fmt.Sprintf("%s %s %s", cursor, tag, mod.Name), m.width) + "\n"
	}

	pad := rows - (end - start)
	if len(m.mods) == 0 {
		pad-- // the "no ... found" line
	}

	s += strings.Repeat("\n", pad)
	if end-start < len(m.mods) {
		s += fmt.Sprintf("(%d-%d of %d)", start+1, end, len(m.mods))
	}

	s += "\n\n(P = modpack, ! = no valid mod.conf, name guessed from folder)\n"
	s += "(up/down to move, enter for details, esc/backspace to go back, q to quit)"

	return s
}
