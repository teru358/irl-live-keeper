package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseとLoadは同じ結果を返す(t *testing.T) {
	for _, data := range []string{strings.ReplaceAll(defaultTemplate, `"*"`, `"validsecret"`), "not: [valid", "admin:\n  port: 0", "unknown: true", ""} {
		t.Run(data[:min(len(data), 16)], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			a, ae := Load(path)
			b, be := Parse([]byte(data))
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("設定が異なる: %v / %v", a, b)
			}
			msg := func(e error) string {
				if e == nil {
					return ""
				}
				return e.Error()
			}
			if msg(ae) != msg(be) {
				t.Fatalf("エラーが異なる: %v / %v", ae, be)
			}
		})
	}
}
