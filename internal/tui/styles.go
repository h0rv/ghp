package tui

import "github.com/charmbracelet/lipgloss"

var (
	// TitleStyle is used for screen titles.
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("62")). // Purple
			MarginBottom(1)

	// SelectedItemStyle is used for highlighted/selected items.
	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("170")). // Light purple
				Bold(true)

	// NormalItemStyle is used for non-selected items.
	NormalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")) // Light gray

	// ErrorStyle is used for error messages.
	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")). // Red
			Bold(true)

	// PromptStyle is used for prompt text.
	PromptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")). // Light blue
			MarginBottom(1)

	// HelpStyle is used for help text.
	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")). // Dark gray
			MarginTop(1)
)
