// Package auth provides GitHub authentication token management.
// It implements a simple interface with multiple providers following the
// "deep modules" principle - simple interface, complex implementation hidden.
package auth

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TokenProvider defines the interface for obtaining a GitHub authentication token.
// Implementations may use different sources (CLI tools, environment variables, etc).
type TokenProvider interface {
	GetToken() (string, error)
}

// GhCliProvider obtains tokens by shelling out to the GitHub CLI (`gh auth token`).
// This is the preferred method as it respects the user's gh CLI authentication state.
type GhCliProvider struct{}

// GetToken shells out to `gh auth token` to retrieve the current token.
// Returns an error if gh CLI is not installed, not authenticated, or the command fails.
func (g *GhCliProvider) GetToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token", "--hostname", "github.com")
	output, err := cmd.Output()
	if err != nil {
		// Check if it's an exec error (gh not found)
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "", errors.New("gh CLI not found in PATH")
		}
		// Other errors (not authenticated, etc)
		return "", fmt.Errorf("gh auth token failed: %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", errors.New("gh auth token returned empty token")
	}

	return token, nil
}

// EnvProvider obtains tokens from the GITHUB_TOKEN environment variable.
// This is the fallback method when gh CLI is not available.
type EnvProvider struct{}

// GetToken reads the GITHUB_TOKEN environment variable.
// Returns an error if the variable is not set or is empty.
func (e *EnvProvider) GetToken() (string, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return "", errors.New("GITHUB_TOKEN environment variable not set or empty")
	}
	return token, nil
}

// GetToken attempts to obtain a GitHub token using the following strategy:
// 1. Try gh CLI first (preferred method)
// 2. Fall back to GITHUB_TOKEN environment variable
// 3. Return a clear, actionable error if both fail
//
// This is the main entry point for token retrieval in the application.
func GetToken() (string, error) {
	// Try gh CLI first
	ghCli := &GhCliProvider{}
	token, err := ghCli.GetToken()
	if err == nil {
		return token, nil
	}

	// Store gh CLI error for later
	ghErr := err

	// Fall back to environment variable
	envProvider := &EnvProvider{}
	token, err = envProvider.GetToken()
	if err == nil {
		return token, nil
	}

	// Both failed - return actionable error
	return "", fmt.Errorf(
		"failed to obtain GitHub token: gh CLI error (%v) and GITHUB_TOKEN not set.\n"+
			"Please either:\n"+
			"  1. Run 'gh auth login' to authenticate with GitHub CLI, or\n"+
			"  2. Set the GITHUB_TOKEN environment variable with a personal access token",
		ghErr,
	)
}
