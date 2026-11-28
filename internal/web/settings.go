package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/settings"
	"gopkg.in/yaml.v3"
)

// SettingsStore is used only during SettingsTransaction. Implementations serialize
// the entire callback with password setup and generation state changes.
type SettingsStore struct {
	Raw        []byte
	Current    *config.Config
	Applying   bool
	Owned      []settings.ListeningPort
	CheckPorts func(*config.Config, *config.Config, []settings.ListeningPort) []settings.PortFailure
	Backup     func() error // S2-29: preserve the running generation before writing.
	Write      func([]byte) error
	MarkHash   func([]byte)
	StartApply func()
	ApplySaved func(old, next *config.Config, summary settings.ChangeSummary, updateOBSInput bool)
}
type SettingsBackend interface {
	SettingsTransaction(func(*SettingsStore)) error
}

type settingsRequest struct {
	Page           string         `json:"page"`
	Values         map[string]any `json:"values"`
	Hash           string         `json:"hash"`
	UpdateOBSInput bool           `json:"update_obs_input"`
	Camera         string         `json:"camera"`
	Target         string         `json:"target"`
}

func serveSettings(w http.ResponseWriter, r *http.Request, b Backend, lang i18n.Lang) {
	w.Header().Set("Cache-Control", "no-store")
	provider, ok := b.(SettingsBackend)
	if !ok {
		writeError(w, 503, lang, i18n.New("api.stopped"))
		return
	}
	local := sourceKind(r) == "local"
	// The body is read before App.mu is taken, so a slow sender cannot stall other
	// saves. Inside the transaction we only decide; the reply is written after.
	var body []byte
	var bodyErr error
	if r.Method != http.MethodGet {
		body, bodyErr = io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	}
	var start func()
	var res reply
	err := provider.SettingsTransaction(func(s *SettingsStore) {
		fail := func(code int, key string) {
			if r.Method != http.MethodGet {
				requestLogger(r).Warn("settings operation failed", "code", key, "source", sourceKind(r))
			}
			res = errorReply(code, lang, settingsMessage(key))
		}
		if r.Method == http.MethodGet {
			cfg, err := config.Parse(s.Raw)
			if err != nil {
				fail(500, "api.internal")
				return
			}
			raw, _ := yaml.Marshal(cfg)
			values := map[string]any{}
			_ = yaml.Unmarshal(raw, &values)
			res = reply{200, map[string]any{"values": values, "local": local, "apply_modes": settings.ConfigFieldModes(), "field_locations": settings.ConfigFieldLocations(), "page_hashes": map[string]string{"cameras": settings.PageHash(cfg, "cameras"), "settings": settings.PageHash(cfg, "settings")}}}
			return
		}
		save := r.Method == http.MethodPut
		regenerate := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/regenerate")
		if (save || regenerate) && s.Applying {
			fail(409, "settings.applying")
			return
		}
		var req settingsRequest
		d := json.NewDecoder(bytes.NewReader(body))
		d.DisallowUnknownFields()
		if bodyErr != nil || d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF {
			fail(400, "settings.invalid")
			return
		}
		if !regenerate {
			if (req.Page != "cameras" && req.Page != "settings") || (req.Values != nil && !settings.PageOwnsValues(req.Page, req.Values)) {
				fail(400, "settings.invalid")
				return
			}
			current, err := config.Parse(s.Raw)
			if err != nil {
				fail(500, "api.internal")
				return
			}
			if req.Hash != settings.PageHash(current, req.Page) {
				fail(409, "settings.hash_conflict")
				return
			}
		}
		old, err := config.Parse(s.Raw)
		if err != nil {
			fail(500, "api.internal")
			return
		}
		// Decode without validating first so the local-only restriction precedes validation.
		raw, _ := yaml.Marshal(old)
		var next config.Config
		_ = yaml.Unmarshal(raw, &next)
		var decodeErr error
		if regenerate {
			var target *string
			switch req.Target {
			case "admin":
				if old.Admin.Password == "" {
					fail(409, "settings.admin_password_unset")
					return
				}
				target = &next.Admin.Password
			case "publish":
				for i := range next.Cameras {
					if next.Cameras[i].ID == req.Camera {
						target = &next.Cameras[i].PublishPass
						break
					}
				}
				if target == nil {
					fail(422, "settings.invalid")
					return
				}
			case "read":
				target = &next.Stream.ReadPass
			default:
				fail(422, "settings.invalid")
				return
			}
			*target, err = config.GeneratePassword()
			if err != nil {
				fail(500, "api.internal")
				return
			}
		} else {
			patch, _ := yaml.Marshal(req.Values)
			decode := yaml.NewDecoder(bytes.NewReader(patch))
			decode.KnownFields(true)
			decodeErr = decode.Decode(&next)
		}
		if !regenerate {
			if err = next.PreserveCredentials(old, false, save); err != nil {
				fail(500, "api.internal")
				return
			}
		}
		next.SortCameras()
		summary := settings.ClassifyChanges(old, &next, local)
		if save && !local {
			for _, f := range summary.Fields {
				if f.LocalOnly {
					fail(403, "settings.local_only")
					return
				}
			}
		}
		fields := map[string]string{}
		validation := next
		validation.Cameras = append([]config.Camera(nil), next.Cameras...)
		if !save && !regenerate {
			for i := range validation.Cameras {
				if validation.Cameras[i].PublishPass == "" {
					validation.Cameras[i].PublishPass = "previewOnly"
				}
			}
		}
		if err = validation.Validate(); err != nil {
			var ve *config.ValidationError
			if errors.As(err, &ve) {
				fields = ve.RenderFields(lang)
			}
		}
		if decodeErr != nil || !regenerate && req.Values == nil {
			fields["values"] = i18n.Render(lang, settingsMessage("settings.invalid"))
		}
		if len(fields) > 0 {
			requestLogger(r).Warn("settings operation failed", "code", "settings.invalid", "source", sourceKind(r))
			res = reply{422, map[string]any{"error": i18n.Render(lang, settingsMessage("settings.invalid")), "fields": fields}}
			return
		}
		if !save && !regenerate {
			res = reply{200, summary}
			return
		}
		if len(summary.Fields) == 0 {
			res = reply{202, map[string]any{"ok": true, "apply": "none"}}
			return
		}
		check := s.CheckPorts
		if check == nil {
			check = settings.CheckPorts
		}
		for _, f := range check(&next, s.Current, s.Owned) {
			fields[f.Path] = i18n.Render(lang, f.Reason)
		}
		if len(fields) > 0 {
			requestLogger(r).Warn("settings operation failed", "code", "settings.invalid", "source", sourceKind(r))
			res = reply{422, map[string]any{"error": i18n.Render(lang, settingsMessage("settings.invalid")), "fields": fields}}
			return
		}
		changes := settingsChanges(&next, summary)
		out, err := settings.Rewrite(s.Raw, changes)
		if err != nil {
			var verification *settings.VerificationError
			if errors.As(err, &verification) {
				fail(500, "api.internal")
			} else {
				fail(422, "settings.invalid")
			}
			return
		}
		if s.Backup != nil {
			if err = s.Backup(); err != nil {
				fail(500, "api.internal")
				return
			}
		}
		if err = s.Write(out); err != nil {
			fail(500, "api.internal")
			return
		}
		if summary.Apply == settings.ApplyImmediate && s.MarkHash != nil {
			s.MarkHash(out)
		}
		if keeper, ok := b.(interface{ KeepSettingsSession(string) }); ok {
			if !regenerate || req.Target == "admin" {
				keeper.KeepSettingsSession(SettingsSession(r))
			}
		}
		paths := make([]string, 0, len(summary.Fields))
		for _, field := range summary.Fields {
			paths = append(paths, field.Path)
		}
		requestLogger(r).Info("settings saved", "paths", paths, "apply", summary.Apply, "source", sourceKind(r))
		if regenerate {
			attrs := []any{"target", req.Target}
			if req.Target == "publish" {
				attrs = append(attrs, "camera", req.Camera)
			}
			requestLogger(r).Info("password regenerated", append(attrs, "source", sourceKind(r))...)
		}
		s.ApplySaved(old, &next, summary, req.UpdateOBSInput)
		response := map[string]any{"ok": true, "apply": summary.Apply}
		if old.Admin.Port != next.Admin.Port || old.Admin.LAN != next.Admin.LAN {
			response["admin_url"] = nil
			if !summary.AdminLANWillClose {
				host := r.Host
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				u := url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(next.Admin.Port))}
				response["admin_url"] = u.String()
			}
		}
		res = reply{202, response}
		start = s.StartApply
	})
	if err != nil {
		res = errorReply(500, lang, i18n.New("api.internal"))
	}
	res.write(w)
	if start != nil {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		start()
	}
}

