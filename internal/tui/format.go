package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var helpKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#7B3FC4", Dark: "#B48CFF"})

// helpLine renders "| key action | key action |" from key/action pairs, with the keys colored,
// wrapped between items to fit width. width <= 0 disables wrapping.
func helpLine(width int, pairs ...string) string {
	var lines, cur []string
	curWidth := 0
	for i := 0; i+1 < len(pairs); i += 2 {
		w := len([]rune(pairs[i])) + 1 + len([]rune(pairs[i+1]))
		if width > 0 && len(cur) > 0 && curWidth+3+w+2 > width {
			lines = append(lines, "| "+strings.Join(cur, " | ")+" |")
			cur, curWidth = nil, 0
		}
		if len(cur) == 0 {
			curWidth = 2 + w
		} else {
			curWidth += 3 + w
		}
		cur = append(cur, helpKeyStyle.Render(pairs[i])+" "+pairs[i+1])
	}
	if len(cur) > 0 {
		lines = append(lines, "| "+strings.Join(cur, " | ")+" |")
	}
	return strings.Join(lines, "\n")
}

// styledLine is a line whose style goes on after it is truncated.
type styledLine struct {
	text  string
	style *lipgloss.Style // nil for plain
}

func (l styledLine) render(width int) string {
	s := truncate(l.text, width)
	if l.style != nil {
		s = l.style.Render(s)
	}
	return s
}

// styleFrom styles line from the first marker on, for use after truncation.
func styleFrom(line, marker string, style lipgloss.Style) string {
	i := strings.Index(line, marker)
	if i < 0 {
		return line
	}
	return line[:i] + style.Render(line[i:])
}

// truncate cuts s to at most width runes; width <= 0 disables it.
func truncate(s string, width int) string {
	if width <= 0 {
		return s
	}

	r := []rune(s)
	if len(r) <= width {
		return s
	}

	return string(r[:width])
}

// wrapText breaks text into lines no wider than width, breaking on
// word boundaries. width <= 0 disables wrapping (single line).
func wrapText(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	if width <= 0 {
		return []string{strings.Join(words, " ")}
	}

	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := len(lines) - 1
		if len(lines[last])+1+len(word) > width {
			lines = append(lines, word)
		} else {
			lines[last] += " " + word
		}
	}

	return lines
}

// wrapLabeled renders "label: word word word ..." wrapped to width,
// with continuation lines indented to align under the first word.
func wrapLabeled(label string, items []string, width int) string {
	text := strings.Join(items, ", ")
	contentWidth := width - len(label)

	lines := wrapText(text, contentWidth)
	if len(lines) == 0 {
		return ""
	}

	indent := strings.Repeat(" ", len(label))

	s := label + lines[0] + "\n"
	for _, line := range lines[1:] {
		s += indent + line + "\n"
	}

	return s
}
