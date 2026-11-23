//go:build windows

package gui

import (
	"context"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/teru358/irl-live-keeper/internal/app"
	"github.com/teru358/irl-live-keeper/internal/winproc"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"runtime"
	"strconv"
	"unsafe"
)

var user = windows.NewLazySystemDLL("user32.dll")
var gdi = windows.NewLazySystemDLL("gdi32.dll")

func call(name string, args ...uintptr) uintptr {
	r, _, _ := user.NewProc(name).Call(args...)
	return r
}
func ptr(s string) uintptr { p, _ := windows.UTF16PtrFromString(s); return uintptr(unsafe.Pointer(p)) }
func rgb(s string) uintptr {
	v, _ := strconv.ParseUint(s[1:], 16, 32)
	return uintptr((v&255)<<16 | v&0xff00 | v>>16)
}
func Run(ctx context.Context, stop context.CancelFunc, class string, a *app.App) (code int) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() {
		if recover() != nil {
			call("MessageBoxW", 0, ptr("Unable to create irl-live-keeper window"), ptr("irl-live-keeper"), 0x10)
			code = 1
		}
	}()
	dark := false
	if k, e := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE); e == nil {
		v, _, e := k.GetIntegerValue("AppsUseLightTheme")
		dark = e == nil && v == 0
		k.Close()
	}
	bg, fg := "#F5F7F6", "#16202A"
	if dark {
		bg, fg = "#131B24", "#E7EDF2"
	}
	brush, _, _ := gdi.NewProc("CreateSolidBrush").Call(rgb(bg))
	defer gdi.NewProc("DeleteObject").Call(brush)
	wnd := ui.NewMain(ui.OptsMain().Title("irl-live-keeper").ClassName(class).Size(ui.Dpi(520, 380)).Center(true))
	var rows []uintptr
	var colors []string
	var notice, open, quit, logs, retry uintptr
	done := make(chan int, 1)
	started := false
	create := func(kind, text string, id int) uintptr {
		return call("CreateWindowExW", 0, ptr(kind), ptr(text), 0x50000000, 12, 12, 480, 24, uintptr(wnd.Hwnd()), uintptr(id), 0, 0)
	}
	move := func(h uintptr, y, height int) { call("MoveWindow", h, 12, uintptr(y), 480, uintptr(height), 1) }
	update := func() {
		s := a.WindowState()
		m := Build(s, dark)
		for len(rows) < len(m.Rows) {
			rows = append(rows, create("STATIC", "", 0))
			colors = append(colors, fg)
		}
		for i, h := range rows {
			if i < len(m.Rows) {
				call("ShowWindow", h, 5)
				call("SetWindowTextW", h, ptr(m.Rows[i].Text))
				colors[i] = m.Rows[i].Color
				move(h, 12+i*28, 24)
			} else {
				call("ShowWindow", h, 0)
			}
		}
		y := 16 + len(m.Rows)*28
		for i, b := range []struct {
			h uintptr
			k string
		}{{open, "window_open"}, {quit, "window_exit"}, {logs, "window_logs"}, {retry, "window_retry"}} {
			call("SetWindowTextW", b.h, ptr(Text(s.Language, b.k)))
			move(b.h, y+i*30, 26)
		}
		call("EnableWindow", open, boolint(m.CanOpen))
		call("EnableWindow", retry, boolint(m.Retry))
		call("ShowWindow", logs, show(m.Retry))
		call("ShowWindow", retry, show(m.Retry))
		move(notice, y+124, 100)
		call("SetWindowTextW", notice, ptr(m.Notice))
		call("SetWindowPos", uintptr(wnd.Hwnd()), 0, 0, 0, 540, uintptr(y+280), 6)
		call("InvalidateRect", uintptr(wnd.Hwnd()), 0, 1)
	}
	exit := func(confirm bool) {
		s := a.WindowState()
		stream := s.Snapshot != nil && s.Snapshot.OBS.Streaming
		if confirm && call("MessageBoxW", uintptr(wnd.Hwnd()), ptr(ExitPrompt(s.Language, stream)), ptr("irl-live-keeper"), 0x24) != 6 {
			return
		}
		stop()
	}
	wnd.On().WmCreate(func(_ ui.WmCreate) int {
		started = true
		open = create("BUTTON", "", 101)
		quit = create("BUTTON", "", 102)
		logs = create("BUTTON", "", 103)
		retry = create("BUTTON", "", 104)
		notice = create("STATIC", "", 0)
		if dark {
			v := int32(1)
			p := windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
			r, _, _ := p.Call(uintptr(wnd.Hwnd()), 20, uintptr(unsafe.Pointer(&v)), 4)
			if r != 0 {
				p.Call(uintptr(wnd.Hwnd()), 19, uintptr(unsafe.Pointer(&v)), 4)
			}
		}
		update()
		wnd.Hwnd().SetTimer(1, 1000)
		go func() { done <- a.RunWindow(ctx) }()
		return 0
	})
	wnd.On().WmTimer(1, func() {
		select {
		case v := <-done:
			code = v
			started = false
			wnd.Hwnd().DestroyWindow()
		default:
			update()
		}
	})
	wnd.On().Wm(co.WM(0x111), func(p ui.Wm) uintptr {
		switch p.WParam & 0xffff {
		case 101:
			u, e := a.WindowURL()
			if e == nil {
				e = winproc.OpenURL(u)
			}
			if e != nil {
				call("MessageBoxW", uintptr(wnd.Hwnd()), ptr(e.Error()), ptr("irl-live-keeper"), 0x10)
			}
		case 102:
			exit(true)
		case 103:
			winproc.OpenFolder(a.WindowState().LogDir)
		case 104:
			a.Restart()
		}
		return 0
	})
	wnd.On().WmClose(func() {
		s := a.WindowState()
		mini, confirm := CloseDecision(s.CloseAction, false)
		if mini {
			call("ShowWindow", uintptr(wnd.Hwnd()), 6)
		} else {
			exit(confirm)
		}
	})
	wnd.On().Wm(co.WM(0x11), func(ui.Wm) uintptr { return 1 })
	wnd.On().WmEndSession(func(p ui.WmEndSession) {
		if p.IsSessionBeingEnded() {
			stop()
			if started {
				code = <-done
				started = false
			}
			wnd.Hwnd().DestroyWindow()
		}
	})
	wnd.On().Wm(co.WM(0x138), func(p ui.Wm) uintptr {
		color := fg
		for i, h := range rows {
			if h == uintptr(p.LParam) {
				color = colors[i]
			}
		}
		gdi.NewProc("SetTextColor").Call(uintptr(p.WParam), rgb(color))
		gdi.NewProc("SetBkColor").Call(uintptr(p.WParam), rgb(bg))
		return brush
	})
	wnd.On().Wm(co.WM(0x14), func(p ui.Wm) uintptr {
		var rect [4]int32
		call("GetClientRect", uintptr(wnd.Hwnd()), uintptr(unsafe.Pointer(&rect)))
		call("FillRect", uintptr(p.WParam), uintptr(unsafe.Pointer(&rect)), brush)
		return 1
	})
	wnd.RunAsMain()
	stop()
	if started {
		return <-done
	}
	return code
}
func boolint(v bool) uintptr {
	if v {
		return 1
	}
	return 0
}
func show(v bool) uintptr {
	if v {
		return 5
	}
	return 0
}
