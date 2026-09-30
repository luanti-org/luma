// Package tui is the interactive terminal UI, built on bubbletea.
// Each screen has its own file (menu.go, mods.go, ...); this file
// holds the shared model and dispatches Update/View to the current
// screen.
package tui

import (
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
	"github.com/luanti-org/luma/internal/update"
)

// cdbRequestTimeout bounds how long a ContentDB request go before timing out
const cdbRequestTimeout = 30 * time.Second

type screen int

const (
	screenMenu screen = iota
	screenMods
	screenModDetail
	screenModpackModDetail
	screenGames
	screenGameDetail
	screenTexturepacks
	screenTexturepackDetail
)

// Used until the first tea.WindowSizeMsg arrives.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

type model struct {
	screen  screen
	cursor  int
	choices []string
	engInfo engine.Info
	cdb     *contentdb.Client
	width   int
	height  int

	mods       []content.Mod
	modsErr    error
	modsCursor int

	// modActionInProgress allows only one check/update at a time; navigation stays free
	modActionInProgress bool

	modsChecking    bool
	modsLastChecked time.Time
	modsUpdateErr   error
	modUpdates      []update.ModUpdate

	modsUpdating     bool
	modUpdateQueue   []update.ModUpdate // what the current/last run updates, one or all
	modUpdateIdx     int                // count of update attempts completed so far, into modUpdateQueue
	modUpdateResults []modUpdateResult

	selectedMod content.Mod

	modpackMods       []content.Mod
	modpackModsCursor int

	selectedModpackMod content.Mod

	games       []content.Game
	gamesErr    error
	gamesCursor int

	selectedGame content.Game

	texturepacks       []content.Texturepack
	texturepacksErr    error
	texturepacksCursor int

	selectedTexturepack content.Texturepack
}

// New returns the initial TUI model, ready to pass to tea.NewProgram.
func New(engInfo engine.Info) model {
	cdb := contentdb.New("")
	cdb.HTTPClient = &http.Client{Timeout: cdbRequestTimeout} // don't share/mutate http.DefaultClient

	return model{
		screen:  screenMenu,
		cursor:  0,
		choices: []string{"Manage mods", "Manage games", "Manage texture packs", "Quit"},
		engInfo: engInfo,
		cdb:     cdb,
		width:   defaultWidth,
		height:  defaultHeight,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = sizeMsg.Width
		m.height = sizeMsg.Height
		return m, nil
	}

	if checkedMsg, ok := msg.(modUpdatesCheckedMsg); ok {
		m.modActionInProgress = false
		m.modsChecking = false
		m.modsUpdateErr = checkedMsg.err
		if checkedMsg.err == nil {
			m.modsLastChecked = time.Now()
			m.modUpdates = checkedMsg.updates
		}
		return m, nil
	}

	if stepMsg, ok := msg.(modUpdateStepMsg); ok {
		return m.handleModUpdateStep(stepMsg)
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if keyMsg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if keyMsg.String() == "q" && m.modActionInProgress {
		return m, nil // quitting mid-download could leave a mod folder half-replaced
	}

	switch m.screen {
	case screenMenu:
		return m.updateMenu(keyMsg)
	case screenMods:
		return m.updateMods(keyMsg)
	case screenModDetail:
		return m.updateModDetail(keyMsg)
	case screenModpackModDetail:
		return m.updateModpackModDetail(keyMsg)
	case screenGames:
		return m.updateGames(keyMsg)
	case screenGameDetail:
		return m.updateGameDetail(keyMsg)
	case screenTexturepacks:
		return m.updateTexturepacks(keyMsg)
	case screenTexturepackDetail:
		return m.updateTexturepackDetail(keyMsg)
	}

	return m, nil
}

func (m model) View() string {
	switch m.screen {
	case screenMods:
		return m.viewMods()
	case screenModDetail:
		return m.viewModDetail()
	case screenModpackModDetail:
		return m.viewModpackModDetail()
	case screenGames:
		return m.viewGames()
	case screenGameDetail:
		return m.viewGameDetail()
	case screenTexturepacks:
		return m.viewTexturepacks()
	case screenTexturepackDetail:
		return m.viewTexturepackDetail()
	default:
		return m.viewMenu()
	}
}

// listRows is how many list rows fit once chrome lines are reserved
func (m model) listRows(chrome int) int {
	return max(m.height-chrome, 1)
}

// listWindow returns the [start, end) slice of items to show,
// keeping the cursor roughly centered
func listWindow(cursor, total, rows int) (int, int) {
	if total <= rows {
		return 0, total
	}

	start := min(max(cursor-rows/2, 0), total-rows)

	return start, start + rows
}
