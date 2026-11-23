// Package i18n は本体の表示文と言語の選択をまとめる。
package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Lang string

const (
	JA Lang = "ja"
	EN Lang = "en"
)

// Msg は文の種類と差し込む名前つきの値。作成後は変更しない。
type Msg struct {
	Code   string
	Values map[string]any
}

// New は名前と値の組から文を作る。
func New(code string, pairs ...string) Msg {
	m := Msg{Code: code, Values: map[string]any{}}
	for i := 0; i+1 < len(pairs); i += 2 {
		m.Values[pairs[i]] = pairs[i+1]
	}
	return m
}

//go:embed locales/*.json
var files embed.FS
var catalogs = func() map[Lang]map[string]string {
	out := map[Lang]map[string]string{}
	for _, lang := range []Lang{JA, EN} {
		b, err := files.ReadFile("locales/" + string(lang) + ".json")
		if err != nil {
			panic(err)
		}
		var c struct {
			Messages map[string]string `json:"messages"`
		}
		if err = json.Unmarshal(b, &c); err != nil {
			panic(err)
		}
		out[lang] = c.Messages
	}
	return out
}()
var placeholder = regexp.MustCompile(`\{([a-z_]+)\}`)

func Render(lang Lang, m Msg) string {
	s, ok := catalogs[lang][m.Code]
	if !ok {
		s, ok = catalogs[EN][m.Code]
	}
	if !ok {
		return m.Code
	}
	return placeholder.ReplaceAllStringFunc(s, func(p string) string {
		if v, ok := m.Values[p[1:len(p)-1]]; ok {
			if child, ok := v.(Msg); ok {
				return Render(lang, child)
			}
			return fmt.Sprint(v)
		}
		return p
	})
}

// Select は明示指定、見出しの順に言語を選ぶ。同じ重みは記載順を保つ。
func Select(lang, accept string, defaultLang ...Lang) Lang {
	if lang == "ja" || lang == "en" {
		return Lang(lang)
	}
	best := EN
	if len(defaultLang) > 0 && (defaultLang[0] == JA || defaultLang[0] == EN) {
		best = defaultLang[0]
	}
	weight := 0.0
	for _, part := range strings.Split(accept, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		base := strings.Split(tag, "-")[0]
		if base != "ja" && base != "en" {
			continue
		}
		q := 1.0
		for _, f := range fields[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(f), "=")
			if ok && k == "q" {
				var err error
				q, err = strconv.ParseFloat(v, 64)
				if err != nil || q < 0 || q > 1 {
					q = 0
				}
			}
		}
		if q > weight {
			best, weight = Lang(base), q
		}
	}
	return best
}

// Resolve は config の設定と OS の言語から本体の言語を決める。
func Resolve(setting string, osLang func() Lang) Lang {
	switch setting {
	case "ja":
		return JA
	case "en":
		return EN
	default:
		if osLang != nil && osLang() == JA {
			return JA
		}
		return EN
	}
}

// Text は外部由来の理由など、翻訳しない値を文として持つ。
func Text(s string) Msg {
	if s == "" {
		return Msg{}
	}
	return New("external.detail", "detail", s)
}

func Strings(lang Lang, ms []Msg) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = Render(lang, m)
	}
	return out
}

// Error は日本語の既存のエラー文字列と表示文を結ぶ。
type Error struct {
	Message Msg
	Cause   error
}

// Error はログとエラーの連鎖のための文で、英語にする（ログは英語だけ。spec v0.11 第14節）。画面には Message を組み立てる
func (e Error) Error() string { return Render(EN, e.Message) }
func (e Error) Unwrap() error { return e.Cause }

// Reason は本体の理由なら文を取り出し、外部の理由なら原文を保つ。
func Reason(err error) Msg {
	var e Error
	if errors.As(err, &e) {
		return e.Message
	}
	return Text(err.Error())
}
