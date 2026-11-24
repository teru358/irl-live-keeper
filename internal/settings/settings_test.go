package settings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/config"
	"gopkg.in/yaml.v3"
)

const base = "admin:\n  password: abcdefgh\nstream:\n  publish_pass: abcdefgh\n  read_pass: abcdefgh\ncameras:\n  - id: main\n    name: メイン\n    on_bad: standby\n"

func TestGolden(t *testing.T) {
	cases := []struct {
		name, src string
		changes   []Change
	}{
		{"comment_short", "obs:\n  host: longvalue   # 維持\n", []Change{{"obs.host", "x"}}},
		{"comment_long", "obs:\n  host: x      # 維持\n", []Change{{"obs.host", "a_much_longer_value"}}},
		{"comment_tight", "obs:\n  host: x#data # 維持\n", []Change{{"obs.host", "long"}}},
		{"fullwidth_comment", "obs:\n  standby_scene: 日  # 維持\n", []Change{{"obs.standby_scene", "日本語"}}},
		{"double_quoted", "obs:\n  host: \"before\" # 維持\n", []Change{{"obs.host", "after"}}},
		{"plain_scalar", "obs:\n  host: before # 維持\n", []Change{{"obs.host", "after"}}},
		{"quote_required", "obs:\n  host: before # 維持\n", []Change{{"obs.host", "yes"}}},
		{"binary", "mediamtx:\n  binary:\n", []Change{{"mediamtx.binary", "bin"}}},
		{"sequence", "", []Change{{"cameras", []config.Camera{{ID: "one", Name: "一", OnBad: "hide"}, {ID: "two", Name: "二", OnBad: "standby"}}}}},
		{"addition", "# 前\nobs:\n  host: localhost # 保持\n\n", []Change{{"obs.port", 4456}, {"gui.close_action", "quit"}}},
		{"section", "obs: {host: localhost, password: 'secret'}\n\n# 別区分\nlog:\n  dir: logs  # 保持\n", []Change{{"obs.password", "changed"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(base + tc.src)
			got, e := Rewrite(src, tc.changes)
			if e != nil {
				t.Fatal("書き換え失敗")
			}
			path := filepath.Join("testdata", tc.name+".golden")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if os.WriteFile(path, got, 0600) != nil {
					t.Fatal("保存失敗")
				}
			}
			want, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("ゴールデン不一致（値は非表示）")
			}
		})
	}
}

func TestGoldenContent(t *testing.T) {
	for _, tc := range []struct{ file, contains, excludes string }{
		{"double_quoted.golden", `host: "after"`, ""},
		{"plain_scalar.golden", "host: after", `host: "after"`},
		{"quote_required.golden", `host: "yes"`, ""},
		{"sequence.golden", "on_bad: standby", "switch: null"},
		{"fullwidth_comment.golden", "standby_scene: 日本語 # 維持", ""},
	} {
		data, err := os.ReadFile(filepath.Join("testdata", tc.file))
		if err != nil {
			t.Fatal("ゴールデンを読めません")
		}
		if !strings.Contains(string(data), tc.contains) || (tc.excludes != "" && strings.Contains(string(data), tc.excludes)) {
			t.Fatalf("ゴールデンの内容が不正: %s", tc.file)
		}
	}
}

