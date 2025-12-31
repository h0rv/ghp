package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/h0rv/ghp/internal/gh"
	"github.com/h0rv/ghp/internal/store"
	"github.com/h0rv/ghp/internal/tui"
	"github.com/spf13/cobra"
)

var (
	// CLI flags
	ownerFlag      string
	projectFlag    int
	groupFieldFlag string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "ghp",
		Short: "Terminal UI for GitHub Projects v2",
		Long: `ghp is a terminal user interface for GitHub Projects v2.

Interactive kanban board with keyboard navigation for managing issues and PRs.

Authentication:
  1. GitHub CLI: Run 'gh auth login' (preferred)
  2. Environment variable: Set GITHUB_TOKEN

The token must have read/write access to projects.`,
		RunE: run,
	}

	// Define CLI flags
	rootCmd.Flags().StringVar(&ownerFlag, "owner", "", "GitHub owner (organization or user login). Skips owner prompt.")
	rootCmd.Flags().IntVar(&projectFlag, "project", 0, "Project number. Requires --owner. Skips project picker.")
	rootCmd.Flags().StringVar(&groupFieldFlag, "group-field", "", "Field name to group by. Skips field picker.")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Validate flags
	if projectFlag != 0 && ownerFlag == "" {
		return fmt.Errorf("--project requires --owner to be specified")
	}

	// Create GitHub client (handles authentication)
	client, err := gh.New()
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w\n\nPlease authenticate using:\n  gh auth login\nor set the GITHUB_TOKEN environment variable", err)
	}

	// Create store
	s := store.New()

	// Create context
	ctx := context.Background()

	// Create app model
	app := tui.NewAppModel(client, s, ctx, ownerFlag, projectFlag, groupFieldFlag)

	// Run Bubble Tea program
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("program error: %w", err)
	}

	return nil
}
