package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/i18n"
)

const sessionLife = 7 * 24 * time.Hour

// Auth はプロセスの寿命で共有する認証の記憶。時計はテストで差し替えられる。
type Auth struct {
	mu           sync.Mutex
	now          func() time.Time
	sessions     map[string]time.Time
	once         map[string]time.Time
	attempts     map[string]passwordAttempt
	attemptOrder uint64
}

func NewAuth(now func() time.Time) *Auth {
	if now == nil {
		now = time.Now
	}
	return &Auth{now: now, sessions: map[string]time.Time{}, once: map[string]time.Time{}, attempts: map[string]passwordAttempt{}}
}
func randomToken() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}

// NewOnce は128ビットの合言葉を作り、60秒だけ記憶する。
func (a *Auth) NewOnce() (string, error) {
	t, err := randomToken()
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for k, v := range a.once {
		if !now.Before(v) {
			delete(a.once, k)
		}
	}
	a.once[t] = now.Add(time.Minute)
	return t, nil
}

// ClearSessions はパスワード変更時にすべてのセッションを失効させる。
func (a *Auth) ClearSessions() { a.mu.Lock(); defer a.mu.Unlock(); clear(a.sessions) }
func (a *Auth) consume(t string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	until, ok := a.once[t]
	delete(a.once, t)
	return ok && a.now().Before(until)
}
func (a *Auth) session(t string) (time.Time, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	until, ok := a.sessions[t]
	if !ok || !now.Before(until) {
		delete(a.sessions, t)
		return time.Time{}, false
	}
	until = now.Add(sessionLife)
	a.sessions[t] = until
	return until, true
}
func cookie(name, value string, until time.Time) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: until}
}
func (a *Auth) issue(w http.ResponseWriter, name string) error {
	t, err := randomToken()
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for k, v := range a.sessions {
		if !now.Before(v) {
			delete(a.sessions, k)
		}
	}
	until := now.Add(sessionLife)
	a.sessions[t] = until
	http.SetCookie(w, cookie(name, t, until))
	return nil
}

const maxPasswordIPs = 4096
const passwordWait = 30 * time.Second

