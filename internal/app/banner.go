package app

import (
	"fmt"
	"strings"
)

// bannerInfo は起動時の表示に使う値。
type bannerInfo struct {
	// Cameras は設定の順。止めたプロトコルの送信用 URL は空文字で、表示しない。
	Cameras []camURLs
	// AdminURL は管理 API の URL。AdminNote は LAN に公開しているときの案内（空なら出さない）。
	AdminURL  string
	AdminNote string
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
	for _, c := range b.Cameras {
		fmt.Fprintf(&s, "  %s\n", c.label())
		if c.PublishSRT != "" {
			fmt.Fprintf(&s, "    SRT : %s\n", c.PublishSRT)
		}
		if c.PublishRTMP != "" {
			fmt.Fprintf(&s, "    RTMP: %s\n", c.PublishRTMP)
		}
	}
	if b.AutoHost {
		if !b.HostFound {
			s.WriteString("  LAN の IPv4 アドレスが見つかりません。host を 127.0.0.1 にしています（stream.publish_host で固定できます）\n")
		}
		s.WriteString("  屋外から送るときは、host をポート転送やトンネルのアドレスに置き換える（stream.publish_host で固定できる）\n")
	}
	s.WriteString("OBS のメディアソース用 URL（カメラごと）\n")
	for _, c := range b.Cameras {
		fmt.Fprintf(&s, "  %s\n    %s\n", c.label(), c.OBSRead)
	}
	s.WriteString("管理 API\n")
	fmt.Fprintf(&s, "  %s （ユーザー名 admin）\n", b.AdminURL)
	if b.AdminNote != "" {
		fmt.Fprintf(&s, "  %s\n", b.AdminNote)
	}
	if b.AdminPassword != "" {
		s.WriteString(adminPasswordLine(b.AdminPassword))
	}
	return s.String()
}

// label はカメラの表示名と id（URL の path）。表示名は重複してよいので、id を並べて区別する。
func (c camURLs) label() string {
	return fmt.Sprintf("%s（%s）", c.Name, c.ID)
}

// adminPasswordLine は、config.yaml を生成したときに出す管理 API のパスワードの行。
func adminPasswordLine(password string) string {
	return fmt.Sprintf("  管理 API のパスワード（config.yaml を生成しました）: %s\n", password)
}

// listenAddr は管理 API の待ち受けのアドレス。lan が false なら 127.0.0.1 だけ、true ならすべてのインターフェース。
func listenAddr(port int, lan bool) string {
	if lan {
		return fmt.Sprintf(":%d", port)
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// adminBannerURL は起動の表示に出す管理 API の URL と注記を返す。
// lan が true で LAN のアドレス（lanHost）が見つかっていればその URL を、見つからなければ 127.0.0.1 と案内を返す。
func adminBannerURL(port int, lan bool, lanHost string, found bool) (url, note string) {
	if !lan {
		return fmt.Sprintf("http://127.0.0.1:%d", port), ""
	}
	if found {
		return fmt.Sprintf("http://%s:%d", lanHost, port), "LAN に公開しています。LAN の機器からもこの URL で開けます"
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), fmt.Sprintf("LAN に公開しています。LAN の機器からは http://<この PC の LAN のアドレス>:%d", port)
}
