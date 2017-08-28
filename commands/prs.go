package commands

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
	"github.com/mitchellh/cli"
)

var wgPrs sync.WaitGroup

const tfTeamId = 1836975

type PRsCommand struct {
	UI cli.Ui
}

func (c PRsCommand) Help() string {
	return "Help - todo"
}

func (c PRsCommand) Synopsis() string {
	return "Synopsis - todo"
}

func (c PRsCommand) Run(args []string) int {
	key := os.Getenv("GITHUB_API_TOKEN")
	if key == "" {
		c.UI.Error("Missing API Token!")
		return 1
	}

	// refactor, this is boilerplate
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: key},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	opt := &github.OrganizationListTeamMembersOptions{Role: "all"}
	members, _, err := client.Organizations.ListTeamMembers(ctx, tfTeamId, opt)
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}

	// filter out junk memebers
	var ml []string
	for _, m := range members {
		if *m.Login != "hashicorp-fossa" && *m.Login != "tf-release-bot" {
			ml = append(ml, *m.Login)
		}
	}

	// combine all the members into a single author string so we only hit GitHub
	// search once
	authorStr := ""
	for _, m := range ml {
		authorStr = fmt.Sprintf("author:%s %s", m, authorStr)
	}

	// search for list by all these authors
	sopt := &github.SearchOptions{}
	sresults, _, err := client.Search.Issues(ctx, fmt.Sprintf("state:open %s type:pr", authorStr), sopt)

	// Filter out PRs that aren't involving Terraform
	tfIssues := []*TFPr{}
	for _, i := range sresults.Issues {
		if !strings.Contains(*i.HTMLURL, "terraform") {
			if !strings.Contains(*i.HTMLURL, "tfteam") {
				if !strings.Contains(*i.HTMLURL, "tf-deploy") {
					continue
				}
			}
		}
		tfpr := TFPr{
			User:    i.User,
			HTMLURL: *i.HTMLURL,
			Number:  *i.Number,
			Title:   *i.Title,
		}
		tfIssues = append(tfIssues, &tfpr)
	}

	// 5 "workers" to do things concurrently
	count := 5
	wgPrs.Add(count)

	// queue of TFPRs to query on the review status
	tfprChan := make(chan *TFPr, len(tfIssues))

	// recieve results from PR review queries
	resultsChan := make(chan *TFPr, len(tfIssues))

	// Setup go() workers
	for gr := 1; gr <= count; gr++ {
		go getApprovalStatus(tfprChan, resultsChan)
	}

	// Feed PRs into the queue
	for _, i := range tfIssues {
		tfprChan <- i
	}

	close(tfprChan)
	wgPrs.Wait()
	close(resultsChan)

	// convert results into a map of users/user prs for sorting
	rl := make(map[string][]*TFPr)
	for r := range resultsChan {
		rl[*r.User.Login] = append(rl[*r.User.Login], r)
	}

	// sort Team members by Alpha order sorry vancluever
	var keys []string
	for k, _ := range rl {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 5, 0, 1, ' ', 0)
	for _, k := range keys {
		fmt.Fprintln(w, k)
		for _, pr := range rl[k] {
			c.UI.Output(fmt.Sprintf("%s%s\t%s", pr.IsApprovedString(), pr.TitleTruncated(), pr.HTMLURL))
		}
		fmt.Fprintln(w)
	}
	w.Flush()

	return 0
}

type ByReviewDate []*github.PullRequestReview

func (a ByReviewDate) Len() int      { return len(a) }
func (a ByReviewDate) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByReviewDate) Less(i, j int) bool {
	// return *a[i].SubmittedAt > *a[j].SubmittedAt
	return a[i].SubmittedAt.Before(*a[j].SubmittedAt)
}

func getApprovalStatus(prsChan <-chan *TFPr, rChan chan<- *TFPr) {
	defer wgPrs.Done()
	// should pass in and reususe context I think?
	key := os.Getenv("GITHUB_API_TOKEN")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: key},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	for pr := range prsChan {
		// parse url to URL, so we can split the parts
		u, err := url.Parse(pr.HTMLURL)
		if err != nil {
			log.Println("error parsing url:", err)
			continue
		}

		parts := strings.Split(u.Path, "/")
		owner := parts[1]
		repo := parts[2]

		reviews, _, err := client.PullRequests.ListReviews(ctx, owner, repo, pr.Number, nil)
		if err != nil {
			log.Printf("error getting review:%s", err)
			continue
		}

		// questionable logic here; pop the last review and use that as the status.
		// A better strategy would probably be collect a map of reviews by reviewer.
		// If any of them are "requested changes", then the status is
		// CHANGES_REQUESTED, even if another reviewer approved. The only way to
		// make approved would be if the reviewer(s) that gave CHANGES_REQUESTED,
		// also but later, gave an APPROVED. STill not 100% but probably better than
		// below
		if len(reviews) > 0 {
			r := reviews[0]
			pr.State = *r.State
		}

		rChan <- pr
	}
}
