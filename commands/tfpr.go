package commands

import "github.com/google/go-github/github"

type TFPr struct {
	*github.User
	HTMLURL  string
	Number   int
	Approved bool
}

func (tfpr *TFPr) String() string {
	url := tfpr.HTMLURL
	if tfpr.Approved {
		url += " - ✅"
	}
	return url
}
