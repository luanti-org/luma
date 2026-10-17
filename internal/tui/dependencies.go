package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/update"
)

// modDepsListedMsg is delivered when listModDepsCmd finishes.
type modDepsListedMsg struct {
	path  string                       // of the mod listed
	index map[string]contentdb.Package // nil if not fetched
	deps  []update.ModDep
	err   error
}

// openModDeps shows the dependencies screen for mod and starts listing them.
func (m model) openModDeps(mod content.Mod) (tea.Model, tea.Cmd) {
	m.depsMod = mod
	m.depsLoading = true
	m.depsErr = nil
	m.deps = nil
	m.depsCursor = 0
	m.screen = screenModDeps

	return m, m.listModDepsCmd(mod)
}

// listModDepsCmd lists mod's dependencies from ContentDB, or from its mod.conf when it has no author.
func (m model) listModDepsCmd(mod content.Mod) tea.Cmd {
	client := m.cdb
	engInfo := m.engInfo
	index := m.packageIndex
	game := m.modsGame

	return func() tea.Msg {
		installed, err := installedModNames(engInfo, game)
		if err != nil {
			return modDepsListedMsg{path: mod.Path, err: err}
		}

		if mod.Author == "" {
			return modDepsListedMsg{path: mod.Path, deps: update.LocalDeps(mod, installed)}
		}

		if index == nil {
			index, err = update.FetchPackageIndex(client, engInfo)
			if err != nil {
				return modDepsListedMsg{path: mod.Path, err: fmt.Errorf("fetching package list: %w", err)}
			}
		}

		deps, err := update.ListDeps(client, mod.Author+"/"+mod.Name, installed, index)
		return modDepsListedMsg{path: mod.Path, index: index, deps: deps, err: err}
	}
}

// handleModDepsListed shows the result, required dependencies first, unless another mod was opened since.
func (m model) handleModDepsListed(msg modDepsListedMsg) (tea.Model, tea.Cmd) {
	if msg.index != nil {
		m.packageIndex = msg.index
	}
	if msg.path != m.depsMod.Path {
		return m, nil
	}

	m.depsLoading = false
	m.depsErr = msg.err

	var required, optional []update.ModDep
	for _, d := range msg.deps {
		if d.Optional {
			optional = append(optional, d)
		} else {
			required = append(required, d)
		}
	}
	m.deps = append(required, optional...)

	return m, nil
}

func (m model) updateModDeps(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenModDetail

	case "up", "k":
		if m.depsCursor > 0 {
			m.depsCursor--
		}

	case "down", "j":
		if m.depsCursor < len(m.deps)-1 {
			m.depsCursor++
		}
	}

	return m, nil
}

// depStatus is d's status column, local when listed from mod.conf and so never looked up.
func depStatus(d update.ModDep, local bool) string {
	switch {
	case d.Installed:
		return "installed"
	case local:
		return "missing"
	case d.Available:
		return "available"
	case d.GameOnly:
		return "from game"
	default:
		return "not found"
	}
}

func (m model) viewModDeps() string {
	mod := m.depsMod
	local := mod.Author == ""

	s := fmt.Sprintf("dependencies of %s\n", mod.Name)
	switch {
	case local && mod.IsModpack:
		s += "(no author in modpack.conf, listed from its mods' mod.conf)\n\n"
	case local:
		s += "(no author in mod.conf, listed from mod.conf)\n\n"
	default:
		s += "(from ContentDB)\n\n"
	}

	help := "(up/down to move, esc/backspace to go back, q to quit)"

	switch {
	case m.depsLoading:
		return s + "Loading...\n\n" + help
	case m.depsErr != nil:
		return s + strings.Join(wrapText(fmt.Sprintf("error: %v", m.depsErr), m.width), "\n") + "\n\n" + help
	case len(m.deps) == 0:
		return s + "(no dependencies)\n\n" + help
	}

	nameWidth := 0
	gameOnly := false
	for _, d := range m.deps {
		nameWidth = max(nameWidth, len(d.Dep))
		gameOnly = gameOnly || (!d.Installed && d.GameOnly)
	}

	var lines []string
	cursorLine := 0
	for i, d := range m.deps {
		if i == 0 && !d.Optional {
			lines = append(lines, "required:")
		}
		if d.Optional && (i == 0 || !m.deps[i-1].Optional) {
			lines = append(lines, "optional:")
		}

		cursor := " "
		if m.depsCursor == i {
			cursor = ">"
			cursorLine = len(lines)
		}

		pkg := ""
		if d.Available && !d.Installed {
			pkg = fmt.Sprintf("%s/%s %d", d.Package.Author, d.Package.Name, d.Package.Release)
		}

		line := fmt.Sprintf("%s %-*s  %-9s  %s", cursor, nameWidth, d.Dep, depStatus(d, local), pkg)
		lines = append(lines, strings.TrimRight(line, " "))
	}

	chrome := 6
	if gameOnly {
		chrome++
	}
	rows := m.listRows(chrome)
	start, end := listWindow(cursorLine, len(lines), rows)

	for _, line := range lines[start:end] {
		s += truncate(line, m.width) + "\n"
	}

	s += strings.Repeat("\n", rows-(end-start))
	if end-start < len(lines) {
		s += fmt.Sprintf("(%d-%d of %d)", start+1, end, len(lines))
	}

	s += "\n\n"
	if gameOnly {
		s += "(from game: set the base game on the mods screen)\n"
	}
	s += help

	return s
}