func settingsMessage(key string) i18n.Msg {
	switch key {
	case "settings.applying":
		return i18n.New("settings.applying")
	case "settings.hash_conflict":
		return i18n.New("settings.hash_conflict")
	case "settings.invalid":
		return i18n.New("settings.invalid")
	case "settings.not_implemented":
		return i18n.New("settings.invalid")
	default:
		return i18n.New(key)
	}
}

// settingsChanges uses only classified differences; values come from the validated config.
func settingsChanges(next *config.Config, summary settings.ChangeSummary) []settings.Change {
	var root yaml.Node
	_ = root.Encode(next)
	var changes []settings.Change
	cameraChanged := false
	for _, field := range summary.Fields {
		if strings.HasPrefix(field.Path, "cameras[") || field.Path == "cameras" {
			cameraChanged = true
			continue
		}
		n := &root
		for _, part := range strings.Split(field.Path, ".") {
			key, index, indexed := strings.Cut(part, "[")
			for i := 0; i < len(n.Content); i += 2 {
				if n.Content[i].Value == key {
					n = n.Content[i+1]
					break
				}
			}
			if indexed {
				i, _ := strconv.Atoi(strings.TrimSuffix(index, "]"))
				n = n.Content[i]
			}
		}
		changes = append(changes, settings.Change{Path: field.Path, Value: n})
	}
	if cameraChanged {
		changes = append(changes, settings.Change{Path: "cameras", Value: next.Cameras})
	}
	return changes
}
