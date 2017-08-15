package commands

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
	"github.com/mitchellh/cli"
)

var wgNIssues sync.WaitGroup

type NotificationsCommand struct {
	UI cli.Ui
}

func (c NotificationsCommand) Help() string {
	return "Help - todo"
}

func (c NotificationsCommand) Synopsis() string {
	return "Synopsis - todo"
}

type NotificationIssue struct {
	Owner    string
	Name     string
	Number   int
	URL      string
	Reviewed bool
}

func (n *NotificationIssue) String() string {
	return fmt.Sprintf("https://github.com/%s/%s/issues/%d", n.Owner, n.Name, n.Number)
}

func (n *NotificationIssue) Repo() string {
	return fmt.Sprintf("%s/%s", n.Owner, n.Name)
}

type ByNumber []*NotificationIssue

func (a ByNumber) Len() int      { return len(a) }
func (a ByNumber) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByNumber) Less(i, j int) bool {
	return a[i].Number > a[j].Number
}

func (c NotificationsCommand) Run(args []string) int {
	key := os.Getenv("GITHUB_API_TOKEN")
	if key == "" {
		c.UI.Error("Missing API Token!")
		return 1
	}

	c.UI.Output("------")
	c.UI.Output("Notifications that have no TF Team Member comment")
	c.UI.Output("------")
	c.UI.Output("")

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

	// filter out junk members
	var ml []string
	for _, m := range members {
		if *m.Login != "hashicorp-fossa" && *m.Login != "tf-release-bot" {
			ml = append(ml, *m.Login)
		}
	}

	// github.NotificationListOptions has useful attributes but for now we'll just
	// do defauls
	nopt := &github.NotificationListOptions{}
	notifications, _, err := client.Activity.ListNotifications(ctx, nopt)

	// NotificationIssues to look for
	var nIssues []*NotificationIssue
	// Filter out PRs/Issues that aren't involving Terraform
	for _, n := range notifications {
		if !strings.Contains(*n.Repository.Owner.Login, "terraform") {
			if !strings.Contains(*n.Repository.Owner.Login, "tfteam") {
				continue
			}
		}

		// find the number by parsing the subject url
		u, err := url.Parse(*n.Subject.URL)
		if err != nil {
			log.Println("error parsing url:", err)
			continue
		}

		parts := strings.Split(u.Path, "/")
		numberRaw := parts[len(parts)-1]
		number, err := strconv.Atoi(numberRaw)
		if err != nil {
			log.Printf("error parsing raw number (%#v): %s", numberRaw, err)
			continue
		}

		ni := NotificationIssue{
			Owner:  *n.Repository.Owner.Login,
			Name:   *n.Repository.Name,
			Number: number,
			URL:    *n.Subject.URL,
		}
		nIssues = append(nIssues, &ni)
	}

	// 5 "workers" to do things concurrently
	count := 5
	wgNIssues.Add(count)

	// queue of NotificationIssues to query on the review status
	niChan := make(chan *NotificationIssue, len(nIssues))

	// recieve results from PR review queries
	resultsChan := make(chan *NotificationIssue, len(nIssues))

	// Setup go() workers
	for gr := 1; gr <= count; gr++ {
		go getReviewStatus(niChan, resultsChan)
	}

	// Feed PRs into the queue
	for _, i := range nIssues {
		niChan <- i
	}

	close(niChan)
	wgNIssues.Wait()
	close(resultsChan)

	repoIssueMap := make(map[string][]*NotificationIssue)
	// range over the results we get. If there is no review, add the
	// NotificationIssue to the repoIssueMap based on its Repo
	for r := range resultsChan {
		if !r.Reviewed {
			repoIssueMap[r.Repo()] = append(repoIssueMap[r.Repo()], r)
		}
	}

	// sort repoIssueMap by Alpha order for consistent
	var keys []string
	for k, _ := range repoIssueMap {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		niList := repoIssueMap[k]
		// omit any repos that have zero things needing review
		if len(niList) > 0 {
			c.UI.Output(k)
			// sub sort the issues/prs by their number
			sort.Sort(ByNumber(niList))

			for _, i := range niList {
				c.UI.Output(fmt.Sprintf("\t- %s", i.String()))
			}
			c.UI.Output("")
		}
	}

	return 0
}

func getReviewStatus(notificationsChan <-chan *NotificationIssue, rChan chan<- *NotificationIssue) {
	defer wgNIssues.Done()
	// should pass in and reususe context I think?
	key := os.Getenv("GITHUB_API_TOKEN")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: key},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	for n := range notificationsChan {
		comments, _, err := client.Issues.ListComments(ctx, n.Owner, n.Name, n.Number, nil)
		if err != nil {
			log.Printf("error getting comments:%s", err)
			continue
		}

		if len(comments) == 0 {
			continue
		}

		// should be dynamic but I'm lazy at this particular moment
		teamMembers := []string{
			"mitchellh",
			"apparentlymart",
			"jbardin",
			"phinze",
			"paddycarver",
			"catsby",
			"radeksimko",
			"tombuildsstuff",
			"grubernaut",
			"mbfrahry",
			"vancluever",
		}

		for _, comment := range comments {
			for _, m := range teamMembers {
				if m == *comment.User.Login {
					n.Reviewed = true
					continue
				}
			}
		}
		rChan <- n
	}
}
