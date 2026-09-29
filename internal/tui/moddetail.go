package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/util"
)

func (m model) updateModDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenMods

	case "u":
		// TODO: hook up to an actual download + install api

	case "up", "k":
		if m.modpackModsCursor > 0 {
			m.modpackModsCursor--
		}

	case "down", "j":
		if m.modpackModsCursor < len(m.modpackMods)-1 {
			m.modpackModsCursor++
		}

	case "enter", " ":
		if len(m.modpackMods) > 0 {
			m.selectedModpackMod = m.modpackMods[m.modpackModsCursor]
			m.screen = screenModpackModDetail
		}
	}

	return m, nil
}

func (m model) viewModDetail() string {
	mod := m.selectedMod

	s := fmt.Sprintf("%s\n\n", mod.Name)

	if mod.Title != "" {
		s += fmt.Sprintf("title:       %s\n", mod.Title)
	}
	if mod.Author != "" {
		s += fmt.Sprintf("author:      %s\n", mod.Author)
	}
	if mod.Description != "" {
		s += fmt.Sprintf("description: %s\n", mod.Description)
	}

	s += fmt.Sprintf("folder:      %s\n", mod.Dir)
	s += fmt.Sprintf("path:        %s\n", mod.Path)

	if !mod.IsModpack {
		if len(mod.Depends) > 0 {
			s += wrapLabeled("depends:     ", mod.Depends, m.width)
		}
		if len(mod.OptionalDepends) > 0 {
			s += wrapLabeled("optional:    ", mod.OptionalDepends, m.width)
		}
	} else {
		s += fmt.Sprintln("mods:")

		rows := m.listRows(10)
		if m.modHasUpdate(mod) {
			rows = rows - 2
		}
		start, end := listWindow(m.modpackModsCursor, len(m.modpackMods), rows)

		for i := start; i < end; i++ {
			mod := m.modpackMods[i]
			cursor := " "
			if m.modpackModsCursor == i {
				cursor = ">"
			}

			tag := "   " // regular, well-formed mod: no flag needed
			if !mod.ConfOK {
				tag = "[!]" // missing/malformed mod.conf, name is a folder-name guess
			}

			s += truncate(fmt.Sprintf("%s %s %s", cursor, tag, mod.Name), m.width) + "\n"
		}

		pad := rows - (end - start)
		if len(m.modpackMods) == 0 {
			s += "(no mods found in modpack)\n"
			pad--
		}

		if pad < 0 {
			pad = 0 // clamp pad to zero to prevent panic
		}
		s += strings.Repeat("\n", pad)
		if end-start < len(m.modpackMods) {
			s += fmt.Sprintf("(%d-%d of %d)", start+1, end, len(m.modpackMods))
		}
	}

	if m.modHasUpdate(mod) {
		s += "\n" + util.UpdateTagStyle.Render("Update available, press u to update") + "\n"
	}

	s += "\n(esc/backspace to go back, q to quit)"

	return s
}
