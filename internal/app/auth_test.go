package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/config"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"github.com/teru358/irl-live-keeper/internal/web"
)

func TestRestartセッションと合言葉の寿命(t *testing.T) {
	for _, change := range []bool{false, true} {
		t.Run(map[bool]string{false: "同じパスワード", true: "パスワード変更"}[change], func(t *testing.T) {
			f := newFailureFixture(t)
			original := f.a.deps.loadConfig
			loads := 0
			f.a.deps.loadConfig = func(base string) (cfgResult *config.Config, generated bool, raw []byte, err error) {
				cfgResult, generated, raw, err = original(base)
				loads++
				if change && loads > 1 {
					copy := *cfgResult
					copy.Admin.Password = "changed-password"
					cfgResult = &copy
				}
				return
			}
			second := &failureHTTP{generationHTTP: generationHTTP{make(chan struct{}), make(chan struct{}), f.r}, serveError: http.ErrServerClosed}
			handlers := make(chan http.Handler, 2)
			n := 0
			f.a.deps.newHTTP = func(c coreRunner, p string, l i18n.Lang) servingHTTP {
				handlers <- web.NewWithAuth(c, p, f.a.adminPort, f.a.auth, l)
				n++
				if n == 1 {
					return f.h
				}
				return second
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan int, 1)
			go func() { done <- f.a.run(ctx) }()
			<-f.h.started
			h := <-handlers
			waitApplyState(t, f.a, ApplyApplied)
			for _, p := range f.parts {
				<-p.started
			}
			token, err := f.a.auth.NewOnce()
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/auth/once?t="+token, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			c := w.Result().Cookies()[0]
			pending, _ := f.a.auth.NewOnce()
			if err := f.a.Restart(); err != nil {
				t.Fatal(err)
			}
			<-second.started
			h = <-handlers
			waitApplyState(t, f.a, ApplyApplied)
			r = httptest.NewRequest("GET", "/", nil)
			r.AddCookie(c)
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
			// ログインの画面は JS で描く（S2-63）ので、サーバーが入れる初めの状態の login で見る
			if strings.Contains(w.Body.String(), `"login":true`) != change {
				t.Fatal("再起動後のセッションが不正")
			}
			r = httptest.NewRequest("GET", "/auth/once?t="+pending, nil)
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if len(w.Result().Cookies()) != 1 {
				t.Fatal("合言葉が再起動で消えた")
			}
			cancel()
			if code := <-done; code != 0 {
				t.Fatal(code)
			}
		})
	}
}
