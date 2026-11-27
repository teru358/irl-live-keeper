package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/teru358/irl-live-keeper/internal/redact"
)

const (
	// dateLayout はファイル名と、日付の比較に使う形式。
	dateLayout = "2006-01-02"
)

// fileNameRe は消す対象の名前の形。日付の妥当性は time.Parse で確かめる。
var fileNameRe = regexp.MustCompile(`^irl-live-keeper-(\d{4}-\d{2}-\d{2})\.log$`)

// crashFileNameRe は落ちたときの出力のファイル名（crash-YYYYMMDD-HHMMSS.log）の形。
var crashFileNameRe = regexp.MustCompile(`^crash-(\d{8}-\d{6})\.log$`)

// Options は Open の設定。
type Options struct {
	Dir      string           // ログのディレクトリ（なければ作る）
	KeepDays int              // これより古い日付のファイルを Open と日付変更時に消す（0 以上）
	Secrets  []string         // 置き換える文字列。空の要素は無視する
	Stdout   io.Writer        // nil なら os.Stdout
	Now      func() time.Time // nil なら time.Now。テストで差し替える
}

// Logger は標準出力と日付ごとのログファイルへ書く。書き込みは Slog が返す slog.Logger を通して行う。
type Logger struct {
	// mu は file、fileDate、closed と、書き先への書き込みを守る。
	mu       sync.Mutex
	dir      string
	keepDays int
	stdout   io.Writer
	now      func() time.Time
	masker   *redact.Masker // 秘密を *** に置き換える

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
		keepDays: o.KeepDays,
		stdout:   o.Stdout,
		now:      o.Now,
		masker:   redact.New(o.Secrets...),
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
		l.log.Warn("cannot remove old log", "err", err)
	}
	return l, nil
}

// Slog は slog のテキスト形式の Logger を返す。書いた行は標準出力とログファイルの両方に出る。
func (l *Logger) Slog() *slog.Logger {
	return l.log
}

// SetKeepDays は、次にログを消すときから使う保存日数を設定する。負の値は無視する。
func (l *Logger) SetKeepDays(n int) {
	if n < 0 {
		return
	}
	l.mu.Lock()
	l.keepDays = n
	l.mu.Unlock()
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

	masked := l.masker.Mask(string(p))
	line := []byte(masked)
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

// AddSecrets は秘密を足す。動作中でも呼べる。空の要素は無視する。
// 足したあとに書く行では、Open で渡した値も足した値も *** に置き換わる。
func (l *Logger) AddSecrets(s ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.masker.Add(s...)
}

// fileFor は date のファイルを返す。開いているのが別の日付なら閉じて開き直す。呼び出し側が mu を持つこと。
// 戻り値の err は、古いファイルを閉じられなかったか、新しいファイルを開けなかったことを表す。
// 新しいファイルが開けたときは、err が非 nil でも f を返す（書き込みは続けられる）。
// 開けなかったときは fileDate を空のままにするので、次の書き込みでもう一度開く。
func (l *Logger) fileFor(date string) (*os.File, error) {
	if l.file != nil && l.fileDate == date {
		return l.file, nil
	}
	rolling := l.file != nil
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
	if rolling {
		// 削除失敗は書き込みに影響させない。mu を保持しているため slog での警告も出さない。
		_ = prune(l.dir, date, l.keepDays)
	}
	return f, closeErr
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

// PruneCrashLogs は dir 直下にある crash-YYYYMMDD-HHMMSS.log の通常ファイルを整理する。
// current は常に残す。空のファイルは削除し、keepKnown のときは日付が保存期間を過ぎたものも削除する。
func PruneCrashLogs(dir, current string, keepDays int, keepKnown bool, today time.Time) error {
	if keepKnown && keepDays < 0 {
		return fmt.Errorf("logging: keep_days は 0 以上（%d）", keepDays)
	}
	cutoff := today
	if keepKnown {
		date, err := time.Parse(dateLayout, today.Format(dateLayout))
		if err != nil {
			return err
		}
		cutoff = date.AddDate(0, 0, -keepDays)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	current = filepath.Base(current)
	var errs []error
	for _, entry := range entries {
		name := entry.Name()
		match := crashFileNameRe.FindStringSubmatch(name)
		if match == nil || name == current {
			continue
		}
		stamp, err := time.Parse("20060102-150405", match[1])
		if err != nil || stamp.Format("20060102-150405") != match[1] {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		remove := info.Size() == 0
		if keepKnown && stamp.Format(dateLayout) < cutoff.Format(dateLayout) {
			remove = true
		}
		if remove {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
		}
	}
	return errors.Join(errs...)
}
