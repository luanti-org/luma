package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/update"
	"github.com/luanti-org/luma/internal/util"
)

// browseFilters are the type tabs on the browse screen, "" shows every type.
var browseFilters = []struct {
	pkgType string
	label   string
}{
	{"", "All"},
	{"mod", "Mods"},
	{"game", "Games"},
	{"txp", "Texture packs"},
}

var (
	browseTabStyle       = lipgloss.NewStyle().Reverse(true)
	browseInstalledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	browseUpdateStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	browseDescStyle      = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#B4B4B4"})
)

// browseFetchRetries is how many times a failed package list fetch is retried, waiting longer each time.
const (
	browseFetchRetries    = 3
	browseFetchRetryDelay = 2 * time.Second
)

// browseFetchedMsg is delivered when fetchBrowseCmd finishes.
type browseFetchedMsg struct {
	maxAge   time.Duration
	packages []contentdb.Package
	err      error
}

// browseRetryMsg is delivered when the wait before a retry is over.
type browseRetryMsg struct {
	maxAge time.Duration
}

// openBrowse shows the browse screen, fetching the package list on the first visit.
func (m model) openBrowse() (tea.Model, tea.Cmd) {
	m.screen = screenBrowse
	m.browseInstalled = m.scanInstalledReleases()

	if m.browsePackages != nil || m.browseLoading {
		return m, nil
	}
	return m.startBrowseFetch(update.PackageListMaxAge)
}

// startBrowseFetch fetches the package list, a cached one younger than maxAge is used instead.
func (m model) startBrowseFetch(maxAge time.Duration) (tea.Model, tea.Cmd) {
	m.browseLoading = true
	m.browseErr = nil
	m.browseRetry = 0
	return m, m.fetchBrowseCmd(maxAge)
}

// fetchBrowseCmd fetches the packages compatible with the engine, a cached list younger than maxAge is used instead.
func (m model) fetchBrowseCmd(maxAge time.Duration) tea.Cmd {
	client := m.cdb
	engInfo := m.engInfo

	return func() tea.Msg {
		packages, err := update.FetchPackages(client, engInfo, maxAge)
		return browseFetchedMsg{maxAge: maxAge, packages: packages, err: err}
	}
}

func (m model) handleBrowseFetched(msg browseFetchedMsg) (tea.Model, tea.Cmd) {
	m.browseErr = msg.err
	if msg.err != nil && m.browseRetry < browseFetchRetries {
		m.browseRetry++
		delay := time.Duration(m.browseRetry) * browseFetchRetryDelay
		return m, tea.Tick(delay, func(time.Time) tea.Msg { return browseRetryMsg{maxAge: msg.maxAge} })
	}

	m.browseLoading = false
	if msg.err == nil {
		// a JSON null is no packages, nil means not fetched yet
		m.browsePackages = append([]contentdb.Package{}, msg.packages...)
		m.browseCursor = min(m.browseCursor, max(len(m.browseList())-1, 0))
	}
	return m, nil
}

func (m model) handleBrowseRetry(msg browseRetryMsg) (tea.Model, tea.Cmd) {
	return m, m.fetchBrowseCmd(msg.maxAge)
}

// browseKey is how scanInstalledReleases keys installed content.
func browseKey(pkgType, author, name string) string {
	return pkgType + " " + strings.ToLower(author+"/"+name)
}

// scanInstalledReleases returns the release of each installed package that names its author.
// Scan errors only cost the installed markers, so they are ignored.
func (m model) scanInstalledReleases() map[string]int {
	installed := make(map[string]int)

	mods, _ := content.ScanMods(m.engInfo.ModsDir)
	for _, mod := range mods {
		if mod.Author != "" {
			installed[browseKey("mod", mod.Author, mod.Name)] = mod.Release
		}
	}

	games, _ := content.ScanGames(m.engInfo.GamesDir)
	for _, g := range games {
		if g.Author != "" {
			installed[browseKey("game", g.Author, g.ID)] = g.Release
		}
	}

	texturepacks, _ := content.ScanTextures(m.engInfo.TexturesDir)
	for _, tp := range texturepacks {
		if tp.Author != "" {
			installed[browseKey("txp", tp.Author, tp.Name)] = tp.Release
		}
	}

	return installed
}

