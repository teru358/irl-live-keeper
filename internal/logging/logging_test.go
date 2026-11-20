package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// jst は UTC とずれたローカル時刻。日付をローカル時刻で決めることを確かめるために使う。
var jst = time.FixedZone("JST", 9*60*60)

// at は jst の時刻を作る。
func at(year int, month time.Month, day, hour, minute, sec int) time.Time {
	return time.Date(year, month, day, hour, minute, sec, 0, jst)
}

// fakeClock はテスト用の時計。複数の goroutine から安全に進められる。
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// openLogger は dir に Logger を開き、テストの終わりに閉じる。
func openLogger(t *testing.T, dir string, keepDays int, secrets []string, clk *fakeClock, stdout *bytes.Buffer) *Logger {
	t.Helper()
	lg, err := Open(Options{Dir: dir, KeepDays: keepDays, Secrets: secrets, Stdout: stdout, Now: clk.Now})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = lg.Close() })
	return lg
}

// fileName は日付 t のログファイルの名前。
func fileName(t time.Time) string {
	return "irl-live-keeper-" + t.Format("2006-01-02") + ".log"
}

// names は dir の直下の名前を並べて返す。
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	slices.Sort(out)
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(b)
}

// writeFile は古いログの代わりに、中身のあるファイルを作る。親のディレクトリもなければ作る。
func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("古い行\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fieldValue は行の中の key=値 の値を返す。なければ空。
func fieldValue(line, key string) string {
	for _, f := range strings.Fields(line) {
		if v, ok := strings.CutPrefix(f, key); ok {
			return v
		}
	}
	return ""
}

// 書いた行がファイルと標準出力の両方に出て、ファイル名の日付はローカル時刻で決まる。
func TestOpenWritesFileAndStdout(t *testing.T) {
	cases := []struct {
		name     string
		sub      string // Dir の下のパス。まだないディレクトリも作らせる
		now      time.Time
		wantFile string
	}{
		{"昼の時刻", "logs", at(2026, 11, 20, 12, 0, 0), "irl-live-keeper-2026-11-20.log"},
		{"UTC では前日になる0時台は、ローカルの日付で決める", "logs", at(2026, 11, 20, 0, 30, 0), "irl-live-keeper-2026-11-20.log"},
		{"月末の夜", "a/b/logs", at(2026, 11, 30, 23, 59, 0), "irl-live-keeper-2026-11-30.log"},
		{"年の最後の日", "logs", at(2026, 12, 31, 23, 59, 59), "irl-live-keeper-2026-12-31.log"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), tc.sub)
			clk := &fakeClock{t: tc.now}
			var stdout bytes.Buffer
			lg := openLogger(t, dir, 14, nil, clk, &stdout)

			lg.Slog().Info("起動した", "port", 8787)
			if err := lg.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			if got := names(t, dir); !slices.Equal(got, []string{tc.wantFile}) {
				t.Fatalf("ディレクトリの中身 = %q, want [%q]", got, tc.wantFile)
			}
			path := filepath.Join(dir, tc.wantFile)
			file := readFile(t, path)
			for _, want := range []string{"msg=起動した", "port=8787"} {
				if !strings.Contains(file, want) {
					t.Errorf("ファイルに %q がない: %q", want, file)
				}
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("標準出力に %q がない: %q", want, stdout.String())
				}
			}
			if file != stdout.String() {
				t.Errorf("ファイルと標準出力が違う\nファイル: %q\n標準出力: %q", file, stdout.String())
			}
			if runtime.GOOS != "windows" {
				fi, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if perm := fi.Mode().Perm(); perm != 0o600 {
					t.Errorf("ファイルの権限 = %o, want 600", perm)
				}
			}
		})
	}
}

