package main

import (
	"fmt"
	"strings"
)

// bannerInfo は起動時の表示に使う値。
type bannerInfo struct {
	PublishSRT  string
	PublishRTMP string
	OBSRead     string
	AdminListen string
	// AdminPassword は config.yaml を生成したときだけ入れる。空なら表示しない。
	AdminPassword string
	// AutoHost は送信用 host を自動で決めたか。HostFound は自動で LAN の IPv4 が見つかったか。
	AutoHost  bool
	HostFound bool
}

// startupBanner は起動時に標準出力へ1回だけ書く文字列を作る。パスワードを含むので、ログには通さない。
func startupBanner(b bannerInfo) string {
	var s strings.Builder
	s.WriteString("== irl-live-keeper ==\n")
	s.WriteString("送信用 URL（配信アプリに設定する）\n")
	fmt.Fprintf(&s, "  SRT : %s\n", b.PublishSRT)
	fmt.Fprintf(&s, "  RTMP: %s\n", b.PublishRTMP)
	if b.AutoHost {
		if !b.HostFound {
			s.WriteString("  LAN の IPv4 アドレスが見つかりません。host を 127.0.0.1 にしています（stream.publish_host で固定できます）\n")
		}
		s.WriteString("  屋外から送るときは、host をポート転送やトンネルのアドレスに置き換える（stream.publish_host で固定できる）\n")
	}
	s.WriteString("OBS のメディアソース用 URL\n")
	fmt.Fprintf(&s, "  %s\n", b.OBSRead)
	s.WriteString("管理 API\n")
	fmt.Fprintf(&s, "  http://%s （ユーザー名 admin）\n", b.AdminListen)
	if b.AdminPassword != "" {
		fmt.Fprintf(&s, "  管理 API のパスワード（config.yaml を生成しました）: %s\n", b.AdminPassword)
	}
	return s.String()
}
