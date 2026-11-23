package app

import (
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"net/url"
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
	AdminPassword   string
	SensitiveOutput bool
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
	if b.SensitiveOutput {
		fmt.Fprintf(&s, "%s\n", render("banner.sensitive_note"))
	}
	return s.String()
}

func maskCameraURLs(cameras []camURLs) []camURLs {
	masked := append([]camURLs(nil), cameras...)
	for i := range masked {
		masked[i].PublishSRT = maskURLPassword(masked[i].PublishSRT)
		masked[i].PublishRTMP = maskURLPassword(masked[i].PublishRTMP)
		masked[i].OBSRead = maskURLPassword(masked[i].OBSRead)
	}
	return masked
}

func maskURLPassword(raw string) string {
	// 並びと書き方を変えないよう、URL を組み立て直さずに、パスワードの部分だけを文字の "***" に置き換える
	// （組み立て直すと、query の順が変わり、"***" も %2A%2A%2A になって読みにくい）
	out := raw
	if i := strings.Index(out, "://"); i >= 0 {
		rest := out[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			if slash := strings.IndexAny(rest, "/?"); slash < 0 || at < slash {
				if colon := strings.Index(rest[:at], ":"); colon >= 0 {
					out = out[:i+3] + rest[:colon+1] + "***" + rest[at:]
				}
			}
		}
	}
	q := strings.Index(out, "?")
	if q < 0 {
		return out
	}
	parts := strings.Split(out[q+1:], "&")
	for j, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch key {
		case "pass":
			parts[j] = key + "=***"
		case "streamid":
			id, err := url.QueryUnescape(value)
			if err != nil {
				continue
			}
			fields := strings.Split(id, ":")
			if len(fields) >= 4 {
				const mark = "\x00"
				fields[len(fields)-1] = mark
				escaped := url.QueryEscape(strings.Join(fields, ":"))
				parts[j] = key + "=" + strings.Replace(escaped, url.QueryEscape(mark), "***", 1)
			}
		}
	}
	return out[:q+1] + strings.Join(parts, "&")
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
