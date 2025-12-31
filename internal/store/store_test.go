package store

import (
	"testing"

	"github.com/h0rv/ghp/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixtures
func createTestProject() *domain.Project {
	return &domain.Project{
		ID:     "proj_123",
		Number: 1,
		Title:  "Test Project",
		Owner:  "testorg",
	}
}

func createTestStatusField() *domain.FieldDef {
	return &domain.FieldDef{
		ID:   "field_status",
		Name: "Status",
		Type: domain.FieldTypeSingleSelect,
		Options: []domain.Option{
			{ID: "opt_todo", Name: "Todo"},
			{ID: "opt_inprogress", Name: "In Progress"},
			{ID: "opt_done", Name: "Done"},
		},
	}
}

func createTestPriorityField() *domain.FieldDef {
	return &domain.FieldDef{
		ID:   "field_priority",
		Name: "Priority",
		Type: domain.FieldTypeSingleSelect,
		Options: []domain.Option{
			{ID: "opt_high", Name: "High"},
			{ID: "opt_low", Name: "Low"},
		},
	}
}

func createTestCards() []*domain.Card {
	return []*domain.Card{
		{
			ItemID:        "item_1",
			ContentType:   domain.ContentTypeIssue,
			Title:         "Fix bug",
			URL:           "https://github.com/test/repo/issues/1",
			Repo:          "test/repo",
			Number:        1,
			GroupOptionID: "opt_todo",
		},
		{
			ItemID:        "item_2",
			ContentType:   domain.ContentTypePullRequest,
			Title:         "Add feature",
			URL:           "https://github.com/test/repo/pull/2",
			Repo:          "test/repo",
			Number:        2,
			GroupOptionID: "opt_inprogress",
		},
		{
			ItemID:        "item_3",
			ContentType:   domain.ContentTypeDraftIssue,
			Title:         "Draft task",
			URL:           "",
			Repo:          "",
			Number:        0,
			GroupOptionID: "", // No status
		},
		{
			ItemID:        "item_4",
			ContentType:   domain.ContentTypeIssue,
			Title:         "Another bug",
			URL:           "https://github.com/test/repo/issues/4",
			Repo:          "test/repo",
			Number:        4,
			GroupOptionID: "opt_done",
		},
	}
}

// TestNew verifies store initialization
func TestNew(t *testing.T) {
	s := New()
	assert.NotNil(t, s)
	assert.NotNil(t, s.cards)
	assert.NotNil(t, s.columns)
	assert.Nil(t, s.project)
	assert.Nil(t, s.groupField)
}

// TestSetProject and GetProject
func TestSetProject(t *testing.T) {
	s := New()
	project := createTestProject()

	s.SetProject(project)

	retrieved := s.GetProject()
	assert.Equal(t, project, retrieved)
}

// TestSetGroupField verifies field setting and column rebuild
func TestSetGroupField(t *testing.T) {
	s := New()
	field := createTestStatusField()

	// Add some cards first
	cards := createTestCards()
	s.UpsertCards(cards)

	// Set group field should trigger column rebuild
	s.SetGroupField(field)

	retrieved := s.GetGroupField()
	assert.Equal(t, field, retrieved)

	// Verify columns were built
	columns, err := s.GetColumns()
	require.NoError(t, err)
	assert.NotEmpty(t, columns)
}

// TestUpsertCards verifies card insertion and update
func TestUpsertCards(t *testing.T) {
	s := New()
	s.SetGroupField(createTestStatusField())

	cards := createTestCards()
	s.UpsertCards(cards)

	// Verify all cards are stored
	assert.Equal(t, len(cards), len(s.cards))

	// Verify we can retrieve each card
	for _, card := range cards {
		retrieved, err := s.GetCard(card.ItemID)
		require.NoError(t, err)
		assert.Equal(t, card, retrieved)
	}

	// Test update: modify existing card
	updatedCard := &domain.Card{
		ItemID:        "item_1",
		ContentType:   domain.ContentTypeIssue,
		Title:         "Updated title",
		URL:           "https://github.com/test/repo/issues/1",
		Repo:          "test/repo",
		Number:        1,
		GroupOptionID: "opt_done", // Changed status
	}
	s.UpsertCards([]*domain.Card{updatedCard})

	retrieved, err := s.GetCard("item_1")
	require.NoError(t, err)
	assert.Equal(t, "Updated title", retrieved.Title)
	assert.Equal(t, "opt_done", retrieved.GroupOptionID)
}

// TestGetCard verifies card retrieval
func TestGetCard(t *testing.T) {
	s := New()
	cards := createTestCards()
	s.UpsertCards(cards)

	t.Run("existing card", func(t *testing.T) {
		card, err := s.GetCard("item_1")
		require.NoError(t, err)
		assert.Equal(t, "item_1", card.ItemID)
	})

	t.Run("nonexistent card", func(t *testing.T) {
		card, err := s.GetCard("nonexistent")
		assert.ErrorIs(t, err, ErrCardNotFound)
		assert.Nil(t, card)
	})
}

// TestGetAllCards verifies retrieving all cards
func TestGetAllCards(t *testing.T) {
	s := New()
	cards := createTestCards()
	s.UpsertCards(cards)

	all := s.GetAllCards()
	assert.Equal(t, len(cards), len(all))
}

// TestGetColumns verifies column grouping logic
func TestGetColumns(t *testing.T) {
	s := New()

	t.Run("no group field set", func(t *testing.T) {
		columns, err := s.GetColumns()
		assert.ErrorIs(t, err, ErrNoGroupField)
		assert.Nil(t, columns)
	})

	t.Run("with group field", func(t *testing.T) {
		s.SetGroupField(createTestStatusField())
		cards := createTestCards()
		s.UpsertCards(cards)

		columns, err := s.GetColumns()
		require.NoError(t, err)

		// Verify expected columns exist
		assert.Contains(t, columns, "opt_todo")
		assert.Contains(t, columns, "opt_inprogress")
		assert.Contains(t, columns, "opt_done")
		assert.Contains(t, columns, NoStatusKey)

		// Verify card counts
		assert.Len(t, columns["opt_todo"], 1)
		assert.Len(t, columns["opt_inprogress"], 1)
		assert.Len(t, columns["opt_done"], 1)
		assert.Len(t, columns[NoStatusKey], 1)

		// Verify card IDs
		assert.Contains(t, columns["opt_todo"], "item_1")
		assert.Contains(t, columns["opt_inprogress"], "item_2")
		assert.Contains(t, columns["opt_done"], "item_4")
		assert.Contains(t, columns[NoStatusKey], "item_3")
	})

	t.Run("immutability check", func(t *testing.T) {
		s := New()
		s.SetGroupField(createTestStatusField())
		s.UpsertCards(createTestCards())

		columns, err := s.GetColumns()
		require.NoError(t, err)

		// Modify returned columns
		columns["opt_todo"] = []string{"fake_id"}

		// Get columns again and verify original data is unchanged
		columns2, err := s.GetColumns()
		require.NoError(t, err)
		assert.NotEqual(t, columns["opt_todo"], columns2["opt_todo"])
		assert.Contains(t, columns2["opt_todo"], "item_1")
	})
}

// TestGetColumnCardIDs verifies retrieving cards for a specific column
func TestGetColumnCardIDs(t *testing.T) {
	s := New()
	s.SetGroupField(createTestStatusField())
	cards := createTestCards()
	s.UpsertCards(cards)

	t.Run("existing column", func(t *testing.T) {
		ids := s.GetColumnCardIDs("opt_todo")
		assert.Len(t, ids, 1)
		assert.Contains(t, ids, "item_1")
	})

	t.Run("no status column", func(t *testing.T) {
		ids := s.GetColumnCardIDs(NoStatusKey)
		assert.Len(t, ids, 1)
		assert.Contains(t, ids, "item_3")
	})

	t.Run("empty column", func(t *testing.T) {
		ids := s.GetColumnCardIDs("nonexistent")
		assert.Empty(t, ids)
	})
}

// TestMoveCard verifies optimistic card moves
func TestMoveCard(t *testing.T) {
	s := New()
	s.SetGroupField(createTestStatusField())
	cards := createTestCards()
	s.UpsertCards(cards)

	t.Run("successful move", func(t *testing.T) {
		err := s.MoveCard("item_1", "opt_done")
		require.NoError(t, err)

		// Verify card moved
		card, err := s.GetCard("item_1")
		require.NoError(t, err)
		assert.Equal(t, "opt_done", card.GroupOptionID)

		// Verify columns updated
		columns, err := s.GetColumns()
		require.NoError(t, err)
		assert.NotContains(t, columns["opt_todo"], "item_1")
		assert.Contains(t, columns["opt_done"], "item_1")
	})

	t.Run("move to empty status", func(t *testing.T) {
		err := s.MoveCard("item_2", "")
		require.NoError(t, err)

		card, err := s.GetCard("item_2")
		require.NoError(t, err)
		assert.Equal(t, "", card.GroupOptionID)

		// Should appear in NoStatusKey bucket
		ids := s.GetColumnCardIDs(NoStatusKey)
		assert.Contains(t, ids, "item_2")
	})

	t.Run("nonexistent card", func(t *testing.T) {
		err := s.MoveCard("nonexistent", "opt_done")
		assert.ErrorIs(t, err, ErrCardNotFound)
	})
}

// TestRollbackMove verifies move rollback functionality
func TestRollbackMove(t *testing.T) {
	s := New()
	s.SetGroupField(createTestStatusField())
	cards := createTestCards()
	s.UpsertCards(cards)

	t.Run("successful rollback", func(t *testing.T) {
		// Get original state
		originalCard, err := s.GetCard("item_1")
		require.NoError(t, err)
		originalStatus := originalCard.GroupOptionID

		// Move card
		err = s.MoveCard("item_1", "opt_done")
		require.NoError(t, err)

		// Verify move happened
		movedCard, err := s.GetCard("item_1")
		require.NoError(t, err)
		assert.Equal(t, "opt_done", movedCard.GroupOptionID)

		// Rollback
		err = s.RollbackMove()
		require.NoError(t, err)

		// Verify rollback
		rolledBackCard, err := s.GetCard("item_1")
		require.NoError(t, err)
		assert.Equal(t, originalStatus, rolledBackCard.GroupOptionID)

		// Verify columns updated
		columns, err := s.GetColumns()
		require.NoError(t, err)
		assert.Contains(t, columns[originalStatus], "item_1")
		assert.NotContains(t, columns["opt_done"], "item_1")
	})

	t.Run("no rollback state", func(t *testing.T) {
		s2 := New()
		err := s2.RollbackMove()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no rollback state")
	})

	t.Run("rollback clears state", func(t *testing.T) {
		s3 := New()
		s3.SetGroupField(createTestStatusField())
		s3.UpsertCards(createTestCards())

		// Move and rollback
		_ = s3.MoveCard("item_1", "opt_done")
		err := s3.RollbackMove()
		require.NoError(t, err)

		// Try to rollback again - should fail
		err = s3.RollbackMove()
		assert.Error(t, err)
	})
}

// TestPagination verifies pagination state management
func TestPagination(t *testing.T) {
	s := New()

	// Initial state
	cursor, hasNext := s.GetPagination()
	assert.Equal(t, "", cursor)
	assert.False(t, hasNext)

	// Set pagination
	s.SetPagination("cursor_123", true)
	cursor, hasNext = s.GetPagination()
	assert.Equal(t, "cursor_123", cursor)
	assert.True(t, hasNext)

	// Update pagination
	s.SetPagination("cursor_456", false)
	cursor, hasNext = s.GetPagination()
	assert.Equal(t, "cursor_456", cursor)
	assert.False(t, hasNext)
}

// TestSelectGroupField_AutoPickStatus verifies Rule 1: auto-pick "Status" field
func TestSelectGroupField_AutoPickStatus(t *testing.T) {
	fields := []*domain.FieldDef{
		createTestPriorityField(),
		createTestStatusField(),
		{
			ID:   "field_team",
			Name: "Team",
			Type: domain.FieldTypeSingleSelect,
			Options: []domain.Option{
				{ID: "opt_team1", Name: "Team 1"},
			},
		},
	}

	selected, candidates, err := SelectGroupField(fields)
	require.NoError(t, err)
	assert.NotNil(t, selected)
	assert.Nil(t, candidates)
	assert.Equal(t, "Status", selected.Name)
}

// TestSelectGroupField_CaseInsensitive verifies case-insensitive "Status" matching
func TestSelectGroupField_CaseInsensitive(t *testing.T) {
	testCases := []string{"STATUS", "status", "StAtUs", "Status"}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			fields := []*domain.FieldDef{
				{
					ID:      "field_status",
					Name:    name,
					Type:    domain.FieldTypeSingleSelect,
					Options: []domain.Option{{ID: "opt_1", Name: "Option 1"}},
				},
			}

			selected, candidates, err := SelectGroupField(fields)
			require.NoError(t, err)
			assert.NotNil(t, selected)
			assert.Nil(t, candidates)
			assert.Equal(t, name, selected.Name)
		})
	}
}

