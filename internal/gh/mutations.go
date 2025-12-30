package gh

import (
	"context"
	"fmt"

	"github.com/machinebox/graphql"
)

// UpdateItemField updates a project item's SINGLE_SELECT field value.
// This is used to move items between columns in the board view.
func (c *Client) UpdateItemField(ctx context.Context, projectID string, itemID string, fieldID string, optionID string) error {
	req := graphql.NewRequest(`
		mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $value: ProjectV2FieldValue!) {
			updateProjectV2ItemFieldValue(
				input: {
					projectId: $projectId
					itemId: $itemId
					fieldId: $fieldId
					value: $value
				}
			) {
				projectV2Item {
					id
				}
			}
		}
	`)

	req.Var("projectId", projectID)
	req.Var("itemId", itemID)
	req.Var("fieldId", fieldID)
	req.Var("value", map[string]interface{}{
		"singleSelectOptionId": optionID,
	})

	var resp struct {
		UpdateProjectV2ItemFieldValue struct {
			ProjectV2Item struct {
				ID string `json:"id"`
			} `json:"projectV2Item"`
		} `json:"updateProjectV2ItemFieldValue"`
	}

	if err := c.makeRequest(ctx, req, &resp); err != nil {
		return fmt.Errorf("failed to update item field: %w", err)
	}

	return nil
}

// AddComment adds a comment to an issue or pull request.
// Uses the REST-style addComment mutation which requires the issue/PR node ID.
func (c *Client) AddComment(ctx context.Context, owner, repo string, number int, body string) error {
	// First, get the issue/PR node ID
	nodeID, err := c.getIssueOrPRNodeID(ctx, owner, repo, number)
	if err != nil {
		return fmt.Errorf("failed to get issue node ID: %w", err)
	}

	// Then add the comment
	req := graphql.NewRequest(`
		mutation($subjectId: ID!, $body: String!) {
			addComment(input: {subjectId: $subjectId, body: $body}) {
				commentEdge {
					node {
						id
					}
				}
			}
		}
	`)

	req.Var("subjectId", nodeID)
	req.Var("body", body)

	var resp struct {
		AddComment struct {
			CommentEdge struct {
				Node struct {
					ID string `json:"id"`
				} `json:"node"`
			} `json:"commentEdge"`
		} `json:"addComment"`
	}

	if err := c.makeRequest(ctx, req, &resp); err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}

	return nil
}

// getIssueOrPRNodeID retrieves the GraphQL node ID for an issue or PR.
func (c *Client) getIssueOrPRNodeID(ctx context.Context, owner, repo string, number int) (string, error) {
	req := graphql.NewRequest(`
		query($owner: String!, $repo: String!, $number: Int!) {
			repository(owner: $owner, name: $repo) {
				issueOrPullRequest(number: $number) {
					... on Issue {
						id
					}
					... on PullRequest {
						id
					}
				}
			}
		}
	`)

	req.Var("owner", owner)
	req.Var("repo", repo)
	req.Var("number", number)

	var resp struct {
		Repository struct {
			IssueOrPullRequest struct {
				ID string `json:"id"`
			} `json:"issueOrPullRequest"`
		} `json:"repository"`
	}

	if err := c.makeRequest(ctx, req, &resp); err != nil {
		return "", err
	}

	if resp.Repository.IssueOrPullRequest.ID == "" {
		return "", fmt.Errorf("issue or PR #%d not found in %s/%s", number, owner, repo)
	}

	return resp.Repository.IssueOrPullRequest.ID, nil
}