// 日付が変わると新しいファイルに書き、前のファイルには前の行だけが残る。
func TestRolloverSwitchesFileByLocalDate(t *testing.T) {
	cases := []struct {
		name     string
		from, to time.Time
	}{
		{"深夜0時をまたぐ", at(2026, 11, 20, 23, 59, 59), at(2026, 11, 21, 0, 0, 1)},
		{"月をまたぐ", at(2026, 11, 30, 23, 0, 0), at(2026, 12, 1, 1, 0, 0)},
		{"年をまたぐ", at(2026, 12, 31, 23, 0, 0), at(2027, 1, 1, 1, 0, 0)},
		{"同じ日の中では切り替えない", at(2026, 11, 20, 8, 0, 0), at(2026, 11, 20, 20, 0, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			clk := &fakeClock{t: tc.from}
			var stdout bytes.Buffer
			lg := openLogger(t, dir, 14, nil, clk, &stdout)

			lg.Slog().Info("切り替え前の行")
			clk.Set(tc.to)
			lg.Slog().Info("切り替え後の行")
			if err := lg.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			fromName, toName := fileName(tc.from), fileName(tc.to)
			if fromName == toName {
				if got := names(t, dir); !slices.Equal(got, []string{fromName}) {
					t.Fatalf("ディレクトリの中身 = %q, want [%q]", got, fromName)
				}
				got := readFile(t, filepath.Join(dir, fromName))
				if !strings.Contains(got, "切り替え前の行") || !strings.Contains(got, "切り替え後の行") {
					t.Errorf("同じ日のファイルに両方の行がない: %q", got)
				}
				return
			}
			if got := names(t, dir); !slices.Equal(got, []string{fromName, toName}) {
				t.Fatalf("ディレクトリの中身 = %q, want [%q %q]", got, fromName, toName)
			}
			before := readFile(t, filepath.Join(dir, fromName))
			after := readFile(t, filepath.Join(dir, toName))
			if !strings.Contains(before, "切り替え前の行") || strings.Contains(before, "切り替え後の行") {
				t.Errorf("前の日のファイルが正しくない: %q", before)
			}
			if !strings.Contains(after, "切り替え後の行") || strings.Contains(after, "切り替え前の行") {
				t.Errorf("次の日のファイルが正しくない: %q", after)
			}
		})
	}
}

