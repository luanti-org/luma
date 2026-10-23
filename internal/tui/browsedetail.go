package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/util"
)

// browseDetailsMsg is delivered when fetchBrowseDetailsCmd finishes.
type browseDetailsMsg struct {
	id      string // "author/name" the details are for
	details *contentdb.PackageDetails
	err     error
}

// openBrowseDetail shows the detail screen for p and starts fetching its details.
func (m model) openBrowseDetail(p contentdb.Package) (tea.Model, tea.Cmd) {
	m.browseSelected = p
	m.browseDetails = nil
	m.browseDetailsErr = nil
	m.browseDetailsLoading = true
	m.browseDescOpen = false
	m.browseDescScroll = 0
	m.screen = screenBrowseDetail

	client := m.cdb
	return m, func() tea.Msg {
		details, err := client.PackageDetails(p.Author, p.Name)
		return browseDetailsMsg{id: p.Author + "/" + p.Name, details: details, err: err}
	}
}

func (m model) handleBrowseDetails(msg browseDetailsMsg) (tea.Model, tea.Cmd) {
	// the user may have opened another package since
	if msg.id != m.browseSelected.Author+"/"+m.browseSelected.Name {
		return m, nil
	}

	m.browseDetailsLoading = false
	m.browseDetails = msg.details
	m.browseDetailsErr = msg.err
	return m, nil
}

func (m model) updateBrowseDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenBrowse

	case "r":
		if m.browseDetailsErr != nil {
			return m.openBrowseDetail(m.browseSelected)
		}

	case "i":
		return m.startBrowseInstall(m.browseSelected)

	case "d":
		if m.browseDetails != nil {
			m.browseDescOpen = !m.browseDescOpen
			m.browseDescScroll = 0
		}

	case "up", "k":
		m.browseDescScroll = max(m.browseDescScroll-1, 0)

	case "down", "j":
		m.browseDescScroll = min(m.browseDescScroll+1, m.browseDescMaxScroll())

	case "pgup":
		m.browseDescScroll = max(m.browseDescScroll-m.browseDescRows(), 0)

	case "pgdown":
		m.browseDescScroll = min(m.browseDescScroll+m.browseDescRows(), m.browseDescMaxScroll())
	}

	return m, nil
}

// browseDescLines returns the long description as plain lines wrapped to the screen, nil while collapsed.
func (m model) browseDescLines() []string {
	if !m.browseDescOpen || m.browseDetails == nil {
		return nil
	}
	return markdownLines(m.browseDetails.LongDescription, m.width)
}

// browseDescRows is how many description lines fit below the rest of the detail screen.
func (m model) browseDescRows() int {
	return max(m.height-strings.Count(m.viewBrowseDetailTop(), "\n")-4, 1)
}

func (m model) browseDescMaxScroll() int {
	return max(len(m.browseDescLines())-m.browseDescRows(), 0)
}

var (
	mdImageRe   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdLinkRe    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdHTMLRe    = regexp.MustCompile(`<[^>]+>`)
	mdHeadingRe = regexp.MustCompile(`^\s*#+\s*`)
	mdEmphRe    = regexp.MustCompile("\\*\\*|__|`")
)

// markdownLines strips the markdown that reads badly as plain text and wraps each line to width.
// Runs of blank lines become one.
func markdownLines(text string, width int) []string {
	var lines []string
	blank := true
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = mdImageRe.ReplaceAllString(line, "")
		line = mdLinkRe.ReplaceAllString(line, "$1")
		line = mdHTMLRe.ReplaceAllString(line, "")
		line = mdHeadingRe.ReplaceAllString(line, "")
		line = mdEmphRe.ReplaceAllString(line, "")

		wrapped := wrapText(line, width)
		if len(wrapped) == 0 {
			if !blank {
				lines = append(lines, "")
			}
			blank = true
			continue
		}
		lines = append(lines, wrapped...)
		blank = false
	}

	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// browseStatus describes whether p is installed, and at which release.
func (m model) browseStatus(p contentdb.Package) string {
	release, installed := m.browseInstalled[browseKey(p.Type, p.Author, p.Name)]
	switch {
	case !installed:
		return "not installed"
	case release < p.Release:
		return fmt.Sprintf("installed release %d, release %d available", release, p.Release)
	default:
		return fmt.Sprintf("installed, up to date (release %d)", release)
	}
}

