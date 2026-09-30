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

// modUpdateResult is one entry's outcome from an update run.
type modUpdateResult struct {
	mod update.ModUpdate
	err error
}

// modUpdateStepMsg is delivered when one mod's download+install finishes.
type modUpdateStepMsg struct {
	index int // position in m.modUpdateQueue that was just attempted
	err   error
}

// updateModStepCmd downloads and installs a single update,
// one mod at a time.
// Chaining these is what lets the UI show per-mod progress as each step completes
func (m model) updateModStepCmd(index int) tea.Cmd {
	client := m.cdb
	u := m.modUpdateQueue[index]

	return func() tea.Msg {
		err := update.UpdateMod(client, u)
		return modUpdateStepMsg{index: index, err: err}
	}
}

// handleModUpdateStep records one step's result and either
// kicks off the next one or, once done, ends the mod action.
func (m model) handleModUpdateStep(msg modUpdateStepMsg) (tea.Model, tea.Cmd) {
	m.modUpdateResults = append(m.modUpdateResults, modUpdateResult{
		mod: m.modUpdateQueue[msg.index],
		err: msg.err,
	})
	m.modUpdateIdx = msg.index + 1

	if m.modUpdateIdx < len(m.modUpdateQueue) {
		return m, m.updateModStepCmd(m.modUpdateIdx)
	}

	m.modsUpdating = false
	m.modActionInProgress = false

	var remaining []update.ModUpdate
	for _, u := range m.modUpdates {
		if !m.modUpdateSucceeded(u.Mod.Path) {
			remaining = append(remaining, u)
		}
	}
	m.modUpdates = remaining

	m.mods, m.modsErr = content.ScanMods(m.engInfo.ModsDir)
	m.modsCursor = min(m.modsCursor, max(modsRowCount+len(m.mods)-1, 0))
	for _, mod := range m.mods {
		if mod.Path == m.selectedMod.Path {
			m.selectedMod = mod // the detail screen may be open on it
		}
	}

	return m, nil
}

// modUpdateSucceeded reports whether the last run updated the mod at path.
func (m model) modUpdateSucceeded(path string) bool {
	for _, r := range m.modUpdateResults {
		if r.mod.Mod.Path == path && r.err == nil {
			return true
		}
	}

	return false
}

// startModUpdates kicks off a run over queue, unless a mod action is already running.
func (m model) startModUpdates(queue []update.ModUpdate) (tea.Model, tea.Cmd) {
	if m.modActionInProgress || len(queue) == 0 {
		return m, nil
	}

	m.modActionInProgress = true
	m.modsUpdating = true
	m.modUpdateQueue = queue
	m.modUpdateIdx = 0
	m.modUpdateResults = nil

	return m, m.updateModStepCmd(0)
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

	case "u":
		if m.modsCursor < modsRowCount || len(m.mods) == 0 {
			break
		}
		mod := m.mods[m.modsCursor-modsRowCount]
		for _, u := range m.modUpdates {
			if u.Mod.Path == mod.Path {
				return m.startModUpdates([]update.ModUpdate{u})
			}
		}

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
				if m.modActionInProgress {
					break
				}
				m.modActionInProgress = true
				m.modsChecking = true
				m.modsUpdateErr = nil
				return m, m.checkModUpdatesCmd()

			case modsCmdUpdateAll:
				queue := append([]update.ModUpdate(nil), m.modUpdates...)
				return m.startModUpdates(queue)
			}
			break
		}

		if len(m.mods) > 0 {
			m.selectedMod = m.mods[m.modsCursor-modsRowCount]
			m.modpackModsCursor = 0
			m.modpackMods = m.mods[m.modsCursor-modsRowCount].ModpackMods
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
		return m.updateAllLabel()
	default:
		return ""
	}
}

func (m model) updateAllLabel() string {
	if m.modsUpdating {
		return fmt.Sprintf("Updating mods... (%d/%d)", m.modUpdateIdx, len(m.modUpdateQueue))
	}

	label := "Update all (no updates available)"
	if len(m.modUpdates) > 0 {
		label = fmt.Sprintf("Update all (%d available)", len(m.modUpdates))
	}
	if len(m.modUpdateResults) > 0 {
		label += fmt.Sprintf(" [last run: %d/%d succeeded]",
			len(m.modUpdateResults)-countModUpdateErrs(m.modUpdateResults), len(m.modUpdateResults))
	}

	return label
}

func countModUpdateErrs(results []modUpdateResult) int {
	n := 0
	for _, r := range results {
		if r.err != nil {
			n++
		}
	}
	return n
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
	s += "(up/down to move, enter to select, u to update mod, esc/backspace to go back, q to quit)"

	return s
}