// Open は keep_days を過ぎたファイルだけを消す。境界、0と1、形の違う名前、サブディレクトリを確かめる。
func TestOpenPrunesOnlyOldLogFiles(t *testing.T) {
	cases := []struct {
		name     string
		today    time.Time
		keepDays int
		files    []string // 作るファイル。サブディレクトリのパスも書ける
		dirs     []string // 作る空のディレクトリ（日付の形の名前でも消さない）
		want     []string // 残る直下の名前（今日のファイルを含む）
	}{
		{
			name:     "keep_days 14: 11-05 以前は消え、11-06 は残る",
			today:    at(2026, 11, 20, 9, 0, 0),
			keepDays: 14,
			files: []string{
				"irl-live-keeper-2026-11-04.log",
				"irl-live-keeper-2026-11-05.log",
				"irl-live-keeper-2026-11-06.log",
				"irl-live-keeper-2026-11-19.log",
			},
			want: []string{"irl-live-keeper-2026-11-06.log", "irl-live-keeper-2026-11-19.log", "irl-live-keeper-2026-11-20.log"},
		},
		{
			name:     "keep_days 0: 今日より前は消え、今日は残る",
			today:    at(2026, 11, 20, 9, 0, 0),
			keepDays: 0,
			files:    []string{"irl-live-keeper-2026-11-19.log", "irl-live-keeper-2026-11-20.log"},
			want:     []string{"irl-live-keeper-2026-11-20.log"},
		},
		{
			name:     "keep_days 1: 昨日は残り、一昨日は消える",
			today:    at(2026, 11, 20, 9, 0, 0),
			keepDays: 1,
			files:    []string{"irl-live-keeper-2026-11-18.log", "irl-live-keeper-2026-11-19.log", "irl-live-keeper-2026-11-20.log"},
			want:     []string{"irl-live-keeper-2026-11-19.log", "irl-live-keeper-2026-11-20.log"},
		},
		{
			name:     "月と年をまたいで数える",
			today:    at(2027, 1, 3, 9, 0, 0),
			keepDays: 14,
			files: []string{
				"irl-live-keeper-2026-12-19.log",
				"irl-live-keeper-2026-12-20.log",
				"irl-live-keeper-2026-12-31.log",
			},
			want: []string{"irl-live-keeper-2026-12-20.log", "irl-live-keeper-2026-12-31.log", "irl-live-keeper-2027-01-03.log"},
		},
		{
			name:     "未来の日付のファイルは残す",
			today:    at(2026, 11, 20, 9, 0, 0),
			keepDays: 14,
			files:    []string{"irl-live-keeper-2026-11-25.log"},
			want:     []string{"irl-live-keeper-2026-11-20.log", "irl-live-keeper-2026-11-25.log"},
		},
		{
			name:     "形の違う名前、日付として不正な名前、サブディレクトリの中のものは残す",
			today:    at(2026, 11, 20, 9, 0, 0),
			keepDays: 14,
			dirs:     []string{"irl-live-keeper-2026-11-01.log"},
			files: []string{
				"other.log",
				"irl-live-keeper-2026-13-01.log",
				"irl-live-keeper-2026-02-30.log",
				"irl-live-keeper-2026-11-05.log.bak",
				"xirl-live-keeper-2026-11-05.log",
				"irl-live-keeper-2026-11-5.log",
				"IRL-LIVE-KEEPER-2026-11-05.LOG",
				"sub/irl-live-keeper-2026-11-05.log",
				"irl-live-keeper-2026-11-05.log/inner.txt",
			},
			want: []string{
				"IRL-LIVE-KEEPER-2026-11-05.LOG",
				"irl-live-keeper-2026-02-30.log",
				"irl-live-keeper-2026-11-01.log",
				"irl-live-keeper-2026-11-05.log",
				"irl-live-keeper-2026-11-05.log.bak",
				"irl-live-keeper-2026-11-20.log",
				"irl-live-keeper-2026-11-5.log",
				"irl-live-keeper-2026-13-01.log",
				"other.log",
				"sub",
				"xirl-live-keeper-2026-11-05.log",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				writeFile(t, filepath.Join(dir, f))
			}
			for _, d := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			clk := &fakeClock{t: tc.today}
			var stdout bytes.Buffer
			lg := openLogger(t, dir, tc.keepDays, nil, clk, &stdout)
			if err := lg.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			want := slices.Clone(tc.want)
			slices.Sort(want)
			if got := names(t, dir); !slices.Equal(got, want) {
				t.Errorf("残ったもの = %q\nwant %q", got, want)
			}
		})
	}
}

// 消せない古いログがあっても Open は失敗せず、警告を出して続ける。
func TestOpenWarnsAndContinuesWhenPruneFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ではディレクトリの権限で削除を止められない")
	}
	if os.Geteuid() == 0 {
		t.Skip("root では権限で削除を止められない")
	}
	dir := t.TempDir()
	old := filepath.Join(dir, "irl-live-keeper-2026-11-01.log")
	today := filepath.Join(dir, fileName(at(2026, 11, 20, 12, 0, 0)))
	writeFile(t, old)
	writeFile(t, today)
	// 削除だけができないように、ディレクトリの書き込みを止める。今日のファイルは既にあるので開ける。
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	clk := &fakeClock{t: at(2026, 11, 20, 12, 0, 0)}
	var stdout bytes.Buffer
	lg, err := Open(Options{Dir: dir, KeepDays: 14, Stdout: &stdout, Now: clk.Now})
	if err != nil {
		t.Fatalf("消せなくても Open は失敗しないはず: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !strings.Contains(stdout.String(), "古いログを消せなかった") {
		t.Errorf("警告が標準出力にない: %q", stdout.String())
	}
	if _, err := os.Stat(old); err != nil {
		t.Errorf("消せなかったファイルがない: %v", err)
	}
}

