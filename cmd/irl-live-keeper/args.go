package main

import (
	"flag"
	"fmt"
)

type arguments struct {
	dir     string
	cli     bool
	restore bool
}
type executionMode struct{ gui, pause bool }

func parseArgs(args []string) (arguments, error) {
	var a arguments
	f := flag.NewFlagSet("irl-live-keeper", flag.ContinueOnError)
	f.StringVar(&a.dir, "dir", "", "folder for config.yaml, logs and data")
	f.BoolVar(&a.cli, "cli", false, "open a console on Windows")
	var filtered []string
	for _, arg := range args {
		if arg == "restore-backup" {
			a.restore = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	err := f.Parse(filtered)
	if a.restore && !a.cli && err == nil {
		err = fmt.Errorf("restore-backup requires --cli")
	}
	return a, err
}
