package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/update"
	"github.com/luanti-org/luma/internal/util"
)

// Command rows shown above the texture pack list on the texture packs screen, in cursor order.
const (
	texturepacksCmdCheckUpdates = iota
	texturepacksCmdUpdateAll
	texturepacksRowCount // one past the last row, i.e. the row count
)

// texturepackUpdatesCheckedMsg is delivered when checkTexturepackUpdatesCmd's ContentDB request finishes
type texturepackUpdatesCheckedMsg struct {
	updates []update.TexturepackUpdate
	err     error
}

// checkTexturepackUpdatesCmd runs the blocking ContentDB update
func (m model) checkTexturepackUpdatesCmd() tea.Cmd {
	texturepacks := m.texturepacks
	client := m.cdb
	engInfo := m.engInfo

	return func() tea.Msg {
		updates, err := update.CheckAllTexturepackUpdates(texturepacks, client, engInfo)
		return texturepackUpdatesCheckedMsg{updates: updates, err: err}
	}
}

// texturepackUpdateResult is one texture pack's outcome from an update run.
type texturepackUpdateResult struct {
	update update.TexturepackUpdate
	err    error
}

// texturepackUpdateStepMsg is delivered when one texture pack's download+install finishes.
type texturepackUpdateStepMsg struct {
	index int // position in m.texturepackUpdateQueue that was just attempted
	err   error
}

// updateTexturepackStepCmd downloads and installs a single texture pack, chained like updateModStepCmd
func (m model) updateTexturepackStepCmd(index int) tea.Cmd {
	client := m.cdb
	u := m.texturepackUpdateQueue[index]

	return func() tea.Msg {
		return texturepackUpdateStepMsg{index: index, err: update.UpdateTexturepack(client, u)}
	}
}

// handleTexturepackUpdateStep records one texture pack's result and either
// kicks off the next one or, once done, ends the texture pack action.
func (m model) handleTexturepackUpdateStep(msg texturepackUpdateStepMsg) (tea.Model, tea.Cmd) {
	m.texturepackUpdateResults = append(m.texturepackUpdateResults, texturepackUpdateResult{
		update: m.texturepackUpdateQueue[msg.index],
		err:    msg.err,
	})
	m.texturepackUpdateIdx = msg.index + 1

	if m.texturepackUpdateIdx < len(m.texturepackUpdateQueue) {
		return m, m.updateTexturepackStepCmd(m.texturepackUpdateIdx)
	}

	m.texturepacksUpdating = false
	m.texturepackActionInProgress = false

	var remaining []update.TexturepackUpdate
	for _, u := range m.texturepackUpdates {
		if !m.texturepackUpdateSucceeded(u.Texturepack.Path) {
			remaining = append(remaining, u)
		}
	}
	m.texturepackUpdates = remaining

	m.texturepacks, m.texturepacksErr = content.ScanTextures(m.engInfo.TexturesDir)
	m.texturepacksCursor = min(m.texturepacksCursor, max(texturepacksRowCount+len(m.texturepacks)-1, 0))
	for _, texturepack := range m.texturepacks {
		if texturepack.Path == m.selectedTexturepack.Path {
			m.selectedTexturepack = texturepack // the detail screen may be open on it
		}
	}

	return m, nil
}

// texturepackUpdateSucceeded reports whether the last run updated the texture pack at path.
func (m model) texturepackUpdateSucceeded(path string) bool {
	for _, r := range m.texturepackUpdateResults {
		if r.update.Texturepack.Path == path && r.err == nil {
			return true
		}
	}

	return false
}

// startTexturepackUpdates begins updating queue, one texture pack at a time.
func (m model) startTexturepackUpdates(queue []update.TexturepackUpdate) (tea.Model, tea.Cmd) {
	if m.busy() || len(queue) == 0 {
		return m, nil
	}

	m.texturepackActionInProgress = true
	m.texturepacksUpdating = true
	m.texturepackUpdateQueue = queue
	m.texturepackUpdateIdx = 0
	m.texturepackUpdateResults = nil

	return m, m.updateTexturepackStepCmd(0)
}

