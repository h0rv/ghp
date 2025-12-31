package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/h0rv/ghp/internal/domain"
	"github.com/h0rv/ghp/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClient implements a minimal mock for testing
type mockClient struct{}

func (m *mockClient) GetItems(ctx context.Context, projectID, fieldName, cursor string, limit int) ([]domain.Card, string, bool, error) {
	return nil, "", false, nil
}

func (m *mockClient) UpdateItemField(ctx context.Context, projectID, itemID, fieldID, optionID string) error {
	return nil
}

// createTestStore creates a store with test data
func createTestStore() *store.Store {
	s := store.New()

	// Set up a test project
	project := &domain.Project{
		ID:     "proj-1",
		Number: 1,
		Title:  "Test Project",
		Owner:  "test-owner",
	}
	s.SetProject(project)

	// Set up a group field with options (simulating Status field)
	groupField := &domain.FieldDef{
		ID:   "field-1",
		Name: "Status",
		Type: domain.FieldTypeSingleSelect,
		Options: []domain.Option{
			{ID: "opt-todo", Name: "Todo"},
			{ID: "opt-progress", Name: "In Progress"},
			{ID: "opt-done", Name: "Done"},
		},
	}
	s.SetGroupField(groupField)

	// Add test cards
	cards := []*domain.Card{
		{ItemID: "card-1", Title: "Task 1", ContentType: domain.ContentTypeIssue, Number: 101, GroupOptionID: "opt-todo"},
		{ItemID: "card-2", Title: "Task 2", ContentType: domain.ContentTypeIssue, Number: 102, GroupOptionID: "opt-todo"},
		{ItemID: "card-3", Title: "Task 3", ContentType: domain.ContentTypeIssue, Number: 103, GroupOptionID: "opt-progress"},
		{ItemID: "card-4", Title: "Task 4", ContentType: domain.ContentTypeIssue, Number: 104, GroupOptionID: "opt-done"},
		{ItemID: "card-5", Title: "Task 5", ContentType: domain.ContentTypeIssue, Number: 105, GroupOptionID: "opt-done"},
		{ItemID: "card-6", Title: "Task 6", ContentType: domain.ContentTypeIssue, Number: 106, GroupOptionID: "opt-done"},
		{ItemID: "card-7", Title: "No Status Task", ContentType: domain.ContentTypeIssue, Number: 107, GroupOptionID: ""},
	}
	s.UpsertCards(cards)

	return s
}

func TestBoardModel_RebuildColumns(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	// Trigger column rebuild
	(&board).rebuildColumns()

	// Should have 4 columns: Todo, In Progress, Done, No Status
	assert.Equal(t, 4, len(board.columns), "Should have 4 columns")

	// Verify column names
	assert.Equal(t, "Todo", board.columnNames["opt-todo"])
	assert.Equal(t, "In Progress", board.columnNames["opt-progress"])
	assert.Equal(t, "Done", board.columnNames["opt-done"])
	assert.Equal(t, "No Status", board.columnNames[store.NoStatusKey])

	// Verify column order matches options order
	assert.Equal(t, "opt-todo", board.columns[0])
	assert.Equal(t, "opt-progress", board.columns[1])
	assert.Equal(t, "opt-done", board.columns[2])
	assert.Equal(t, store.NoStatusKey, board.columns[3])
}

func TestBoardModel_ApplyFilter(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	(&board).rebuildColumns()
	(&board).applyFilter()

	// Verify cards are grouped correctly
	assert.Equal(t, 2, len(board.filteredCards["opt-todo"]), "Todo should have 2 cards")
	assert.Equal(t, 1, len(board.filteredCards["opt-progress"]), "In Progress should have 1 card")
	assert.Equal(t, 3, len(board.filteredCards["opt-done"]), "Done should have 3 cards")
	assert.Equal(t, 1, len(board.filteredCards[store.NoStatusKey]), "No Status should have 1 card")
}

func TestBoardModel_ApplyFilterWithText(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	(&board).rebuildColumns()
	board.filterText = "Task 1"
	(&board).applyFilter()

	// Only "Task 1" should match in Todo
	assert.Equal(t, 1, len(board.filteredCards["opt-todo"]), "Should have 1 matching card")
	assert.Equal(t, 0, len(board.filteredCards["opt-progress"]), "Should have 0 matching cards")
}

func TestBoardModel_Navigation(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	(&board).rebuildColumns()
	(&board).applyFilter()

	// Initial state
	assert.Equal(t, 0, board.selectedColumn)

	// Move right
	model, _ := board.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	board = model.(BoardModel)
	assert.Equal(t, 1, board.selectedColumn)

	// Move right again
	model, _ = board.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	board = model.(BoardModel)
	assert.Equal(t, 2, board.selectedColumn)

	// Move left
	model, _ = board.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	board = model.(BoardModel)
	assert.Equal(t, 1, board.selectedColumn)
}

