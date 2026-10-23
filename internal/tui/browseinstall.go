package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/update"
	"github.com/luanti-org/luma/internal/util"
)

// Rows of the install dialog, in screen order.
const (
	browseDialogGame = iota
	browseDialogContinue
	browseDialogCancel
)

// browseRunStep is one package of an install run, dependencies come first.
type browseRunStep struct {
	pkg   contentdb.Package
	isDep bool
	dep   update.DepInstall
}

// browseResolvedMsg is delivered when resolveBrowseInstallCmd finishes.
type browseResolvedMsg struct {
	id       string                       // "author/name" of the mod resolved
	index    map[string]contentdb.Package // nil if not fetched
	hasDeps  bool                         // the mod has hard or optional dependencies
	plan     update.DepPlan
	optional []update.ModDep // not installed, nor a hard dependency
	err      error
}

// browseInstallStepMsg is delivered when one step of the install run finishes.
type browseInstallStepMsg struct {
	err error // the run was rolled back if set
}

func packageID(p contentdb.Package) string {
	return p.Author + "/" + p.Name
}

// browseInstallable reports whether i installs p from the detail screen.
func (m model) browseInstallable(p contentdb.Package) bool {
	switch p.Type {
	case "mod", "game", "txp":
		return m.browseInstalled.Tag(p) == ""
	}
	return false
}

// startBrowseInstall installs a game or texture pack right away, a mod once its dependencies are checked.
func (m model) startBrowseInstall(p contentdb.Package) (tea.Model, tea.Cmd) {
	if m.busy() || m.browseResolving || !m.browseInstallable(p) {
		return m, nil
	}

	m.browseInstallPkg = p
	m.browseInstallErr = nil
	m.browseRunQueue = nil
	m.browsePlan = update.DepPlan{}
	m.browseOptional = nil
	m.browseResolveErr = nil
	m.browseMissing = nil

	// fail before downloading dependencies, the install itself would refuse too
	dest, err := update.PackageDir(m.engInfo, p)
	if err == nil {
		if _, statErr := os.Stat(dest); statErr == nil {
			err = fmt.Errorf("folder %s already exists", dest)
		}
	}
	if err != nil {
		m.browseInstallErr = err
		return m, nil
	}

	if p.Type != "mod" {
		return m.beginBrowseRun([]browseRunStep{{pkg: p}})
	}

	m.browseResolving = true
	return m, m.resolveBrowseInstallCmd(p)
}

// resolveBrowseInstallCmd lists p's dependencies, and plans the hard ones if it has any,
// counting the base game's mods as installed.
func (m model) resolveBrowseInstallCmd(p contentdb.Package) tea.Cmd {
	client := m.cdb
	engInfo := m.engInfo
	index := m.packageIndex
	game := m.modsGame
	id := packageID(p)

	return func() tea.Msg {
		installed, err := installedModNames(engInfo, game)
		if err != nil {
			return browseResolvedMsg{id: id, err: err}
		}

		if index == nil {
			index, err = update.FetchPackageIndex(client, engInfo)
			if err != nil {
				return browseResolvedMsg{id: id, err: fmt.Errorf("fetching package list: %w", err)}
			}
		}

		deps, err := update.ListDeps(client, id, installed, index)
		if err != nil || len(deps) == 0 {
			return browseResolvedMsg{id: id, index: index, err: err}
		}

		// a modpack's mods may list one dependency as both hard and optional, or more than once
		skip := make(map[string]bool)
		for _, d := range deps {
			skip[d.Dep] = skip[d.Dep] || !d.Optional || d.Installed
		}
		var optional []update.ModDep
		for _, d := range deps {
			if !skip[d.Dep] {
				skip[d.Dep] = true
				optional = append(optional, d)
			}
		}

		plan, err := update.ResolveInstallDeps(client, []string{id}, nil, installed, index)
		return browseResolvedMsg{id: id, index: index, hasDeps: true, plan: plan, optional: optional, err: err}
	}
}

// browseInstallShown reports whether the screen shown is about the package being installed.
func (m model) browseInstallShown() bool {
	if m.screen != screenBrowseDetail && m.screen != screenBrowseInstall && m.screen != screenGamePicker {
		return false
	}
	return packageID(m.browseSelected) == packageID(m.browseInstallPkg)
}