type passwordAttempt struct {
	failures int
	until    time.Time
	order    uint64
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// passwordWaiting は照合前に待ちを確かめる。呼び出し側が mu を持つ。
func (a *Auth) passwordWaiting(r *http.Request) time.Duration {
	now := a.now()
	for ip, entry := range a.attempts {
		if !entry.until.IsZero() && !now.Before(entry.until) {
			delete(a.attempts, ip)
		}
	}
	return a.attempts[remoteIP(r)].until.Sub(now)
}

// checkPassword は待ちの判定から照合結果の記録までを直列にする。
func (a *Auth) checkPassword(r *http.Request, check func() bool) (bool, time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if wait := a.passwordWaiting(r); wait > 0 {
		return false, wait
	}
	if check == nil {
		return false, 0
	}
	ip := remoteIP(r)
	if check() {
		delete(a.attempts, ip)
		return true, 0
	}
	entry, exists := a.attempts[ip]
	if !exists && len(a.attempts) >= maxPasswordIPs {
		var oldest string
		var order uint64 = ^uint64(0)
		for key, value := range a.attempts {
			if value.order < order {
				oldest, order = key, value.order
			}
		}
		delete(a.attempts, oldest)
	}
	a.attemptOrder++
	entry.order = a.attemptOrder
	entry.failures++
	if entry.failures == 5 {
		entry.until = a.now().Add(passwordWait)
	}
	a.attempts[ip] = entry
	return false, 0
}

func New(b Backend, password string, langs ...i18n.Lang) http.Handler {
	return NewWithAuth(b, password, 8787, NewAuth(nil), langs...)
}

// NewWithAuth は世代ごとのルーターに、App が持つ認証の記憶を渡す。
func NewWithAuth(b Backend, password string, port int, a *Auth, langs ...i18n.Lang) http.Handler {
	api := apiHandler(b, langs...)
	name := "irl_session_" + strconv.Itoa(port)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 合言葉を残さないよう、要求のログにはパスだけを書く。
		slog.Debug("Admin request", "method", r.Method, "path", r.URL.Path)
		lang := i18n.Select(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"), langs...)
		fail := func(status int, code string) {
			if status == 401 && len(r.Header.Values("Authorization")) > 0 {
				w.Header().Set("WWW-Authenticate", challenge)
			}
			writeError(w, status, lang, i18n.New(code))
		}
		method := func(want string) bool {
			if r.Method != want {
				w.Header().Set("Allow", want)
				fail(405, "api.method")
				return false
			}
			return true
		}
		waiting := func(wait time.Duration) bool {
			if wait > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int((wait+time.Second-1)/time.Second)))
				fail(429, "api.too_many_attempts")
				return true
			}
			return false
		}
		issue := func() bool {
			if a.issue(w, name) != nil {
				fail(500, "api.internal")
				return false
			}
			return true
		}
		switch r.URL.Path {
		case "/assets/login.js":
			if !method("GET") {
				return
			}
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			io.WriteString(w, loginJS)
			return
		case "/auth/once":
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			if !method("GET") {
				return
			}
			if a.consume(r.URL.Query().Get("t")) {
				if !issue() {
					return
				}
				http.Redirect(w, r, "/", 303)
			} else {
				http.Redirect(w, r, "/?login", 303)
			}
			return
		case "/api/login":
			if !method("POST") {
				return
			}
			if r.Header.Get(headerName) != "1" {
				fail(403, "api.forbidden")
				return
			}
			_, wait := a.checkPassword(r, nil)
			if waiting(wait) {
				return
			}
			var body struct {
				Password string `json:"password"`
			}
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
			if d.Decode(&body) != nil {
				fail(400, "api.invalid")
				return
			}
			var extra any
			if d.Decode(&extra) != io.EOF {
				fail(400, "api.invalid")
				return
			}
			ok, wait := a.checkPassword(r, func() bool { return password != "" && equalHashed(body.Password, password) == 1 })
			if waiting(wait) {
				return
			}
			if !ok {
				fail(401, "api.unauthorized")
				return
			}
			if issue() {
				writeJSON(w, 200, map[string]bool{"ok": true})
			}
			return
		}
		_, _, known := route(r.URL.Path)
		if !known && r.URL.Path != "/" && r.URL.Path != "/api/logout" {
			fail(404, "api.not_found")
			return
		}
		valid := false
		c, err := r.Cookie(name)
		if err == nil {
			if until, ok := a.session(c.Value); ok {
				valid = true
				http.SetCookie(w, cookie(name, c.Value, until))
			}
		}
		if r.URL.Path == "/" {
			if !method("GET") {
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			if !valid || r.URL.Query().Has("login") {
				io.WriteString(w, loginHTML)
			} else {
				io.WriteString(w, "<!doctype html><title>IRL Live Keeper</title>")
			}
			return
		}
		if !valid {
			ok := false
			if len(r.Header.Values("Authorization")) > 0 {
				var wait time.Duration
				ok, wait = a.checkPassword(r, func() bool { return authorized(r, password) })
				if waiting(wait) {
					return
				}
			}
			if !ok {
				fail(401, "api.unauthorized")
				return
			}
		}
		if r.URL.Path == "/api/logout" {
			if !method("POST") {
				return
			}
			if r.Header.Get(headerName) != "1" {
				fail(403, "api.forbidden")
				return
			}
			a.mu.Lock()
			if c != nil {
				delete(a.sessions, c.Value)
			}
			a.mu.Unlock()
			expired := cookie(name, "", time.Unix(1, 0))
			expired.MaxAge = -1
			http.SetCookie(w, expired)
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		api.ServeHTTP(w, r)
	})
}

const loginHTML = `<!doctype html><meta charset="utf-8"><title>Login</title><form id="login"><input name="password" type="password" autocomplete="current-password" required><button>Login</button></form><p id="error"></p><script src="/assets/login.js" defer></script>`
const loginJS = `document.querySelector('#login').addEventListener('submit',async e=>{e.preventDefault();try{const r=await fetch('/api/login'+location.search,{method:'POST',headers:{'Content-Type':'application/json','X-IRL-Live-Keeper':'1'},body:JSON.stringify({password:e.target.password.value})});if(r.ok)location.replace('/');else document.querySelector('#error').textContent=(await r.json()).error;}catch(e){document.querySelector('#error').textContent=String(e);}});`
