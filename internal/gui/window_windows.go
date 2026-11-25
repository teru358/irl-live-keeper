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
var uxtheme = windows.NewLazySystemDLL("uxtheme.dll")

type logFontW struct {
	Height, Width, Escapement, Orientation, Weight                                              int32
	Italic, Underline, StrikeOut, CharSet, OutPrecision, ClipPrecision, Quality, PitchAndFamily byte
	FaceName                                                                                    [32]uint16
}

type nonClientMetrics struct {
	Size                                                                uint32
	BorderWidth, ScrollWidth, ScrollHeight, CaptionWidth, CaptionHeight int32
	SmallCaptionWidth, SmallCaptionHeight, MenuWidth, MenuHeight        int32
	CaptionFont, SmallCaptionFont, MenuFont, StatusFont, MessageFont    logFontW
	PaddedBorderWidth                                                   int32
}

func call(name string, args ...uintptr) uintptr {
	r, _, _ := user.NewProc(name).Call(args...)
	return r
}
func ptr(s string) uintptr { p, _ := windows.UTF16PtrFromString(s); return uintptr(unsafe.Pointer(p)) }
func rgb(s string) uintptr {
	v, _ := strconv.ParseUint(s[1:], 16, 32)
	return uintptr((v&255)<<16 | v&0xff00 | v>>16)
}
func Run(ctx context.Context, stop context.CancelFunc, class string, a *app.App, cleanup func()) (code int) {
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
	buttons := make([]*ui.Button, 5)
	for i := range buttons {
		buttons[i] = ui.NewButton(wnd, ui.OptsButton().CtrlId(uint16(100+i)).Text("").Position(ui.Dpi(12, 12)).Width(ui.DpiX(480)))
	}
	var rows []uintptr
	var colors []string
	var notice, open, quit, logs, retry, restore uintptr
	var font uintptr
	var dpi int
	var lastNotice string
	done := make(chan int, 1)
	started := false
	applyFont := func() {
		metrics := nonClientMetrics{Size: uint32(unsafe.Sizeof(nonClientMetrics{}))}
		if call("SystemParametersInfoW", 0x29, uintptr(metrics.Size), uintptr(unsafe.Pointer(&metrics)), 0) == 0 {
			return
		}
		systemDPI := int(call("GetDpiForSystem"))
		if systemDPI == 0 {
			systemDPI = 96
		}
		metrics.MessageFont.Height = int32(int(metrics.MessageFont.Height) * dpi / systemDPI)
		newFont, _, _ := gdi.NewProc("CreateFontIndirectW").Call(uintptr(unsafe.Pointer(&metrics.MessageFont)))
		if newFont == 0 {
			return
		}
		oldFont := font
		font = newFont
		for _, h := range []uintptr{open, quit, logs, retry, restore, notice} {
			if h != 0 {
				call("SendMessageW", h, 0x30, font, 1)
			}
		}
		if oldFont != 0 {
			gdi.NewProc("DeleteObject").Call(oldFont)
		}
	}
	create := func(kind, text string, id int) uintptr {
		h := call("CreateWindowExW", 0, ptr(kind), ptr(text), 0x50000000, 12, 12, 480, 24, uintptr(wnd.Hwnd()), uintptr(id), 0, 0)
		if font != 0 {
			call("SendMessageW", h, 0x30, font, 1)
		}
		if dark {
			uxtheme.NewProc("SetWindowTheme").Call(h, ptr("DarkMode_Explorer"), 0)
		}
		return h
	}
	move := func(h uintptr, x, y, width, height int) {
		call("MoveWindow", h, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 1)
	}
	update := func() {
		s := a.WindowState()
		m := Build(s, dark)
		noticeText := NoticeForWindow(m)
		layout := layoutForDPI(dpi, len(m.Rows))
		for len(rows) < len(m.Rows) {
			rows = append(rows, create("STATIC", "", 0))
			colors = append(colors, fg)
		}
		for i, h := range rows {
			if i < len(m.Rows) {
				call("ShowWindow", h, 5)
				call("SetWindowTextW", h, ptr(m.Rows[i].Text))
				colors[i] = m.Rows[i].Color
				move(h, layout.Left, scaleDPI(12+i*28, dpi), layout.NoticeWidth, layout.RowHeight)
			} else {
				call("ShowWindow", h, 0)
			}
		}
		y := layout.ButtonTop
		for i, b := range []struct {
			h uintptr
			k string
		}{{open, "window_open"}, {quit, "window_exit"}, {logs, "window_logs"}, {retry, "window_retry"}, {restore, "window_restore"}} {
			call("SetWindowTextW", b.h, ptr(Text(s.Language, b.k)))
			move(b.h, layout.Left, y+i*layout.ButtonStep, layout.NoticeWidth, layout.ButtonHeight)
		}
		call("EnableWindow", open, boolint(m.CanOpen))
		call("EnableWindow", retry, boolint(m.Retry))
		call("ShowWindow", logs, show(m.Retry))
		call("ShowWindow", retry, show(m.Retry))
		call("EnableWindow", restore, boolint(m.Restore))
		call("ShowWindow", restore, show(m.Restore))

		// EDIT と同じフォントで折り返し後の高さを測る。
		dc := call("GetDC", notice)
		font := call("SendMessageW", notice, 0x31, 0, 0)
		oldFont, _, _ := gdi.NewProc("SelectObject").Call(dc, font)
		rect := [4]int32{0, 0, int32(layout.NoticeTextWidth), 0}
		call("DrawTextW", dc, ptr(noticeText), ^uintptr(0), uintptr(unsafe.Pointer(&rect)), 0x10|0x400|0x800)
		gdi.NewProc("SelectObject").Call(dc, oldFont)
		call("ReleaseDC", notice, dc)
		height := noticeAreaHeight(int(rect[3]), layout.NoticeMinHeight, layout.Padding)
		var work [4]int32
		monitor := call("MonitorFromWindow", uintptr(wnd.Hwnd()), 2)
		info := struct {
			Size          uint32
			Monitor, Work [4]int32
			Flags         uint32
		}{}
		info.Size = uint32(unsafe.Sizeof(info))
		call("GetMonitorInfoW", monitor, uintptr(unsafe.Pointer(&info)))
		work = info.Work
		available := int(work[3] - work[1])
		var outer, client [4]int32
		call("GetWindowRect", uintptr(wnd.Hwnd()), uintptr(unsafe.Pointer(&outer)))
		call("GetClientRect", uintptr(wnd.Hwnd()), uintptr(unsafe.Pointer(&client)))
		frame := int(outer[3] - outer[1] - client[3])
		top := layout.NoticeTop
		total := layoutWindowHeight(layout, len(m.Rows), height, frame)
		if available > 0 && total > available {
			total = available
			height = max(scaleDPI(24, dpi), total-frame-layout.Padding-top)
		}
		move(notice, layout.Left, top, layout.NoticeWidth, height)
		if lastNotice != noticeText {
			call("SetWindowTextW", notice, ptr(noticeText))
			lastNotice = noticeText
		}
		call("ShowScrollBar", notice, 1, boolint(int(rect[3])+layout.Padding > height))
		if int(outer[3]-outer[1]) != total || int(outer[2]-outer[0]) != layout.Width {
			newY := int(outer[1])
			if available > 0 && newY+total > int(work[3]) {
				newY = int(work[3]) - total
			}
			call("SetWindowPos", uintptr(wnd.Hwnd()), 0, uintptr(outer[0]), uintptr(newY), uintptr(layout.Width), uintptr(total), 4)
		}
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
		// go-winres の APP 資源を大小のタイトルアイコンに使う。
		instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
		for _, size := range []struct{ kind, pixels uintptr }{{0, 16}, {1, 32}} {
			icon := call("LoadImageW", uintptr(instance), ptr("APP"), 1, size.pixels, size.pixels, 0x8000)
			if icon == 0 {
				icon = call("LoadImageW", uintptr(instance), 1, 1, size.pixels, size.pixels, 0x8000)
			}
			if icon != 0 {
				call("SendMessageW", uintptr(wnd.Hwnd()), 0x80, size.kind, icon)
			}
		}
		open = uintptr(buttons[0].Hwnd())
		quit = uintptr(buttons[1].Hwnd())
		logs = uintptr(buttons[2].Hwnd())
		retry = uintptr(buttons[3].Hwnd())
		restore = uintptr(buttons[4].Hwnd())
		// 読み取り専用の複数行欄。作業領域を超える長文はスクロールできる。
		notice = call("CreateWindowExW", 0, ptr("EDIT"), ptr(""), 0x50210844, 12, 12, 480, 100, uintptr(wnd.Hwnd()), 104, 0, 0)
		dpi = int(call("GetDpiForWindow", uintptr(wnd.Hwnd())))
		if dpi == 0 {
			dpi = 96
		}
		applyFont()
		if dark {
			for _, h := range []uintptr{open, quit, logs, retry, restore, notice} {
				uxtheme.NewProc("SetWindowTheme").Call(h, ptr("DarkMode_Explorer"), 0)
			}
		}
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
	wnd.On().Wm(co.WM(0x02e0), func(p ui.Wm) uintptr {
		dpi = int(p.WParam & 0xffff)
		if dpi <= 0 {
			dpi = int(call("GetDpiForWindow", uintptr(wnd.Hwnd())))
		}
		var suggested [4]int32
		if p.LParam != 0 {
			windows.NewLazySystemDLL("kernel32.dll").NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&suggested[0])), uintptr(p.LParam), unsafe.Sizeof(suggested))
			call("SetWindowPos", uintptr(wnd.Hwnd()), 0, uintptr(suggested[0]), uintptr(suggested[1]), uintptr(suggested[2]-suggested[0]), uintptr(suggested[3]-suggested[1]), 4)
		}
		applyFont()
		update()
		return 0
	})
	buttons[0].On().BnClicked(func() {
		u, e := a.WindowURL()
		if e == nil {
			e = winproc.OpenURL(u)
		}
		if e != nil {
			call("MessageBoxW", uintptr(wnd.Hwnd()), ptr(e.Error()), ptr("irl-live-keeper"), 0x10)
		}
	})
	buttons[1].On().BnClicked(func() { exit(ExitConfirmation()) })
	buttons[2].On().BnClicked(func() { winproc.OpenFolder(a.WindowState().LogDir) })
	buttons[3].On().BnClicked(func() { a.Retry() })
	buttons[4].On().BnClicked(func() { a.RestoreBackup() })
	wnd.On().Wm(co.WM(0x8001), func(ui.Wm) uintptr {
		a.SecondInstanceBlocked()
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
			cleanup()
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
	if font != 0 {
		gdi.NewProc("DeleteObject").Call(font)
	}
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