// KeepDays が負のときは Open がエラーを返し、何も消さない。
func TestOpenRejectsNegativeKeepDaysWithoutDeleting(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "irl-live-keeper-2026-01-01.log")
	writeFile(t, old)

	var stdout bytes.Buffer
	clk := &fakeClock{t: at(2026, 11, 20, 12, 0, 0)}
	if _, err := Open(Options{Dir: dir, KeepDays: -1, Stdout: &stdout, Now: clk.Now}); err == nil {
		t.Fatal("負の KeepDays は Open がエラーを返すはず")
	}
	if _, err := os.Stat(old); err != nil {
		t.Errorf("エラーのあとに古いファイルが消えた: %v", err)
	}
}

// Open は設定が使えないときにエラーを返す。
func TestOpenRejectsBadDir(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T) string
	}{
		{
			name:  "Dir が空",
			setup: func(t *testing.T) string { return "" },
		},
		{
			name: "ディレクトリを作れない（通常のファイルの下）",
			setup: func(t *testing.T) string {
				blocker := filepath.Join(t.TempDir(), "blocker")
				writeFile(t, blocker)
				return filepath.Join(blocker, "logs")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.setup(t)
			var stdout bytes.Buffer
			clk := &fakeClock{t: at(2026, 11, 20, 12, 0, 0)}
			if _, err := Open(Options{Dir: dir, KeepDays: 14, Stdout: &stdout, Now: clk.Now}); err == nil {
				t.Fatal("Open がエラーを返すはず")
			}
		})
	}
}

// 秘密は標準出力にもファイルにも出ず、*** に置き換わる。
func TestSecretsAreMaskedInStdoutAndFile(t *testing.T) {
	const pass = "SECRET123"
	cases := []struct {
		name    string
		secrets []string
		log     func(l *slog.Logger)
		must    []string // 両方に含まれるべきもの
		mustNot []string // どちらにも含まれてはいけないもの
	}{
		{
			name:    "slog の属性の値として渡した秘密",
			secrets: []string{pass},
			log:     func(l *slog.Logger) { l.Info("publish", "pass", pass, "port", 1935) },
			must:    []string{"pass=***", "port=1935"},
			mustNot: []string{pass},
		},
		{
			name:    "URL の中の秘密は、その部分だけが *** になる",
			secrets: []string{pass},
			log: func(l *slog.Logger) {
				l.Info("送信用 URL", "url", "srt://host:8890?streamid=publish:live:irl:SECRET123")
			},
			must:    []string{"srt://host:8890?streamid=publish:live:irl:***"},
			mustNot: []string{pass},
		},
		{
			name:    "メッセージの中の秘密",
			secrets: []string{pass},
			log:     func(l *slog.Logger) { l.Info("ログインを試した SECRET123") },
			must:    []string{"ログインを試した ***"},
			mustNot: []string{pass},
		},
		{
			name:    "部分文字列の関係: 短い秘密を先に渡しても、長い方が *** になる",
			secrets: []string{"abc", "abcdef"},
			log:     func(l *slog.Logger) { l.Info("x", "v", "abcdef") },
			must:    []string{"v=***"},
			mustNot: []string{"def"},
		},
		{
			name:    "部分文字列の関係: 長い秘密を先に渡した場合も同じ",
			secrets: []string{"abcdef", "abc"},
			log:     func(l *slog.Logger) { l.Info("x", "v", "abcdef") },
			must:    []string{"v=***"},
			mustNot: []string{"def"},
		},
		{
			name:    "空の秘密は無視し、ほかの秘密は置き換える",
			secrets: []string{"", pass},
			log:     func(l *slog.Logger) { l.Info("起動した", "pass", pass, "port", 8787) },
			must:    []string{"msg=起動した", "port=8787", "pass=***"},
			mustNot: []string{pass},
		},
		{
			name:    "slog が引用してエスケープする形の秘密も置き換える",
			secrets: []string{`p"w\d`},
			log:     func(l *slog.Logger) { l.Info("x", "pass", `p"w\d`) },
			must:    []string{`pass="***"`},
			mustNot: []string{`p"w\d`, `p\"w\\d`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			clk := &fakeClock{t: at(2026, 11, 20, 12, 0, 0)}
			var stdout bytes.Buffer
			lg := openLogger(t, dir, 14, tc.secrets, clk, &stdout)
			tc.log(lg.Slog())
			if err := lg.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			file := readFile(t, filepath.Join(dir, "irl-live-keeper-2026-11-20.log"))

			for _, out := range []struct{ where, text string }{{"標準出力", stdout.String()}, {"ファイル", file}} {
				for _, s := range tc.must {
					if !strings.Contains(out.text, s) {
						t.Errorf("%s に %q がない:\n%s", out.where, s, out.text)
					}
				}
				for _, s := range tc.mustNot {
					if strings.Contains(out.text, s) {
						t.Errorf("%s に %q が出た:\n%s", out.where, s, out.text)
					}
				}
			}
		})
	}
}

