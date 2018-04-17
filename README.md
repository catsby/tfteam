# tf-team

This is a prototype/proof of concept of a tool I want to create assist the team
in reviewing each others PRs. More things may come but in another location, this
is just a proof of concept

### Install:

```
$ go get -u github.com/catsby/tfteam
```

If that doesn't work, clone and build:

```
$ git clone [...]
$ cd tfteam
$ go install
```

### Prerequisite:

Personal access token from here https://github.com/settings/tokens I think it just needs `public_repo`, `read:org` and `notifications`


    $ export GITHUB_API_TOKEN=""

### Usage:

    $ tfteam -h
    Usage: tfteam [--help] <command> [<args>]
    
    Available commands are:
        notifications    Aggregate GitHub notifications for Terraform* repositories, filtering out
                            notifications that have a reply from a HashiCorp colleague
        prs              List PRs opened by Terraform team, Collaborators, or specific users
        releases         List providers by last release date based on GitHub tag
        triage           List issues from Terraform* repositories with no label
        waiting          Show issues that have the 'waiting-response' label
