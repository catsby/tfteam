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

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
	"github.com/mitchellh/cli"
)

type NotificationsCleanupCommand struct {
	UI cli.Ui
}

func (c NotificationsCleanupCommand) Help() string {
	return "Help - todo"
}

func (c NotificationsCleanupCommand) Synopsis() string {
	return "Synopsis - todo"
}

func (c NotificationsCleanupCommand) Run(args []string) int {
	if len(args) > 0 {
		c.UI.Output("Args:")
		for i, k := range args {
			c.UI.Output(fmt.Sprintf("\t%d - %s", i, k))
		}
	}
	key := os.Getenv("GITHUB_API_TOKEN")
	if key == "" {
		c.UI.Error("Missing API Token!")
		return 1
	}

	c.UI.Output("------")
	c.UI.Output("Notifications cleanup")
	c.UI.Output("------")
	c.UI.Output("")

	// refactor, this is boilerplate
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: key},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	// github.NotificationListOptions has useful attributes but for now we'll just
	// do defauls
	nopt := &github.NotificationListOptions{}
	var notifications []*github.Notification
	var lo int
	for {
		part, resp, err := client.Activity.ListNotifications(ctx, nopt)
		if err != nil {
			c.UI.Warn(fmt.Sprintf("Error listing notifications: %s", err))
			return 1
		}
		notifications = append(notifications, part...)
		if resp.NextPage == 0 {
			break
		}
		nopt.Page = resp.NextPage
		lo++
	}

	// NotificationIssues to look for
	var nIssues []*NotificationIssue
	// Filter out PRs/Issues that aren't involving Terraform
	for _, n := range notifications {
		if !strings.Contains(*n.Subject.URL, "hashicorp/terraform") {
			if !strings.Contains(*n.Subject.URL, "catsby/tfteam") {
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
			ID:     *n.ID,
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
		go markReadIfClosed(niChan, resultsChan)
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
		if r.Closed {
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
				if i.Closed {
					c.UI.Output(fmt.Sprintf("\t- %s", i.String()))
				}
			}
			c.UI.Output("")
		}
	}

	return 0
}

func markReadIfClosed(notificationsChan <-chan *NotificationIssue, rChan chan<- *NotificationIssue) {
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
		issue, _, err := client.Issues.Get(ctx, n.Owner, n.Name, n.Number)
		if err != nil {
			// log.Printf("error getting comments:%s", err)
			// Error could be a glitch or the source could be private and right now
			// the tool is only scoped for public things
			continue
		}

		// log.Printf("issue state for (%s): %s", n.String(), *issue.State)
		if "closed" == *issue.State {
			_, err := client.Activity.MarkThreadRead(ctx, n.ID)
			if err != nil {
				log.Printf("Error marking (%s) Thread (%s) as read: %s", n.String(), n.ID, err)
			} else {
				n.Closed = true
			}
		}

		rChan <- n
	}
}
