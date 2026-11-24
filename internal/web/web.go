package web

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/obs"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/teru358/irl-live-keeper/internal/core"
	"github.com/teru358/irl-live-keeper/internal/logging"
	"github.com/teru358/irl-live-keeper/internal/switcher"
)

type loggerKey struct{}

func requestLogger(r *http.Request) *slog.Logger {
	if l, ok := r.Context().Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

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
func apiHandler(b Backend, recent interface {
	Recent(int, slog.Level) []logging.Line
}, defaultLang ...i18n.Lang) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := i18n.Select(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"), defaultLang...)
		kind, method, known := route(r.URL.Path)
		if r.URL.Path == "/api/settings" && r.Method == http.MethodPut {
			method = http.MethodPut
		}
		if !known {
			writeError(w, http.StatusNotFound, lang, i18n.New("api.not_found"))
			return
		}

		if r.Method != method {
			w.Header().Set("Allow", method)
			writeError(w, http.StatusMethodNotAllowed, lang, i18n.New("api.method"))
			return
		}

		if (method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete) && r.Header.Get(headerName) != "1" {
			writeError(w, http.StatusForbidden, lang, i18n.New("api.forbidden"))
			return
		}

		if r.URL.Path == "/api/settings" || r.URL.Path == "/api/settings/preview" {
			serveSettings(w, r, b, lang)
			return
		}

		if r.URL.Path == "/api/obs/scenes" || r.URL.Path == "/api/obs/scene" {
			serveScenes(w, r, b, lang)
			return
		}
		if r.URL.Path == "/api/obs/inputs" {
			serveInputs(w, b, lang)
			return
		}
		if r.URL.Path == "/api/sender" {
			w.Header().Set("Cache-Control", "no-store")
			if provider, ok := b.(interface{ Sender() SenderSettings }); ok {
				writeJSON(w, 200, provider.Sender())
			} else {
				writeJSON(w, 503, map[string]string{"error": i18n.Render(lang, i18n.New("api.stopped"))})
			}
			return
		}
		if r.URL.Path == "/api/logs" {
			raw := r.URL.Query().Get("lines")
			n := 500
			if r.URL.Query().Has("lines") {
				parsed, err := strconv.Atoi(raw)
				if err != nil || parsed < 1 {
					writeError(w, 400, lang, i18n.New("api.invalid", "reason", "lines"))
					return
				}
				n = min(parsed, 2000)
			}
			minLevel := slog.LevelInfo
			switch strings.ToUpper(r.URL.Query().Get("level")) {
			case "", "INFO":
			case "WARN":
				minLevel = slog.LevelWarn
			case "ERROR":
				minLevel = slog.LevelError
			default:
				writeError(w, 400, lang, i18n.New("api.invalid", "reason", "level"))
				return
			}
			lines := []logging.Line{}
			if recent != nil {
				lines = recent.Recent(n, minLevel)
			}
			values := make([]map[string]any, 0, len(lines))
			for _, line := range lines {
				values = append(values, map[string]any{"t": line.At, "level": strings.ToUpper(line.Level.String()), "text": line.Text})
			}
			writeJSON(w, 200, values)
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
			writeJSON(w, http.StatusOK, statusValue(b, snapshot.Render(lang)))
			return
		}

		if r.URL.Path == "/api/restart" {
			if b, ok := b.(interface{ RestartHTTP() (int, error) }); ok {
				code, err := b.RestartHTTP()
				if err != nil {
					writeError(w, code, lang, i18n.New("api.internal"))
					return
				}
				writeJSON(w, code, map[string]bool{"ok": true})
				return
			}
			writeError(w, 503, lang, i18n.New("api.stopped"))
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
	case "/api/settings":
		return 0, http.MethodGet, true
	case "/api/settings/preview":
		return 0, http.MethodPost, true
	case "/api/sender", "/api/obs/scenes", "/api/obs/inputs", "/api/events", "/api/status", "/api/logs":
		return 0, http.MethodGet, true
	case "/api/obs/scene":
		return 0, http.MethodPost, true
	case "/api/restart":
		return 0, http.MethodPost, true
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

func serveInputs(w http.ResponseWriter, b Backend, lang i18n.Lang) {
	a, ok := b.(interface{ Inputs() ([]obs.Input, error) })
	if !ok {
		writeError(w, http.StatusServiceUnavailable, lang, i18n.New("api.disconnected"))
		return
	}
	list, err := a.Inputs()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, lang, i18n.New("api.disconnected"))
		return
	}
	writeJSON(w, http.StatusOK, list)
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

// statusValue は世代の表示情報を API と SSE に同じ形で加える。
func statusValue(b Backend, value any) any {
	raw, _ := json.Marshal(value)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	if extra, ok := b.(interface{ DisplayStatus() map[string]any }); ok {
		for k, v := range extra.DisplayStatus() {
			out[k] = v
		}
	}
	return out
}

func serveScenes(w http.ResponseWriter, r *http.Request, b Backend, lang i18n.Lang) {
	a, ok := b.(interface {
		Scenes() ([]obs.Scene, error)
		SelectScene(context.Context, string) error
	})
	if !ok {
		writeError(w, 503, lang, i18n.New("api.disconnected"))
		return
	}
	var err error
	if r.Method == http.MethodGet {
		var list []obs.Scene
		list, err = a.Scenes()
		if err == nil {
			writeJSON(w, 200, list)
			return
		}
	} else {
		var body struct {
			UUID string `json:"scene_uuid"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		if d.Decode(&body) != nil || body.UUID == "" {
			writeError(w, 400, lang, i18n.New("api.invalid", "reason", "scene_uuid"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), commandTimeout)
		defer cancel()
		name := ""
		if scenes, e := a.Scenes(); e == nil {
			for _, scene := range scenes {
				if scene.UUID == body.UUID {
					name = scene.Name
					break
				}
			}
		}
		requestLogger(r).Info("scene selected from admin page", "scene", name, "scene_uuid", body.UUID, "source", sourceKind(r))
		err = a.SelectScene(ctx, body.UUID)
		if err == nil {
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	code, msg := 503, "api.disconnected"
	switch {
	case errors.Is(err, obs.ErrAuxRejected):
		code, msg = 502, "api.scene_failed"
	case errors.Is(err, core.ErrSceneMissing):
		code, msg = 404, "api.scene_missing"
	case errors.Is(err, obs.ErrAuxUncertain), errors.Is(err, obs.ErrAuxNotSent), errors.Is(err, obs.ErrAuxUnavailable), errors.Is(err, obs.ErrAuxClosed), errors.Is(err, obs.ErrAuxTimeout), errors.Is(err, obs.ErrAuxMismatch), errors.Is(err, core.ErrStopped), errors.Is(err, context.DeadlineExceeded):
		code, msg = 503, "api.disconnected"
	}
	writeError(w, code, lang, i18n.New(msg))
}
