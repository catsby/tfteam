package commands

import (
	"github.com/google/go-github/github"
)

type TFPr struct {
	*github.User
	HTMLURL  string
	Number   int
	Approved bool
	Title    string
}

func (tfpr *TFPr) IsApprovedString() string {
	var approved string
	if tfpr.Approved {
		approved = "✅"
	}
	return approved
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
