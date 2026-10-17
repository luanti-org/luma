package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/update"
)

// Action rows shown below the list on the dependencies screen, see depsActions.
const (
	depsActInstall = iota
	depsActBack
)

// modDepsListedMsg is delivered when listModDepsCmd finishes.
type modDepsListedMsg struct {
	path  string                       // of the mod listed
	index map[string]contentdb.Package // nil if not fetched
	deps  []update.ModDep
	local bool // listed from mod.conf
	err   error
}

// modDepsPlannedMsg is delivered when planModDepsCmd finishes.
type modDepsPlannedMsg struct {
	root string // "author/name" of the mod the picks are dependencies of
	plan update.DepPlan
	err  error
}

// openModDeps shows the dependencies screen for mod and starts listing them.
func (m model) openModDeps(mod content.Mod) (tea.Model, tea.Cmd) {
	m.depsMod = mod
	m.depsLoading = true
	m.depsErr = nil
	m.depsPlanErr = nil
	m.deps = nil
	m.depsCursor = 0
	m.screen = screenModDeps

	return m, m.listModDepsCmd(mod)
}

// listModDepsCmd lists mod's dependencies from ContentDB, or from its mod.conf when ContentDB can't have it.
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
			return modDepsListedMsg{path: mod.Path, deps: update.LocalDeps(mod, installed), local: true}
		}

		if index == nil {
			index, err = update.FetchPackageIndex(client, engInfo)
			if err != nil {
				return modDepsListedMsg{path: mod.Path, err: fmt.Errorf("fetching package list: %w", err)}
			}
		}

		// index keys are lowercase, and a package missing there would only 404
		if _, ok := index[strings.ToLower(mod.Author+"/"+mod.Name)]; !ok {
			return modDepsListedMsg{path: mod.Path, index: index, deps: update.LocalDeps(mod, installed), local: true}
		}

		deps, err := update.ListDeps(client, mod.Author+"/"+mod.Name, installed, index)
		return modDepsListedMsg{path: mod.Path, index: index, deps: deps, err: err}
	}
}

// handleModDepsListed shows the result, required dependencies first, unless another mod was opened since.
// Missing required dependencies start picked.
func (m model) handleModDepsListed(msg modDepsListedMsg) (tea.Model, tea.Cmd) {
	if msg.index != nil {
		m.packageIndex = msg.index
	}
	if msg.path != m.depsMod.Path {
		return m, nil
	}

	m.depsLoading = false
	m.depsErr = msg.err
	m.depsLocal = msg.local

	var required, optional []update.ModDep
	for _, d := range msg.deps {
		if d.Optional {
			optional = append(optional, d)
		} else {
			required = append(required, d)
		}
	}
	m.deps = append(required, optional...)

	m.depsPicked = make(map[string]bool)
	for _, d := range required {
		if depPickable(d) {
			m.depsPicked[d.Dep] = true
		}
	}
	m.depsCursor = min(m.depsCursor, max(m.depsRowCount()-1, 0))

	return m, nil
}

// planModDepsCmd plans installing picks with their own hard dependencies.
func (m model) planModDepsCmd(picks []update.ModDep) tea.Cmd {
	client := m.cdb
	engInfo := m.engInfo
	index := m.packageIndex
	game := m.modsGame
	root := m.depsMod.Author + "/" + m.depsMod.Name

	return func() tea.Msg {
		installed, err := installedModNames(engInfo, game)
		if err != nil {
			return modDepsPlannedMsg{root: root, err: err}
		}

		plan, err := update.PlanDeps(client, picks, installed, index, nil)
		return modDepsPlannedMsg{root: root, plan: plan, err: err}
	}
}

// handleModDepsPlanned installs right away when the plan is just the picks, else asks first.
func (m model) handleModDepsPlanned(msg modDepsPlannedMsg) (tea.Model, tea.Cmd) {
	m.depsPlanning = false
	m.modActionInProgress = false

	if msg.err != nil {
		m.depsPlanErr = msg.err
		return m, nil
	}

	// picks are required by the mod itself, anything else was pulled in by a pick
	extra := slices.ContainsFunc(msg.plan.Install, func(d update.DepInstall) bool { return d.RequiredBy != msg.root })
	if !extra && len(msg.plan.NotFound) == 0 {
		return m.beginDepsInstall(msg.plan.Install)
	}

	m.pendingUpdates = nil
	m.pendingDeps = msg.plan
	m.modsResolveErr = nil
	m.confirmInstallDeps = true
	m.confirmReturn = m.screen
	m.confirmCursor = len(m.confirmRows()) - 1
	m.screen = screenUpdateConfirm

	return m, nil
}

// beginDepsInstall runs deps, with the results shown on the dependencies screen.
func (m model) beginDepsInstall(deps []update.DepInstall) (tea.Model, tea.Cmd) {
	m.depsRunPath = m.depsMod.Path
	return m.beginModRun(nil, deps)
}

// depPickable reports whether d can be picked for install.
func depPickable(d update.ModDep) bool {
	return d.Available && !d.Installed
}

