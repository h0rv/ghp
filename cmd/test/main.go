package main

import (
	"context"
	"fmt"
	"log"

	"github.com/robby/ghp/internal/gh"
)

func main() {
	client, err := gh.New()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Use sweetspot-ai org
	ownerType, ownerID, err := client.ResolveOwner(ctx, "sweetspot-ai")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Owner: sweetspot-ai (%s) ID=%s\n\n", ownerType, ownerID)

	// List projects
	projects, err := client.ListProjects(ctx, ownerType, ownerID, "sweetspot-ai")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Projects (%d):\n", len(projects))
	for _, p := range projects {
		fmt.Printf("  #%d: %s (ID=%s)\n", p.Number, p.Title, p.ID)
	}

	if len(projects) == 0 {
		return
	}

	// Get fields for project #2 (Engineering Backlog based on screenshot)
	var project = projects[0]
	for _, p := range projects {
		if p.Number == 2 {
			project = p
			break
		}
	}
	fmt.Printf("\nUsing project: %s (#%d)\n\n", project.Title, project.Number)

	fields, err := client.GetProjectFields(ctx, project.ID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Fields (%d):\n", len(fields))
	var statusFieldName string
	for _, f := range fields {
		fmt.Printf("  - %s (type=%s, options=%d)\n", f.Name, f.Type, len(f.Options))
		if f.Type == "SINGLE_SELECT" {
			for _, opt := range f.Options {
				fmt.Printf("      Option: %s (ID=%s)\n", opt.Name, opt.ID)
			}
			if f.Name == "Status" {
				statusFieldName = f.Name
			}
		}
	}

	if statusFieldName == "" {
		fmt.Println("\nNo Status field found!")
		return
	}

	// Get items
	fmt.Printf("\nFetching items grouped by %s...\n\n", statusFieldName)
	cards, cursor, hasMore, err := client.GetItems(ctx, project.ID, statusFieldName, "", 50)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Items (%d, hasMore=%v, cursor=%s):\n", len(cards), hasMore, cursor[:20]+"...")
	
	// Group by status
	groups := make(map[string]int)
	for _, c := range cards {
		key := c.GroupOptionID
		if key == "" {
			key = "(no status)"
		}
		groups[key]++
	}

	fmt.Printf("\nGrouped items (%d groups):\n", len(groups))
	for groupID, count := range groups {
		fmt.Printf("  Group %s: %d items\n", groupID, count)
	}
}
