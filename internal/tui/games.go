package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/update"
	"github.com/luanti-org/luma/internal/util"
)

// Command rows shown above the game list on the games screen, in cursor order.
const (
	gamesCmdCheckUpdates = iota
	gamesCmdUpdateAll
	gamesRowCount // one past the last row, i.e. the row count
)

// gameUpdatesCheckedMsg is delivered when checkGameUpdatesCmd's ContentDB request finishes
type gameUpdatesCheckedMsg struct {
	updates []update.GameUpdate
	err     error
}

// checkGameUpdatesCmd runs the blocking ContentDB update
func (m model) checkGameUpdatesCmd() tea.Cmd {
	games := m.games
	client := m.cdb
	engInfo := m.engInfo

	return func() tea.Msg {
		updates, err := update.CheckAllGameUpdates(games, client, engInfo)
		return gameUpdatesCheckedMsg{updates: updates, err: err}
	}
}

// gameUpdateResult is one game's outcome from an update run.
type gameUpdateResult struct {
	update update.GameUpdate
	err    error
}

// gameUpdateStepMsg is delivered when one game's download+install finishes.
type gameUpdateStepMsg struct {
	index int // position in m.gameUpdateQueue that was just attempted
	err   error
}

// updateGameStepCmd downloads and installs a single game, chained like updateModStepCmd
func (m model) updateGameStepCmd(index int) tea.Cmd {
	client := m.cdb
	u := m.gameUpdateQueue[index]

	return func() tea.Msg {
		return gameUpdateStepMsg{index: index, err: update.UpdateGame(client, u)}
	}
}

// handleGameUpdateStep records one game's result and either
// kicks off the next one or, once done, ends the game action.
func (m model) handleGameUpdateStep(msg gameUpdateStepMsg) (tea.Model, tea.Cmd) {
	m.gameUpdateResults = append(m.gameUpdateResults, gameUpdateResult{
		update: m.gameUpdateQueue[msg.index],
		err:    msg.err,
	})
	m.gameUpdateIdx = msg.index + 1

	if m.gameUpdateIdx < len(m.gameUpdateQueue) {
		return m, m.updateGameStepCmd(m.gameUpdateIdx)
	}

	m.gamesUpdating = false
	m.gameActionInProgress = false

	var remaining []update.GameUpdate
	for _, u := range m.gameUpdates {
		if !m.gameUpdateSucceeded(u.Game.Path) {
			remaining = append(remaining, u)
		}
	}
	m.gameUpdates = remaining

	m.games, m.gamesErr = content.ScanGames(m.engInfo.GamesDir)
	m.gamesCursor = min(m.gamesCursor, max(gamesRowCount+len(m.games)-1, 0))
	for _, game := range m.games {
		if game.Path == m.selectedGame.Path {
			m.selectedGame = game // the detail screen may be open on it
		}
	}

	return m, nil
}

// gameUpdateSucceeded reports whether the last run updated the game at path.
func (m model) gameUpdateSucceeded(path string) bool {
	for _, r := range m.gameUpdateResults {
		if r.update.Game.Path == path && r.err == nil {
			return true
		}
	}

	return false
}

// startGameUpdates begins updating queue, one game at a time.
func (m model) startGameUpdates(queue []update.GameUpdate) (tea.Model, tea.Cmd) {
	if m.busy() || len(queue) == 0 {
		return m, nil
	}

	m.gameActionInProgress = true
	m.gamesUpdating = true
	m.gameUpdateQueue = queue
	m.gameUpdateIdx = 0
	m.gameUpdateResults = nil

	return m, m.updateGameStepCmd(0)
}

func (m model) updateGames(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxCursor := 0
	if len(m.games) > 0 {
		maxCursor = gamesRowCount + len(m.games) - 1
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenMenu

	case "u":
		if m.gamesCursor < gamesRowCount || len(m.games) == 0 {
			break
		}
		game := m.games[m.gamesCursor-gamesRowCount]
		for _, u := range m.gameUpdates {
			if u.Game.Path == game.Path {
				return m.startGameUpdates([]update.GameUpdate{u})
			}
		}

	case "up", "k":
		if m.gamesCursor > 0 {
			m.gamesCursor--
		}

	case "down", "j":
		if m.gamesCursor < maxCursor {
			m.gamesCursor++
		}

	case "enter", " ":
		if m.gamesErr != nil {
			break
		}

		if m.gamesCursor < gamesRowCount {
			switch m.gamesCursor {
			case gamesCmdCheckUpdates:
				if m.busy() {
					break
				}
				m.gameActionInProgress = true
				m.gamesChecking = true
				m.gamesUpdateErr = nil
				return m, m.checkGameUpdatesCmd()

			case gamesCmdUpdateAll:
				queue := append([]update.GameUpdate(nil), m.gameUpdates...)
				return m.startGameUpdates(queue)
			}
			break
		}

		if len(m.games) > 0 {
			m.selectedGame = m.games[m.gamesCursor-gamesRowCount]
			m.screen = screenGameDetail
		}
	}

	return m, nil
}

