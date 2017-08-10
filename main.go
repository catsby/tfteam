package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
)

var wgPrs sync.WaitGroup

const tfTeamId = 1836975

type UserPRs struct {
	Login string
	PRs   map[int]string
}

func main() {
	fmt.Println("TF Team PRs")

	key := os.Getenv("GITHUB_API_TOKEN")
	if key == "" {
		fmt.Println("Missing API Token!")
		os.Exit(1)
	}

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

	// 3 "workers" to do things concurrently
	count := 3
	wgPrs.Add(count)

	// queue of users to query PRs on
	users := make(chan string, len(ml))
	// recieves results from user queries
	results := make(chan UserPRs, len(ml))
	for gr := 1; gr <= count; gr++ {
		go prsForUser(users, results)
	}

	// Feed users into the queue
	for _, m := range ml {
		users <- m
	}

	close(users)
	wgPrs.Wait()
	close(results)

	// convert results into a map for sorting
	rl := make(map[string]UserPRs)
	for r := range results {
		if len(r.PRs) > 0 {
			rl[r.Login] = r
		}
	}

	// sort Team members by Alpha order sorry vancluever
	var keys []string
	for k, _ := range rl {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, k := range keys {
		prs := rl[k].PRs
		fmt.Printf("\n%s:\n", k)
		// grap PR numbers to sort on number
		// TODO sort more on approval status, then number. Approvals at the bottom
		// Ex:
		// - http://pr/1
		// - http://pr/2
		// - http://pr/3 - ✅
		var keys []int
		for k := range prs {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			fmt.Printf("\t- %s\n", prs[k])
		}
	}
}

func prsForUser(users <-chan string, results chan<- UserPRs) {
	defer wgPrs.Done()
	// should pass in and reususe context I think?
	key := os.Getenv("GITHUB_API_TOKEN")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: key},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)
	opt := &github.SearchOptions{}

	for u := range users {
		sresults, resp, err := client.Search.Issues(ctx, fmt.Sprintf("state:open author:%s type:pr", u), opt)
		if err != nil {
			fmt.Println("Error: %s", err)
			continue
		}

		if resp.StatusCode != 200 {
			fmt.Println("status code: %d", resp.StatusCode)
		}

		mi := make(map[int]string)
		for _, i := range sresults.Issues {
			if !strings.Contains(*i.HTMLURL, "terraform") {
				continue
			}

			// parse url to URL, so we can split the parts
			u, err := url.Parse(*i.HTMLURL)
			if err != nil {
				log.Println("error parsing url:", err)
				continue
			}

			parts := strings.Split(u.Path, "/")
			owner := parts[1]
			repo := parts[2]

			reviews, _, err := client.PullRequests.ListReviews(ctx, owner, repo, *i.Number, nil)
			if err != nil {
				log.Printf("error getting review:%s")
				continue
			}

			// questionable logic here; if any of the reviews are "APPROVED" then
			// consider this approved. This isn't necessarily true because one
			// reviewer could approve then another follow up ask for changes, but for
			// my indvidiual puproses of reviewing TF team member PRs, this is
			// suffecient. If they need another review from me, I'll get a ping in
			// some fasion
			approved := ""
			for _, r := range reviews {
				// fmt.Printf("\t%d - (%d) review State: %s\n", j, *i.Number, *r.State)
				if "APPROVED" == *r.State {
					approved = " - ✅"
					break
				}
			}

			// format the output
			mi[*i.Number] = fmt.Sprintf("%s%s", *i.HTMLURL, approved)
		}

		results <- UserPRs{Login: u, PRs: mi}
	}
}