// 途中で重なる秘密、隣り合う秘密、同じ秘密が1行に複数回出る場合を確かめる。
// 重なる区間と隣り合う区間は1つにまとめるので、どの秘密の2文字以上の断片も残らない。
func TestSecretsOverlappingSpansMerged(t *testing.T) {
	cases := []struct {
		name    string
		secrets []string
		value   string   // 属性の値として書く行
		want    string   // 置き換えたあとの値
		frags   []string // 残ってはいけない断片（秘密の2文字以上の部分）
	}{
		{"途中で重なる2つの秘密（xab と bcd）", []string{"xab", "bcd"}, "xabcd", "***", []string{"xa", "ab", "bc", "cd"}},
		{"重なる2つの秘密を逆の順で渡しても同じ", []string{"bcd", "xab"}, "xabcd", "***", []string{"xa", "ab", "bc", "cd"}},
		{"重なりが連鎖する3つの秘密", []string{"ab", "bc", "cd"}, "abcd", "***", []string{"ab", "bc", "cd"}},
		{"隣り合う2つの秘密は1つの *** になる", []string{"ab", "cd"}, "abcd", "***", []string{"ab", "cd"}},
		{"離れた2つの秘密はそれぞれ *** になる", []string{"ab", "cd"}, "abXcd", "***X***", []string{"ab", "cd"}},
		{"同じ秘密が1行に2回出る", []string{"SECRET"}, "SECRET-SECRET", "***-***", []string{"SECRET"}},
		{"長い秘密の中に短い秘密がある", []string{"abc", "abcdef"}, "xabcdefy", "x***y", []string{"abc", "def"}},
		{"秘密が自分自身と重なる（aa は aaa で2回出る）", []string{"aa"}, "xaaay", "x***y", []string{"aa"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			clk := &fakeClock{t: at(2026, 11, 20, 12, 0, 0)}
			var stdout bytes.Buffer
			lg := openLogger(t, dir, 14, tc.secrets, clk, &stdout)
			lg.Slog().Info("x", "v", tc.value, "end", "E")
			if err := lg.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			file := readFile(t, filepath.Join(dir, "irl-live-keeper-2026-11-20.log"))

			want := "v=" + tc.want + " end=E"
			for _, out := range []struct{ where, text string }{{"標準出力", stdout.String()}, {"ファイル", file}} {
				if !strings.Contains(out.text, want) {
					t.Errorf("%s に %q がない:\n%s", out.where, want, out.text)
				}
				for _, f := range tc.frags {
					if strings.Contains(out.text, f) {
						t.Errorf("%s に秘密の断片 %q が残った:\n%s", out.where, f, out.text)
					}
				}
			}
		})
	}
}

