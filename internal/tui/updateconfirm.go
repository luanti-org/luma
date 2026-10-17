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
	"github.com/luanti-org/luma/internal/engine"
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
		installed, err := installedModNames(engInfo, game)
		if err != nil {
			return modDepsResolvedMsg{err: err}
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

// installedModNames returns the mod names in the mods folder, plus those of game if set.
func installedModNames(engInfo engine.Info, game content.Game) (map[string]bool, error) {
	mods, err := content.ScanMods(engInfo.ModsDir)
	if err != nil {
		return nil, err
	}
	installed := content.ProvidedModNames(mods)

	if game.ID != "" {
		if _, err := os.Stat(game.Path); err != nil {
			return nil, fmt.Errorf("game not installed: %s", game.ID)
		}

		gameMods, err := content.ScanMods(filepath.Join(game.Path, "mods"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("scanning mods of game %s: %w", game.ID, err)
		}
		maps.Copy(installed, content.ProvidedModNames(gameMods))
	}

	return installed, nil
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

// confirmRows lists the selectable rows, the checkbox only for updates that would install something.
func (m model) confirmRows() []int {
	if len(m.pendingDeps.Install) == 0 || len(m.pendingUpdates) == 0 {
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
			if len(m.pendingUpdates) == 0 {
				return m.beginDepsInstall(deps)
			}
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
	installOnly := len(m.pendingUpdates) == 0

	verb, n, noun := "update", len(m.pendingUpdates), "mod"
	if installOnly {
		verb, n, noun = "install", len(m.pendingDeps.Install), "package"
	}
	if n != 1 {
		noun += "s"
	}
	s := fmt.Sprintf("%s %d %s\n\n", verb, n, noun)

	var body []string

	if m.modsResolveErr != nil {
		body = append(body, wrapText(fmt.Sprintf("could not resolve dependencies: %v", m.modsResolveErr), m.width)...)
		body = append(body, "")
	}

	if len(m.pendingDeps.Install) > 0 {
		heading := "new dependencies:"
		if installOnly {
			heading = "to install:"
		}
		body = append(body, heading)
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
	if !installOnly {
		body = append(body, "mods to update:")
	}
	for _, u := range m.pendingUpdates {
		current := "?"
		if u.Mod.Release > 0 {
			current = fmt.Sprint(u.Mod.Release)
		}
		body = append(body, fmt.Sprintf("  %s %s -> %d", u.Mod.Name, current, u.LatestRelease))
	}
	if n := len(body); n > 0 && body[n-1] == "" {
		body = body[:n-1]
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
