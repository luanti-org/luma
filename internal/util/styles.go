package util

import "github.com/charmbracelet/lipgloss"

// Styles color text, apply them after truncation, not before!
var (
	UpdateTagStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	AvailableStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	MutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	WarnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	ErrorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)
