package logging

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// dateLayout はファイル名と、日付の比較に使う形式。
	dateLayout = "2006-01-02"
	// mask は秘密の代わりに書く文字列。
	mask = "***"
)

// fileNameRe は消す対象の名前の形。日付の妥当性は time.Parse で確かめる。
var fileNameRe = regexp.MustCompile(`^irl-live-keeper-(\d{4}-\d{2}-\d{2})\.log$`)

// Options は Open の設定。
type Options struct {
	Dir      string           // ログのディレクトリ（なければ作る）
	KeepDays int              // これより古い日付のファイルを Open で消す（0 以上）
	Secrets  []string         // 置き換える文字列。空の要素は無視する
	Stdout   io.Writer        // nil なら os.Stdout
	Now      func() time.Time // nil なら time.Now。テストで差し替える
}

// Logger は標準出力と日付ごとのログファイルへ書く。書き込みは Slog が返す slog.Logger を通して行う。
type Logger struct {
	// mu は file、fileDate、closed と、書き先への書き込みを守る。
	mu       sync.Mutex
	dir      string
	stdout   io.Writer
	now      func() time.Time
	patterns []string // 置き換える秘密のパターン（newPatterns の結果）

	file     *os.File // 今日のファイル。Close まで開いたまま
	fileDate string   // file の日付（YYYY-MM-DD）。file が nil のときは空
	closed   bool

	log *slog.Logger
}

// Open はログのディレクトリを作り、今日のファイルを開き、keep_days を過ぎたファイルを消す。
// 消せなかったファイルがあっても Open は失敗せず、警告を出して続ける。
func Open(o Options) (*Logger, error) {
	if o.Dir == "" {
		return nil, errors.New("logging: ログのディレクトリが空")
	}
	if o.KeepDays < 0 {
		return nil, fmt.Errorf("logging: keep_days は 0 以上（%d）", o.KeepDays)
	}
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("logging: ログのディレクトリを作れない: %w", err)
	}
	l := &Logger{
		dir:      o.Dir,
		stdout:   o.Stdout,
		now:      o.Now,
		patterns: newPatterns(o.Secrets),
	}
	if l.stdout == nil {
		l.stdout = os.Stdout
	}
	if l.now == nil {
		l.now = time.Now
	}
	l.log = slog.New(slog.NewTextHandler(sink{l}, nil))

	date := l.now().Format(dateLayout)
	l.mu.Lock()
	_, err := l.fileFor(date)
	l.mu.Unlock()
	if err != nil {
		return nil, err
	}

	if err := prune(o.Dir, date, o.KeepDays); err != nil {
		l.log.Warn("古いログを消せなかった", "err", err)
	}
	return l, nil
}

// Slog は slog のテキスト形式の Logger を返す。書いた行は標準出力とログファイルの両方に出る。
func (l *Logger) Slog() *slog.Logger {
	return l.log
}

// Close はログファイルを閉じる。2回目以降の Close は何もせず nil を返す。
// Close のあとに書いた行は、標準出力にもファイルにも出ず、書き込みはエラーになる（slog は捨てる）。
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	l.fileDate = ""
	return err
}

// sink は slog の出力先。slog は1レコードを1回の Write で書くので、p は1行である。
type sink struct{ l *Logger }

func (s sink) Write(p []byte) (int, error) {
	return s.l.write(p)
}

// write は1行に秘密の置き換えを施し、標準出力とその日のファイルへ書く。
// 標準出力への書き込みが失敗してもファイルへは書き、両方のエラーを返す。
func (l *Logger) write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Close のあとに、日付の変化で新しいファイルを開いてしまわないよう、先に確かめる
	if l.closed {
		return 0, os.ErrClosed
	}

	line := []byte(maskSecrets(string(p), l.patterns))
	var errs []error
	if _, err := l.stdout.Write(line); err != nil {
		errs = append(errs, err)
	}
	f, err := l.fileFor(l.now().Format(dateLayout))
	if err != nil {
		errs = append(errs, err)
	}
	if f != nil {
		if _, err := f.Write(line); err != nil {
			errs = append(errs, err)
		}
	}
	return len(p), errors.Join(errs...)
}

// fileFor は date のファイルを返す。開いているのが別の日付なら閉じて開き直す。呼び出し側が mu を持つこと。
// 戻り値の err は、古いファイルを閉じられなかったか、新しいファイルを開けなかったことを表す。
// 新しいファイルが開けたときは、err が非 nil でも f を返す（書き込みは続けられる）。
// 開けなかったときは fileDate を空のままにするので、次の書き込みでもう一度開く。
func (l *Logger) fileFor(date string) (*os.File, error) {
	if l.file != nil && l.fileDate == date {
		return l.file, nil
	}
	var closeErr error
	if l.file != nil {
		closeErr = l.file.Close()
		l.file = nil
		l.fileDate = ""
	}
	f, err := os.OpenFile(filepath.Join(l.dir, "irl-live-keeper-"+date+".log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, errors.Join(closeErr, err)
	}
	l.file = f
	l.fileDate = date
	return f, closeErr
}

// newPatterns は置き換える秘密のパターンを作る。空の秘密は無視する。
// slog の TextHandler は、" や \ などを含む値を strconv.Quote の形で書くので、その形のパターンも加える。
func newPatterns(secrets []string) []string {
	var pats []string
	for _, s := range secrets {
		if s == "" {
			continue
		}
		pats = append(pats, s)
		if q := strconv.Quote(s); q[1:len(q)-1] != s {
			pats = append(pats, q[1:len(q)-1])
		}
	}
	return pats
}

// span は行の中の区間 [from, to)。
type span struct{ from, to int }

// maskSecrets は line の中で、いずれかのパターンに一致する箇所をすべて mask に置き換える。
// 一致は重なるものも含めてすべて集め、重なる区間と隣り合う区間を1つにまとめてから置き換える。
// そのため、途中で重なる2つの秘密の、どちらの文字も残らない。
func maskSecrets(line string, patterns []string) string {
	var spans []span
	for _, p := range patterns {
		// 1バイトずつずらして探し、重なる一致も漏らさない
		for from := 0; from < len(line); {
			i := strings.Index(line[from:], p)
			if i < 0 {
				break
			}
			start := from + i
			spans = append(spans, span{start, start + len(p)})
			from = start + 1
		}
	}
	if len(spans) == 0 {
		return line
	}

	slices.SortFunc(spans, func(a, b span) int { return cmp.Compare(a.from, b.from) })
	merged := spans[:1]
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.from <= last.to { // 重なる、または隣り合う
			last.to = max(last.to, s.to)
		} else {
			merged = append(merged, s)
		}
	}

	var b strings.Builder
	prev := 0
	for _, s := range merged {
		b.WriteString(line[prev:s.from])
		b.WriteString(mask)
		prev = s.to
	}
	b.WriteString(line[prev:])
	return b.String()
}

// prune は dir の直下にある、名前が irl-live-keeper-YYYY-MM-DD.log の形の通常ファイルのうち、
// 日付が today の keepDays 日前より前のものを消す。日付の計算は暦の日付で行う。
// 形の違う名前、日付として不正な名前、ディレクトリ、シンボリックリンクには触らない。
func prune(dir, today string, keepDays int) error {
	t, err := time.Parse(dateLayout, today)
	if err != nil {
		return err
	}
	cutoff := t.AddDate(0, 0, -keepDays)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		m := fileNameRe.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		d, err := time.Parse(dateLayout, m[1])
		if err != nil {
			continue // 日付として不正な名前は残す
		}
		if d.Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
