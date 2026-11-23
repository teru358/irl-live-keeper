//go:build !windows

package main

import "time"

func prepareCrashOutput(string, time.Time) (string, string, func(), error) {
	return "", "", func() {}, nil
}
