package app

import (
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/i18n"
)

// mainCam と subCam は2台の表示の例（id は main と sub、表示名は メインカメラ と サブカメラ）。
var mainCam = camURLs{
	Name:        "メインカメラ",
	ID:          "main",
	PublishSRT:  "srt://192.168.1.10:8890?streamid=publish:main:irl:PPPP",
	PublishRTMP: "rtmp://192.168.1.10:1935/main?user=irl&pass=PPPP",
	OBSRead:     "rtsp://obs:RRRR@127.0.0.1:8554/main",
}

func TestStartupBannerEnglish(t *testing.T) {
	got := startupBanner(bannerInfo{Lang: i18n.EN, Cameras: []camURLs{mainCam}, AdminURL: "http://127.0.0.1:8787", AdminPassword: "ADMINPW"})
	if strings.Contains(got, "送信用 URL") || strings.Contains(got, "管理 API のパスワード") {
		t.Fatalf("Japanese banner text in English mode:\n%s", got)
	}
	for _, want := range []string{"Publish URLs", "OBS media source URLs", "Admin API", "ADMINPW", "メインカメラ (main)"} {
		if !strings.Contains(got, want) {
			t.Errorf("English banner missing %q:\n%s", want, got)
		}
	}
}

var subCam = camURLs{
	Name:        "サブカメラ",
	ID:          "sub",
	PublishSRT:  "srt://192.168.1.10:8890?streamid=publish:sub:irl:PPPP",
	PublishRTMP: "rtmp://192.168.1.10:1935/sub?user=irl&pass=PPPP",
	OBSRead:     "rtsp://obs:RRRR@127.0.0.1:8554/sub",
}

func TestStartupBanner(t *testing.T) {
	base := bannerInfo{
		Cameras:   []camURLs{mainCam},
		AdminURL:  "http://127.0.0.1:8787",
		HostFound: true,
	}
	noRTMP := func(c camURLs) camURLs { c.PublishRTMP = ""; return c }
	noSRT := func(c camURLs) camURLs { c.PublishSRT = ""; return c }
	tests := []struct {
		name    string
		mutate  func(b *bannerInfo)
		want    []string
		wantNot []string
	}{
		{
			name:    "1台、手動で、config を生成していない",
			mutate:  func(b *bannerInfo) {},
			want:    []string{mainCam.PublishSRT, mainCam.PublishRTMP, mainCam.OBSRead, "メインカメラ（main）", "http://127.0.0.1:8787"},
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
		{
			name:    "2台、両方（それぞれの名前と URL を出す）",
			mutate:  func(b *bannerInfo) { b.Cameras = []camURLs{mainCam, subCam} },
			want:    []string{"メインカメラ（main）", "サブカメラ（sub）", mainCam.PublishSRT, mainCam.PublishRTMP, mainCam.OBSRead, subCam.PublishSRT, subCam.PublishRTMP, subCam.OBSRead},
			wantNot: []string{"パスワード"},
		},
		{
			name:    "RTMP を止めた（どのカメラも RTMP の行と URL を出さない）",
			mutate:  func(b *bannerInfo) { b.Cameras = []camURLs{noRTMP(mainCam), noRTMP(subCam)} },
			want:    []string{mainCam.PublishSRT, subCam.PublishSRT, mainCam.OBSRead, subCam.OBSRead},
			wantNot: []string{"RTMP:", mainCam.PublishRTMP, subCam.PublishRTMP},
		},
		{
			name:    "SRT を止めた（どのカメラも SRT の行と URL を出さない）",
			mutate:  func(b *bannerInfo) { b.Cameras = []camURLs{noSRT(mainCam), noSRT(subCam)} },
			want:    []string{mainCam.PublishRTMP, subCam.PublishRTMP, mainCam.OBSRead, subCam.OBSRead},
			wantNot: []string{"SRT :", mainCam.PublishSRT, subCam.PublishSRT},
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

// 2台のとき、設定の順（main、sub）に並ぶ。自動の host の案内は1回だけ。
func TestStartupBannerOrderAndNoteOnce(t *testing.T) {
	got := startupBanner(bannerInfo{
		Cameras:   []camURLs{mainCam, subCam},
		AdminURL:  "http://127.0.0.1:8787",
		AutoHost:  true,
		HostFound: true,
	})
	at := func(s string) int {
		i := strings.Index(got, s)
		if i < 0 {
			t.Fatalf("表示に %q がない:\n%s", s, got)
		}
		return i
	}
	if at(mainCam.PublishSRT) > at(subCam.PublishSRT) {
		t.Errorf("送信用の並びが設定の順になっていない:\n%s", got)
	}
	if at(mainCam.OBSRead) > at(subCam.OBSRead) {
		t.Errorf("OBS 用の並びが設定の順になっていない:\n%s", got)
	}
	if n := strings.Count(got, "屋外から送るときは"); n != 1 {
		t.Errorf("自動の host の案内が %d 回出ている（1回のはず）:\n%s", n, got)
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
