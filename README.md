# tf-team

This is a prototype/proof of concept of a tool I want to create assist the team
in reviewing each others PRs. More things may come but in another location, this
is just a proof of concept

Install:

    $ go build . -o tfteam
    $ mv tfteam $GOPATH/bin/

Prerequisite:

Personal access token from here https://github.com/settings/tokens I think it just needs `public_repo` and `read:org`


    $ export GITHUB_API_TOKEN=""

Usage:

    $ tfteam prs