// TestSelectGroupField_SingleField verifies Rule 2: auto-pick single SINGLE_SELECT
func TestSelectGroupField_SingleField(t *testing.T) {
	fields := []*domain.FieldDef{
		{
			ID:   "field_text",
			Name: "Description",
			Type: domain.FieldTypeText,
		},
		createTestPriorityField(),
		{
			ID:   "field_number",
			Name: "Points",
			Type: domain.FieldTypeNumber,
		},
	}

	selected, candidates, err := SelectGroupField(fields)
	require.NoError(t, err)
	assert.NotNil(t, selected)
	assert.Nil(t, candidates)
	assert.Equal(t, "Priority", selected.Name)
}

// TestSelectGroupField_MultipleCandidates verifies Rule 3: return candidates
func TestSelectGroupField_MultipleCandidates(t *testing.T) {
	fields := []*domain.FieldDef{
		createTestPriorityField(),
		{
			ID:   "field_team",
			Name: "Team",
			Type: domain.FieldTypeSingleSelect,
			Options: []domain.Option{
				{ID: "opt_team1", Name: "Team 1"},
			},
		},
		{
			ID:   "field_text",
			Name: "Description",
			Type: domain.FieldTypeText,
		},
	}

	selected, candidates, err := SelectGroupField(fields)
	require.NoError(t, err)
	assert.Nil(t, selected)
	assert.NotNil(t, candidates)
	assert.Len(t, candidates, 2)

	// Verify both SINGLE_SELECT fields are in candidates
	names := []string{candidates[0].Name, candidates[1].Name}
	assert.Contains(t, names, "Priority")
	assert.Contains(t, names, "Team")
}

