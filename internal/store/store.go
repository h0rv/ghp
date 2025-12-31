// Package store provides an in-memory state management layer for GitHub Projects data.
// It handles card storage, column grouping, and field selection logic following
// the "deep modules" principle - simple interface hiding complex grouping logic.
package store

import (
	"errors"
	"fmt"
	"strings"

	"github.com/h0rv/ghp/internal/domain"
)

var (
	// ErrNoProject indicates no project has been set in the store.
	ErrNoProject = errors.New("no project set")
	// ErrNoGroupField indicates no grouping field has been set.
	ErrNoGroupField = errors.New("no grouping field set")
	// ErrCardNotFound indicates the requested card does not exist.
	ErrCardNotFound = errors.New("card not found")
	// ErrInvalidOption indicates an invalid option ID was provided.
	ErrInvalidOption = errors.New("invalid option ID")
)

// NoStatusKey is the special key used for cards without a grouping field value.
const NoStatusKey = "_no_status_"

// Store manages the in-memory state of a GitHub Project.
// It provides methods for setting project metadata, upserting cards,
// and querying the grouped column structure.
type Store struct {
	// Project metadata
	project    *domain.Project
	groupField *domain.FieldDef

	// Current user (viewer) login for filtering
	viewerLogin string

	// Card storage
	cards map[string]*domain.Card // ItemID -> Card

	// Column mapping: optionID -> []ItemID
	// Special key NoStatusKey holds cards without a group value
	columns map[string][]string

	// Pagination state
	cursor      string
	hasNextPage bool

	// Rollback state for optimistic updates
	rollbackCard *domain.Card
}

// New creates a new empty Store instance.
func New() *Store {
	return &Store{
		cards:   make(map[string]*domain.Card),
		columns: make(map[string][]string),
	}
}

// SetProject sets the current project metadata.
func (s *Store) SetProject(project *domain.Project) {
	s.project = project
}

// GetProject returns the current project, or nil if not set.
func (s *Store) GetProject() *domain.Project {
	return s.project
}

// SetViewerLogin sets the current authenticated user's login.
func (s *Store) SetViewerLogin(login string) {
	s.viewerLogin = login
}

// GetViewerLogin returns the current authenticated user's login.
func (s *Store) GetViewerLogin() string {
	return s.viewerLogin
}

// SetGroupField sets the field used for grouping cards into columns.
// This will trigger a rebuild of the column mapping.
func (s *Store) SetGroupField(field *domain.FieldDef) {
	s.groupField = field
	s.rebuildColumns()
}

// GetGroupField returns the current grouping field, or nil if not set.
func (s *Store) GetGroupField() *domain.FieldDef {
	return s.groupField
}

// UpsertCards adds or updates multiple cards in the store.
// After upserting, column mappings are automatically rebuilt.
func (s *Store) UpsertCards(cards []*domain.Card) {
	for _, card := range cards {
		s.cards[card.ItemID] = card
	}
	s.rebuildColumns()
}

// GetCard retrieves a card by ItemID, returning ErrCardNotFound if not found.
func (s *Store) GetCard(itemID string) (*domain.Card, error) {
	card, exists := s.cards[itemID]
	if !exists {
		return nil, ErrCardNotFound
	}
	return card, nil
}

// GetAllCards returns all cards in the store.
func (s *Store) GetAllCards() []*domain.Card {
	cards := make([]*domain.Card, 0, len(s.cards))
	for _, card := range s.cards {
		cards = append(cards, card)
	}
	return cards
}

// GetColumns returns the column structure as a map of optionID -> []ItemID.
// The special key NoStatusKey contains cards without a group value.
// Returns ErrNoGroupField if no grouping field is set.
func (s *Store) GetColumns() (map[string][]string, error) {
	if s.groupField == nil {
		return nil, ErrNoGroupField
	}

	// Return a copy to prevent external modification
	result := make(map[string][]string, len(s.columns))
	for optionID, itemIDs := range s.columns {
		// Copy the slice
		ids := make([]string, len(itemIDs))
		copy(ids, itemIDs)
		result[optionID] = ids
	}
	return result, nil
}

// GetColumnCardIDs returns the card IDs for a specific column (optionID or NoStatusKey).
func (s *Store) GetColumnCardIDs(optionID string) []string {
	ids, exists := s.columns[optionID]
	if !exists {
		return []string{}
	}
	// Return a copy
	result := make([]string, len(ids))
	copy(result, ids)
	return result
}

