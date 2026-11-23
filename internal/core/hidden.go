package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teru358/irl-live-keeper/internal/switcher"
)

// 隠した項目の記録のファイル hidden.json（カメラの spec 第7節）。置き場所は config.yaml のあるフォルダの
// state/hidden.json。一時ファイルに書いてから名前を替える。

// hiddenFileVersion は hidden.json の形の版。知らない版は壊れたファイルとして扱う。
const hiddenFileVersion = 1

const (
	// hiddenSyncLimit は、隠す前の同期の保存（MustPersist）の上限。超えたら失敗として扱う。
	hiddenSyncLimit = time.Second
	// defaultHiddenRetry は、通常の保存に失敗したときに書き直す間隔（次の変化があればそれより早く書き直す）。
	defaultHiddenRetry = 5 * time.Second
	// DefaultStopTimeout は Stopping が RestoreAll の完了を待つ上限（spec 第7節「世代を止めるとき」）。
	DefaultStopTimeout = 2 * time.Second
)

type hiddenFile struct {
	Version   int         `json:"version"`
	WrittenAt time.Time   `json:"written_at"`
	Entries   []hiddenRec `json:"entries"`
}

type hiddenRec struct {
	Camera     string     `json:"camera"`
	Host       string     `json:"host"`
	Port       int        `json:"port"`
	Collection string     `json:"collection"`
	Container  string     `json:"container"`
	Item       int        `json:"item"`
	Source     string     `json:"source"`
	State      string     `json:"state"` // pending / hidden / unsettled
	SettledAt  *time.Time `json:"settled_at"`
	WrittenAt  time.Time  `json:"written_at"`
}

func stateName(s switcher.HiddenState) string {
	switch s {
	case switcher.HiddenH:
		return "hidden"
	case switcher.HiddenU:
		return "unsettled"
	}
	return "pending"
}

func stateOf(name string) (switcher.HiddenState, bool) {
	switch name {
	case "pending":
		return switcher.HiddenP, true
	case "hidden":
		return switcher.HiddenH, true
	case "unsettled":
		return switcher.HiddenU, true
	}
	return 0, false
}

// encodeHidden は記録を hidden.json の中身にする。now は書いた時刻。
func encodeHidden(es []switcher.HiddenEntry, now time.Time) []byte {
	f := hiddenFile{Version: hiddenFileVersion, WrittenAt: now.UTC(), Entries: make([]hiddenRec, 0, len(es))}
	for _, e := range es {
		r := hiddenRec{Camera: e.CameraID, Host: e.Host, Port: e.Port, Collection: e.Collection,
			Container: e.ContainerUUID, Item: e.ItemID, Source: e.SourceUUID, State: stateName(e.State),
			WrittenAt: e.WrittenAt.UTC()}
		if e.SettledAt != nil {
			t := e.SettledAt.UTC()
			r.SettledAt = &t
		}
		f.Entries = append(f.Entries, r)
	}
	b, _ := json.MarshalIndent(f, "", "  ")
	return append(b, '\n')
}

func decodeHidden(b []byte) ([]switcher.HiddenEntry, error) {
	var f hiddenFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if f.Version != hiddenFileVersion {
		return nil, i18n.Error{Message: i18n.New("hidden.version", "version", fmt.Sprint(f.Version))}
	}
	var out []switcher.HiddenEntry
	for i, r := range f.Entries {
		st, ok := stateOf(r.State)
		if !ok || r.Container == "" {
			return nil, i18n.Error{Message: i18n.New("hidden.entry", "index", fmt.Sprint(i), "state", fmt.Sprintf("%q", r.State))}
		}
		e := switcher.HiddenEntry{CameraID: r.Camera, Host: r.Host, Port: r.Port, Collection: r.Collection,
			ContainerUUID: r.Container, ItemID: r.Item, SourceUUID: r.Source, State: st, WrittenAt: r.WrittenAt}
		if r.SettledAt != nil {
			t := *r.SettledAt
			e.SettledAt = &t
		}
		out = append(out, e)
	}
	return out, nil
}

// LoadHidden は起動のとき（core を作る前）に hidden.json を読む。ファイルがなければ空で警告もなし。
// 読めない（壊れている、知らない版）ときは、空として扱い、警告の文を返し、元のファイルは
// hidden.json.broken に名前を替えて残す。
func LoadHidden(path string) (entries []switcher.HiddenEntry, warning string) {
	entries, msg := LoadHiddenMessage(path)
	return entries, i18n.Render(i18n.JA, msg)
}

