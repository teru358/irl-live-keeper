package app

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"strings"
)

// bannerInfo は起動時の表示に使う値。
type bannerInfo struct {
	Lang i18n.Lang
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
	lang := b.Lang
	if lang != i18n.EN {
		lang = i18n.JA
	}
	render := func(code string, values ...string) string { return i18n.Render(lang, i18n.New(code, values...)) }
	var s strings.Builder
	fmt.Fprintf(&s, "%s\n", render("banner.title"))
	fmt.Fprintf(&s, "%s\n", render("banner.publish"))
	for _, c := range b.Cameras {
		fmt.Fprintf(&s, "  %s\n", c.label(lang))
		if c.PublishSRT != "" {
			fmt.Fprintf(&s, "    SRT : %s\n", c.PublishSRT)
		}
		if c.PublishRTMP != "" {
			fmt.Fprintf(&s, "    RTMP: %s\n", c.PublishRTMP)
		}
	}
	if b.AutoHost {
		if !b.HostFound {
			fmt.Fprintf(&s, "  %s\n", render("banner.host_missing"))
		}
		fmt.Fprintf(&s, "  %s\n", render("banner.outdoor"))
	}
	fmt.Fprintf(&s, "%s\n", render("banner.obs"))
	for _, c := range b.Cameras {
		fmt.Fprintf(&s, "  %s\n    %s\n", c.label(lang), c.OBSRead)
	}
	fmt.Fprintf(&s, "%s\n", render("banner.admin"))
	fmt.Fprintf(&s, "  %s %s\n", b.AdminURL, render("banner.admin_user"))
	if b.AdminNote != "" {
		fmt.Fprintf(&s, "  %s\n", b.AdminNote)
	}
	if b.AdminPassword != "" {
		s.WriteString(adminPasswordLine(b.AdminPassword, lang))
	}
	return s.String()
}

// label はカメラの表示名と id（URL の path）。表示名は重複してよいので、id を並べて区別する。
func (c camURLs) label(languages ...i18n.Lang) string {
	lang := i18n.JA
	if len(languages) > 0 && languages[0] == i18n.EN {
		lang = i18n.EN
	}
	return i18n.Render(lang, i18n.New("banner.camera_label", "name", c.Name, "id", c.ID))
}

// adminPasswordLine は、config.yaml を生成したときに出す管理 API のパスワードの行。
func adminPasswordLine(password string, languages ...i18n.Lang) string {
	lang := i18n.JA
	if len(languages) > 0 && languages[0] == i18n.EN {
		lang = i18n.EN
	}
	return fmt.Sprintf("  %s\n", i18n.Render(lang, i18n.New("banner.password", "password", password)))
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
func adminBannerURL(port int, lan bool, lanHost string, found bool, languages ...i18n.Lang) (url, note string) {
	lang := i18n.JA
	if len(languages) > 0 && languages[0] == i18n.EN {
		lang = i18n.EN
	}
	if !lan {
		return fmt.Sprintf("http://127.0.0.1:%d", port), ""
	}
	if found {
		return fmt.Sprintf("http://%s:%d", lanHost, port), i18n.Render(lang, i18n.New("banner.lan_note"))
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), i18n.Render(lang, i18n.New("banner.lan_missing", "port", fmt.Sprint(port)))
}
