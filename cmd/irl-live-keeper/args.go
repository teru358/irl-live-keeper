package main

import "flag"

type arguments struct {
	dir string
	cli bool
}
type executionMode struct{ gui, pause bool }

func parseArgs(args []string) (arguments, error) {
	var a arguments
	f := flag.NewFlagSet("irl-live-keeper", flag.ContinueOnError)
	f.StringVar(&a.dir, "dir", "", "folder for config.yaml, logs and data")
	f.BoolVar(&a.cli, "cli", false, "open a console on Windows")
	err := f.Parse(args)
	return a, err
}