// handleBrowseResolved installs a mod with no dependencies, else shows the install dialog.
// Nothing happens if the user left the package meanwhile.
func (m model) handleBrowseResolved(msg browseResolvedMsg) (tea.Model, tea.Cmd) {
	if msg.index != nil {
		m.packageIndex = msg.index
	}
	if msg.id != packageID(m.browseInstallPkg) {
		return m, nil
	}

	m.browseResolving = false
	if !m.browseInstallShown() {
		return m, nil
	}

	m.browseResolveErr = msg.err
	m.browsePlan = msg.plan
	m.browseOptional = msg.optional

	if msg.err == nil && !msg.hasDeps {
		m.screen = screenBrowseDetail
		return m.beginBrowseRun([]browseRunStep{{pkg: m.browseInstallPkg}})
	}

	if m.screen == screenBrowseDetail {
		m.screen = screenBrowseInstall
		// on Cancel, so a key pressed before the dialog appeared installs nothing
		m.browseDialogCursor = browseDialogCancel
	}
	return m, nil
}

func (m model) updateBrowseInstall(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenBrowseDetail

	case "up", "k":
		m.browseDialogCursor = max(m.browseDialogCursor-1, browseDialogGame)

	case "down", "j":
		m.browseDialogCursor = min(m.browseDialogCursor+1, browseDialogCancel)

	case "enter", " ":
		switch m.browseDialogCursor {
		case browseDialogGame:
			if !m.browseResolving {
				return m.openGamePicker(screenBrowseInstall), nil
			}

		case browseDialogContinue:
			if m.browseResolving || m.busy() {
				break
			}

			var steps []browseRunStep
			if m.browseResolveErr == nil {
				for _, d := range m.browsePlan.Install {
					steps = append(steps, browseRunStep{pkg: d.Package, isDep: true, dep: d})
				}
				m.browseMissing = m.browsePlan.NotFound
			}
			steps = append(steps, browseRunStep{pkg: m.browseInstallPkg})

			m.screen = screenBrowseDetail
			return m.beginBrowseRun(steps)

		case browseDialogCancel:
			m.screen = screenBrowseDetail
		}
	}

	return m, nil
}

// rerunBrowseResolve checks the dependencies again, after the base game changed.
func (m model) rerunBrowseResolve() (tea.Model, tea.Cmd) {
	m.browseResolving = true
	m.browseResolveErr = nil
	return m, m.resolveBrowseInstallCmd(m.browseInstallPkg)
}

func (m model) beginBrowseRun(steps []browseRunStep) (tea.Model, tea.Cmd) {
	m.browseInstalling = true
	m.browseRunQueue = steps
	m.browseRunIdx = 0
	m.browseRun = &update.InstallRun{}

	return m, m.browseInstallStepCmd()
}

// browseInstallStepCmd installs the next step, rolling back the whole run if it fails.
func (m model) browseInstallStepCmd() tea.Cmd {
	client := m.cdb
	engInfo := m.engInfo
	run := m.browseRun
	step := m.browseRunQueue[m.browseRunIdx]

	return func() tea.Msg {
		dest, err := update.PackageDir(engInfo, step.pkg)
		if err == nil {
			err = run.Step(dest, func() error {
				if step.isDep {
					return update.InstallDep(client, engInfo.ModsDir, step.dep)
				}
				return update.InstallPackage(client, engInfo, step.pkg)
			})
		}

		if err != nil {
			err = fmt.Errorf("%s: %w", packageID(step.pkg), err)
			return browseInstallStepMsg{err: errors.Join(err, run.Rollback())}
		}
		return browseInstallStepMsg{}
	}
}

func (m model) handleBrowseInstallStep(msg browseInstallStepMsg) (tea.Model, tea.Cmd) {
	m.browseRunIdx++
	if msg.err == nil && m.browseRunIdx < len(m.browseRunQueue) {
		return m, m.browseInstallStepCmd()
	}

	m.browseInstalling = false
	m.browseInstallErr = msg.err
	m.browseRun = nil
	m.browseInstalled = update.ScanInstalledReleases(m.engInfo)
	return m, nil
}

