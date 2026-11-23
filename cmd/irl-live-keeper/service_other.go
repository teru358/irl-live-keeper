//go:build !linux

package main

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"io"
)

func runService(_ []string, _ io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stderr, i18n.Render(i18n.OSLang(), i18n.New("service.linux_only")))
	return 1
}
