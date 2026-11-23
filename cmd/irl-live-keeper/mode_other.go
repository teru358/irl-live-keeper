//go:build !windows

package main

import "context"

func prepareMode(bool) (executionMode, error)                    { return executionMode{pause: true}, nil }
func acquireInstance(string, bool) (func(), string, bool, error) { return func() {}, "", false, nil }
func runMode(ctx context.Context, _ context.CancelFunc, _ bool, _ string, run func(context.Context) int) int {
	return run(ctx)
}
