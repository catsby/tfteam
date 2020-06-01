package commands

import (
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