// browseList returns the packages the current type tab shows, ordered like the client:
// installed first, those with an update before the rest, by title, then the others in ContentDB's order.
func (m model) browseList() []contentdb.Package {
	pkgType := browseFilters[m.browseFilter].pkgType

	var installed, rest []contentdb.Package
	for _, p := range m.browsePackages {
		switch {
		case pkgType != "" && p.Type != pkgType:
		case m.browseTag(p) != "":
			installed = append(installed, p)
		default:
			rest = append(rest, p)
		}
	}

	slices.SortStableFunc(installed, func(a, b contentdb.Package) int {
		if au, bu := m.browseTag(a) == "[U]", m.browseTag(b) == "[U]"; au != bu {
			if au {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Title, b.Title)
	})

	return append(installed, rest...)
}

// browseTag returns "[U]" for an installed package with a newer release, "[I]" for one without, else "".
func (m model) browseTag(p contentdb.Package) string {
	release, installed := m.browseInstalled[browseKey(p.Type, p.Author, p.Name)]
	switch {
	case installed && release < p.Release:
		return "[U]"
	case installed:
		return "[I]"
	}
	return ""
}

// browseRows is how many packages fit on screen, each takes two lines.
func (m model) browseRows() int {
	return max(m.listRows(9+strings.Count(m.browseHelp(), "\n"))/2, 1)
}

func (m model) browseHelp() string {
	return helpLine(m.width, "up/down/pgup/pgdown", "to move", "enter", "for details", "tab or 1-4", "to filter", "r", "to refresh", "esc/backspace", "to go back", "q", "to quit")
}

func (m model) setBrowseFilter(filter int) model {
	if filter != m.browseFilter {
		m.browseFilter = filter
		m.browseCursor = 0
	}
	return m
}

func (m model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	last := max(len(m.browseList())-1, 0)

	switch key := msg.String(); key {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenMenu

	case "up", "k":
		m.browseCursor = max(m.browseCursor-1, 0)

	case "down", "j":
		m.browseCursor = min(m.browseCursor+1, last)

	case "pgup":
		m.browseCursor = max(m.browseCursor-m.browseRows(), 0)

	case "pgdown":
		m.browseCursor = min(m.browseCursor+m.browseRows(), last)

	case "home", "g":
		m.browseCursor = 0

	case "end", "G":
		m.browseCursor = last

	case "tab", "right", "l":
		m = m.setBrowseFilter((m.browseFilter + 1) % len(browseFilters))

	case "shift+tab", "left", "h":
		m = m.setBrowseFilter((m.browseFilter + len(browseFilters) - 1) % len(browseFilters))

	case "1", "2", "3", "4":
		m = m.setBrowseFilter(int(key[0] - '1'))

	case "enter", " ":
		if list := m.browseList(); len(list) > 0 {
			return m.openBrowseDetail(list[m.browseCursor])
		}

	case "r":
		if !m.browseLoading {
			return m.startBrowseFetch(0)
		}
	}

	return m, nil
}

func (m model) viewBrowse() string {
	retrying := ""
	if m.browseLoading && m.browseRetry > 0 {
		retrying = fmt.Sprintf(" (retrying %d/%d)", m.browseRetry, browseFetchRetries)
	}

	s := "Browse ContentDB"
	if m.browseLoading && m.browsePackages != nil {
		s += " (refreshing...)" + retrying
	}
	s += "\n\n"

	var tabs []string
	for i, f := range browseFilters {
		label := fmt.Sprintf(" %d %s ", i+1, f.label)
		if i == m.browseFilter {
			label = browseTabStyle.Render(label)
		}
		tabs = append(tabs, label)
	}
	s += strings.Join(tabs, " ") + "\n\n"

	help := m.browseHelp()

	switch {
	case m.browsePackages == nil && m.browseLoading:
		s += "Loading..." + retrying + "\n"
		if m.browseErr != nil {
			s += util.MutedStyle.Render(truncate(fmt.Sprintf("last error: %v", m.browseErr), m.width)) + "\n"
		}
		return s + "\n" + help

	case m.browseErr != nil && !m.browseLoading:
		s += util.ErrorStyle.Render(truncate(fmt.Sprintf("error: %v", m.browseErr), m.width)) + "\n"
		return s + "\n" + help
	}

	list := m.browseList()
	if len(list) == 0 {
		s += "(no packages)\n"
	}

	rows := m.browseRows()
	start, end := listWindow(m.browseCursor, len(list), rows)
	showType := browseFilters[m.browseFilter].pkgType == ""

	for i := start; i < end; i++ {
		p := list[i]
		cursor := " "
		if i == m.browseCursor {
			cursor = ">"
		}

		tag := m.browseTag(p)
		if tag == "" {
			tag = "   "
		}

		line := fmt.Sprintf("%s %s %s  %s/%s", cursor, tag, p.Title, p.Author, p.Name)
		if showType {
			line += "  [" + browseTypeName(p.Type) + "]"
		}
		line = truncate(line, m.width)
		switch tag {
		case "[U]":
			line = strings.Replace(line, "[U]", browseUpdateStyle.Render("[U]"), 1)
		case "[I]":
			line = strings.Replace(line, "[I]", browseInstalledStyle.Render("[I]"), 1)
		}

		s += line + "\n"
		s += browseDescStyle.Render(truncate("      "+p.ShortDescription, m.width)) + "\n"
	}

	pad := 2 * (rows - (end - start))
	if len(list) == 0 {
		pad--
	}
	s += strings.Repeat("\n", max(pad, 0))
	if len(list) > 0 {
		s += fmt.Sprintf("(%d of %d)", m.browseCursor+1, len(list))
	}

	s += "\n\n(I = installed, U = update available)\n"
	s += help

	return s
}

func browseTypeName(pkgType string) string {
	if pkgType == "txp" {
		return "texture pack"
	}
	return pkgType
}