// TestSelectGroupField_NoSingleSelectFields verifies error when no SINGLE_SELECT
func TestSelectGroupField_NoSingleSelectFields(t *testing.T) {
	fields := []*domain.FieldDef{
		{
			ID:   "field_text",
			Name: "Description",
			Type: domain.FieldTypeText,
		},
		{
			ID:   "field_number",
			Name: "Points",
			Type: domain.FieldTypeNumber,
		},
	}

	selected, candidates, err := SelectGroupField(fields)
	assert.Error(t, err)
	assert.Nil(t, selected)
	assert.Nil(t, candidates)
	assert.Contains(t, err.Error(), "no SINGLE_SELECT fields found")
}

// TestSelectGroupField_EmptyFields verifies error with no fields
func TestSelectGroupField_EmptyFields(t *testing.T) {
	fields := []*domain.FieldDef{}

	selected, candidates, err := SelectGroupField(fields)
	assert.Error(t, err)
	assert.Nil(t, selected)
	assert.Nil(t, candidates)
}

// TestValidateOption verifies option validation
func TestValidateOption(t *testing.T) {
	s := New()

	t.Run("no group field", func(t *testing.T) {
		err := s.ValidateOption("opt_todo")
		assert.ErrorIs(t, err, ErrNoGroupField)
	})

	s.SetGroupField(createTestStatusField())

	t.Run("valid option", func(t *testing.T) {
		err := s.ValidateOption("opt_todo")
		assert.NoError(t, err)
	})

	t.Run("empty string is valid", func(t *testing.T) {
		err := s.ValidateOption("")
		assert.NoError(t, err)
	})

	t.Run("invalid option", func(t *testing.T) {
		err := s.ValidateOption("opt_nonexistent")
		assert.ErrorIs(t, err, ErrInvalidOption)
	})
}

