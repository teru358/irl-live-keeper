package web

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/i18n"
)

func validPhoneHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func servePhoneLink(w http.ResponseWriter, r *http.Request, b Backend, a *Auth, port int, lang i18n.Lang) {
	w.Header().Set("Cache-Control", "no-store")
	fail := func(status int, code string) { writeError(w, status, lang, i18n.New(code)) }
	if sourceKind(r) != "local" || r.Header.Get(headerName) != "1" {
		fail(403, "api.forbidden")
		return
	}
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		fail(405, "api.method")
		return
	}
	provider, ok := b.(SettingsBackend)
	if !ok {
		fail(503, "api.stopped")
		return
	}
	enabled := false
	err := provider.SettingsTransaction(func(s *SettingsStore) {
		if s.Current != nil {
			enabled = s.Current.Admin.LAN
		}
	})
	if err != nil {
		fail(503, "api.stopped")
		return
	}
	if !enabled {
		fail(409, "phone.lan_disabled")
		return
	}
	var body struct {
		Address string `json:"address"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || !validPhoneHost(body.Address) {
		fail(400, "phone.invalid_host")
		return
	}
	token, err := a.NewOnce()
	if err != nil {
		fail(500, "api.internal")
		return
	}
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(body.Address, strconv.Itoa(port)), Path: "/auth/once", RawQuery: url.Values{"t": {token}}.Encode()}
	requestLogger(r).Info("phone link created", "source", sourceKind(r))
	writeJSON(w, 200, map[string]string{"url": u.String()})
}
