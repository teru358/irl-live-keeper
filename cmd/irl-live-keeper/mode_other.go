//go:build !windows

package main

import (
	"context"
	"github.com/teru358/irl-live-keeper/internal/app"
)

func prepareMode(bool) (executionMode, error)                    { return executionMode{pause: true}, nil }
func acquireInstance(string, bool) (func(), string, bool, error) { return func() {}, "", false, nil }
func runMode(ctx context.Context, _ context.CancelFunc, _ bool, _ string, runner *app.App, cleanup func()) int {
	return runner.Run(ctx)
}
