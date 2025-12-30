package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/robby/ghp/internal/gh"
)

// ownerItem represents an owner in the list.
type ownerItem struct {
	owner gh.Owner
}

func (i ownerItem) FilterValue() string { return i.owner.Login }

// ownerItemDelegate handles rendering of owner items.
type ownerItemDelegate struct{}

func (d ownerItemDelegate) Height() int                             { return 1 }
func (d ownerItemDelegate) Spacing() int                            { return 0 }
func (d ownerItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d ownerItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(ownerItem)
	if !ok {
		return
	}

	// Format: login (type)
	typeLabel := "user"
	if i.owner.Type == gh.OwnerTypeOrganization {
		typeLabel = "org"
	}
	str := fmt.Sprintf("%s (%s)", i.owner.Login, typeLabel)

	fn := NormalItemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return SelectedItemStyle.Render("> " + s[0])
		}
	}

	fmt.Fprint(w, fn(str))
}

// OwnerPickerModel lets the user select from available owners.
type OwnerPickerModel struct {
	list   list.Model
	owners []gh.Owner
	err    error
}

// NewOwnerPickerModel creates a new owner picker with the given owners.
func NewOwnerPickerModel(owners []gh.Owner) OwnerPickerModel {
	items := make([]list.Item, len(owners))
	for i, owner := range owners {
		items[i] = ownerItem{owner: owner}
	}

	// Start with a reasonable default - will be resized by WindowSizeMsg
	l := list.New(items, ownerItemDelegate{}, 80, 20)
	l.Title = "Select Owner"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = TitleStyle
	l.Styles.PaginationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	l.Styles.HelpStyle = HelpStyle

	return OwnerPickerModel{
		list:   l,
		owners: owners,
	}
}

// Init initializes the model.
func (m OwnerPickerModel) Init() tea.Cmd {
	// Request window size on init to properly size the list
	return tea.WindowSize()
}

// Update handles messages.
func (m OwnerPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(ownerItem); ok {
				return m, func() tea.Msg {
					return OwnerSelectedMsg{
						Owner:     item.owner.Login,
						OwnerType: item.owner.Type,
						OwnerID:   item.owner.ID,
					}
				}
			}
		case "q", "esc":
			if !m.list.SettingFilter() {
				return m, func() tea.Msg {
					return QuitMsg{}
				}
			}
		}

	case tea.WindowSizeMsg:
		// Use full terminal width and height (minus small margin for borders)
		m.list.SetWidth(msg.Width - 2)
		m.list.SetHeight(msg.Height - 2)
		return m, nil

	case ErrorMsg:
		m.err = msg.Err
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the model.
func (m OwnerPickerModel) View() string {
	if m.err != nil {
		return ErrorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}
	return m.list.View()
}