// browseInstallLines is the install progress or outcome for the package on the detail screen, if any.
func (m model) browseInstallLines() string {
	if packageID(m.browseInstallPkg) != packageID(m.browseSelected) {
		return ""
	}

	var lines []styledLine
	add := func(text string, style *lipgloss.Style) {
		for _, line := range wrapText(text, m.width) {
			lines = append(lines, styledLine{line, style})
		}
	}

	n := len(m.browseRunQueue)
	switch {
	case m.browseResolving:
		add("Checking dependencies...", nil)

	case m.browseInstalling && n > 1:
		add(fmt.Sprintf("Installing %d/%d...", m.browseRunIdx+1, n), nil)

	case m.browseInstalling:
		add("Installing...", nil)

	case m.browseInstallErr != nil:
		add(fmt.Sprintf("install failed: %v", m.browseInstallErr), &util.ErrorStyle)

	case n > 1:
		var ids []string
		for _, st := range m.browseRunQueue[:n-1] {
			ids = append(ids, packageID(st.pkg))
		}
		add("installed with dependencies: "+strings.Join(ids, ", "), &util.MutedStyle)
	}

	if !m.browseInstalling && m.browseInstallErr == nil && len(m.browseRunQueue) > 0 && len(m.browseMissing) > 0 {
		add("required dependencies still missing: "+strings.Join(missingNames(m.browseMissing), ", "), &util.WarnStyle)
	}

	var s string
	for _, l := range lines {
		s += l.render(m.width) + "\n"
	}
	return s
}

func missingNames(missing []update.MissingDep) []string {
	var names []string
	for _, d := range missing {
		names = append(names, d.Dep)
	}
	return names
}

// missingReason says why d won't be installed.
func missingReason(d update.MissingDep) string {
	switch {
	case d.Ambiguous:
		return "several packages provide it"
	case d.GameOnly:
		return "only games provide it"
	default:
		return "not found"
	}
}

func (m model) browseDialogLabel(row int) string {
	switch row {
	case browseDialogGame:
		return "Base game for dependencies: " + gameLabel(m.modsGame)
	case browseDialogContinue:
		if m.browseResolveErr != nil {
			return "Install without dependencies"
		}
		return "Continue"
	default:
		return "Cancel"
	}
}

func (m model) viewBrowseInstall() string {
	p := m.browseInstallPkg
	s := truncate(fmt.Sprintf("install %s (%s %d)", p.Title, packageID(p), p.Release), m.width) + "\n\n"

	cursor := func(row int) string {
		if m.browseDialogCursor == row {
			return ">"
		}
		return " "
	}

	s += truncate(cursor(browseDialogGame)+" "+m.browseDialogLabel(browseDialogGame), m.width) + "\n"
	s += "\n"

	var body []styledLine
	add := func(text string, style *lipgloss.Style) { body = append(body, styledLine{text, style}) }

	switch {
	case m.browseResolving:
		add("Checking dependencies...", nil)

	case m.browseResolveErr != nil:
		for _, line := range wrapText(fmt.Sprintf("could not resolve dependencies: %v", m.browseResolveErr), m.width) {
			add(line, &util.ErrorStyle)
		}

	default:
		add("to install:", nil)
		add(fmt.Sprintf("  %s %d", packageID(p), p.Release), nil)
		for _, d := range m.browsePlan.Install {
			add(fmt.Sprintf("  %s %d (%s, needed by %s)", packageID(d.Package), d.Package.Release, d.Dep, d.RequiredBy), nil)
		}

		if len(m.browsePlan.NotFound) > 0 {
			add("", nil)
			add("required dependencies not installed:", nil)
			for _, d := range m.browsePlan.NotFound {
				add(fmt.Sprintf("  %s (%s, needed by %s)", d.Dep, missingReason(d), d.RequiredBy), &util.WarnStyle)
			}
		}

		if len(m.browseOptional) > 0 {
			add("", nil)
			add("optional dependencies not installed:", nil)
			nameWidth := 0
			for _, d := range m.browseOptional {
				nameWidth = max(nameWidth, len(d.Dep))
			}
			for _, d := range m.browseOptional {
				status := depStatus(d)
				pkg := ""
				if d.Available && !d.Installed {
					pkg = fmt.Sprintf("%s %d", packageID(d.Package), d.Package.Release)
				}
				add(strings.TrimRight(fmt.Sprintf("  %-*s  %-9s  %s", nameWidth, d.Dep, status, pkg), " "), depStatusStyle(status))
			}
		}
	}

	help := helpLine(m.width, "up/down", "to move", "enter", "to select", "esc/backspace", "to cancel", "q", "to quit")
	space := m.listRows(9 + strings.Count(help, "\n"))
	if len(body) > space {
		hidden := len(body) - space + 1
		body = append(body[:space-1:space-1], styledLine{text: fmt.Sprintf("  (%d more lines)", hidden)})
	}
	for _, line := range body {
		s += line.render(m.width) + "\n"
	}
	s += "\n"

	for _, row := range []int{browseDialogContinue, browseDialogCancel} {
		s += truncate(cursor(row)+" "+m.browseDialogLabel(row), m.width) + "\n"
	}

	return s + "\n" + help
}
