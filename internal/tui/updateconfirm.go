package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/update"
)

// Rows of the confirm screen, see confirmRows.
const (
	confirmRowDeps = iota
	confirmRowContinue
	confirmRowCancel
)

// modDepsResolvedMsg is delivered when resolveModDepsCmd finishes.
type modDepsResolvedMsg struct {
	index map[string]contentdb.Package // nil if it could not be fetched
	plan  update.DepPlan
	err   error
}

// resolveModDepsCmd plans the new dependencies of queue, counting the base game's mods as installed.
func (m model) resolveModDepsCmd(queue []update.ModUpdate) tea.Cmd {
	client := m.cdb
	engInfo := m.engInfo
	index := m.packageIndex
	game := m.modsGame

	return func() tea.Msg {
		mods, err := content.ScanMods(engInfo.ModsDir)
		if err != nil {
			return modDepsResolvedMsg{err: err}
		}
		installed := content.ProvidedModNames(mods)

		if game.ID != "" {
			if _, err := os.Stat(game.Path); err != nil {
				return modDepsResolvedMsg{err: fmt.Errorf("game not installed: %s", game.ID)}
			}

			gameMods, err := content.ScanMods(filepath.Join(game.Path, "mods"))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return modDepsResolvedMsg{err: fmt.Errorf("scanning mods of game %s: %w", game.ID, err)}
			}
			maps.Copy(installed, content.ProvidedModNames(gameMods))
		}

		if index == nil {
			index, err = update.FetchPackageIndex(client, engInfo)
			if err != nil {
				return modDepsResolvedMsg{err: fmt.Errorf("fetching package list: %w", err)}
			}
		}

		plan, err := update.ResolveModDeps(client, queue, installed, index, nil)
		return modDepsResolvedMsg{index: index, plan: plan, err: err}
	}
}

// handleModDepsResolved starts the run when nothing new is needed and resolving worked, else asks first.
func (m model) handleModDepsResolved(msg modDepsResolvedMsg) (tea.Model, tea.Cmd) {
	m.modsResolving = false
	m.modActionInProgress = false

	if msg.index != nil {
		m.packageIndex = msg.index
	}

	if msg.err == nil && len(msg.plan.Install) == 0 && len(msg.plan.NotFound) == 0 {
		return m.beginModRun(m.pendingUpdates, nil)
	}

	m.modsResolveErr = msg.err
	m.pendingDeps = msg.plan
	m.confirmInstallDeps = true
	m.confirmReturn = m.screen
	// on Cancel, so a key pressed before the screen appeared changes nothing
	m.confirmCursor = len(m.confirmRows()) - 1
	m.screen = screenUpdateConfirm

	return m, nil
}

// confirmRows lists the selectable rows, the checkbox only when there is something to install.
func (m model) confirmRows() []int {
	if len(m.pendingDeps.Install) == 0 {
		return []int{confirmRowContinue, confirmRowCancel}
	}
	return []int{confirmRowDeps, confirmRowContinue, confirmRowCancel}
}

func (m model) updateUpdateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.confirmRows()

	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		return m.cancelUpdateConfirm(), nil

	case "up", "k":
		if m.confirmCursor > 0 {
			m.confirmCursor--
		}

	case "down", "j":
		if m.confirmCursor < len(rows)-1 {
			m.confirmCursor++
		}

	case "enter", " ":
		switch rows[m.confirmCursor] {
		case confirmRowDeps:
			m.confirmInstallDeps = !m.confirmInstallDeps

		case confirmRowContinue:
			var deps []update.DepInstall
			if m.confirmInstallDeps {
				deps = m.pendingDeps.Install
			}
			m.screen = m.confirmReturn
			return m.beginModRun(m.pendingUpdates, deps)

		case confirmRowCancel:
			return m.cancelUpdateConfirm(), nil
		}
	}

	return m, nil
}

func (m model) cancelUpdateConfirm() model {
	m.screen = m.confirmReturn
	m.pendingUpdates = nil
	m.pendingDeps = update.DepPlan{}
	m.modsResolveErr = nil

	return m
}

func (m model) confirmRowLabel(row int) string {
	switch row {
	case confirmRowDeps:
		box := "[ ]"
		if m.confirmInstallDeps {
			box = "[x]"
		}
		return box + " Install missing dependencies"
	case confirmRowContinue:
		if m.modsResolveErr != nil {
			return "Update without dependencies"
		}
		return "Continue"
	default:
		return "Cancel"
	}
}

func (m model) viewUpdateConfirm() string {
	noun := "mods"
	if len(m.pendingUpdates) == 1 {
		noun = "mod"
	}
	s := fmt.Sprintf("update %d %s\n\n", len(m.pendingUpdates), noun)

	var body []string

	if m.modsResolveErr != nil {
		body = append(body, wrapText(fmt.Sprintf("could not resolve dependencies: %v", m.modsResolveErr), m.width)...)
		body = append(body, "")
	}

	if len(m.pendingDeps.Install) > 0 {
		body = append(body, "new dependencies:")
		for _, d := range m.pendingDeps.Install {
			body = append(body, fmt.Sprintf("  %s/%s %d (%s, needed by %s)",
				d.Package.Author, d.Package.Name, d.Package.Release, d.Dep, d.RequiredBy))
		}
		body = append(body, "")
	}

	if len(m.pendingDeps.NotFound) > 0 {
		body = append(body, "dependencies not found:")
		for _, d := range m.pendingDeps.NotFound {
			body = append(body, fmt.Sprintf("  %s (needed by %s)", d.Dep, d.RequiredBy))
		}
		body = append(body, "")
	}

	// last, so a short terminal cuts this list rather than the dependencies
	body = append(body, "mods to update:")
	for _, u := range m.pendingUpdates {
		current := "?"
		if u.Mod.Release > 0 {
			current = fmt.Sprint(u.Mod.Release)
		}
		body = append(body, fmt.Sprintf("  %s %s -> %d", u.Mod.Name, current, u.LatestRelease))
	}

	rows := m.confirmRows()
	space := m.listRows(5 + len(rows))
	if len(body) > space {
		hidden := len(body) - space + 1
		body = append(body[:space-1:space-1], fmt.Sprintf("  (%d more lines)", hidden))
	}

	for _, line := range body {
		s += truncate(line, m.width) + "\n"
	}
	s += "\n"

	for i, row := range rows {
		cursor := " "
		if m.confirmCursor == i {
			cursor = ">"
		}
		s += truncate(fmt.Sprintf("%s %s", cursor, m.confirmRowLabel(row)), m.width) + "\n"
	}

	s += "\n(up/down to move, enter to select, esc/backspace to cancel, q to quit)"

	return s
}
