package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/sources"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets/*
var assets embed.FS

func serveAsset(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if path == "/assets/app/html_rules.json" {
		w.Header().Set("Content-Type", "application/json")
		w.Write(sources.HTMLRules)
		return true
	}
	if path == "/favicon.ico" {
		path = "/assets/favicon-32.png"
	}
	if !strings.HasPrefix(path, "/assets/") {
		return false
	}
	sub, _ := fs.Sub(assets, "assets")
	assetRequest := r.Clone(r.Context())
	assetRequest.URL.Path = strings.TrimPrefix(path, "/assets/")
	http.FileServer(http.FS(sub)).ServeHTTP(w, assetRequest)
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
	fmt.Fprint(w, strings.Replace(html, "BOOTSTRAP", string(data), 1))
}
