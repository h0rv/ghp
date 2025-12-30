package gh

import (
	"context"
	"fmt"

	"github.com/machinebox/graphql"
	"github.com/robby/ghp/internal/domain"
)

// OwnerType represents whether an owner is an organization or user.
type OwnerType string

const (
	OwnerTypeOrganization OwnerType = "Organization"
	OwnerTypeUser         OwnerType = "User"
)

// Owner represents an owner (user or organization) that can have projects.
type Owner struct {
	Login string
	ID    string
	Type  OwnerType
}

// GetViewerAndOrgs returns the authenticated user and their organizations.
// This allows users to pick from available owners without typing.
func (c *Client) GetViewerAndOrgs(ctx context.Context) ([]Owner, error) {
	req := graphql.NewRequest(`
		query {
			viewer {
				login
				id
				organizations(first: 100) {
					nodes {
						login
						id
					}
				}
			}
		}
	`)

	var resp struct {
		Viewer struct {
			Login         string `json:"login"`
			ID            string `json:"id"`
			Organizations struct {
				Nodes []struct {
					Login string `json:"login"`
					ID    string `json:"id"`
				} `json:"nodes"`
			} `json:"organizations"`
		} `json:"viewer"`
	}

	if err := c.makeRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get viewer and orgs: %w", err)
	}

	owners := make([]Owner, 0, 1+len(resp.Viewer.Organizations.Nodes))

	// Add the authenticated user first
	owners = append(owners, Owner{
		Login: resp.Viewer.Login,
		ID:    resp.Viewer.ID,
		Type:  OwnerTypeUser,
	})

	// Add organizations
	for _, org := range resp.Viewer.Organizations.Nodes {
		owners = append(owners, Owner{
			Login: org.Login,
			ID:    org.ID,
			Type:  OwnerTypeOrganization,
		})
	}

	return owners, nil
}

// ResolveOwner determines if a login is an organization or user.
// Returns the owner type, owner ID, and error if the login doesn't exist.
func (c *Client) ResolveOwner(ctx context.Context, login string) (OwnerType, string, error) {
	req := graphql.NewRequest(`
		query($login: String!) {
			organization(login: $login) {
				id
			}
			user(login: $login) {
				id
			}
		}
	`)
	req.Var("login", login)

	var resp struct {
		Organization *struct {
			ID string `json:"id"`
		} `json:"organization"`
		User *struct {
			ID string `json:"id"`
		} `json:"user"`
	}

	if err := c.makeRequest(ctx, req, &resp); err != nil {
		return "", "", fmt.Errorf("failed to resolve owner: %w", err)
	}

	// Check organization first
	if resp.Organization != nil {
		return OwnerTypeOrganization, resp.Organization.ID, nil
	}

	// Then check user
	if resp.User != nil {
		return OwnerTypeUser, resp.User.ID, nil
	}

	return "", "", fmt.Errorf("login '%s' not found (neither organization nor user)", login)
}

// ListProjects lists all projects for a given owner.
// The ownerType should be "Organization" or "User".
// Returns a slice of domain.Project objects.
func (c *Client) ListProjects(ctx context.Context, ownerType OwnerType, ownerID string, login string) ([]domain.Project, error) {
	var query string

	if ownerType == OwnerTypeOrganization {
		query = `
			query($id: ID!, $first: Int!) {
				node(id: $id) {
					... on Organization {
						projectsV2(first: $first) {
							nodes {
								id
								number
								title
							}
						}
					}
				}
			}
		`
	} else {
		query = `
			query($id: ID!, $first: Int!) {
				node(id: $id) {
					... on User {
						projectsV2(first: $first) {
							nodes {
								id
								number
								title
							}
						}
					}
				}
			}
		`
	}

	req := graphql.NewRequest(query)
	req.Var("id", ownerID)
	req.Var("first", 100) // Fetch up to 100 projects

	var resp struct {
		Node struct {
			ProjectsV2 struct {
				Nodes []struct {
					ID     string `json:"id"`
					Number int    `json:"number"`
					Title  string `json:"title"`
				} `json:"nodes"`
			} `json:"projectsV2"`
		} `json:"node"`
	}

	if err := c.makeRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	projects := make([]domain.Project, 0, len(resp.Node.ProjectsV2.Nodes))
	for _, node := range resp.Node.ProjectsV2.Nodes {
		projects = append(projects, domain.Project{
			ID:     node.ID,
			Number: node.Number,
			Title:  node.Title,
			Owner:  login,
		})
	}

	return projects, nil
}

