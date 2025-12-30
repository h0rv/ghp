// Package gh provides a GraphQL client for GitHub Projects v2 API.
// It implements a deep module interface - simple methods hiding complex GraphQL queries.
package gh

import (
	"context"
	"fmt"

	"github.com/machinebox/graphql"
	"github.com/robby/ghp/internal/auth"
)

// Client is a GitHub GraphQL API client for Projects v2.
// It provides high-level methods for querying and mutating project data.
type Client struct {
	gql   *graphql.Client
	token string
}

// New creates a new GitHub GraphQL client.
// It obtains an authentication token using the auth package.
// Returns an error if token retrieval fails.
func New() (*Client, error) {
	token, err := auth.GetToken()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain GitHub token: %w", err)
	}

	client := graphql.NewClient("https://api.github.com/graphql")

	return &Client{
		gql:   client,
		token: token,
	}, nil
}

// makeRequest executes a GraphQL request with authentication.
// This is a helper method to avoid repeating the authorization header setup.
func (c *Client) makeRequest(ctx context.Context, req *graphql.Request, resp interface{}) error {
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.gql.Run(ctx, req, resp)
}
