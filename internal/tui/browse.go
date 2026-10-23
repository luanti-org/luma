package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	browseSearchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
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
	m.browseInstalled = update.ScanInstalledReleases(m.engInfo)

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

// browseList returns the packages the current type tab shows that match the search, installed first like the client.
func (m model) browseList() []contentdb.Package {
	pkgType := browseFilters[m.browseFilter].pkgType
	return m.browseInstalled.InstalledFirst(update.MatchPackages(m.browsePackages, m.browseQuery, pkgType))
}

// browseRows is how many packages fit on screen, each takes two lines.
func (m model) browseRows() int {
	chrome := 9 + strings.Count(m.browseHelp(), "\n")
	if m.browseSearchShown() {
		chrome += 2
	}
	return max(m.listRows(chrome)/2, 1)
}

func (m model) browseSearchShown() bool {
	return m.browseTyping || m.browseQuery != ""
}

func (m model) browseHelp() string {
	if m.browseTyping {
		return helpLine(m.width, "up/down/pgup/pgdown", "to move", "left/right/home/end", "to edit", "tab", "to filter", "enter", "to keep search", "ctrl+u", "to clear", "esc", "to cancel search")
	}
	back := "to go back"
	if m.browseQuery != "" {
		back = "to clear search"
	}
	return helpLine(m.width, "up/down/pgup/pgdown", "to move", "enter", "for details", "/", "to search", "tab or 1-4", "to filter", "r", "to refresh", "esc/backspace", back, "q", "to quit")
}

// setBrowseQuery changes the search, moving the cursor to the top if it changed.
func (m model) setBrowseQuery(query string) model {
	if query != m.browseQuery {
		m.browseQuery = query
		m.browseCursor = 0
	}
	return m
}

// updateBrowseSearch handles a key typed into the search line.
func (m model) updateBrowseSearch(msg tea.KeyMsg) model {
	query := []rune(m.browseQuery)
	pos := min(m.browseQueryPos, len(query))

	switch msg.String() {
	case "enter":
		m.browseTyping = false
	case "esc":
		m.browseTyping = false
		m.browseQueryPos = 0
		return m.setBrowseQuery("")
	case "ctrl+u":
		m.browseQueryPos = 0
		return m.setBrowseQuery("")
	case "left":
		m.browseQueryPos = max(pos-1, 0)
	case "right":
		m.browseQueryPos = min(pos+1, len(query))
	case "home":
		m.browseQueryPos = 0
	case "end":
		m.browseQueryPos = len(query)
	case "backspace":
		if pos > 0 {
			m.browseQueryPos = pos - 1
			m = m.setBrowseQuery(string(query[:pos-1]) + string(query[pos:]))
		}
	case "delete":
		if pos < len(query) {
			m = m.setBrowseQuery(string(query[:pos]) + string(query[pos+1:]))
		}
	default:
		if (msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace) && !msg.Alt {
			typed := []rune(strings.Map(func(r rune) rune {
				if !unicode.IsPrint(r) {
					return -1
				}
				return r
			}, string(msg.Runes)))
			m.browseQueryPos = pos + len(typed)
			m = m.setBrowseQuery(string(query[:pos]) + string(typed) + string(query[pos:]))
		}
	}
	return m
}

func (m model) setBrowseFilter(filter int) model {
	if filter != m.browseFilter {
		m.browseFilter = filter
		m.browseCursor = 0
	}
	return m
}

func (m model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.browseTyping {
		switch msg.String() {
		case "up", "down", "pgup", "pgdown", "tab", "shift+tab":
		default:
			return m.updateBrowseSearch(msg), nil
		}
	}

	last := max(len(m.browseList())-1, 0)

	switch key := msg.String(); key {
	case "q":
		return m, tea.Quit

	case "/":
		m.browseTyping = true
		m.browseQueryPos = len([]rune(m.browseQuery))

	case "esc", "backspace":
		if m.browseQuery != "" {
			m.browseQueryPos = 0
			m = m.setBrowseQuery("")
		} else {
			m.screen = screenMenu
		}

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

	if m.browseSearchShown() {
		s += m.viewBrowseSearch() + "\n\n"
	}

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
	if len(list) == 0 && m.browseQuery != "" {
		s += "(no matches)\n"
	} else if len(list) == 0 {
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

		tag := m.browseInstalled.Tag(p)
		if tag == "" {
			tag = "   "
		}

		head := fmt.Sprintf("%s %s %s", cursor, tag, p.Title)
		id := "  " + p.Author + "/" + p.Name
		line := head + id
		if showType {
			line += "  [" + browseTypeName(p.Type) + "]"
		}
		line = truncate(line, m.width)
		if r, h := []rune(line), len([]rune(head)); len(r) > h {
			end := min(len(r), h+len([]rune(id)))
			line = string(r[:h]) + browseDescStyle.Render(string(r[h:end])) + string(r[end:])
		}
		switch tag {
		case "[U]":
			line = strings.Replace(line, "[U]", browseUpdateStyle.Render("[U]"), 1)
		case "[I]":
			line = strings.Replace(line, "[I]", browseInstalledStyle.Render("[I]"), 1)
		}

		s += line + "\n"
		s += browseDescStyle.Render(truncate("        "+p.ShortDescription, m.width)) + "\n"
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

// viewBrowseSearch renders the search line, scrolled to keep the text cursor in view.
func (m model) viewBrowseSearch() string {
	query := []rune(m.browseQuery)
	pos := len(query)
	if m.browseTyping {
		pos = min(m.browseQueryPos, len(query))
	}

	start, end := 0, len(query)
	if room := m.width - len("search: "); m.width > 0 && room > 0 {
		start = max(pos-room+1, 0)
		end = min(end, start+room)
	}

	label := browseSearchStyle.Render("search: ")
	if !m.browseTyping {
		return label + string(query[start:end])
	}

	cursor := " "
	if pos < end {
		cursor = string(query[pos])
	}
	after := ""
	if pos+1 < end {
		after = string(query[pos+1 : end])
	}
	return label + browseSearchStyle.Render(string(query[start:pos])) +
		browseSearchStyle.Reverse(true).Render(cursor) + browseSearchStyle.Render(after)
}

func browseTypeName(pkgType string) string {
	if pkgType == "txp" {
		return "texture pack"
	}
	return pkgType
}
