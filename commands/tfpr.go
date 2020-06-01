package commands

import (
	"fmt"
	"math"
	"time"

	"github.com/google/go-github/github"
)

// TFPr is a struct that holds PR information that we need for display
type TFPr struct {
	*github.User
	HTMLURL string
	Number  int
	State   string
	Title   string

	Owner string
	Repo  string

	CreatedAt *time.Time
	UpdatedAt *time.Time
}

var nowish = time.Now()

// TimeAgoHumanized returns a "humanized" version of "time ago", just to
// emphasize how old a PR is. If less than 24 hours, then this is returned as "X
// hours ago". If greater than 24 hours, then returned as "days ago".
func (pr *TFPr) TimeAgoHumanized() string {
	since := nowish.Sub(*pr.CreatedAt)
	hours := since.Hours()
	if hours > 24.0 {
		days := math.Floor(hours / 24.0)
		return fmt.Sprintf("%d days ago", int(days))
	}
	return fmt.Sprintf("%d hours ago", int(hours))
}

// TFPRGroup implements sorting interface
type TFPRGroup []*TFPr

func (t TFPRGroup) Len() int      { return len(t) }
func (t TFPRGroup) Swap(i, j int) { t[i], t[j] = t[j], t[i] }
func (t TFPRGroup) Less(i, j int) bool {
	return !t[i].CreatedAt.After(*t[j].CreatedAt)
}

// IsApprovedString generates a table friendly string of approval
func (tfpr *TFPr) IsApprovedString() string {
	approved := "[ ]   "
	if "APPROVED" == tfpr.State {
		approved = "[+]  "
	}
	if "COMMENTED" == tfpr.State {
		approved = "[?]  "
	}
	if "CHANGES_REQUESTED" == tfpr.State {
		approved = "[-]  "
	}
	return approved
}

// StatusCode clean up the status
func (tfpr *TFPr) StatusCode() PRReviewStatus {
	status := statusWaiting
	if "APPROVED" == tfpr.State {
		status = statusApproved
	}
	if "COMMENTED" == tfpr.State {
		status = statusComments
	}
	if "CHANGES_REQUESTED" == tfpr.State {
		status = statusChanges
	}
	return status
}

func (tfpr *TFPr) TitleTruncated() string {
	width := 50
	if len(tfpr.Title) < width {
		padding := " "
		needed := width - len(tfpr.Title)
		formatted := tfpr.Title
		for i := 0; i < needed; i++ {
			formatted = formatted + padding
		}
		return formatted
	}
	return tfpr.Title[:45] + "[...]"
}
