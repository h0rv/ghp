package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/h0rv/ghp/internal/domain"
)

// projectItem wraps a domain.Project for use in bubbles/list.
type projectItem struct {
	project domain.Project
}

func (i projectItem) FilterValue() string {
	return i.project.Title
}

func (i projectItem) Title() string {
	return fmt.Sprintf("%d: %s", i.project.Number, i.project.Title)
}

func (i projectItem) Description() string {
	return fmt.Sprintf("Owner: %s", i.project.Owner)
}

// projectDelegate is a custom item delegate for project items.
type projectDelegate struct{}

func (d projectDelegate) Height() int                             { return 2 }
func (d projectDelegate) Spacing() int                            { return 1 }
func (d projectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d projectDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(projectItem)
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

// ProjectPickerModel displays a list of projects for the user to select.
type ProjectPickerModel struct {
	list list.Model
	err  error
}

// NewProjectPickerModel creates a new ProjectPickerModel.
func NewProjectPickerModel(projects []domain.Project) ProjectPickerModel {
	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = projectItem{project: p}
	}

	l := list.New(items, projectDelegate{}, 80, 20)
	l.Title = "Select a Project"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = TitleStyle

	return ProjectPickerModel{
		list: l,
	}
}

// Init initializes the model.
func (m ProjectPickerModel) Init() tea.Cmd {
	return tea.WindowSize()
}

// Update handles messages and updates the model state.
func (m ProjectPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width - 2)
		m.list.SetHeight(msg.Height - 2)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, func() tea.Msg {
				return QuitMsg{}
			}
		case "enter":
			// Get selected project
			if item, ok := m.list.SelectedItem().(projectItem); ok {
				return m, func() tea.Msg {
					return ProjectSelectedMsg{Project: item.project}
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
func (m ProjectPickerModel) View() string {
	view := m.list.View()

	if m.err != nil {
		errorMsg := ErrorStyle.Render(fmt.Sprintf("\nError: %v", m.err))
		view += errorMsg
	}

	return view
}
