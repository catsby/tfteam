package commands

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"os"
	"sort"
	"strings"

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
	"github.com/mitchellh/cli"
)

// TriageCommand represents the go-cli command
type TriageCommand struct {
	UI cli.Ui
}

var resultsMap = make(map[string][]github.Issue)

// Help outputs text usage help
func (c TriageCommand) Help() string {
	helpText := `
Usage: tfteam triage [options] 
	
	List unlabeld issues from terraform-providers org from the past 24 hours. 

Options:

	--all, -a          List all issues and Pull Requests

	--pulls, -p        Only list Pull Requests 

Examples:

  $ tfteam triage          // Show all things
  Status  Repo                    Author          Title        Link
  +      repositories            paddycarver     Something    https://github
         terraform               jbardin         [WIP] Input  https://github
  -      provider-aws            catsby          whoops       https://github


`
	return strings.TrimSpace(helpText)
}

// Synopsis should do something, but it doesn't
func (c TriageCommand) Synopsis() string {
	return "Synopsis - todo"
}

// searchResult wraps the result of a search done concurrently
type searchResult struct {
	Name   string
	Issues []github.Issue
}

// Run executes the command
func (c TriageCommand) Run(args []string) int {
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
	filter := "is:issue"
	if len(args) > 0 {
		for _, a := range args {
			if a == "--pulls" || a == "-p" {
				filter = "is:pr"
			}
			if a == "--all" || a == "-a" {
				filter = ""
			}
		}
	}

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

	// repoStr := "repo:"
	repoStr := "repo:"
	var rs []string
	for _, r := range repos {
		if !*r.HasIssues {
			log.Printf("\n@@@ repo (%s) does not have issues\n", *r.Name)
			continue
		}
		rs = append(rs, "terraform-providers/"+*r.Name)
	}

	// cut the string in half and search 2x b/c github search was barfing on one
	// giant string
	half := len(rs) / 2
	p1 := rs[:half]
	p2 := rs[half:]

	repoStr1 := repoStr + strings.Join(p1, " repo:")
	repoStr2 := repoStr + strings.Join(p2, " repo:")
	parts := []string{repoStr1, repoStr2}

	var issues []github.Issue

	for _, s := range parts {
		sopt := &github.SearchOptions{Sort: "updated"}

		for {
			sresults, resp, err := client.Search.Issues(ctx, fmt.Sprintf("state:open %s no:label %s", s, filter), sopt)
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

type report struct {
	RepoCount       int
	TotalIssueCount int
	SortedKeys      []string
	Results         map[string][]github.Issue
}

const templ = `Count of Repos with unlabled issues: {{.RepoCount}}
Total unlabeled issue count: {{.TotalIssueCount}}
{{range .SortedKeys}}----------

{{.}} ({{. | repoIssueCount}})
{{. | issueList}}
{{end}}`

func issueList(key string) string {
	l := resultsMap[key]

	var result string
	for _, i := range l {
		itemType := "[i]"
		if i.PullRequestLinks != nil {
			itemType = "[p]"
		}
		str := fmt.Sprintf("#%6d %s %-75s %s", *i.Number, itemType, *i.HTMLURL, *i.Title)
		result = result + "\n" + str
	}
	return result
}

func repoIssueCount(key string) int {
	return len(resultsMap[key])
}
