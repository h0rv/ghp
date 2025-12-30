package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/robby/ghp/internal/domain"
)

// fieldItem wraps a domain.FieldDef for use in bubbles/list.
type fieldItem struct {
	field domain.FieldDef
}

func (i fieldItem) FilterValue() string {
	return i.field.Name
}

func (i fieldItem) Title() string {
	return i.field.Name
}

func (i fieldItem) Description() string {
	return fmt.Sprintf("Type: %s, Options: %d", i.field.Type, len(i.field.Options))
}

// fieldDelegate is a custom item delegate for field items.
type fieldDelegate struct{}

func (d fieldDelegate) Height() int                             { return 2 }
func (d fieldDelegate) Spacing() int                            { return 1 }
func (d fieldDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d fieldDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(fieldItem)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i.Title())
	desc := i.Description()

	if index == m.Index() {
		// Selected item
		fmt.Fprint(w, SelectedItemStyle.Render("> "+str))
		fmt.Fprint(w, "\n  "+NormalItemStyle.Render(desc))
	} else {
		// Normal item
		fmt.Fprint(w, NormalItemStyle.Render("  "+str))
		fmt.Fprint(w, "\n  "+lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(desc))
	}
}

// GroupFieldPickerModel displays a list of SINGLE_SELECT fields for the user to select.
// This model is auto-skipped if a "Status" field exists or only one option is available.
type GroupFieldPickerModel struct {
	list list.Model
	err  error
}

// NewGroupFieldPickerModel creates a new GroupFieldPickerModel.
func NewGroupFieldPickerModel(fields []domain.FieldDef) GroupFieldPickerModel {
	items := make([]list.Item, len(fields))
	for i, f := range fields {
		items[i] = fieldItem{field: f}
	}

	l := list.New(items, fieldDelegate{}, 80, 20)
	l.Title = "Select a Grouping Field"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = TitleStyle

	return GroupFieldPickerModel{
		list: l,
	}
}

// Init initializes the model.
func (m GroupFieldPickerModel) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update handles messages and updates the model state.
func (m GroupFieldPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 4)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, func() tea.Msg {
				return QuitMsg{}
			}
		case "enter":
			// Get selected field
			if item, ok := m.list.SelectedItem().(fieldItem); ok {
				return m, func() tea.Msg {
					return FieldSelectedMsg{Field: item.field}
				}
			}
		}

	case ErrorMsg:
		m.err = msg.Err
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the model.
func (m GroupFieldPickerModel) View() string {
	view := m.list.View()

	if m.err != nil {
		errorMsg := ErrorStyle.Render(fmt.Sprintf("\nError: %v", m.err))
		view += errorMsg
	}

	return view
}
