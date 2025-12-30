package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/lipgloss"
)

var (
	// HelpOverlayStyle defines the style for the help overlay container.
	HelpOverlayStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		MarginTop(2)
)

// HelpModel wraps the bubbles help component.
type HelpModel struct {
	help   help.Model
	keymap KeyMap
}

// NewHelpModel creates a new help overlay model.
func NewHelpModel(keymap KeyMap) HelpModel {
	h := help.New()
	h.ShowAll = true

	return HelpModel{
		help:   h,
		keymap: keymap,
	}
}

// View renders the help overlay.
func (m HelpModel) View(width int) string {
	m.help.Width = width - 8 // Account for padding and border
	helpView := m.help.View(m.keymap)
	return HelpOverlayStyle.Render(helpView)
}
