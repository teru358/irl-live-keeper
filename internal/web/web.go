package web

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"net/http"
	"slices"
	"time"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

const commandTimeout = 5 * time.Second

const (
	username        = "admin"
	headerName      = "X-IRL-Live-Keeper"
	challenge       = `Basic realm="irl-live-keeper"`
	jsonContentType = "application/json; charset=utf-8"
)

// Backend は API が参照する状態と操作を提供する。
type Backend interface {
	Snapshot() core.Snapshot
	Command(ctx context.Context, kind switcher.CommandKind) error
}

// New は管理用の JSON API を返す。
func New(b Backend, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := i18n.Select(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
		kind, method, known := route(r.URL.Path)
		if !known {
			writeError(w, http.StatusNotFound, lang, i18n.New("api.not_found"))
			return
		}

		if !authorized(r, password) {
			w.Header().Set("WWW-Authenticate", challenge)
			writeError(w, http.StatusUnauthorized, lang, i18n.New("api.unauthorized"))
			return
		}

		if r.Method != method {
			w.Header().Set("Allow", method)
			writeError(w, http.StatusMethodNotAllowed, lang, i18n.New("api.method"))
			return
		}

		if method == http.MethodPost && r.Header.Get(headerName) != "1" {
			writeError(w, http.StatusForbidden, lang, i18n.New("api.forbidden"))
			return
		}

		if r.URL.Path == "/api/status" {
			snapshot := b.Snapshot()
			if snapshot.OBS.Warnings == nil {
				snapshot.OBS.Warnings = []i18n.Msg{}
			}
			snapshot.Cameras = slices.Clone(snapshot.Cameras) // 共有の列を書き換えない
			for i := range snapshot.Cameras {
				if snapshot.Cameras[i].Sources == nil {
					snapshot.Cameras[i].Sources = []string{}
				}
			}
			writeJSON(w, http.StatusOK, snapshot.Render(i18n.Select(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), commandTimeout)
		defer cancel()
		if err := b.Command(ctx, kind); err != nil {
			code := http.StatusInternalServerError
			message := i18n.New("api.internal")
			var invalid switcher.ErrInvalidState
			var disconnected switcher.ErrNotConnected
			switch {
			case errors.As(err, &invalid):
				code, message = http.StatusConflict, invalid.Message()
			case errors.As(err, &disconnected):
				code, message = http.StatusServiceUnavailable, i18n.New("api.disconnected")
			case errors.Is(err, core.ErrStopped), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
				code = http.StatusServiceUnavailable
				switch {
				case errors.Is(err, core.ErrStopped):
					message = i18n.New("api.stopped")
				case errors.Is(err, context.DeadlineExceeded):
					message = i18n.New("api.deadline")
				default:
					message = i18n.New("api.canceled")
				}
			}
			writeError(w, code, lang, message)
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

func route(path string) (switcher.CommandKind, string, bool) {
	switch path {
	case "/api/status":
		return 0, http.MethodGet, true
	case "/api/stream/start":
		return switcher.CmdStreamStart, http.MethodPost, true
	case "/api/stream/stop":
		return switcher.CmdStreamStop, http.MethodPost, true
	case "/api/standby/on":
		return switcher.CmdStandbyOn, http.MethodPost, true
	case "/api/standby/off":
		return switcher.CmdStandbyOff, http.MethodPost, true
	case "/api/cameras/show-all":
		// カメラのソースをすべて表示に戻す（0b）。OBS に未接続なら 503、終了中なら 409。
		return switcher.CmdShowAllCameras, http.MethodPost, true
	default:
		return 0, "", false
	}
}

func authorized(r *http.Request, password string) bool {
	if password == "" {
		return false
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		user, pass = "", ""
	}
	// 長さで比較の時間が変わらないよう、どちらも SHA-256 にしてから比べる。
	userOK := equalHashed(user, username)
	passOK := equalHashed(pass, password)
	return userOK&passOK == 1
}

func equalHashed(a, b string) int {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:])
}

func writeError(w http.ResponseWriter, status int, lang i18n.Lang, message i18n.Msg) {
	writeJSON(w, status, map[string]string{"error": i18n.Render(lang, message), "code": message.Code})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
