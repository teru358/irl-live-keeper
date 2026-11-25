package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets/*
var assets embed.FS

func serveAsset(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/assets/") {
		return false
	}
	sub, _ := fs.Sub(assets, "assets")
	http.StripPrefix("/assets/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
	return true
}
func servePage(w http.ResponseWriter, b Backend, login bool, setup ...bool) {
	language := "auto"
	if v, ok := b.(interface{ PageLanguage() string }); ok {
		language = v.PageLanguage()
	}
	data, _ := json.Marshal(map[string]any{"local": len(setup) > 1 && setup[1], "language": language, "login": login, "setup": len(setup) > 0 && setup[0], "catalogs": map[string]any{"ja": i18n.UI(i18n.JA), "en": i18n.UI(i18n.EN)}})
	page, _ := assets.ReadFile("assets/page.html")
	html := string(page)
	if !login {
		start := strings.Index(html, "<form")
		end := strings.Index(html, "</form>") + len("</form>")
		html = html[:start] + html[end:]
	}
	fmt.Fprint(w, strings.Replace(html, "BOOTSTRAP", string(data), 1))
}