// 複数の goroutine から同時に書いても、行は失われず壊れない。日付が変わった後も同じ。
func TestConcurrentWritesKeepEveryLine(t *testing.T) {
	const writers, perWriter = 8, 50
	dir := t.TempDir()
	clk := &fakeClock{t: at(2026, 11, 20, 12, 0, 0)}
	var stdout bytes.Buffer
	lg := openLogger(t, dir, 14, nil, clk, &stdout)

	burst := func() {
		var wg sync.WaitGroup
		for g := range writers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// 書き手ごとに別の slog.Logger（別の TextHandler）を使う。直列化は Logger の mutex だけに頼る
				l := slog.New(slog.NewTextHandler(sink{lg}, nil))
				for i := range perWriter {
					l.Info("並行", "g", g, "i", i)
				}
			}()
		}
		wg.Wait()
	}
	burst()
	clk.Set(at(2026, 11, 21, 12, 0, 0))
	burst()
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := strings.Count(stdout.String(), "\n"); got != 2*writers*perWriter {
		t.Errorf("標準出力の行数 = %d, want %d", got, 2*writers*perWriter)
	}
	for _, day := range []time.Time{at(2026, 11, 20, 12, 0, 0), at(2026, 11, 21, 12, 0, 0)} {
		text := readFile(t, filepath.Join(dir, fileName(day)))
		lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
		if len(lines) != writers*perWriter {
			t.Errorf("%s の行数 = %d, want %d", fileName(day), len(lines), writers*perWriter)
		}
		seen := map[string]int{}
		for _, line := range lines {
			g, i := fieldValue(line, "g="), fieldValue(line, "i=")
			if !strings.Contains(line, "msg=並行") || g == "" || i == "" {
				t.Errorf("壊れた行: %q", line)
				continue
			}
			seen[g+"/"+i]++
		}
		if len(seen) != writers*perWriter {
			t.Errorf("%s の異なる行の数 = %d, want %d", fileName(day), len(seen), writers*perWriter)
		}
		for k, n := range seen {
			if n != 1 {
				t.Errorf("%s の行 %s が %d 回ある", fileName(day), k, n)
			}
		}
	}
}

// Close のあとに書いてもパニックせず、ファイルも標準出力も変わらず、新しいファイルも作らない。
func TestWriteAfterCloseDoesNotPanicOrReopen(t *testing.T) {
	dir := t.TempDir()
	clk := &fakeClock{t: at(2026, 11, 20, 12, 0, 0)}
	var stdout bytes.Buffer
	lg := openLogger(t, dir, 14, nil, clk, &stdout)

	lg.Slog().Info("閉じる前")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	stdoutBefore := stdout.String()
	fileBefore := readFile(t, filepath.Join(dir, "irl-live-keeper-2026-11-20.log"))

	// 日付を進めても、閉じた Logger は新しいファイルを開かない
	clk.Set(at(2026, 11, 21, 12, 0, 0))
	lg.Slog().Info("閉じたあと")
	if err := lg.Close(); err != nil {
		t.Errorf("2回目の Close: %v", err)
	}

	if stdout.String() != stdoutBefore {
		t.Errorf("Close のあとに標準出力が変わった: %q", stdout.String())
	}
	if got := names(t, dir); !slices.Equal(got, []string{"irl-live-keeper-2026-11-20.log"}) {
		t.Errorf("Close のあとにファイルが増えた: %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "irl-live-keeper-2026-11-20.log")); got != fileBefore {
		t.Errorf("Close のあとにファイルが変わった: %q", got)
	}
}

// Close が書き込みと並行しても壊れない。閉じるまでに書けた行は標準出力とファイルに同じように出る。
func TestCloseRacingWithWritesStaysConsistent(t *testing.T) {
	dir := t.TempDir()
	clk := &fakeClock{t: at(2026, 11, 20, 12, 0, 0)}
	var stdout bytes.Buffer
	lg := openLogger(t, dir, 14, nil, clk, &stdout)

	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l := slog.New(slog.NewTextHandler(sink{lg}, nil))
			for i := range 200 {
				l.Info("競合", "g", g, "i", i)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := lg.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	wg.Wait()

	file := readFile(t, filepath.Join(dir, "irl-live-keeper-2026-11-20.log"))
	if file != stdout.String() {
		t.Errorf("閉じるまでの行が標準出力とファイルで違う\nファイル: %d バイト\n標準出力: %d バイト", len(file), stdout.Len())
	}
	for _, line := range strings.Split(strings.TrimSuffix(file, "\n"), "\n") {
		if file != "" && !strings.Contains(line, "msg=競合") {
			t.Errorf("壊れた行: %q", line)
		}
	}
}
