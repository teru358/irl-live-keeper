package gui

import (
	"os"
	"strings"
	"testing"
)

// Windows の GUI を動かせない環境でも spike と同じ通知経路を守る。
func TestButtonNotificationStructure(t *testing.T) {
	b, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "ui.NewButton(wnd,") || strings.Count(s, ".On().BnClicked(") != 4 {
		t.Fatal("ボタンは windigo の専用通知を使うこと")
	}

}
