package util

import "github.com/charmbracelet/lipgloss"

// UpdateTagStyle colors the "update available" tag; apply after truncation, not before!
var UpdateTagStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