func (m model) updateTexturepacks(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxCursor := 0
	if len(m.texturepacks) > 0 {
		maxCursor = texturepacksRowCount + len(m.texturepacks) - 1
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc", "backspace":
		m.screen = screenMenu

	case "u":
		if m.texturepacksCursor < texturepacksRowCount || len(m.texturepacks) == 0 {
			break
		}
		texturepack := m.texturepacks[m.texturepacksCursor-texturepacksRowCount]
		for _, u := range m.texturepackUpdates {
			if u.Texturepack.Path == texturepack.Path {
				return m.startTexturepackUpdates([]update.TexturepackUpdate{u})
			}
		}

	case "up", "k":
		if m.texturepacksCursor > 0 {
			m.texturepacksCursor--
		}

	case "down", "j":
		if m.texturepacksCursor < maxCursor {
			m.texturepacksCursor++
		}

	case "enter", " ":
		if m.texturepacksErr != nil {
			break
		}

		if m.texturepacksCursor < texturepacksRowCount {
			switch m.texturepacksCursor {
			case texturepacksCmdCheckUpdates:
				if m.busy() {
					break
				}
				m.texturepackActionInProgress = true
				m.texturepacksChecking = true
				m.texturepacksUpdateErr = nil
				return m, m.checkTexturepackUpdatesCmd()

			case texturepacksCmdUpdateAll:
				queue := append([]update.TexturepackUpdate(nil), m.texturepackUpdates...)
				return m.startTexturepackUpdates(queue)
			}
			break
		}

		if len(m.texturepacks) > 0 {
			m.selectedTexturepack = m.texturepacks[m.texturepacksCursor-texturepacksRowCount]
			m.screen = screenTexturepackDetail
		}
	}

	return m, nil
}

// texturepackHasUpdate reports whether texturepack was flagged by the last update check.
func (m model) texturepackHasUpdate(texturepack content.Texturepack) bool {
	for _, u := range m.texturepackUpdates {
		if u.Texturepack.Path == texturepack.Path {
			return true
		}
	}

	return false
}

// texturepacksCommandLabel returns the label for a command row (< texturepacksRowCount).
func (m model) texturepacksCommandLabel(row int) string {
	switch row {
	case texturepacksCmdCheckUpdates:
		return checkUpdatesLabel(m.texturepacksChecking, m.texturepacksUpdateErr, m.texturepacksLastChecked)
	case texturepacksCmdUpdateAll:
		return m.texturepacksUpdateAllLabel()
	default:
		return ""
	}
}

func (m model) texturepacksUpdateAllLabel() string {
	if m.texturepacksUpdating {
		return fmt.Sprintf("Updating texture packs... (%d/%d)", m.texturepackUpdateIdx, len(m.texturepackUpdateQueue))
	}

	label := "Update all (no updates available)"
	if len(m.texturepackUpdates) > 0 {
		label = fmt.Sprintf("Update all (%d available)", len(m.texturepackUpdates))
	}
	if len(m.texturepackUpdateResults) > 0 {
		failed := 0
		for _, r := range m.texturepackUpdateResults {
			if r.err != nil {
				failed++
			}
		}
		label += fmt.Sprintf(" [last run: %d/%d succeeded]", len(m.texturepackUpdateResults)-failed, len(m.texturepackUpdateResults))
	}

	return label
}

func (m model) viewTexturepacks() string {
	s := fmt.Sprintf("texture packs in %s\n\n", m.engInfo.TexturesDir)

	if m.texturepacksErr != nil {
		s += fmt.Sprintf("error: %v\n", m.texturepacksErr)
		s += "\n(backspace to go back)\n"
		return s
	}

	for row := 0; row < texturepacksRowCount; row++ {
		cursor := " "
		if m.texturepacksCursor == row {
			cursor = ">"
		}
		s += truncate(fmt.Sprintf("%s %s", cursor, m.texturepacksCommandLabel(row)), m.width) + "\n"
	}
	s += "\n"

	if len(m.texturepacks) == 0 {
		s += "(no texture packs found)\n"
	}

	rows := m.listRows(7 + texturepacksRowCount)
	listCursor := max(m.texturepacksCursor-texturepacksRowCount, 0)
	start, end := listWindow(listCursor, len(m.texturepacks), rows)

	for i := start; i < end; i++ {
		texturepack := m.texturepacks[i]
		cursor := " "
		if m.texturepacksCursor == i+texturepacksRowCount {
			cursor = ">"
		}

		updateTag := "   "
		if m.texturepackHasUpdate(texturepack) {
			updateTag = "[U]" // update available
		}

		tag := "   " // regular, well-formed texture pack: no flag needed
		if !texturepack.ConfOK {
			tag = "[!]" // missing/malformed texture_pack.conf, name is a folder-name guess
		}

		line := truncate(fmt.Sprintf("%s %s %s %s", cursor, updateTag, tag, texturepack.Name), m.width)
		if m.texturepackHasUpdate(texturepack) {
			line = strings.Replace(line, "[U]", util.UpdateTagStyle.Render("[U]"), 1)
		}

		s += line + "\n"
	}

	pad := rows - (end - start)
	if len(m.texturepacks) == 0 {
		pad-- // the "no ... found" line
	}

	s += strings.Repeat("\n", max(pad, 0))
	if end-start < len(m.texturepacks) {
		s += fmt.Sprintf("(%d-%d of %d)", start+1, end, len(m.texturepacks))
	}

	s += "\n\n(! = no valid texture_pack.conf, name guessed from folder, U = update available)\n"
	s += "(up/down to move, enter to select, u to update texture pack, esc/backspace to go back, q to quit)"

	return s
}
