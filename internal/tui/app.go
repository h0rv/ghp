package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/h0rv/ghp/internal/domain"
	"github.com/h0rv/ghp/internal/gh"
	"github.com/h0rv/ghp/internal/store"
)

// AppScreen represents the different screens in the application flow.
type AppScreen int

const (
	ScreenLoading AppScreen = iota
	ScreenOwner
	ScreenProjectPicker
	ScreenFieldPicker
	ScreenBoard
	ScreenDetail
)

// AppModel is the root Bubble Tea model that manages screen transitions.
// It orchestrates the flow from owner selection -> project selection -> field selection -> board view.
type AppModel struct {
	// Dependencies
	client *gh.Client
	store  *store.Store
	ctx    context.Context

	// CLI flags (pre-filled values)
	ownerFlag      string
	projectFlag    int
	groupFieldFlag string

	// Current state
	currentScreen AppScreen
	currentModel  tea.Model
	err           error
	loadingMsg    string

	// Resolved context (accumulated through the flow)
	ownerLogin string
	ownerType  gh.OwnerType
	ownerID    string
	project    *domain.Project
	fields     []domain.FieldDef
	groupField *domain.FieldDef

	// Cached models to preserve state across screen transitions
	boardModel *BoardModel
}

// NewAppModel creates a new app model with optional CLI flag values.
// Pass empty string or 0 to skip pre-filling.
func NewAppModel(client *gh.Client, store *store.Store, ctx context.Context, ownerFlag string, projectFlag int, groupFieldFlag string) AppModel {
	return AppModel{
		client:         client,
		store:          store,
		ctx:            ctx,
		ownerFlag:      ownerFlag,
		projectFlag:    projectFlag,
		groupFieldFlag: groupFieldFlag,
		currentScreen:  ScreenLoading,
		loadingMsg:     "Connecting to GitHub...",
	}
}

// Init initializes the app model.
func (m AppModel) Init() tea.Cmd {
	// If owner flag is provided, skip owner prompt and resolve immediately
	if m.ownerFlag != "" {
		return m.resolveOwner(m.ownerFlag)
	}

	// Otherwise, fetch available owners (viewer + orgs)
	return m.fetchOwners()
}

// Update handles messages and transitions between screens.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global quit handler
		if msg.String() == "ctrl+c" && m.currentScreen != ScreenBoard {
			return m, tea.Quit
		}

	case ErrorMsg:
		m.err = msg.Err
		return m, nil

	case QuitMsg:
		return m, tea.Quit

	case ownersLoadedMsg:
		// Store viewer login for "assigned to me" filtering
		if len(msg.owners) > 0 {
			m.store.SetViewerLogin(msg.owners[0].Login)
		}
		// Owners fetched, show picker
		m.currentScreen = ScreenOwner
		pickerModel := NewOwnerPickerModel(msg.owners)
		m.currentModel = pickerModel
		return m, pickerModel.Init()

	case OwnerSelectedMsg:
		// Owner selected from picker
		m.ownerLogin = msg.Owner
		// If the picker provided pre-resolved info, use it
		if msg.OwnerID != "" {
			m.ownerType = msg.OwnerType
			m.ownerID = msg.OwnerID
			m.loadingMsg = fmt.Sprintf("Loading projects for %s...", m.ownerLogin)
			m.currentModel = nil
			return m, m.listProjects()
		}
		// Otherwise resolve the owner
		m.loadingMsg = fmt.Sprintf("Resolving %s...", m.ownerLogin)
		m.currentModel = nil
		return m, m.resolveOwner(msg.Owner)

	case ownerResolvedMsg:
		// Owner resolved, now list projects
		m.ownerType = msg.ownerType
		m.ownerID = msg.ownerID
		m.loadingMsg = fmt.Sprintf("Loading projects for %s...", m.ownerLogin)
		return m, m.listProjects()

	case projectsLoadedMsg:
		// Projects loaded
		// If project flag is provided, find and select it
		if m.projectFlag > 0 {
			for _, proj := range msg.projects {
				if proj.Number == m.projectFlag {
					m.project = &proj
					m.store.SetProject(&proj)
					m.loadingMsg = fmt.Sprintf("Loading fields for %s...", proj.Title)
					return m, m.loadFields()
				}
			}
			// Project number not found
			m.err = fmt.Errorf("project #%d not found for owner %s", m.projectFlag, m.ownerLogin)
			return m, nil
		}

		// Show project picker
		m.currentScreen = ScreenProjectPicker
		pickerModel := NewProjectPickerModel(msg.projects)
		m.currentModel = pickerModel
		return m, pickerModel.Init()

	case ProjectSelectedMsg:
		// Project selected, load fields
		m.project = &msg.Project
		m.store.SetProject(&msg.Project)
		m.loadingMsg = fmt.Sprintf("Loading fields for %s...", msg.Project.Title)
		m.currentModel = nil
		return m, m.loadFields()

	case fieldsLoadedMsg:
		// Fields loaded, run field selection heuristic
		m.fields = msg.fields

		// Convert to pointer slice for SelectGroupField
		fieldPtrs := make([]*domain.FieldDef, len(m.fields))
		for i := range m.fields {
			fieldPtrs[i] = &m.fields[i]
		}

		selected, candidates, err := store.SelectGroupField(fieldPtrs)
		if err != nil {
			m.err = err
			return m, nil
		}

		// If group field flag is provided, find and use it
		if m.groupFieldFlag != "" {
			for i := range m.fields {
				if m.fields[i].Name == m.groupFieldFlag {
					m.groupField = &m.fields[i]
					m.store.SetGroupField(&m.fields[i])
					return m, m.loadItemsAndShowBoard()
				}
			}
			// Field name not found
			m.err = fmt.Errorf("field '%s' not found in project", m.groupFieldFlag)
			return m, nil
		}

		// Auto-selected (Status field or only one option)
		if selected != nil {
			m.groupField = selected
			m.store.SetGroupField(selected)
			return m, m.loadItemsAndShowBoard()
		}

		// Multiple candidates, show picker
		candidateValues := make([]domain.FieldDef, len(candidates))
		for i, c := range candidates {
			candidateValues[i] = *c
		}

		m.currentScreen = ScreenFieldPicker
		pickerModel := NewGroupFieldPickerModel(candidateValues)
		m.currentModel = pickerModel
		return m, pickerModel.Init()

	case FieldSelectedMsg:
		// Field selected, load items and show board
		m.groupField = &msg.Field
		m.store.SetGroupField(&msg.Field)
		m.currentModel = nil
		return m, m.loadItemsAndShowBoard()

	case boardReadyMsg:
		// Items loaded, show board
		m.currentScreen = ScreenBoard
		boardModel := NewBoardModel(m.store, m.client, m.ctx)
		m.boardModel = &boardModel
		m.currentModel = m.boardModel
		return m, boardModel.Init()

	case changeGroupFieldMsg:
		// User wants to change grouping field from board view
		fieldValues := make([]domain.FieldDef, 0)
		for i := range m.fields {
			if m.fields[i].Type == domain.FieldTypeSingleSelect {
				fieldValues = append(fieldValues, m.fields[i])
			}
		}

		if len(fieldValues) == 0 {
			m.err = fmt.Errorf("no SINGLE_SELECT fields available")
			return m, nil
		}

		m.currentScreen = ScreenFieldPicker
		pickerModel := NewGroupFieldPickerModel(fieldValues)
		m.currentModel = pickerModel
		return m, pickerModel.Init()

	case openDetailMsg:
		// User wants to view card details
		m.currentScreen = ScreenDetail
		detailModel := NewDetailModel(msg.card, m.client, m.ctx)
		m.currentModel = detailModel
		return m, detailModel.Init()

	case closeDetailMsg:
		// Return to board from detail view
		m.currentScreen = ScreenBoard
		m.currentModel = m.boardModel
		// Request window size to ensure proper rendering
		return m, tea.WindowSize()
	}

	// Delegate to current screen's model
	if m.currentModel != nil {
		var cmd tea.Cmd
		m.currentModel, cmd = m.currentModel.Update(msg)
		// Keep boardModel in sync when on board screen
		if m.currentScreen == ScreenBoard {
			if bm, ok := m.currentModel.(BoardModel); ok {
				m.boardModel = &bm
			}
		}
		return m, cmd
	}

	return m, nil
}