func TestBoardModel_CardNavigation(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	(&board).rebuildColumns()
	(&board).applyFilter()
	board.width = 120
	board.height = 40

	// Start at column 0 (Todo), card 0
	assert.Equal(t, 0, board.selectedCard["opt-todo"])

	// Move down
	model, _ := board.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	board = model.(BoardModel)
	assert.Equal(t, 1, board.selectedCard["opt-todo"])

	// Move up
	model, _ = board.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	board = model.(BoardModel)
	assert.Equal(t, 0, board.selectedCard["opt-todo"])

	// Try to move up past top (should stay at 0)
	model, _ = board.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	board = model.(BoardModel)
	assert.Equal(t, 0, board.selectedCard["opt-todo"])
}

func TestBoardModel_RenderColumns_Horizontal(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	(&board).rebuildColumns()
	(&board).applyFilter()
	board.width = 150 // Wide enough for multiple columns
	board.height = 30

	view := board.renderAllColumns()

	// The view should contain multiple column headers on the same or nearby lines
	// indicating horizontal layout
	assert.Contains(t, view, "Todo")
	assert.Contains(t, view, "In Progress")
	assert.Contains(t, view, "Done")
}

func TestBoardModel_EmptyColumn(t *testing.T) {
	s := store.New()

	project := &domain.Project{ID: "proj-1", Number: 1, Title: "Test", Owner: "test"}
	s.SetProject(project)

	groupField := &domain.FieldDef{
		ID:   "field-1",
		Name: "Status",
		Type: domain.FieldTypeSingleSelect,
		Options: []domain.Option{
			{ID: "opt-1", Name: "Todo"},
			{ID: "opt-2", Name: "Done"},
		},
	}
	s.SetGroupField(groupField)

	// Only add cards to one column
	cards := []*domain.Card{
		{ItemID: "card-1", Title: "Task 1", GroupOptionID: "opt-1"},
	}
	s.UpsertCards(cards)

	board := NewBoardModel(s, nil, context.Background())
	(&board).rebuildColumns()
	(&board).applyFilter()
	board.width = 100
	board.height = 20

	// Both columns should exist
	assert.Equal(t, 3, len(board.columns)) // opt-1, opt-2, NoStatus

	// Empty columns should have empty filteredCards
	assert.Equal(t, 1, len(board.filteredCards["opt-1"]))
	assert.Equal(t, 0, len(board.filteredCards["opt-2"]))

	// Rendering should not panic
	view := board.renderAllColumns()
	assert.Contains(t, view, "Todo")
	assert.Contains(t, view, "Done")
}

func TestBoardModel_WindowResize(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	// Simulate window size message
	model, _ := board.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	board = model.(BoardModel)

	assert.Equal(t, 120, board.width)
	assert.Equal(t, 40, board.height)
}

func TestBoardModel_View_NotPanic(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	// Before any initialization, View should not panic
	require.NotPanics(t, func() {
		board.View()
	})

	// After rebuild, View should not panic
	(&board).rebuildColumns()
	(&board).applyFilter()
	board.width = 100
	board.height = 30

	require.NotPanics(t, func() {
		view := board.View()
		assert.NotEmpty(t, view)
	})
}

func TestBoardModel_AllColumnsRendered(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	(&board).rebuildColumns()
	(&board).applyFilter()
	board.width = 200 // Very wide
	board.height = 30

	view := board.View()

	// All columns should be mentioned
	assert.Contains(t, view, "Todo")
	assert.Contains(t, view, "In Progress")
	assert.Contains(t, view, "Done")
	assert.Contains(t, view, "No Status")
}

func TestBoardModel_CardCount(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	(&board).rebuildColumns()
	(&board).applyFilter()

	// Count total cards across all columns
	total := 0
	for _, cards := range board.filteredCards {
		total += len(cards)
	}

	assert.Equal(t, 7, total, "Should have all 7 cards distributed across columns")
}

func TestRenderCard_Truncation(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	longTitle := "This is a very long title that should be truncated to fit the column width properly"
	card := &domain.Card{
		ItemID:      "card-long",
		Title:       longTitle,
		ContentType: domain.ContentTypeIssue,
		Number:      999,
	}

	rendered := board.renderCard(card)

	// Should be truncated (contains ellipsis) - renderCard uses width of 30
	if len(longTitle) > 25 {
		assert.Contains(t, rendered, "…")
	}
	// Should contain issue number
	assert.Contains(t, rendered, "#999")
}

func TestBoardModel_ColumnStyles(t *testing.T) {
	s := createTestStore()
	board := NewBoardModel(s, nil, context.Background())

	(&board).rebuildColumns()
	(&board).applyFilter()
	board.width = 150
	board.height = 30
	board.selectedColumn = 1 // Select "In Progress"

	view := board.renderAllColumns()

	// View should render without panic and contain content
	assert.NotEmpty(t, view)

	// The lines should form a grid-like structure (multiple columns visible)
	lines := strings.Split(view, "\n")
	assert.Greater(t, len(lines), 1, "Should have multiple lines")
}
