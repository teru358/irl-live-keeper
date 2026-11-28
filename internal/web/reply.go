package web

import (
	"net/http"

	"github.com/teru358/irl-live-keeper/internal/i18n"
)

// reply is a response decided while a lock is held and written after the lock is
// released, so a slow reader never holds a store or App.mu.
type reply struct {
	status int
	value  any
}

func errorReply(status int, lang i18n.Lang, message i18n.Msg) reply {
	return reply{status, map[string]string{"error": i18n.Render(lang, message), "code": message.Code}}
}

func (r reply) write(w http.ResponseWriter) {
	if r.status == 0 {
		writeJSON(w, 500, map[string]string{"code": "api.internal"})
		return
	}
	writeJSON(w, r.status, r.value)
}