// gameHasUpdate reports whether game was flagged by the last update check.
func (m model) gameHasUpdate(game content.Game) bool {
	for _, u := range m.gameUpdates {
		if u.Game.Path == game.Path {
			return true
		}
	}

	return false
}

// gamesCommandLabel returns the label for a command row (< gamesRowCount).
func (m model) gamesCommandLabel(row int) string {
	switch row {
	case gamesCmdCheckUpdates:
		return checkUpdatesLabel(m.gamesChecking, m.gamesUpdateErr, m.gamesLastChecked)
	case gamesCmdUpdateAll:
		return m.gamesUpdateAllLabel()
	default:
		return ""
	}
}

func (m model) gamesUpdateAllLabel() string {
	if m.gamesUpdating {
		return fmt.Sprintf("Updating games... (%d/%d)", m.gameUpdateIdx, len(m.gameUpdateQueue))
	}

	label := "Update all (no updates available)"
	if len(m.gameUpdates) > 0 {
		label = fmt.Sprintf("Update all (%d available)", len(m.gameUpdates))
	}
	if len(m.gameUpdateResults) > 0 {
		failed := countGameUpdateErrs(m.gameUpdateResults)
		label += fmt.Sprintf(" [last run: %d/%d succeeded]", len(m.gameUpdateResults)-failed, len(m.gameUpdateResults))
	}

	return label
}

func countGameUpdateErrs(results []gameUpdateResult) int {
	n := 0
	for _, r := range results {
		if r.err != nil {
			n++
		}
	}
	return n
}

func (m model) viewGames() string {
	s := fmt.Sprintf("games in %s\n\n", m.engInfo.GamesDir)

	if m.gamesErr != nil {
		s += util.ErrorStyle.Render(fmt.Sprintf("error: %v", m.gamesErr)) + "\n"
		s += "\n" + helpLine(m.width, "esc/backspace", "to go back", "q", "to quit") + "\n"
		return s
	}

	for row := 0; row < gamesRowCount; row++ {
		cursor := " "
		if m.gamesCursor == row {
			cursor = ">"
		}
		line := truncate(fmt.Sprintf("%s %s", cursor, m.gamesCommandLabel(row)), m.width)
		switch {
		case row == gamesCmdCheckUpdates && m.gamesUpdateErr != nil:
			line = util.ErrorStyle.Render(line)
		case row == gamesCmdUpdateAll && countGameUpdateErrs(m.gameUpdateResults) > 0:
			line = styleFrom(line, "[last run:", util.WarnStyle)
		}
		s += line + "\n"
	}
	s += "\n"

	if len(m.games) == 0 {
		s += "(no games found)\n"
	}

	help := helpLine(m.width, "up/down", "to move", "enter", "to select", "u", "to update game", "esc/backspace", "to go back", "q", "to quit")
	rows := m.listRows(7 + gamesRowCount + strings.Count(help, "\n"))
	listCursor := max(m.gamesCursor-gamesRowCount, 0)
	start, end := listWindow(listCursor, len(m.games), rows)

	for i := start; i < end; i++ {
		game := m.games[i]
		cursor := " "
		if m.gamesCursor == i+gamesRowCount {
			cursor = ">"
		}

		updateTag := "   "
		if m.gameHasUpdate(game) {
			updateTag = "[U]" // update available
		}

		tag := "   " // regular, well-formed game: no flag needed
		if !game.ConfOK {
			tag = "[!]" // missing/malformed game.conf, title guessed from folder
		}

		line := truncate(fmt.Sprintf("%s %s %s %s", cursor, updateTag, tag, game.Title), m.width)
		if m.gameHasUpdate(game) {
			line = strings.Replace(line, "[U]", util.UpdateTagStyle.Render("[U]"), 1)
		}
		if tag == "[!]" {
			line = strings.Replace(line, "[!]", util.WarnStyle.Render("[!]"), 1)
		}

		s += line + "\n"
	}

	pad := rows - (end - start)
	if len(m.games) == 0 {
		pad-- // the "no ... found" line
	}

	s += strings.Repeat("\n", max(pad, 0))
	if end-start < len(m.games) {
		s += fmt.Sprintf("(%d-%d of %d)", start+1, end, len(m.games))
	}

	s += "\n\n(! = no valid game.conf, title guessed from folder, U = update available)\n"
	s += help

	return s
}