// GetProjectFields fetches all fields for a project, including options for SINGLE_SELECT fields.
// Options are returned in their configured order from GitHub (the order shown in the project UI).
func (c *Client) GetProjectFields(ctx context.Context, projectID string) ([]domain.FieldDef, error) {
	req := graphql.NewRequest(`
		query($projectId: ID!) {
			node(id: $projectId) {
				... on ProjectV2 {
					fields(first: 50) {
						nodes {
							... on ProjectV2Field {
								id
								name
								dataType
							}
							... on ProjectV2SingleSelectField {
								id
								name
								dataType
								options {
									id
									name
									color
								}
							}
							... on ProjectV2IterationField {
								id
								name
								dataType
							}
						}
					}
				}
			}
		}
	`)
	req.Var("projectId", projectID)

	var resp struct {
		Node struct {
			Fields struct {
				Nodes []struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					DataType string `json:"dataType"`
					Options  []struct {
						ID    string `json:"id"`
						Name  string `json:"name"`
						Color string `json:"color"`
					} `json:"options"`
				} `json:"nodes"`
			} `json:"fields"`
		} `json:"node"`
	}

	if err := c.makeRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get project fields: %w", err)
	}

	fields := make([]domain.FieldDef, 0, len(resp.Node.Fields.Nodes))
	for idx, node := range resp.Node.Fields.Nodes {
		field := domain.FieldDef{
			ID:   node.ID,
			Name: node.Name,
			Type: node.DataType,
		}

		// Only SINGLE_SELECT fields have options
		// The API returns options in their configured order
		if node.DataType == domain.FieldTypeSingleSelect && len(node.Options) > 0 {
			field.Options = make([]domain.Option, 0, len(node.Options))
			for optIdx, opt := range node.Options {
				field.Options = append(field.Options, domain.Option{
					ID:    opt.ID,
					Name:  opt.Name,
					Color: opt.Color,
					Order: optIdx, // Preserve order from API response
				})
			}
		}

		// Store field order as well
		field.Order = idx
		fields = append(fields, field)
	}

	return fields, nil
}

