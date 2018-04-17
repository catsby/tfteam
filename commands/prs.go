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

var collaborators bool
var all bool
var includeUsers []string
var filterUsers []string
var tableFormat bool

type PRReviewStatus uint

var filter PRReviewStatus

const tfTeamId = 1836975

const (
	StatusAll PRReviewStatus = iota
	StatusWaiting
	StatusComments
	StatusChanges
	StatusApproved
)

type PRsCommand struct {
	UI cli.Ui
}

func (c PRsCommand) Help() string {
	helpText := `
Usage: tfteam prs [options] 

	List pull requests that are opened by team members. The output includes the
	status of the pull request, author, repo, title, and link.

	Pull requests are in 1 of 4 states: 
          - " " No review 
          - "+  " Reviewed, Approved!
          - "?  " Reviewed, with Comments
          - "-  " Reviewed, with Changes requested

	If no arguments are given, list just pull requests  and their status for
	Terraform OSS team members only, grouped by user.

Options:

	--collaborators, -c        Only Pull Requests from repository collaborators 
	
	--all, -a                  Pull Requests from team and repository collaborators 

	--users, -u                A comma seperated list of users to include pull
                             requests from 

	--filter, -f               A comma seperated list of users to only show
                             results for. This takes precedence over all other user modifing arguments

	--waiting, -w              Only show pull requests that have no reviews
	
	--table, -t                Show the output in a single table, sorted by
	                           repository


Examples:

  $ tfteam prs -t          // Show Team member PRs, in a single table    						 
  Status  Repo                    Author          Title        Link
  +      repositories            paddycarver     Something    https://github
         terraform               jbardin         [WIP] Input  https://github
  -      provider-aws            catsby          whoops       https://github



  $ tfteam prs -c          // Show collab PRs, by user
  selmanj
  ?  provider-google      Stuff        https://github.com/terraform-providers/
  
  $ tfteam prs -a          // Show team/collab PRs, by user
  selmanj
  ?  provider-google      Stuff        https://github.com/terraform-providers/
  
  vancluever
  +  provider-vsphere     Things       https://github.com/terraform-providers/

	$ tfteam prs -u=grubernaut
	  [..]
		radeksimko
		?  provider-kubernetes  r/service: Make spec.port.target_port optional          https://github.com/terraform-providers/terraform-provider-kubernetes/pull/69
		
		grubernaut
		   provider-aws         [WIP] provider/aws: Add support for Network L[...]      https://github.com/terraform-providers/terraform-provider-aws/pull/1629
	  [..]

	$ tfteam prs -f=catsby
		catsby
		?  tf-deploy    Fix issue releasing Core                                https://github.com/hashicorp/tf-deploy/pull/7

`
	return strings.TrimSpace(helpText)
}

func (c PRsCommand) Synopsis() string {
	return "List PRs opened by Terraform team, Collaborators, or specific users"
}