// TestClear verifies clearing cards while preserving metadata
func TestClear(t *testing.T) {
	s := New()
	project := createTestProject()
	field := createTestStatusField()

	s.SetProject(project)
	s.SetGroupField(field)
	s.UpsertCards(createTestCards())
	s.SetPagination("cursor_123", true)

	s.Clear()

	// Verify cards and columns cleared
	assert.Empty(t, s.cards)
	assert.Empty(t, s.columns)

	// Verify pagination cleared
	cursor, hasNext := s.GetPagination()
	assert.Equal(t, "", cursor)
	assert.False(t, hasNext)

	// Verify metadata preserved
	assert.Equal(t, project, s.GetProject())
	assert.Equal(t, field, s.GetGroupField())
}

// TestReset verifies complete store reset
func TestReset(t *testing.T) {
	s := New()
	s.SetProject(createTestProject())
	s.SetGroupField(createTestStatusField())
	s.UpsertCards(createTestCards())
	s.SetPagination("cursor_123", true)

	s.Reset()

	// Verify everything cleared
	assert.Empty(t, s.cards)
	assert.Empty(t, s.columns)
	assert.Nil(t, s.GetProject())
	assert.Nil(t, s.GetGroupField())

	cursor, hasNext := s.GetPagination()
	assert.Equal(t, "", cursor)
	assert.False(t, hasNext)
}

