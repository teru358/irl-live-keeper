//go:build windows

// Package gui は Windows の窓と本体の寿命をつなぐ。
package gui

import (
	"context"
	"github.com/rodrigocfd/windigo/ui"
	"runtime"
)

func Run(ctx context.Context, stop context.CancelFunc, class string, run func(context.Context) int) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	wnd := ui.NewMain(ui.OptsMain().Title("irl-live-keeper").ClassName(class).Size(ui.Dpi(520, 260)).Center(true))
	done := make(chan int, 1)
	started := false
	wnd.On().WmCreate(func(_ ui.WmCreate) int {
		started = true
		go func() {
			code := run(ctx)
			wnd.UiThread(func() { wnd.Hwnd().DestroyWindow() })
			done <- code
		}()
		return 0
	})
	wnd.On().WmClose(func() { stop() })
	wnd.RunAsMain()
	stop()
	if started {
		return <-done
	}
	return 1
}
