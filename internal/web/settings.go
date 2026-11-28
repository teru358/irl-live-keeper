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
	BackupRaw  []byte
	Write      func([]byte) error
	MarkHash   func([]byte)
	StartApply func()
	ApplySaved func(old, next *config.Config, summary settings.ChangeSummary, updateOBSInput bool)
}
type SettingsBackend interface {
	SettingsTransaction(func(*SettingsStore)) error
}

type settingsRequest struct {
	Values         map[string]any `json:"values"`
	Hash           string         `json:"hash"`
	Revert         bool           `json:"revert"`
	UpdateOBSInput bool           `json:"update_obs_input"`
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
	var start func()
	err := provider.SettingsTransaction(func(s *SettingsStore) {
		fail := func(code int, key string) {
			if r.Method != http.MethodGet {
				requestLogger(r).Warn("settings operation failed", "code", key, "source", sourceKind(r))
			}
			writeError(w, code, lang, settingsMessage(key))
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
			writeJSON(w, 200, map[string]any{"values": values, "hash": settings.Hash(s.Raw), "local": local, "apply_modes": settings.ConfigFieldModes()})
			return
		}
		save := r.Method == http.MethodPut
		revert := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/revert")
		regenerate := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/regenerate")
		if (save || revert || regenerate) && s.Applying {
			fail(409, "settings.applying")
			return
		}
		var req settingsRequest
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		d.DisallowUnknownFields()
		if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF {
			fail(400, "settings.invalid")
			return
		}
		if (save || regenerate) && req.Hash != settings.Hash(s.Raw) {
			fail(409, "settings.hash_conflict")
			return
		}
		if req.Revert || revert {
			if len(s.BackupRaw) == 0 {
				fail(409, "settings.backup_missing")
				return
			}
			if _, err := config.Parse(s.BackupRaw); err != nil {
				fail(500, "api.internal")
				return
			}
			if req.Revert { // preview compares the backup against the current file
				old, err := config.Parse(s.Raw)
				if err != nil {
					fail(500, "api.internal")
					return
				}
				next, err := config.Parse(s.BackupRaw)
				if err != nil {
					fail(500, "api.internal")
					return
				}
				summary := settings.ClassifyChanges(old, next, local)
				if len(summary.Fields) == 0 {
					fail(409, "settings.no_revert_difference")
					return
				}
				writeJSON(w, 200, summary)
				return
			}
			backup, err := config.Parse(s.BackupRaw)
			if err != nil {
				fail(500, "api.internal")
				return
			}
			raw, _ := yaml.Marshal(backup)
			req.Values = map[string]any{}
			_ = yaml.Unmarshal(raw, &req.Values)
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
				target = &next.Stream.PublishPass
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
		next.SortCameras()
		summary := settings.ClassifyChanges(old, &next, local)
		if (save || revert) && !local {
			for _, f := range summary.Fields {
				if f.LocalOnly {
					fail(403, "settings.local_only")
					return
				}
			}
		}
		fields := map[string]string{}
		if err = next.Validate(); err != nil {
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
			writeJSON(w, 422, map[string]any{"error": i18n.Render(lang, settingsMessage("settings.invalid")), "fields": fields})
			return
		}
		if !save && !revert && !regenerate {
			writeJSON(w, 200, summary)
			return
		}
		if len(summary.Fields) == 0 {
			if revert {
				fail(409, "settings.no_revert_difference")
				return
			}
			writeJSON(w, 202, map[string]any{"ok": true, "apply": "none"})
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
			writeJSON(w, 422, map[string]any{"error": i18n.Render(lang, settingsMessage("settings.invalid")), "fields": fields})
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
		if !revert {
			requestLogger(r).Info("settings saved", "paths", paths, "apply", summary.Apply, "source", sourceKind(r))
		}
		if revert {
			requestLogger(r).Info("settings reverted", "source", sourceKind(r))
		}
		if regenerate {
			requestLogger(r).Info("password regenerated", "target", req.Target, "source", sourceKind(r))
		}
		s.ApplySaved(old, &next, summary, req.UpdateOBSInput)
		if revert {
			if marker, ok := b.(interface{ MarkRevert() }); ok {
				marker.MarkRevert()
			}
		}
		response := map[string]any{"ok": true, "apply": summary.Apply}
		if revert {
			response["kind"] = "revert"
		}
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
		writeJSON(w, 202, response)
		start = s.StartApply
	})
	if err != nil {
		writeError(w, 500, lang, i18n.New("api.internal"))
	}
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
	case "settings.backup_missing", "settings.no_revert_difference":
		return i18n.New(key)
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