// GetItems fetches project items with pagination.
// Fetches grouping field value and assignees for filtering.
// Returns cards, next cursor, and whether there are more items.
func (c *Client) GetItems(ctx context.Context, projectID string, groupFieldName string, cursor string, limit int) ([]domain.Card, string, bool, error) {
	query := `
		query($projectId: ID!, $first: Int!, $after: String, $fieldName: String!) {
			node(id: $projectId) {
				... on ProjectV2 {
					items(first: $first, after: $after) {
						pageInfo {
							hasNextPage
							endCursor
						}
						nodes {
							id
							fieldValueByName(name: $fieldName) {
								... on ProjectV2ItemFieldSingleSelectValue {
									optionId
								}
							}
							content {
								__typename
								... on Issue {
									title
									body
									url
									number
									state
									createdAt
									author {
										login
									}
									repository {
										nameWithOwner
									}
									assignees(first: 10) {
										nodes {
											login
										}
									}
									labels(first: 10) {
										nodes {
											name
										}
									}
								}
								... on PullRequest {
									title
									body
									url
									number
									state
									createdAt
									author {
										login
									}
									repository {
										nameWithOwner
									}
									assignees(first: 10) {
										nodes {
											login
										}
									}
									labels(first: 10) {
										nodes {
											name
										}
									}
								}
								... on DraftIssue {
									title
								}
							}
						}
					}
				}
			}
		}
	`

	req := graphql.NewRequest(query)
	req.Var("projectId", projectID)
	req.Var("first", limit)
	req.Var("fieldName", groupFieldName)
	if cursor != "" {
		req.Var("after", cursor)
	} else {
		req.Var("after", nil)
	}

	var resp struct {
		Node struct {
			Items struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					ID               string `json:"id"`
					FieldValueByName *struct {
						OptionID string `json:"optionId"`
					} `json:"fieldValueByName"`
					Content *struct {
						Typename  string `json:"__typename"`
						Title     string `json:"title"`
						Body      string `json:"body"`
						URL       string `json:"url"`
						Number    int    `json:"number"`
						State     string `json:"state"`
						CreatedAt string `json:"createdAt"`
						Author    *struct {
							Login string `json:"login"`
						} `json:"author"`
						Repository *struct {
							NameWithOwner string `json:"nameWithOwner"`
						} `json:"repository"`
						Assignees *struct {
							Nodes []struct {
								Login string `json:"login"`
							} `json:"nodes"`
						} `json:"assignees"`
						Labels *struct {
							Nodes []struct {
								Name string `json:"name"`
							} `json:"nodes"`
						} `json:"labels"`
					} `json:"content"`
				} `json:"nodes"`
			} `json:"items"`
		} `json:"node"`
	}

	if err := c.makeRequest(ctx, req, &resp); err != nil {
		return nil, "", false, fmt.Errorf("failed to get items: %w", err)
	}

	cards := make([]domain.Card, 0, len(resp.Node.Items.Nodes))
	for _, node := range resp.Node.Items.Nodes {
		card := domain.Card{
			ItemID: node.ID,
		}

		// Extract group option ID if present
		if node.FieldValueByName != nil {
			card.GroupOptionID = node.FieldValueByName.OptionID
		}

		// Handle content union (Issue/PR/Draft/null)
		if node.Content == nil {
			// Null content (private or deleted item)
			card.ContentType = domain.ContentTypePrivate
			card.Title = "(private item)"
		} else {
			// Extract assignees
			if node.Content.Assignees != nil {
				card.Assignees = make([]string, 0, len(node.Content.Assignees.Nodes))
				for _, a := range node.Content.Assignees.Nodes {
					card.Assignees = append(card.Assignees, a.Login)
				}
			}

			// Extract labels
			if node.Content.Labels != nil {
				card.Labels = make([]string, 0, len(node.Content.Labels.Nodes))
				for _, l := range node.Content.Labels.Nodes {
					card.Labels = append(card.Labels, l.Name)
				}
			}

			// Extract author and createdAt
			card.CreatedAt = node.Content.CreatedAt
			if node.Content.Author != nil {
				card.Author = node.Content.Author.Login
			}

			switch node.Content.Typename {
			case "Issue":
				card.ContentType = domain.ContentTypeIssue
				card.Title = node.Content.Title
				card.Body = node.Content.Body
				card.URL = node.Content.URL
				card.Number = node.Content.Number
				card.State = node.Content.State
				if node.Content.Repository != nil {
					card.Repo = node.Content.Repository.NameWithOwner
				}
			case "PullRequest":
				card.ContentType = domain.ContentTypePullRequest
				card.Title = node.Content.Title
				card.Body = node.Content.Body
				card.URL = node.Content.URL
				card.Number = node.Content.Number
				card.State = node.Content.State
				if node.Content.Repository != nil {
					card.Repo = node.Content.Repository.NameWithOwner
				}
			case "DraftIssue":
				card.ContentType = domain.ContentTypeDraftIssue
				card.Title = node.Content.Title
				card.URL = node.Content.URL // May be empty for drafts
			default:
				// Unknown type - treat as private
				card.ContentType = domain.ContentTypePrivate
				card.Title = "(unknown item type)"
			}
		}

		cards = append(cards, card)
	}

	return cards, resp.Node.Items.PageInfo.EndCursor, resp.Node.Items.PageInfo.HasNextPage, nil
}

// GetComments fetches comments for an issue or pull request.
func (c *Client) GetComments(ctx context.Context, owner, repo string, number int) ([]domain.Comment, error) {
	req := graphql.NewRequest(`
		query($owner: String!, $repo: String!, $number: Int!) {
			repository(owner: $owner, name: $repo) {
				issueOrPullRequest(number: $number) {
					... on Issue {
						comments(first: 100) {
							nodes {
								id
								author {
									login
								}
								body
								createdAt
								updatedAt
							}
						}
					}
					... on PullRequest {
						comments(first: 100) {
							nodes {
								id
								author {
									login
								}
								body
								createdAt
								updatedAt
							}
						}
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
				Comments struct {
					Nodes []struct {
						ID     string `json:"id"`
						Author *struct {
							Login string `json:"login"`
						} `json:"author"`
						Body      string `json:"body"`
						CreatedAt string `json:"createdAt"`
						UpdatedAt string `json:"updatedAt"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"issueOrPullRequest"`
		} `json:"repository"`
	}

	if err := c.makeRequest(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to get comments: %w", err)
	}

	comments := make([]domain.Comment, 0, len(resp.Repository.IssueOrPullRequest.Comments.Nodes))
	for _, node := range resp.Repository.IssueOrPullRequest.Comments.Nodes {
		comment := domain.Comment{
			ID:        node.ID,
			Body:      node.Body,
			CreatedAt: node.CreatedAt,
			UpdatedAt: node.UpdatedAt,
		}

		// Handle deleted users (author is nil)
		if node.Author != nil {
			comment.Author = node.Author.Login
		} else {
			comment.Author = ""
		}

		comments = append(comments, comment)
	}

	return comments, nil
}