// TestColumnRebuildOnGroupFieldChange verifies columns rebuild when group field changes
func TestColumnRebuildOnGroupFieldChange(t *testing.T) {
	s := New()
	statusField := createTestStatusField()
	priorityField := createTestPriorityField()

	// Add cards with status grouping
	cards := []*domain.Card{
		{
			ItemID:        "item_1",
			ContentType:   domain.ContentTypeIssue,
			Title:         "Task 1",
			GroupOptionID: "opt_todo",
		},
		{
			ItemID:        "item_2",
			ContentType:   domain.ContentTypeIssue,
			Title:         "Task 2",
			GroupOptionID: "opt_high", // Priority field option
		},
	}

	s.SetGroupField(statusField)
	s.UpsertCards(cards)

	// Get columns with status field
	columns1, err := s.GetColumns()
	require.NoError(t, err)
	assert.Contains(t, columns1, "opt_todo")

	// Change to priority field - should rebuild columns
	s.SetGroupField(priorityField)

	columns2, err := s.GetColumns()
	require.NoError(t, err)
	// Both cards should now be in their respective columns based on GroupOptionID
	assert.Contains(t, columns2, "opt_todo") // item_1 still has opt_todo
	assert.Contains(t, columns2, "opt_high") // item_2 has opt_high
}

// TestConcurrentCardOperations verifies store behavior with multiple operations
func TestConcurrentCardOperations(t *testing.T) {
	s := New()
	s.SetGroupField(createTestStatusField())

	// Add initial cards
	cards1 := []*domain.Card{
		{ItemID: "item_1", ContentType: domain.ContentTypeIssue, Title: "Task 1", GroupOptionID: "opt_todo"},
	}
	s.UpsertCards(cards1)

	// Add more cards
	cards2 := []*domain.Card{
		{ItemID: "item_2", ContentType: domain.ContentTypeIssue, Title: "Task 2", GroupOptionID: "opt_inprogress"},
		{ItemID: "item_3", ContentType: domain.ContentTypeIssue, Title: "Task 3", GroupOptionID: "opt_done"},
	}
	s.UpsertCards(cards2)

	// Move a card
	err := s.MoveCard("item_1", "opt_done")
	require.NoError(t, err)

	// Verify final state
	columns, err := s.GetColumns()
	require.NoError(t, err)

	assert.Len(t, columns["opt_done"], 2) // item_1 and item_3
	assert.Len(t, columns["opt_inprogress"], 1)
	assert.NotContains(t, columns, "opt_todo") // Empty columns might not exist in map
}
