package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
	"github.com/mitchellh/cli"
)

type ReleasesCommand struct {
	UI cli.Ui
}

func (c ReleasesCommand) Help() string {
	return "Help - todo"
}

func (c ReleasesCommand) Synopsis() string {
	return "Synopsis - todo"
}

// This was just RepoRelease in the begining, but our release process doesn't
// actually create "Releases" in the GitHub API, we just have tags (except maybe
// hashicorp/terraform?), so it got converted to release tags ¯\_(ツ)_/¯
type RepoReleaseTag struct {
	Owner   string
	Name    string
	TagName string
	Date    *time.Time
}

// Formating for table view output, giving relative information on when the last
// release was.
// Ex:
//  9 days ago
//  59 days ago
//  18 days ago
//  < 24 hours
//  59 days ago
//  < 12 hours
func (r *RepoReleaseTag) LastReleaseString() string {
	since := time.Since(*r.Date)
	rawSince := since.Hours() / 24
	daysSince := strconv.FormatFloat(rawSince, 'f', 0, 32)
	layout := "%s\t\t%s"
	if daysSince == "0" {
		return fmt.Sprintf(layout, r.Date.Format("Mon Jan 2 15:04:05 MST 2006"), "< 12 hours")
	} else if daysSince == "1" {
		return fmt.Sprintf(layout, r.Date.Format("Mon Jan 2 15:04:05 MST 2006"), "< 24 hours")
	} else {
		return fmt.Sprintf(layout, r.Date.Format("Mon Jan 2 15:04:05 MST 2006"), daysSince+" days ago")
	}
	return "-"
}

func (c ReleasesCommand) Run(args []string) int {
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

	// get list of repositories across terraform-repositories, and add in
	// hashicorp/terraform
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

	var rList []*RepoReleaseTag
	for _, n := range repos {
		ni := RepoReleaseTag{
			Owner: *n.Owner.Login,
			Name:  *n.Name,
		}
		rList = append(rList, &ni)
	}

	// Add hashicorp/terraform
	rList = append(rList, &RepoReleaseTag{
		Owner: "hashicorp",
		Name:  "terraform",
	})

	// 5 "workers" to do things concurrently
	wCount := 5
	wgNIssues.Add(wCount)

	// queue of RepoReleaseTags to query on the release status
	niChan := make(chan *RepoReleaseTag, len(rList))

	// recieve results from Release queries
	resultsChan := make(chan *RepoReleaseTag, len(rList))

	for gr := 1; gr <= wCount; gr++ {
		go getLatestRelease(niChan, resultsChan)
	}

	// Feed things into queue
	for _, r := range rList {
		niChan <- r
	}

	// Close and wait on workers
	close(niChan)
	wgNIssues.Wait()
	close(resultsChan)

	var tfCore *RepoReleaseTag
	var releases []*RepoReleaseTag
	for r := range resultsChan {
		if r.Name == "terraform" {
			tfCore = r
			break
		}
		releases = append(releases, r)
	}

	// sort by "days ago"
	sort.Sort(ByDaysAgo(releases))

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 5, 0, 1, ' ', 0)
	fmt.Fprintln(w, "  Core\tTag\tDate")
	fmt.Fprintln(w, fmt.Sprintf("  %s\t%s\t%s", tfCore.Name, tfCore.TagName, tfCore.LastReleaseString()))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Provider\tTag\tDate\t")
	for _, rTag := range releases {
		fmt.Fprintln(w, fmt.Sprintf("  %s\t%s\t%s", rTag.Name, rTag.TagName, rTag.LastReleaseString()))
	}
	w.Flush()

	return 0
}

// When listing releases, list by most recently released first
type ByDaysAgo []*RepoReleaseTag

func (a ByDaysAgo) Len() int      { return len(a) }
func (a ByDaysAgo) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByDaysAgo) Less(i, j int) bool {
	return a[i].Date.After(*a[j].Date)
}

type ByTag []*github.RepositoryTag

func (a ByTag) Len() int      { return len(a) }
func (a ByTag) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByTag) Less(i, j int) bool {
	// ignore it if it doesn't have a v prefix
	if !strings.HasPrefix(*a[i].Name, "v") {
		return false
	}
	if !strings.HasPrefix(*a[j].Name, "v") {
		return true
	}

	// hacky semvar sorting so that v0.10 is sorted ahead of v0.9
	aiTag := strings.Trim(*a[i].Name, "v")
	ajTag := strings.Trim(*a[j].Name, "v")
	iparts := strings.Split(aiTag, ".")
	jparts := strings.Split(ajTag, ".")
	if len(iparts[1]) == 1 {
		iparts[1] = "0" + iparts[1]
	}
	if len(jparts[1]) == 1 {
		jparts[1] = "0" + jparts[1]
	}

	aiTag = strings.Join(iparts, ".")
	ajTag = strings.Join(jparts, ".")

	return aiTag > ajTag
}

func getLatestRelease(reposChan <-chan *RepoReleaseTag, rChan chan<- *RepoReleaseTag) {
	defer wgNIssues.Done()
	// should pass in and reususe context I think?
	key := os.Getenv("GITHUB_API_TOKEN")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: key},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	for n := range reposChan {
		// For some reaons I don't get, this doesn't work for most of our repos...
		// it works on Rancher, it has "Latest release" badge on it, same with
		// hashicorp/terraform. Maybe because we're using the API to release now,
		// and rancher some how got a manual one? dunno ౿(ఠ_ఠఎ)
		// TODO: modify our release process to issue "create release" call in GitHub
		// to make actual releases out of our vTags
		// release, _, err := client.Repositories.GetLatestRelease(ctx, n.Owner, n.Name)

		nopt := &github.ListOptions{}
		var tags []*github.RepositoryTag
		for {
			part, resp, err := client.Repositories.ListTags(ctx, n.Owner, n.Name, nopt)

			if err != nil {
				log.Printf("Error listing tags for (%s/%s): %s", n.Owner, n.Name, err)
				continue
			}
			tags = append(tags, part...)
			if resp.NextPage == 0 {
				break
			}
			nopt.Page = resp.NextPage
		}

		if len(tags) == 0 {
			// dunno if this could happen, but saftey first
			log.Printf("no Tags for (%s/%s)", n.Owner, n.Name)
			n.TagName = "-"
			continue
		}

		sort.Sort(ByTag(tags))

		// in Sort I trust
		tag := tags[0]
		n.TagName = *tag.Name

		// query git commit info to get the date for this commit, because we don't
		// have true "releases"
		commit, _, err := client.Git.GetCommit(ctx, n.Owner, n.Name, *tag.Commit.SHA)
		if err != nil {
			log.Printf("Error getting commit infor for (%s/%s) tag (%s): %s", n.Owner, n.Name, *tag.Commit.SHA)
		}
		n.Date = commit.Author.Date

		rChan <- n
	}
}