// MoveCard performs an optimistic move of a card to a new column.
// It updates the card's GroupOptionID and rebuilds columns.
// The previous state is saved for potential rollback.
// Returns ErrCardNotFound if the card doesn't exist.
func (s *Store) MoveCard(itemID string, newOptionID string) error {
	card, exists := s.cards[itemID]
	if !exists {
		return ErrCardNotFound
	}

	// Save rollback state (copy the card)
	s.rollbackCard = &domain.Card{
		ItemID:        card.ItemID,
		ContentType:   card.ContentType,
		Title:         card.Title,
		URL:           card.URL,
		Repo:          card.Repo,
		Number:        card.Number,
		GroupOptionID: card.GroupOptionID,
	}

	// Update the card
	card.GroupOptionID = newOptionID
	s.rebuildColumns()

	return nil
}

// RollbackMove reverts the last MoveCard operation.
// This should be called when a mutation fails on the server.
// Returns an error if there is no rollback state.
func (s *Store) RollbackMove() error {
	if s.rollbackCard == nil {
		return errors.New("no rollback state available")
	}

	// Restore the card
	s.cards[s.rollbackCard.ItemID] = s.rollbackCard
	s.rebuildColumns()

	// Clear rollback state
	s.rollbackCard = nil

	return nil
}

// SetPagination updates the pagination state.
func (s *Store) SetPagination(cursor string, hasNextPage bool) {
	s.cursor = cursor
	s.hasNextPage = hasNextPage
}

// GetPagination returns the current pagination state.
func (s *Store) GetPagination() (cursor string, hasNextPage bool) {
	return s.cursor, s.hasNextPage
}

// rebuildColumns reconstructs the column mapping from current cards.
// Cards are grouped by their GroupOptionID, with empty values going to NoStatusKey.
func (s *Store) rebuildColumns() {
	// Clear existing columns
	s.columns = make(map[string][]string)

	// Group cards by their GroupOptionID
	for itemID, card := range s.cards {
		key := card.GroupOptionID
		if key == "" {
			key = NoStatusKey
		}
		s.columns[key] = append(s.columns[key], itemID)
	}
}

// SelectGroupField implements the field selection heuristic from the spec:
// 1. Auto-pick: field name equals "Status" (case-insensitive) AND type SINGLE_SELECT
// 2. Else if exactly one SINGLE_SELECT field exists, pick it
// 3. Else return all SINGLE_SELECT fields for user to choose
//
// Returns:
//   - selected field (if auto-picked)
//   - list of candidate fields (if user choice needed)
//   - error if no SINGLE_SELECT fields exist
func SelectGroupField(fields []*domain.FieldDef) (selected *domain.FieldDef, candidates []*domain.FieldDef, err error) {
	// Collect all SINGLE_SELECT fields
	var singleSelectFields []*domain.FieldDef
	for _, field := range fields {
		if field.Type == domain.FieldTypeSingleSelect {
			singleSelectFields = append(singleSelectFields, field)
		}
	}

	// No SINGLE_SELECT fields - error
	if len(singleSelectFields) == 0 {
		return nil, nil, errors.New("no SINGLE_SELECT fields found in project")
	}

	// Rule 1: Auto-pick "Status" field (case-insensitive)
	for _, field := range singleSelectFields {
		if strings.EqualFold(field.Name, "Status") {
			return field, nil, nil
		}
	}

	// Rule 2: Exactly one SINGLE_SELECT field - auto-pick it
	if len(singleSelectFields) == 1 {
		return singleSelectFields[0], nil, nil
	}

	// Rule 3: Multiple SINGLE_SELECT fields - return candidates for user choice
	return nil, singleSelectFields, nil
}

// ValidateOption checks if an option ID is valid for the current grouping field.
// Returns ErrNoGroupField if no grouping field is set, ErrInvalidOption if invalid.
func (s *Store) ValidateOption(optionID string) error {
	if s.groupField == nil {
		return ErrNoGroupField
	}

	// Empty string is valid (represents "no status")
	if optionID == "" {
		return nil
	}

	// Check if option exists in current group field
	for _, option := range s.groupField.Options {
		if option.ID == optionID {
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrInvalidOption, optionID)
}

// Clear resets the store to empty state, preserving project and group field.
func (s *Store) Clear() {
	s.cards = make(map[string]*domain.Card)
	s.columns = make(map[string][]string)
	s.cursor = ""
	s.hasNextPage = false
	s.rollbackCard = nil
}

// Reset completely resets the store to initial state.
func (s *Store) Reset() {
	s.project = nil
	s.groupField = nil
	s.Clear()
}
