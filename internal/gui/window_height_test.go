package gui

import (
	"fmt"
	"testing"
)

func TestNoticeAreaHeight(t *testing.T) {
	for _, tc := range []struct{ text, minimum, padding, want int }{
		{40, 100, 12, 100},
		{100, 100, 12, 112},
		{240, 100, 12, 252},
	} {
		if got := noticeAreaHeight(tc.text, tc.minimum, tc.padding); got != tc.want {
			t.Errorf("noticeAreaHeight(%d, %d, %d) = %d, want %d", tc.text, tc.minimum, tc.padding, got, tc.want)
		}
	}
}

func TestWindowHeight(t *testing.T) {
	for _, tc := range []struct{ top, notice, padding, frame, minimum, want int }{
		{250, 100, 12, 39, 431, 431},
		{250, 252, 12, 39, 431, 553},
	} {
		if got := windowHeight(tc.top, tc.notice, tc.padding, tc.frame, tc.minimum); got != tc.want {
			t.Errorf("windowHeight(...) = %d, want %d", got, tc.want)
		}
	}
}

func TestLayoutFitsAtCommonDPIs(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		dpi := dpi
		t.Run(fmt.Sprintf("%d", dpi), func(t *testing.T) {
			const rows = 4
			layout := layoutForDPI(dpi, rows, 5)
			noticeHeight := layout.NoticeMinHeight
			const frame = 39
			height := layoutWindowHeight(layout, rows, 5, noticeHeight, frame)
			clientHeight := height - frame
			rowsBottom := layout.Left + (rows-1)*layout.RowStep + layout.RowHeight
			buttonsBottom := layout.ButtonTop + 4*layout.ButtonStep + layout.ButtonHeight
			noticeBottom := layout.NoticeTop + noticeHeight
			bottom := max(rowsBottom, max(buttonsBottom, noticeBottom))
			if bottom+layout.Padding > clientHeight {
				t.Fatalf("DPI %d: content bottom %d + padding %d exceeds client height %d", dpi, bottom, layout.Padding, clientHeight)
			}
		})
	}
}

func TestVisibleButtonLayoutsDoNotOverlapAndFit(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		for _, rows := range []int{3, 4, 5} {
			for _, buttons := range []int{3, 4, 5} {
				for _, noticeHeight := range []int{100, 180, 320, 520} {
					l := layoutForDPI(dpi, rows, buttons)
					total := layoutWindowHeight(l, rows, buttons, noticeHeight, 39)
					client := total - 39
					buttonsBottom := l.ButtonTop + (buttons-1)*l.ButtonStep + l.ButtonHeight
					if l.NoticeTop < buttonsBottom || l.NoticeTop+noticeHeight+l.Padding > client {
						t.Fatalf("dpi=%d rows=%d buttons=%d notice=%d: layout=%+v total=%d", dpi, rows, buttons, noticeHeight, l, total)
					}
				}
			}
		}
	}
}
