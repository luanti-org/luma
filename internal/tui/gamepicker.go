package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
)

const noGameLabel = "<None>"

// openGamePicker rescans the games and shows the picker with the cursor on the current pick.
func (m model) openGamePicker() model {
	m.pickerGames, m.pickerErr = content.ScanGames(m.engInfo.GamesDir)

	m.pickerCursor = 0
	for i, game := range m.pickerGames {
		if m.modsGame.ID != "" && game.ID == m.modsGame.ID {
			m.pickerCursor = i + 1 // row 0 is <None>
		}
	}

	m.screen = screenGamePicker

	return m
}

func (m model) updateGamePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenMods

	case "up", "k":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}

	case "down", "j":
		if m.pickerCursor < len(m.pickerGames) {
			m.pickerCursor++
		}

	case "enter", " ":
		if m.pickerErr != nil {
			m.screen = screenMods // the list is incomplete, keep the current pick
			break
		}

		m.modsGame = content.Game{}
		if m.pickerCursor > 0 {
			m.modsGame = m.pickerGames[m.pickerCursor-1]
		}
		m.screen = screenMods
	}

	return m, nil
}

// gameLabel is a game's title with its folder name, or <None> for no game.
func gameLabel(game content.Game) string {
	if game.ID == "" {
		return noGameLabel
	}
	if game.Title == game.ID {
		return game.ID
	}

	return fmt.Sprintf("%s (%s)", game.Title, game.ID)
}

func (m model) viewGamePicker() string {
	s := fmt.Sprintf("base game for mod dependencies, from %s\n\n", m.engInfo.GamesDir)

	if m.pickerErr != nil {
		s += fmt.Sprintf("error: %v\n\n", m.pickerErr)
	}

	total := len(m.pickerGames) + 1
	rows := m.listRows(6)
	if m.pickerErr != nil {
		rows = max(rows-2, 1)
	}
	start, end := listWindow(m.pickerCursor, total, rows)

	for i := start; i < end; i++ {
		cursor := " "
		if m.pickerCursor == i {
			cursor = ">"
		}

		label := noGameLabel
		if i > 0 {
			label = gameLabel(m.pickerGames[i-1])
		}

		s += truncate(fmt.Sprintf("%s %s", cursor, label), m.width) + "\n"
	}

	s += strings.Repeat("\n", rows-(end-start))
	if end-start < total {
		s += fmt.Sprintf("(%d-%d of %d)", start+1, end, total)
	}

	s += "\n\n(mods of the picked game count as installed)\n"
	s += "(up/down to move, enter to pick, esc/backspace to go back, q to quit)"

	return s
}