func (m model) viewBrowseDetail() string {
	s := m.viewBrowseDetailTop()

	switch {
	case m.browseDetailsLoading:
		return s + "\nLoading...\n\n" + m.browseDetailHelp("esc/backspace to go back, q to quit")

	case m.browseDetailsErr != nil:
		s += "\n" + util.ErrorStyle.Render(truncate(fmt.Sprintf("error: %v", m.browseDetailsErr), m.width)) + "\n"
		return s + "\n" + m.browseDetailHelp("r to retry, esc/backspace to go back, q to quit")
	}

	s += "\n"
	switch {
	case strings.TrimSpace(m.browseDetails.LongDescription) == "":
		s += "description: (none)\n"
		return s + "\n" + m.browseDetailHelp("esc/backspace to go back, q to quit")

	case !m.browseDescOpen:
		s += "description: (d to expand)\n"
		return s + "\n" + m.browseDetailHelp("d to expand description, esc/backspace to go back, q to quit")
	}

	lines := m.browseDescLines()
	rows := m.browseDescRows()
	start := min(m.browseDescScroll, m.browseDescMaxScroll())
	end := min(start+rows, len(lines))
	s += fmt.Sprintf("description: (lines %d-%d of %d)\n", start+1, end, len(lines))
	for _, line := range lines[start:end] {
		s += browseDescStyle.Render(line) + "\n"
	}

	return s + "\n" + m.browseDetailHelp("up/down/pgup/pgdown to scroll, d to collapse, esc to go back, q to quit")
}

// browseDetailHelp is the help line with keys, plus the install key when it applies.
func (m model) browseDetailHelp(keys string) string {
	if m.browseInstallable(m.browseSelected) && !m.busy() && !m.browseResolving {
		keys = "i to install, " + keys
	}
	return "(" + keys + ")"
}

// viewBrowseDetailTop renders the detail screen down to the links, or to the status while there are no details.
func (m model) viewBrowseDetailTop() string {
	p := m.browseSelected

	s := fmt.Sprintf("%s\n%s/%s  [%s]\n\n", p.Title, p.Author, p.Name, browseTypeName(p.Type))
	for _, line := range wrapText(p.ShortDescription, m.width) {
		s += browseDescStyle.Render(line) + "\n"
	}
	s += "\n"

	status := m.browseStatus(p)
	switch m.browseTag(p) {
	case "[U]":
		status = browseUpdateStyle.Render(status)
	case "[I]":
		status = browseInstalledStyle.Render(status)
	}
	s += "status:      " + status + "\n"
	s += m.browseInstallLines()

	d := m.browseDetails
	if d == nil {
		return s
	}

	if len(d.ContentWarnings) > 0 {
		var warnings []string
		for _, w := range d.ContentWarnings {
			warnings = append(warnings, strings.ReplaceAll(w, "_", " "))
		}
		s += util.WarnStyle.Render(strings.TrimSuffix(wrapLabeled("warnings:    ", warnings, m.width), "\n")) + "\n"
	}

	s += fmt.Sprintf("release:     %d\n", p.Release)
	if d.License != "" {
		s += fmt.Sprintf("license:     %s\n", d.License)
	}
	if d.DevState != "" {
		s += fmt.Sprintf("state:       %s\n", strings.ReplaceAll(strings.ToLower(d.DevState), "_", " "))
	}
	s += fmt.Sprintf("downloads:   %d\n", d.Downloads)
	if len(d.Tags) > 0 {
		s += wrapLabeled("tags:        ", d.Tags, m.width)
	}
	if len(d.Maintainers) > 0 {
		s += wrapLabeled("maintainers: ", d.Maintainers, m.width)
	}
	if p.Type == "mod" {
		if games := supportedGames(d); len(games) > 0 {
			s += wrapLabeled("games:       ", games, m.width)
		}
	}

	s += "\n"
	s += fmt.Sprintf("page:        %s/packages/%s/%s/\n", strings.TrimSuffix(m.cdb.BaseURL, "/"), p.Author, p.Name)
	if d.Repo != "" {
		s += fmt.Sprintf("source:      %s\n", d.Repo)
	}

	return s
}

// supportedGames returns the titles of the games d says it supports, or "all games".
func supportedGames(d *contentdb.PackageDetails) []string {
	if d.SupportsAllGames {
		return []string{"all games"}
	}

	var games []string
	for _, g := range d.GameSupport {
		if g.Supports {
			games = append(games, g.Game.Title)
		}
	}
	return games
}