// View renders the current screen.
func (m AppModel) View() string {
	// Show error if present
	if m.err != nil {
		return ErrorStyle.Render(fmt.Sprintf("Error: %v\n\nPress Ctrl+C to quit", m.err))
	}

	// Delegate to current screen
	if m.currentModel != nil {
		return m.currentModel.View()
	}

	// Show loading state
	return m.loadingMsg + "\n\nPress Ctrl+C to quit"
}

// fetchOwners creates a command to fetch the viewer and their organizations.
func (m AppModel) fetchOwners() tea.Cmd {
	return func() tea.Msg {
		owners, err := m.client.GetViewerAndOrgs(m.ctx)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to fetch owners: %w", err)}
		}
		return ownersLoadedMsg{owners: owners}
	}
}

// resolveOwner creates a command to resolve the owner type.
// Also fetches viewer login for "assigned to me" filtering.
func (m AppModel) resolveOwner(login string) tea.Cmd {
	return func() tea.Msg {
		// Fetch viewer login for "assigned to me" filtering
		owners, err := m.client.GetViewerAndOrgs(m.ctx)
		if err == nil && len(owners) > 0 {
			m.store.SetViewerLogin(owners[0].Login)
		}

		ownerType, ownerID, err := m.client.ResolveOwner(m.ctx, login)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to resolve owner '%s': %w", login, err)}
		}
		return ownerResolvedMsg{ownerType: ownerType, ownerID: ownerID}
	}
}

// listProjects creates a command to list projects for the owner.
func (m AppModel) listProjects() tea.Cmd {
	return func() tea.Msg {
		projects, err := m.client.ListProjects(m.ctx, m.ownerType, m.ownerID, m.ownerLogin)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to list projects: %w", err)}
		}

		if len(projects) == 0 {
			return ErrorMsg{Err: fmt.Errorf("no projects found for owner '%s'", m.ownerLogin)}
		}

		return projectsLoadedMsg{projects: projects}
	}
}

// loadFields creates a command to load project fields.
func (m AppModel) loadFields() tea.Cmd {
	return func() tea.Msg {
		fields, err := m.client.GetProjectFields(m.ctx, m.project.ID)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to load project fields: %w", err)}
		}
		return fieldsLoadedMsg{fields: fields}
	}
}

// loadItemsAndShowBoard shows the board immediately and starts background loading.
func (m AppModel) loadItemsAndShowBoard() tea.Cmd {
	// Return boardReadyMsg immediately to show the board
	// The board will start loading items in the background via its Init()
	return func() tea.Msg {
		return boardReadyMsg{}
	}
}

// Custom messages for app transitions.
type (
	ownersLoadedMsg struct {
		owners []gh.Owner
	}

	ownerResolvedMsg struct {
		ownerType gh.OwnerType
		ownerID   string
	}

	projectsLoadedMsg struct {
		projects []domain.Project
	}

	fieldsLoadedMsg struct {
		fields []domain.FieldDef
	}

	boardReadyMsg struct{}
)
