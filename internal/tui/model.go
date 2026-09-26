// Package tui is the interactive terminal UI, built on bubbletea.
// Each screen has its own file (menu.go, mods.go, ...); this file
// holds the shared model and dispatches Update/View to the current
// screen.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
)

type screen int

const (
	screenMenu screen = iota
	screenMods
	screenModDetail
	screenGames
	screenGameDetail
)

// defaultWidth is used until the first tea.WindowSizeMsg arrives.
const defaultWidth = 80

type model struct {
	screen   screen
	cursor   int
	choices  []string
	modsDir  string
	gamesDir string
	width    int

	mods       []content.Mod
	modsErr    error
	modsCursor int

	selectedMod content.Mod

	games       []content.Game
	gamesErr    error
	gamesCursor int

	selectedGame content.Game
}

// New returns the initial TUI model, ready to pass to tea.NewProgram.
func New(modsDir, gamesDir string) model {
	return model{
		screen:   screenMenu,
		cursor:   0,
		choices:  []string{"Manage mods", "Manage games", "Quit"},
		modsDir:  modsDir,
		gamesDir: gamesDir,
		width:    defaultWidth,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = sizeMsg.Width
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if keyMsg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.screen {
	case screenMenu:
		return m.updateMenu(keyMsg)
	case screenMods:
		return m.updateMods(keyMsg)
	case screenModDetail:
		return m.updateModDetail(keyMsg)
	case screenGames:
		return m.updateGames(keyMsg)
	case screenGameDetail:
		return m.updateGameDetail(keyMsg)
	}

	return m, nil
}

func (m model) View() string {
	switch m.screen {
	case screenMods:
		return m.viewMods()
	case screenModDetail:
		return m.viewModDetail()
	case screenGames:
		return m.viewGames()
	case screenGameDetail:
		return m.viewGameDetail()
	default:
		return m.viewMenu()
	}
}
