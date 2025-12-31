// Package tui provides Bubble Tea models for the interactive TUI.
package tui

import (
	"github.com/h0rv/ghp/internal/domain"
	"github.com/h0rv/ghp/internal/gh"
)

// OwnerSelectedMsg is emitted when the user selects an owner.
type OwnerSelectedMsg struct {
	Owner     string
	OwnerType gh.OwnerType // Pre-resolved owner type (if available)
	OwnerID   string       // Pre-resolved owner ID (if available)
}

// ProjectSelectedMsg is emitted when the user selects a project.
type ProjectSelectedMsg struct {
	Project domain.Project
}

// FieldSelectedMsg is emitted when the user selects a grouping field.
type FieldSelectedMsg struct {
	Field domain.FieldDef
}

// ErrorMsg is emitted when an error occurs.
type ErrorMsg struct {
	Err error
}

// QuitMsg is emitted when the user requests to quit.
type QuitMsg struct{}
