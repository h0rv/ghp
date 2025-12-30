package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all key bindings for the board view.
type KeyMap struct {
	// Navigation
	Left  key.Binding
	Right key.Binding
	Up    key.Binding
	Down  key.Binding

	// Actions
	Move         key.Binding
	Open         key.Binding
	Filter       key.Binding
	Refresh      key.Binding
	LoadMore     key.Binding
	ChangeGroup  key.Binding
	Help         key.Binding
	Quit         key.Binding
	ConfirmQuit  key.Binding
	CancelQuit   key.Binding
	ApplyFilter  key.Binding
	CancelFilter key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "previous column"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "next column"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "previous card"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "next card"),
		),
		Move: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "move card"),
		),
		Open: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open in browser"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter cards"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		LoadMore: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "load more"),
		),
		ChangeGroup: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "change grouping field"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "quit"),
		),
		ConfirmQuit: key.NewBinding(
			key.WithKeys("ctrl+c"),
		),
		CancelQuit: key.NewBinding(
			key.WithKeys("esc"),
		),
		ApplyFilter: key.NewBinding(
			key.WithKeys("enter"),
		),
		CancelFilter: key.NewBinding(
			key.WithKeys("esc"),
		),
	}
}

// ShortHelp returns key bindings to be shown in the mini help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

// FullHelp returns key bindings for the expanded help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Move, k.Open, k.Filter, k.Refresh},
		{k.LoadMore, k.ChangeGroup, k.Help, k.Quit},
	}
}
