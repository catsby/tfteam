package main

import (
	"log"
	"os"

	"github.com/catsby/tfteam/commands"
	"github.com/mitchellh/cli"
)

func main() {
	c := cli.NewCLI("tfteam", "0.0.1")
	c.Args = os.Args[1:]

	ui := &cli.ColoredUi{
		OutputColor: cli.UiColorNone,
		InfoColor:   cli.UiColorNone,
		ErrorColor:  cli.UiColorRed,
		WarnColor:   cli.UiColorYellow,

		Ui: &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stdout,
			ErrorWriter: os.Stderr,
		},
	}

	c.Commands = map[string]cli.CommandFactory{
		"prs": func() (cli.Command, error) {
			return &commands.PRsCommand{
				UI: ui,
			}, nil
		},
		"notifications": func() (cli.Command, error) {
			return &commands.NotificationsCommand{
				UI: ui,
			}, nil
		},
		"releases": func() (cli.Command, error) {
			return &commands.ReleasesCommand{
				UI: ui,
			}, nil
		},
		"triage": func() (cli.Command, error) {
			return &commands.TriageCommand{
				UI: ui,
			}, nil
		},
		"waiting": func() (cli.Command, error) {
			return &commands.WaitingCommand{
				UI: ui,
			}, nil
		},
	}

	exitStatus, err := c.Run()
	if err != nil {
		log.Println(err)
	}

	os.Exit(exitStatus)
}