// depsActions lists the action rows, Install only when something can be picked.
func (m model) depsActions() []int {
	if slices.ContainsFunc(m.deps, depPickable) {
		return []int{depsActInstall, depsActBack}
	}
	return []int{depsActBack}
}

// depsRowCount is how many rows the cursor moves over, none until the list is shown.
func (m model) depsRowCount() int {
	if m.depsLoading || m.depsErr != nil || len(m.deps) == 0 {
		return 0
	}
	return len(m.deps) + len(m.depsActions())
}

func (m model) depsPicks() []update.ModDep {
	var picks []update.ModDep
	for _, d := range m.deps {
		if depPickable(d) && m.depsPicked[d.Dep] {
			picks = append(picks, d)
		}
	}
	return picks
}

// depsRunShown reports whether the current or last run was started from this screen.
func (m model) depsRunShown() bool {
	return m.depsRunPath != "" && m.depsRunPath == m.depsMod.Path
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
		if m.depsCursor < m.depsRowCount()-1 {
			m.depsCursor++
		}

	case "enter", " ":
		if m.depsCursor >= m.depsRowCount() {
			break
		}

		if m.depsCursor < len(m.deps) {
			if d := m.deps[m.depsCursor]; depPickable(d) {
				m.depsPicked[d.Dep] = !m.depsPicked[d.Dep]
			}
			break
		}

		switch m.depsActions()[m.depsCursor-len(m.deps)] {
		case depsActInstall:
			picks := m.depsPicks()
			if m.busy() || len(picks) == 0 {
				break
			}
			m.modActionInProgress = true
			m.depsPlanning = true
			m.depsPlanErr = nil
			return m, m.planModDepsCmd(picks)

		case depsActBack:
			m.screen = screenModDetail
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

func (m model) depsActionLabel(action int) string {
	switch action {
	case depsActInstall:
		switch {
		case m.depsPlanning:
			return "Resolving dependencies..."
		case m.modsUpdating && m.depsRunShown():
			return fmt.Sprintf("Installing... (%d/%d)", m.modUpdateIdx, len(m.modUpdateQueue))
		default:
			return fmt.Sprintf("Install selected (%d)", len(m.depsPicks()))
		}
	default:
		return "Back"
	}
}

// depsResultLines describes the plan error or the last run from this screen, with its failures.
func (m model) depsResultLines() []string {
	if m.depsPlanErr != nil {
		return []string{fmt.Sprintf("could not resolve dependencies: %v", m.depsPlanErr)}
	}
	if !m.depsRunShown() || m.modsUpdating || len(m.modUpdateResults) == 0 {
		return nil
	}

	results := m.modUpdateResults
	lines := []string{fmt.Sprintf("last run: %d/%d installed", len(results)-countModUpdateErrs(results), len(results))}
	for _, r := range results {
		if r.err != nil {
			lines = append(lines, fmt.Sprintf("  %s/%s: %v", r.step.dep.Package.Author, r.step.dep.Package.Name, r.err))
		}
	}

	return lines
}

func (m model) viewModDeps() string {
	mod := m.depsMod
	local := m.depsLocal

	s := fmt.Sprintf("dependencies of %s\n", mod.Name)
	help := "(up/down to move, enter/space to toggle or select, esc/backspace to go back, q to quit)"

	if m.depsLoading {
		return s + "\nLoading...\n\n" + help
	}

	reason := "no compatible package on ContentDB"
	if mod.Author == "" {
		reason = "no author in " + mod.ConfName()
	}
	source := "mod.conf"
	if mod.IsModpack {
		source = "its mods' mod.conf"
	}
	if local {
		s += fmt.Sprintf("(%s, listed from %s)\n\n", reason, source)
	} else {
		s += "(from ContentDB)\n\n"
	}

	switch {
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
		}
		if i == min(m.depsCursor, len(m.deps)-1) {
			cursorLine = len(lines)
		}

		box := "   "
		if depPickable(d) {
			box = "[ ]"
			if m.depsPicked[d.Dep] {
				box = "[x]"
			}
		}

		pkg := ""
		if d.Available && !d.Installed {
			pkg = fmt.Sprintf("%s/%s %d", d.Package.Author, d.Package.Name, d.Package.Release)
		}

		line := fmt.Sprintf("%s %s %-*s  %-9s  %s", cursor, box, nameWidth, d.Dep, depStatus(d, local), pkg)
		lines = append(lines, strings.TrimRight(line, " "))
	}

	var below []string
	actions := m.depsActions()
	if !slices.Contains(actions, depsActInstall) {
		below = append(below, "  (nothing to install from ContentDB)")
	}
	for i, action := range actions {
		cursor := " "
		if m.depsCursor == len(m.deps)+i {
			cursor = ">"
		}
		below = append(below, fmt.Sprintf("%s %s", cursor, m.depsActionLabel(action)))
	}
	if results := m.depsResultLines(); len(results) > 0 {
		below = append(below, "")
		below = append(below, results...)
	}

	chrome := 7 + len(below)
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

	for _, line := range below {
		s += truncate(line, m.width) + "\n"
	}

	s += "\n"
	if gameOnly {
		s += "(from game: set the base game on the mods screen)\n"
	}
	s += help

	return s
}
