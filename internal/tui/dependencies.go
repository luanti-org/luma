package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/update"
	"github.com/luanti-org/luma/internal/util"
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
// Those from mod.conf still get their packages from ContentDB.
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

		if index == nil {
			index, err = update.FetchPackageIndex(client, engInfo)
			if err != nil {
				return modDepsListedMsg{path: mod.Path, err: fmt.Errorf("fetching package list: %w", err)}
			}
		}

		// index keys are lowercase, and a package missing there would only 404
		if _, ok := index[strings.ToLower(mod.Author+"/"+mod.Name)]; mod.Author == "" || !ok {
			deps, err := update.ListLocalDeps(client, mod, installed, index)
			return modDepsListedMsg{path: mod.Path, index: index, deps: deps, local: true, err: err}
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
	// as RequiredBy names it, see update.ModDep
	root := m.depsMod.Author + "/" + m.depsMod.Name
	if m.depsLocal {
		root = m.depsMod.Name
	}

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

// depStatus is d's status column.
func depStatus(d update.ModDep) string {
	switch {
	case d.Installed:
		return "installed"
	case d.Available:
		return "available"
	case d.Ambiguous:
		return "ambiguous"
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
func (m model) depsResultLines() []styledLine {
	if m.depsPlanErr != nil {
		return []styledLine{{fmt.Sprintf("could not resolve dependencies: %v", m.depsPlanErr), &util.ErrorStyle}}
	}
	if !m.depsRunShown() || m.modsUpdating || len(m.modUpdateResults) == 0 {
		return nil
	}

	results := m.modUpdateResults
	failed := countModUpdateErrs(results)
	summary := styledLine{text: fmt.Sprintf("last run: %d/%d installed", len(results)-failed, len(results))}
	if failed > 0 {
		summary.style = &util.WarnStyle
	}

	lines := []styledLine{summary}
	for _, r := range results {
		if r.err != nil {
			text := fmt.Sprintf("  %s/%s: %v", r.step.dep.Package.Author, r.step.dep.Package.Name, r.err)
			lines = append(lines, styledLine{text, &util.ErrorStyle})
		}
	}

	return lines
}

// depStatusStyle colors a status from depStatus, nil for none.
func depStatusStyle(status string) *lipgloss.Style {
	switch status {
	case "available":
		return &util.AvailableStyle
	case "not found", "from game":
		return &util.MutedStyle
	case "ambiguous":
		return &util.WarnStyle
	default:
		return nil
	}
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
		for _, line := range wrapText(fmt.Sprintf("error: %v", m.depsErr), m.width) {
			s += util.ErrorStyle.Render(line) + "\n"
		}
		return s + "\n" + help
	case len(m.deps) == 0:
		return s + "(no dependencies)\n\n" + help
	}

	nameWidth := 0
	gameOnly, ambiguous := false, false
	for _, d := range m.deps {
		nameWidth = max(nameWidth, len(d.Dep))
		gameOnly = gameOnly || (!d.Installed && d.GameOnly)
		ambiguous = ambiguous || (!d.Installed && d.Ambiguous)
	}

	var lines []string
	statuses := make(map[int]string) // line index to its status, styled after truncation
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

		status := depStatus(d)
		statuses[len(lines)] = status
		line := fmt.Sprintf("%s %s %-*s  %-9s  %s", cursor, box, nameWidth, d.Dep, status, pkg)
		lines = append(lines, strings.TrimRight(line, " "))
	}

	var below []styledLine
	actions := m.depsActions()
	if !slices.Contains(actions, depsActInstall) {
		below = append(below, styledLine{text: "  (nothing to install from ContentDB)"})
	}
	for i, action := range actions {
		cursor := " "
		if m.depsCursor == len(m.deps)+i {
			cursor = ">"
		}
		below = append(below, styledLine{text: fmt.Sprintf("%s %s", cursor, m.depsActionLabel(action))})
	}
	if results := m.depsResultLines(); len(results) > 0 {
		below = append(below, styledLine{})
		below = append(below, results...)
	}

	chrome := 7 + len(below)
	if gameOnly {
		chrome++
	}
	if ambiguous {
		chrome++
	}
	rows := m.listRows(chrome)
	start, end := listWindow(cursorLine, len(lines), rows)

	statusCol := nameWidth + 8 // after cursor, box and the padded name
	for i := start; i < end; i++ {
		line := truncate(lines[i], m.width)
		if style := depStatusStyle(statuses[i]); style != nil && len(line) > statusCol {
			stop := min(statusCol+len(statuses[i]), len(line))
			line = line[:statusCol] + style.Render(line[statusCol:stop]) + line[stop:]
		}
		s += line + "\n"
	}

	s += strings.Repeat("\n", rows-(end-start))
	if end-start < len(lines) {
		s += fmt.Sprintf("(%d-%d of %d)", start+1, end, len(lines))
	}
	s += "\n\n"

	for _, line := range below {
		s += line.render(m.width) + "\n"
	}

	s += "\n"
	if gameOnly {
		s += "(from game: set the base game on the mods screen)\n"
	}
	if ambiguous {
		s += "(ambiguous: several packages provide it, install one manually)\n"
	}
	s += help

	return s
}
