package commands

import (
	"os"
	"strings"

	"github.com/mitchellh/cli"
)

type ReviewsCommand struct {
	UI cli.Ui
}

func (c ReviewsCommand) Help() string {
	helpText := `
Usage: tfteam prs [options] 

	List pull requests that are opened by team members. The output includes the
	status of the pull request, author, repo, title, and link.

`
	return strings.TrimSpace(helpText)
}

func (c ReviewsCommand) Synopsis() string {
	return "Synopsis - todo"
}

func (c ReviewsCommand) Run(args []string) int {
	key := os.Getenv("GITHUB_API_TOKEN")
	if key == "" {
		c.UI.Error("Missing API Token!")
		return 1
	}

	// refactor, this is boilerplate
	// ctx := context.Background()
	// ts := oauth2.StaticTokenSource(
	// 	&oauth2.Token{AccessToken: key},
	// )
	// tc := oauth2.NewClient(ctx, ts)

	// client := github.NewClient(tc)

	// opt := &github.OrganizationListTeamMembersOptions{Role: "all"}
	// members, _, err := client.Organizations.ListTeamMembers(ctx, tfTeamId, opt)
	// if err != nil {
	// 	fmt.Println("Error: ", err)
	// 	os.Exit(1)
	// }

	// if -c or --collaborators, call orgs/tf-providers/outside_collaborators
	// and append non-junk users to ml slice above

	// if tableFormat {
	// 	// convert results into a map of users/user prs for sorting
	// 	rl := make(map[string][]*TFPr)
	// 	for r := range resultsChan {
	// 		rl[r.Repo] = append(rl[r.Repo], r)
	// 	}

	// 	var keys []string
	// 	for k, _ := range rl {
	// 		keys = append(keys, k)
	// 	}

	// 	sort.Strings(keys)

	// 	w := new(tabwriter.Writer)
	// 	// w.Init(os.Stdout, 5, 2, 1, '\t', 0)
	// 	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	// 	// change table format to remove status column if we're just looking at
	// 	// waiting reviews
	// 	tableFormat := "Status\tRepo\tAuthor\tTitle\tLink"
	// 	if filter == statusWaiting {
	// 		tableFormat = "Repo\tAuthor\tTitle\tLink"
	// 	}
	// 	fmt.Fprintln(w, tableFormat)
	// 	for _, k := range keys {
	// 		for _, pr := range rl[k] {
	// 			// there's better logic here for this kind of sort, using > and the
	// 			// ordering of the status, but I'm going on like 4 hours of sleep so
	// 			// ¯\_(ツ)_/¯
	// 			if filter > 0 {
	// 				if filter != pr.StatusCode() {
	// 					continue
	// 				}
	// 			}
	// 			if filter == statusWaiting {
	// 				fmt.Fprintln(w, fmt.Sprintf("%s\t%s\t%s\t%s", strings.TrimPrefix(k, "terraform-"), *pr.User.Login, pr.TitleTruncated(), pr.HTMLURL))
	// 			} else {
	// 				fmt.Fprintln(w, fmt.Sprintf("%s\t%s\t%s\t%s\t%s", pr.IsApprovedString(), strings.TrimPrefix(k, "terraform-"), *pr.User.Login, pr.TitleTruncated(), pr.HTMLURL))
	// 			}
	// 		}
	// 	}
	// 	w.Flush()
	// } else {
	// User format
	return 0
}
