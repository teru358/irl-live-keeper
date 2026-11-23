//go:build linux

package main

import (
	"io"
	"os"

	"github.com/teru358/irl-live-keeper/internal/service"
)

func runService(args []string, stdout, stderr io.Writer) int {
	out, okOut := stdout.(*os.File)
	errOut, okErr := stderr.(*os.File)
	if !okOut || !okErr {
		return 1
	}
	return service.Run(args, out, errOut)
}
