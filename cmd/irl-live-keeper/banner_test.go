package main

import (
	"strings"
	"testing"
)

func TestStartupBanner(t *testing.T) {
	base := bannerInfo{
		PublishSRT:  "srt://192.168.1.10:8890?streamid=publish:live:irl:PPPP",
		PublishRTMP: "rtmp://192.168.1.10:1935/live?user=irl&pass=PPPP",
		OBSRead:     "rtsp://obs:RRRR@127.0.0.1:8554/live",
		AdminURL:    "http://127.0.0.1:8787",
		HostFound:   true,
	}
	tests := []struct {
		name    string
		mutate  func(b *bannerInfo)
		want    []string
		wantNot []string
	}{
		{
			name:    "手動で、config を生成していない",
			mutate:  func(b *bannerInfo) {},
			want:    []string{base.PublishSRT, base.PublishRTMP, base.OBSRead, "http://127.0.0.1:8787"},
			wantNot: []string{"パスワード", "置き換え", "見つかりません"},
		},
		{
			name:   "管理 API の案内（LAN の注記）を出す",
			mutate: func(b *bannerInfo) { b.AdminNote = "LAN の機器からは http://192.168.1.10:8787" },
			want:   []string{"LAN の機器からは http://192.168.1.10:8787"},
		},
		{
			name:   "config を生成したときだけ管理 API のパスワード",
			mutate: func(b *bannerInfo) { b.AdminPassword = "ADMINPW" },
			want:   []string{"ADMINPW", "パスワード"},
		},
		{
			name:   "自動のときは host の置き換えの案内",
			mutate: func(b *bannerInfo) { b.AutoHost = true },
			want:   []string{"屋外から送るときは、host をポート転送やトンネルのアドレスに置き換える", "stream.publish_host"},
		},
		{
			name:   "自動で見つからなかったときは、その旨も出す",
			mutate: func(b *bannerInfo) { b.AutoHost, b.HostFound = true, false },
			want:   []string{"見つかりません", "127.0.0.1", "stream.publish_host"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := base
			tt.mutate(&b)
			got := startupBanner(b)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("表示に %q がない:\n%s", w, got)
				}
			}
			for _, w := range tt.wantNot {
				if strings.Contains(got, w) {
					t.Errorf("表示に %q があってはいけない:\n%s", w, got)
				}
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("末尾が改行でない: %q", got)
			}
		})
	}
}

func TestListenAddr(t *testing.T) {
	if got := listenAddr(8787, false); got != "127.0.0.1:8787" {
		t.Errorf("lan=false: %q", got)
	}
	if got := listenAddr(18787, true); got != ":18787" {
		t.Errorf("lan=true: %q", got)
	}
}

func TestAdminBannerURL(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		lan       bool
		host      string
		found     bool
		wantURL   string
		wantNote  string
		noteEmpty bool
	}{
		{"lan=false", 8787, false, "192.168.1.10", true, "http://127.0.0.1:8787", "", true},
		{"lan=true で LAN のアドレスが見つかった", 8787, true, "192.168.1.10", true, "http://192.168.1.10:8787", "LAN の機器から", false},
		{"lan=true で見つからない", 9000, true, "127.0.0.1", false, "http://127.0.0.1:9000", "http://<この PC の LAN のアドレス>:9000", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, note := adminBannerURL(tt.port, tt.lan, tt.host, tt.found)
			if url != tt.wantURL {
				t.Errorf("url = %q, want %q", url, tt.wantURL)
			}
			if tt.noteEmpty && note != "" {
				t.Errorf("note = %q", note)
			}
			if !tt.noteEmpty && !strings.Contains(note, tt.wantNote) {
				t.Errorf("note = %q, want %q を含む", note, tt.wantNote)
			}
		})
	}
}
