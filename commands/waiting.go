package commands

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
	"github.com/mitchellh/cli"
)

// WaitingCommand represents the go-cli command
type WaitingCommand struct {
	UI cli.Ui
}

// reuse from commands/triage.go
// TODO: don't reuse
// var resultsMap = make(map[string][]github.Issue)

// Help outputs text usage help
func (c WaitingCommand) Help() string {
	helpText := `
Usage: tfteam waiting [options] 
	
List issues that are labeled with 'waiting-response' and have been updated in
the past 72 hours. TODO: only list those that are waiting, and have a reply
since the label

Options:
  -e,--expired 		Show items with 'waiting-response' with no update in 14+ days
`
	return strings.TrimSpace(helpText)
}

// Synopsis should do something, but it doesn't
func (c WaitingCommand) Synopsis() string {
	return `Show issues that have the 'waiting-response' label`
}

// waitinSearchResult wraps the result of a search done concurrently
type waitinSearchResult struct {
	Name   string
	Issues []github.Issue
}

// Run executes the command
func (c WaitingCommand) Run(args []string) int {
	key := os.Getenv("GITHUB_API_TOKEN")
	if key == "" {
		c.UI.Error("Missing API Token!")
		return 1
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: key},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// by default, only show issues
	repoNameFilter := []string{
		"terraform-providers/terraform-provider-aws",
		"terraform-providers/terraform-provider-azurerm",
		"terraform-providers/terraform-provider-consul",
		"terraform-providers/terraform-provider-google",
		"terraform-providers/terraform-provider-kubernetes",
		"terraform-providers/terraform-provider-nomad",
		"terraform-providers/terraform-provider-opc",
		"terraform-providers/terraform-provider-vault",
		"terraform-providers/terraform-provider-vsphere",
	}
	filter := "is:issue"
	var expired bool
	if len(args) > 0 {
		for _, a := range args {
			if a == "-e" || a == "-expired" {
				expired = true
			}
		}
	}

	// only get org repos if we aren't filtering
	if len(repoNameFilter) == 0 {
		// get list of repositories across terraform-repositories
		// TODO: this was copy-pasta'd from commands/releases.go
		nopt := &github.RepositoryListByOrgOptions{
			Type: "public",
		}
		var repos []*github.Repository
		for {
			part, resp, err := client.Repositories.ListByOrg(ctx, "terraform-providers", nopt)

			if err != nil {
				c.UI.Warn(fmt.Sprintf("Error listing Repositories: %s", err))
				return 1
			}
			repos = append(repos, part...)
			if resp.NextPage == 0 {
				break
			}
			nopt.Page = resp.NextPage
		}

		for _, r := range repos {
			if !*r.HasIssues {
				continue
			}
			repoNameFilter = append(repoNameFilter, "terraform-providers/"+*r.Name)
		}
	}

	// cut the string in half and search 2x b/c github search was barfing on one
	// giant string
	half := len(repoNameFilter) / 2
	p1 := repoNameFilter[:half]
	p2 := repoNameFilter[half:]

	// repoStr := "repo:"
	repoStr := "repo:"

	repoStr1 := repoStr + strings.Join(p1, " repo:")
	repoStr2 := repoStr + strings.Join(p2, " repo:")
	parts := []string{repoStr1, repoStr2}

	var issues []github.Issue

	for _, s := range parts {
		sopt := &github.SearchOptions{Sort: "updated"}

		now := time.Now()
		var updatedFilter string
		if expired {
			// find 14 days ago
			daysAgo := now.AddDate(0, 0, -14)
			updatedFilter = fmt.Sprintf("updated:<=%s", daysAgo.Format("2006-01-02"))
		} else {
			// find 72 hours ago
			threeDaysAgo := now.AddDate(0, 0, -3)
			// intent is to not show items that you just flagged as waiting-reply
			threeHoursAgo := now.Add(-time.Hour * 1)
			// golang reference time
			// Mon Jan 2 15:04:05 -0700 MST 2006
			updatedFilter = fmt.Sprintf("updated:%s..%s", threeDaysAgo.Format("2006-01-02"), threeHoursAgo.Format("2006-01-02T15:04:05"))
		}

		for {
			searchStr := fmt.Sprintf("state:open label:waiting-response %s %s %s", s, filter, updatedFilter)
			// log.Printf("\ns: %s\n", searchStr)
			sresults, resp, err := client.Search.Issues(ctx, searchStr, sopt)
			if err != nil {
				log.Printf("Error Searching Issues: %s", err)
				break
			}
			issues = append(issues, sresults.Issues...)
			if resp.NextPage == 0 {
				break
			}
			sopt.Page = resp.NextPage
		}
	}

	fmt.Printf("Results count: %d\n\n", len(issues))

	for _, i := range issues {
		key := strings.TrimPrefix(*i.RepositoryURL, "https://api.github.com/repos/terraform-providers/")
		resultsMap[key] = append(resultsMap[key], i)
	}

	var keys []string
	for k := range resultsMap {
		keys = append(keys, k)
	}

	// alpha sort
	sort.Strings(keys)

	r := report{
		RepoCount:       len(resultsMap),
		TotalIssueCount: len(issues),
		SortedKeys:      keys,
		Results:         resultsMap,
	}

	rp, err := template.New("report").
		Funcs(template.FuncMap{
			"issueList":      issueList,
			"repoIssueCount": repoIssueCount,
		}).
		Parse(templ)
	if err != nil {
		log.Fatalf("error parsing template: %s", err)
	}

	if err := rp.Execute(os.Stdout, r); err != nil {
		log.Fatalf("error executing template result: %s", err)
	}

	return 0
}