func (c PRsCommand) Run(args []string) int {
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

	// if -c or --collaborators, call orgs/tf-providers/outside_collaborators
	// and append non-junk users to ml slice above
	if len(args) > 0 {
		for _, a := range args {
			if a == "--collaborators" || a == "-c" {
				collaborators = true
			}
			if a == "--table" || a == "-t" {
				tableFormat = true
			}
			if a == "--waiting" || a == "-w" {
				filter = StatusWaiting
			}
			if a == "--all" || a == "-a" {
				all = true
			}
			if strings.Contains(a, "--users") || strings.Contains(a, "-u") {
				parts := strings.Split(a, "=")
				// parts 0 is "--users" or "-u"
				if len(parts) > 1 {
					includeUsers = strings.Split(parts[1], ",")
				} else {
					log.Printf("no user given")
				}
			}
			if strings.Contains(a, "--filter") || strings.Contains(a, "-f") {
				parts := strings.Split(a, "=")
				// parts 0 is "--users" or "-u"
				if len(parts) > 1 {
					filterUsers = strings.Split(parts[1], ",")
				} else {
					log.Printf("no filter user given")
				}
			}
		}
	}

	ml := make(map[string]string)

	var members []*github.User
	// refactor, this is boilerplate
	if !collaborators || all {
		opt := &github.OrganizationListTeamMembersOptions{Role: "all"}
		// TODO check pagination
		teamMembers, _, err := client.Organizations.ListTeamMembers(ctx, tfTeamId, opt)
		if err != nil {
			fmt.Println("Error: ", err)
			os.Exit(1)
		}
		members = append(members, teamMembers...)
	}

	if collaborators || all {
		var collabMembers []*github.User
		copt := &github.ListOutsideCollaboratorsOptions{}
		for {
			outsideCollaborators, resp, err := client.Organizations.ListOutsideCollaborators(ctx, "terraform-providers", copt)
			if err != nil {
				log.Printf("Error getting collabs")
			} else {
				collabMembers = append(collabMembers, outsideCollaborators...)
			}
			if resp.NextPage == 0 {
				break
			}
			copt.Page = resp.NextPage
		}
		members = append(members, collabMembers...)
	}

	// filter out junk memebers
	for _, m := range members {
		if *m.Login != "hashicorp-fossa" && *m.Login != "tf-release-bot" {
			ml[*m.Login] = *m.Login
		}
	}

	for _, u := range includeUsers {
		ml[u] = u
	}

	if len(filterUsers) > 0 {
		// only look at these users, skip later blocks
		newList := make(map[string]string)
		collaborators = false
		all = false
		for _, u := range filterUsers {
			for _, v := range ml {
				if strings.Contains(v, u) {
					newList[v] = v
				}
			}
		}
		ml = newList
	}

	// Remove Martin and Bardin FOR NOW b/c they tend to have each other review
	// PRs regularly
	delete(ml, "apparentlymart")
	delete(ml, "jbardin")

	// combine all the members into a single author string so we only hit GitHub
	// search once
	authorStr := ""
	for _, m := range ml {
		authorStr = fmt.Sprintf("author:%s %s", m, authorStr)
	}

	// search for list by all these authors
	sopt := &github.SearchOptions{}

	var issues []github.Issue
	for {
		sresults, resp, err := client.Search.Issues(ctx, fmt.Sprintf("state:open %s type:pr", authorStr), sopt)
		if err != nil {
			c.UI.Warn(fmt.Sprintf("Error Searching Issues: %s", err))
			return 1
		}
		issues = append(issues, sresults.Issues...)
		if resp.NextPage == 0 {
			break
		}
		sopt.Page = resp.NextPage
	}

	// Filter out PRs that aren't involving Terraform
	tfIssues := []*TFPr{}
	for _, i := range issues {
		// sneak some other related projects in. This cascading if statements look
		// hilarious
		if !strings.Contains(*i.HTMLURL, "terraform") {
			if !strings.Contains(*i.HTMLURL, "tfteam") {
				if !strings.Contains(*i.HTMLURL, "engservices-teamcity") {
					if !strings.Contains(*i.HTMLURL, "tf-deploy") {
						continue
					}
				}
			}
		}

		var repo string
		var owner string
		u, err := url.Parse(*i.HTMLURL)
		if err != nil {
			log.Println("error parsing url:", err)
		} else {
			parts := strings.Split(u.Path, "/")
			owner = parts[1]
			repo = parts[2]
		}
		tfpr := TFPr{
			User:      i.User,
			HTMLURL:   *i.HTMLURL,
			Number:    *i.Number,
			Title:     *i.Title,
			CreatedAt: i.CreatedAt,
			UpdatedAt: i.UpdatedAt,
			Owner:     owner,
			Repo:      repo,
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

	if tableFormat {
		// convert results into a map of users/user prs for sorting
		rl := make(map[string][]*TFPr)
		for r := range resultsChan {
			rl[r.Repo] = append(rl[r.Repo], r)
		}

		var keys []string
		for k, _ := range rl {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		w := new(tabwriter.Writer)
		// w.Init(os.Stdout, 5, 2, 1, '\t', 0)
		w.Init(os.Stdout, 0, 8, 0, '\t', 0)
		// change table format to remove status column if we're just looking at
		// waiting reviews
		tableFormat := "Status\tCreated At\tRepo\tAuthor\tTitle\tLink"
		if filter == StatusWaiting {
			tableFormat = "Repo\tAuthor\tTitle\tLink"
		}
		fmt.Fprintln(w, tableFormat)
		for _, k := range keys {
			for _, pr := range rl[k] {
				// there's better logic here for this kind of sort, using > and the
				// ordering of the status, but I'm going on like 4 hours of sleep so
				// ¯\_(ツ)_/¯
				if filter > 0 {
					if filter != pr.StatusCode() {
						continue
					}
				}
				if filter == StatusWaiting {
					fmt.Fprintln(w, fmt.Sprintf("%s\t%s\t%s\t%s", strings.TrimPrefix(k, "terraform-"), *pr.User.Login, pr.TitleTruncated(), pr.HTMLURL))
				} else {
					fmt.Fprintln(w, fmt.Sprintf("%s\t%s\t%s\t%s\t%s", pr.IsApprovedString(), strings.TrimPrefix(k, "terraform-"), *pr.User.Login, pr.TitleTruncated(), pr.HTMLURL))
				}
			}
		}
		w.Flush()
	} else {
		// User format
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
		w.Init(os.Stdout, 0, 8, 0, '\t', 0)
		for _, k := range keys {
			if len(rl[k]) > 0 {
				// if we're filtering out to just show waiting ones, make sure we have
				// some
				var waitingCount int
				for _, pr := range rl[k] {
					if filter != pr.StatusCode() {
						continue
					}
					waitingCount++
				}

				if filter == StatusWaiting && waitingCount == 0 {
					continue
				}
				fmt.Fprintln(w, k)

				for _, pr := range rl[k] {
					// there's better logic here for this kind of sort, using > and the
					// ordering of the status, but I'm going on like 4 hours of sleep so
					// ¯\_(ツ)_/¯
					if filter > 0 {
						if filter != pr.StatusCode() {
							continue
						}
					}
					fmt.Fprintln(w, fmt.Sprintf("%s\t%s\t%s\t%s\t%s", pr.IsApprovedString(), pr.CreatedAt.Format("Mon Jan 2 15:04:05 MST 2006"), strings.TrimPrefix(pr.Repo, "terraform-"), pr.TitleTruncated(), pr.HTMLURL))
				}
				fmt.Fprintln(w)
			}
			w.Flush()
		}
	}

	return 0
}

type ByReviewDate []*github.PullRequestReview

func (a ByReviewDate) Len() int      { return len(a) }
func (a ByReviewDate) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByReviewDate) Less(i, j int) bool {
	// return *a[i].SubmittedAt > *a[j].SubmittedAt
	return a[j].SubmittedAt.Before(*a[i].SubmittedAt)
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
		reviews, _, err := client.PullRequests.ListReviews(ctx, pr.Owner, pr.Repo, pr.Number, nil)
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
		sort.Sort(ByReviewDate(reviews))
		if len(reviews) > 0 {
			r := reviews[0]
			pr.State = *r.State
		}

		rChan <- pr
	}
}
