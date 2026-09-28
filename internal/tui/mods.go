package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/update"
	"github.com/luanti-org/luma/internal/util"
)

// Command rows shown above the mod list on the mods screen, in cursor order.
const (
	modsCmdCheckUpdates = iota
	modsCmdUpdateAll
	modsRowCount // one past the last row, i.e. the row count
)

// modUpdatesCheckedMsg is delivered when checkModUpdatesCmd's ContentDB request finishes
type modUpdatesCheckedMsg struct {
	updates []update.ModUpdate
	err     error
}

// checkModUpdatesCmd runs the blocking ContentDB update
func (m model) checkModUpdatesCmd() tea.Cmd {
	mods := m.mods
	client := m.cdb
	engInfo := m.engInfo

	return func() tea.Msg {
		updates, err := update.CheckAllModUpdates(mods, client, engInfo)
		return modUpdatesCheckedMsg{updates: updates, err: err}
	}
}

func (m model) updateMods(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxCursor := 0
	if len(m.mods) > 0 {
		maxCursor = modsRowCount + len(m.mods) - 1
	}

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
		if m.modsCursor < maxCursor {
			m.modsCursor++
		}

	case "enter", " ":
		if m.modsErr != nil {
			break
		}

		if m.modsCursor < modsRowCount {
			switch m.modsCursor {
			case modsCmdCheckUpdates:
				if m.modsChecking {
					break
				}
				m.modsChecking = true
				m.modsUpdateErr = nil
				return m, m.checkModUpdatesCmd()

			case modsCmdUpdateAll:
				// TODO: apply all updates
			}
			break
		}

		if len(m.mods) > 0 {
			m.selectedMod = m.mods[m.modsCursor-modsRowCount]
			m.screen = screenModDetail
		}
	}

	return m, nil
}

// modHasUpdate reports whether mod was flagged by the last update check.
func (m model) modHasUpdate(mod content.Mod) bool {
	for _, u := range m.modUpdates {
		if u.Mod.Path == mod.Path {
			return true
		}
	}

	return false
}

// modsCommandLabel returns the label for a command row (< modsRowCount).
func (m model) modsCommandLabel(row int) string {
	switch row {
	case modsCmdCheckUpdates:
		return m.checkUpdatesLabel()
	case modsCmdUpdateAll:
		return "Update all (not implemented yet)"
	default:
		return ""
	}
}

func (m model) checkUpdatesLabel() string {
	switch {
	case m.modsChecking:
		return "Checking for updates..."
	case m.modsUpdateErr != nil:
		return fmt.Sprintf("Check for updates (last check failed: %v)", m.modsUpdateErr)
	case !m.modsLastChecked.IsZero():
		age := time.Since(m.modsLastChecked).Round(time.Second)
		return fmt.Sprintf("Check for updates [last checked: %s ago]", age)
	default:
		return "Check for updates"
	}
}

func (m model) viewMods() string {
	s := fmt.Sprintf("mods in %s\n\n", m.engInfo.ModsDir)

	if m.modsErr != nil {
		s += fmt.Sprintf("error: %v\n", m.modsErr)
		s += "\n(backspace to go back)\n"
		return s
	}

	for row := 0; row < modsRowCount; row++ {
		cursor := " "
		if m.modsCursor == row {
			cursor = ">"
		}
		s += truncate(fmt.Sprintf("%s %s", cursor, m.modsCommandLabel(row)), m.width) + "\n"
	}
	s += "\n"

	if len(m.mods) == 0 {
		s += "(no mods found)\n"
	}

	rows := m.listRows(7 + modsRowCount)
	listCursor := max(m.modsCursor-modsRowCount, 0)
	start, end := listWindow(listCursor, len(m.mods), rows)

	for i := start; i < end; i++ {
		mod := m.mods[i]
		cursor := " "
		if m.modsCursor == i+modsRowCount {
			cursor = ">"
		}

		updateTag := "   "
		if m.modHasUpdate(mod) {
			updateTag = "[U]" // update available
		}

		tag := "   " // regular, well-formed mod: no flag needed
		if mod.IsModpack {
			tag = "[P]" // modpack, not scanned further yet
			// } else if !mod.ConfOK {
			// 	tag = "[!]" // missing/malformed mod.conf, name is a folder-name guess
		}

		line := truncate(fmt.Sprintf("%s %s %s %s", cursor, updateTag, tag, mod.Name), m.width)
		if m.modHasUpdate(mod) {
			line = strings.Replace(line, "[U]", util.UpdateTagStyle.Render("[U]"), 1)
		}

		s += line + "\n"
	}

	pad := rows - (end - start)
	if len(m.mods) == 0 {
		pad-- // the "no ... found" line
	}

	s += strings.Repeat("\n", pad)
	if end-start < len(m.mods) {
		s += fmt.Sprintf("(%d-%d of %d)", start+1, end, len(m.mods))
	}

	s += "\n\n(P = modpack, U = update available)\n"
	s += "(up/down to move, enter to select, esc/backspace to go back, q to quit)"

	return s
}
