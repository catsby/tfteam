package commands

import (
	"time"

	"github.com/google/go-github/github"
)

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

// A collection of PRs that can be sorted
type TFPRGroup []*TFPr

func (t TFPRGroup) Len() int      { return len(t) }
func (t TFPRGroup) Swap(i, j int) { t[i], t[j] = t[j], t[i] }
func (t TFPRGroup) Less(i, j int) bool {
	return !t[i].CreatedAt.After(*t[j].CreatedAt)
}

func (tfpr *TFPr) IsApprovedString() string {
	approved := "   "
	if "APPROVED" == tfpr.State {
		approved = "+  "
	}
	if "COMMENTED" == tfpr.State {
		approved = "?  "
	}
	if "CHANGES_REQUESTED" == tfpr.State {
		approved = "-  "
	}
	return approved
}

func (tfpr *TFPr) StatusCode() PRReviewStatus {
	status := StatusWaiting
	if "APPROVED" == tfpr.State {
		status = StatusApproved
	}
	if "COMMENTED" == tfpr.State {
		status = StatusComments
	}
	if "CHANGES_REQUESTED" == tfpr.State {
		status = StatusChanges
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
