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
	ID        string
	Owner     string
	Name      string
	Number    int
	Title     string
	URL       string
	Reviewed  bool
	Closed    bool
	IsRelease bool
}

func (n *NotificationIssue) String() string {
	return fmt.Sprintf("%s - https://github.com/%s/%s/issues/%d", n.Title, n.Owner, n.Name, n.Number)
}

func (n *NotificationIssue) Repo() string {
	return fmt.Sprintf("%s/%s", n.Owner, n.Name)
}

type ByNumber []*NotificationIssue

func (a ByNumber) Len() int      { return len(a) }
func (a ByNumber) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByNumber) Less(i, j int) bool {
	return a[i].Number < a[j].Number
}

func (c NotificationsCommand) Run(args []string) int {
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
		if !strings.Contains(*n.Repository.Name, "terraform") {
			if !strings.Contains(*n.Repository.Name, "tfteam") {
				continue
			}
		}

		// We don't currently check private repos
		if *n.Repository.Private {
			continue
		}

		// find the number by parsing the subject url
		u, err := url.Parse(*n.Subject.URL)
		if err != nil {
			log.Println("error parsing url:", err)
			continue
		}

		// not sure what to do about commits, b/c they aren't "closed" or "merged".
		// Skip for now.
		if "Commit" == *n.Subject.Type {
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
			Title:  *n.Subject.Title,
		}
		// The Notifications API gives notifications for releases at an extended
		// endpoint: owner/repo/releases/number
		release := parts[len(parts)-2]
		if "releases" == release {
			ni.IsRelease = true
		}
		nIssues = append(nIssues, &ni)
	}

	// 5 "workers" to do things concurrently
	wCount := 5
	wgNIssues.Add(wCount)

	// queue of NotificationIssues to query on the review status
	niChan := make(chan *NotificationIssue, len(nIssues))

	// recieve results from PR review queries
	resultsChan := make(chan *NotificationIssue, len(nIssues))

	// figure out what kind of work we're doing by looking for any flags
	// hacky because it only reads the first arg, but probably ugly for other
	// reasons too
	var action string
	var modifier string
	if len(args) > 0 {
		action = args[0]
	}
	if len(args) > 1 {
		modifier = args[1]
	}
	if "--cleanup" == action {
		dryOutput := ""
		if modifier != "" {
			dryOutput = " - dry run"
		}
		c.UI.Output("------")
		c.UI.Output(fmt.Sprintf("%s%s", "Notifications cleanup", dryOutput))
		c.UI.Output("------")
		c.UI.Output("")

		// Setup go() workers to mark things as viewed
		dryRun := "--dry-run" == modifier
		for gr := 1; gr <= wCount; gr++ {
			go markReadIfClosed(niChan, resultsChan, dryRun)
		}
	} else {
		c.UI.Output("------")
		c.UI.Output("Notifications that have no TF Team Member comment")
		c.UI.Output("------")
		c.UI.Output("")

		// Setup go() workers for review status, the default
		for gr := 1; gr <= wCount; gr++ {
			go getReviewStatus(niChan, resultsChan)
		}
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

	var count int
	for _, k := range keys {
		niList := repoIssueMap[k]
		// omit any repos that have zero things needing review
		display := niList
		if "--cleanup" == action {
			display = nil
			for _, i := range niList {
				if i.Closed {
					display = append(display, i)
				}
			}
		}
		if len(display) > 0 {
			c.UI.Output(k)
			// sub sort the issues/prs by their number
			sort.Sort(ByNumber(display))
			count += len(display)
			for _, i := range display {
				c.UI.Output(fmt.Sprintf("  - %s", i.String()))
			}
			c.UI.Output("")
		}
	}
	c.UI.Output(fmt.Sprintf("Total count: %d", count))

	// exercise for tomorrow: tab format the output
	// w := new(tabwriter.Writer)
	// // Format right-aligned in space-separated columns of minimal width 5
	// // and at least one blank of padding (so wider column entries do not
	// // touch each other).
	// w.Init(os.Stdout, 5, 0, 1, ' ', 0)
	// fmt.Fprintln(w, "a\tb\tc\td\t.")
	// fmt.Fprintln(w, "123\t12345\t1234567\t123456789\t.")
	// fmt.Fprintln(w)
	// w.Flush()

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
		if !n.IsRelease {
			comments, _, err := client.Issues.ListComments(ctx, n.Owner, n.Name, n.Number, nil)
			if err != nil {
				log.Printf("error getting comments for (%s): %s", n.String(), err)
				// could error if it's a private repo; right now we are scoped to only
				// public things
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
		}
		rChan <- n
	}
}

// Function that marks closed issues/prs as "read"
func markReadIfClosed(notificationsChan <-chan *NotificationIssue, rChan chan<- *NotificationIssue, dryRun bool) {
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
			// Error could be a glitch or the source could be private and right now
			// the tool is only scoped for public things
			continue
		}

		// log.Printf("issue state for (%s): %s", n.String(), *issue.State)
		if "closed" == *issue.State {
			if !dryRun {
				_, err := client.Activity.MarkThreadRead(ctx, n.ID)
				if err != nil {
					log.Printf("Error marking (%s) Thread (%s) as read: %s", n.String(), n.ID, err)
				} else {
					n.Closed = true
				}
			} else {
				// hack - mark it closed so we see the review of what we would close
				n.Closed = true
			}
		}

		rChan <- n
	}
}