// LoadHiddenMessage は表示言語を決めずに読取り結果を返す。
func LoadHiddenMessage(path string) (entries []switcher.HiddenEntry, warning i18n.Msg) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, i18n.Msg{}
	}
	if err == nil {
		entries, err = decodeHidden(b)
		if err == nil {
			return entries, i18n.Msg{}
		}
	}
	broken := path + ".broken"
	if rerr := os.Rename(path, broken); rerr != nil {
		m := i18n.New("hidden.load_failed", "rename_error", rerr.Error())
		m.Values["error"] = i18n.Reason(err)
		return nil, m
	}
	m := i18n.New("hidden.load")
	m.Values["error"] = i18n.Reason(err)
	return nil, m
}

// atomicWrite は path の中身を data にする。同じフォルダの一時ファイルに書いてから名前を替える。
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".hidden-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if werr == nil {
		werr = f.Sync()
	}
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
	}
	return werr
}

// hiddenStore は hidden.json への書き込みを1本にそろえる。新しい番号（HiddenRevision）を書いたあとは、
// 古い番号の書き込みを捨てる（遅れて終わった古い書き込みが新しい中身を上書きしないため）。
type hiddenStore struct {
	path  string
	write func(path string, data []byte) error

	mu      sync.Mutex
	lastRev int
	// writtenRev は最後に書けた番号。書けていなければ -1。
	writtenRev atomic.Int64
	failing    atomic.Bool // 最後の書き込みが失敗した
}

func newHiddenStore(path string, write func(string, []byte) error, startRev int) *hiddenStore {
	if write == nil {
		write = atomicWrite
	}
	s := &hiddenStore{path: path, write: write, lastRev: -1}
	s.writtenRev.Store(int64(startRev))
	return s
}

func (s *hiddenStore) put(rev int, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rev < s.lastRev {
		return nil
	}
	if err := s.write(s.path, data); err != nil {
		s.failing.Store(true)
		return err
	}
	s.lastRev = rev
	s.writtenRev.Store(int64(rev))
	s.failing.Store(false)
	return nil
}

// persistReq は書きたい最新の中身。
type persistReq struct {
	rev  int
	data []byte
}

// persister は、中核ループを止めずに hidden.json を書き直す goroutine（H-06）。最新の中身だけを持ち、
// 書けなければ retry の間隔で、次の変化があればそれより早く書き直す。
type persister struct {
	store *hiddenStore
	retry time.Duration

	mu     sync.Mutex
	latest *persistReq
	wake   chan struct{}
}

func newPersister(s *hiddenStore, retry time.Duration) *persister {
	if retry <= 0 {
		retry = defaultHiddenRetry
	}
	return &persister{store: s, retry: retry, wake: make(chan struct{}, 1)}
}

// submit は最新の中身を渡す（止まらない）。
func (p *persister) submit(rev int, data []byte) {
	p.mu.Lock()
	p.latest = &persistReq{rev: rev, data: data}
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// run は管理の goroutine。ディスクへの書き込み（取り消せない）は別の worker goroutine で行い、run は
// ctx が終わったら、worker の終わりを待たずに戻る。ディスクが固まっていれば worker だけが、ディスクの
// 応答まで残る（既知の制約。Run と本体の終了は止めない）。worker は同時に1つだけ。
func (p *persister) run(ctx context.Context) {
	var retry <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	var inflight chan error // 書き込み中の worker の結果（容量1。run が戻っても worker は詰まらない）
	var started *persistReq
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.wake:
		case <-retry:
		case err := <-inflight:
			inflight = nil
			if err != nil {
				if timer == nil {
					timer = time.NewTimer(p.retry)
				} else {
					timer.Reset(p.retry)
				}
				retry = timer.C
				continue
			}
			p.mu.Lock()
			if p.latest == started {
				p.latest = nil
			}
			again := p.latest != nil
			p.mu.Unlock()
			if !again {
				continue
			}
		}
		if inflight != nil {
			continue // 書き込み中。終わったら最新の中身を見直す
		}
		retry = nil
		p.mu.Lock()
		req := p.latest
		p.mu.Unlock()
		if req == nil {
			continue
		}
		started = req
		inflight = make(chan error, 1)
		go func(ch chan error) { ch <- p.store.put(req.rev, req.data) }(inflight)
	}
}
