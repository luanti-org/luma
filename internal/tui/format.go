package tui

import "strings"

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
