// Package domain defines the normalized domain types for GitHub Projects v2.
// These types represent the core concepts independent of the GitHub GraphQL API structure.
package domain

// Project represents a GitHub Project v2 instance.
type Project struct {
	ID     string // GitHub Project node ID
	Number int    // Project number within the owner's namespace
	Title  string // Project title
	Owner  string // Owner login (organization or user)
}

// FieldDef represents a project field definition with its metadata.
type FieldDef struct {
	ID      string   // GitHub field node ID
	Name    string   // Field name (e.g., "Status")
	Type    string   // Field type (e.g., "SINGLE_SELECT", "TEXT", etc.)
	Options []Option // Available options for SINGLE_SELECT fields
	Order   int      // Field order in the project (from API response order)
}

// Option represents a single option value for a SINGLE_SELECT field.
type Option struct {
	ID    string // GitHub option node ID
	Name  string // Option name displayed to users (e.g., "In Progress", "Done")
	Color string // Option color (e.g., "GREEN", "YELLOW")
	Order int    // Option order within the field (from API response order)
}

// Card represents a project item (Issue, PR, or Draft) in a normalized format.
type Card struct {
	ItemID        string   // GitHub ProjectV2Item node ID
	ContentType   string   // Type: "Issue", "PullRequest", "DraftIssue", or "Private"
	Title         string   // Item title
	URL           string   // Item URL (may be empty for drafts or private items)
	Repo          string   // Repository nameWithOwner (e.g., "owner/repo"), only for Issue/PR
	Number        int      // Issue/PR number, only for Issue/PR (0 for drafts/private)
	GroupOptionID string   // Current value of the grouping field (option ID), empty if unset
	Assignees     []string // Login names of assigned users
	Body          string   // Issue/PR body (for detail view)
	State         string   // Issue/PR state (OPEN, CLOSED, MERGED)
	Labels        []string // Label names
	Author        string   // Author login (issue/PR creator)
	CreatedAt     string   // ISO8601 timestamp of creation
}

// Comment represents a comment on an Issue or PR.
type Comment struct {
	ID        string // GitHub comment node ID
	Author    string // Author login (may be empty if user deleted)
	Body      string // Comment body text
	CreatedAt string // ISO8601 timestamp
	UpdatedAt string // ISO8601 timestamp
}

// FieldType constants for commonly used field types.
const (
	FieldTypeSingleSelect = "SINGLE_SELECT"
	FieldTypeText         = "TEXT"
	FieldTypeNumber       = "NUMBER"
	FieldTypeDate         = "DATE"
	FieldTypeIteration    = "ITERATION"
)

// ContentType constants for card types.
const (
	ContentTypeIssue       = "Issue"
	ContentTypePullRequest = "PullRequest"
	ContentTypeDraftIssue  = "DraftIssue"
	ContentTypePrivate     = "Private"
)