func TestValuesThatNeedQuotes(t *testing.T) {
	for _, value := range []string{"yes", "null", "123", "# comment", "a: b"} {
		t.Run(value, func(t *testing.T) {
			got, err := Rewrite([]byte(base+"obs:\n  host: before\n"), []Change{{"obs.host", value}})
			if err != nil {
				t.Fatal("値の変更失敗")
			}
			if !strings.Contains(string(got), "host: "+strconv.Quote(value)) {
				t.Fatalf("必要な引用符がない: %s", got)
			}
			cfg, err := config.Parse(got)
			if err != nil || cfg.OBS.Host != value {
				t.Fatal("読み直した文字列が一致しない")
			}
		})
	}
}
func TestColumn(t *testing.T) {
	var n yaml.Node
	if yaml.Unmarshal([]byte("日本語: 値\n"), &n) != nil {
		t.Fatal("解析失敗")
	}
	if n.Content[0].Content[1].Column != 6 {
		t.Fatal("Column は文字数である必要がある")
	}
}
func TestFallbackStyles(t *testing.T) {
	for _, value := range []string{"'before'", "|\n    before\n", ">\n    before\n", "\"before\n    after\"", "&value before", "*value"} {
		t.Run(value, func(t *testing.T) {
			prefix := ""
			if value == "*value" {
				prefix = "  host: &value localhost\n"
			}
			src := base + "obs:\n" + prefix + "  password: " + value + "\n"
			suffix := "\n# 外側\nlog:\n  dir: logs   # 保持\n"
			got, e := Rewrite([]byte(src+suffix), []Change{{"obs.password", "after"}})
			if e != nil {
				t.Fatal("特殊表記の変更失敗")
			}
			if !strings.HasSuffix(string(got), suffix) {
				t.Fatal("別区分が変更された")
			}
		})
	}
}
func TestSafety(t *testing.T) {
	src := []byte(base)
	cfg, e := config.Parse(src)
	if e != nil {
		t.Fatal("解析失敗")
	}
	if _, e = rewrite(src, []Change{{"admin.port", 8788}}, func([]byte) (*config.Config, error) { return cfg, nil }); !errors.As(e, new(*VerificationError)) {
		t.Fatal("不一致を検証エラーとして拒否しなかった")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if os.WriteFile(path, src, 0600) != nil {
		t.Fatal("準備失敗")
	}
	if e = writeFile(path, src, func(string, string) error { return errors.New("故障") }); e == nil {
		t.Fatal("故障を無視")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(src, got) {
		t.Fatal("元ファイルを変更")
	}
	if e = WriteFile(path, src); e != nil {
		t.Fatal(e)
	}
	if Hash(src) == Hash(append(src, '\n')) {
		t.Fatal("ハッシュ不変")
	}
	if _, e = Rewrite(src, []Change{{"admin.password", map[string]string{"secret": "sensitive"}}}); e == nil || strings.Contains(e.Error(), "sensitive") {
		t.Fatal("秘密の扱いが不正")
	}
}

// 初回生成の各末端値を個別に変更し、起動と同じ読み方で期待値と照合する。
func TestGeneratedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if _, e := config.Generate(path); e != nil {
		t.Fatal("生成失敗")
	}
	src, _ := os.ReadFile(path)
	src = bytes.Replace(src, []byte("  scene: \"\""), []byte("  scene: Live"), 1)
	var doc yaml.Node
	yaml.Unmarshal(src, &doc)
	var walk func(*yaml.Node, string)
	walk = func(n *yaml.Node, p string) {
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i < len(n.Content); i += 2 {
				q := n.Content[i].Value
				if p != "" {
					q = p + "." + q
				}
				walk(n.Content[i+1], q)
			}
		case yaml.SequenceNode:
			if len(n.Content) == 0 {
				return
			}
			for i, c := range n.Content {
				walk(c, p+"["+strconvI(i)+"]")
			}
		default:
			var value any
			n.Decode(&value)
			switch v := value.(type) {
			case bool:
				value = !v
			case int:
				value = v + 1
				if p == "switch.down_sec" {
					value = 1
				}
			case string:
				value = v + "x"
				switch p {
				case "language":
					value = "ja"
				case "gui.close_action":
					value = "quit"
				case "cameras[0].on_bad":
					value = "hide"
				case "obs.studio_mode_hide":
					value = "ignore"
				case "stream.obs_read_host", "stream.publish_host":
					value = "localhost"
				case "admin.password":
					value = "initial-password" // 初回の生成では空（S2-61）。検証は空か8文字以上
				}
			}
			t.Run(p, func(t *testing.T) {
				got, e := Rewrite(src, []Change{{p, value}})
				if e != nil {
					t.Fatal("項目の変更失敗")
				}
				cfg, e := config.Parse(got)
				if e != nil || cfg == nil {
					t.Fatal("読み直し失敗")
				}
				var wantDoc yaml.Node
				yaml.Unmarshal(src, &wantDoc)
				steps, _ := parsePath(p)
				target := wantDoc.Content[0]
				for _, s := range steps {
					target, _ = child(target, s, false)
				}
				target.Encode(value)
				b, _ := encode(&wantDoc)
				want, _ := config.Parse(b)
				if !reflect.DeepEqual(cfg, want) {
					t.Fatal("値の不一致")
				}
			})
		}
	}
	walk(doc.Content[0], "")
}
func strconvI(i int) string { return fmt.Sprint(i) }

func TestSequenceFields(t *testing.T) {
	for _, tc := range []struct {
		path  string
		value any
	}{
		{"obs.scene_fallbacks", []config.SceneFallback{{Scene: "Live", Fallback: "Other"}}},
		{"cameras[0].switch", map[string]int{"fail_kbps": 400}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if _, e := Rewrite([]byte(base), []Change{{tc.path, tc.value}}); e != nil {
				t.Fatal("シーケンス・省略項目の変更失敗")
			}
		})
	}
}
func TestAnchorAndAlias(t *testing.T) {
	src := []byte(base + "obs:\n  host: &host localhost\n  password: *host\n")
	if _, e := Rewrite(src, []Change{{"obs.host", "127.0.0.1"}}); e != nil {
		t.Fatal("アンカーと別名の変更失敗")
	}
}

func TestScalarSequencePatch(t *testing.T) {
	src := "items:\n  - 日本語\n  # 要素間\n  - \"two\"\nother: keep\n"
	var doc yaml.Node
	yaml.Unmarshal([]byte(src), &doc)
	var value yaml.Node
	value.Encode([]string{"新しい値", "three"})
	got, ok := patch(src, doc.Content[0].Content[1], &value)
	if !ok || got != "items:\n  - 新しい値\n  - three\nother: keep\n" {
		t.Fatal("スカラーシーケンスの差し替え不一致")
	}
}
