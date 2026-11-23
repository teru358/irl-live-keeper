package obs

import (
	"context"
	"errors"
	"github.com/teru358/irl-live-keeper/internal/i18n"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// 見張りの時間（カメラの spec 第3節「メディアソースの見張り」）。
const (
	mediaWatchFirstWait = 5 * time.Second  // Watch してから最初に見るまで
	mediaWatchReadGap   = time.Second      // 状態を2回読む間
	mediaWatchRecheck   = 10 * time.Second // 入れ直してから見直すまで
)

const mediaStatePlaying = "OBS_MEDIA_STATE_PLAYING"

// MediaSource は見張るメディアソース1つ（入力の UUID と名前）。
type MediaSource struct {
	UUID string
	Name string
}

// MediaAux は MediaWatch が使う補助の接続の要求。Aux.MediaAux で得る。
type MediaAux interface {
	GetMediaInputStatus(inputUUID string) (MediaStatus, error)
	// GetInputSettings は入力の今の設定を返す。
	GetInputSettings(inputUUID string) (map[string]any, error)
	// SetInputSettings は設定を overlay で書く。
	SetInputSettings(inputUUID string, settings map[string]any, overlay bool) error
}

type auxMedia struct{ a *Aux }

// MediaAux は Aux を MediaWatch が使う形にする。
func (a *Aux) MediaAux() MediaAux { return auxMedia{a} }

func (m auxMedia) GetMediaInputStatus(u string) (MediaStatus, error) {
	return m.a.GetMediaInputStatus(u)
}

func (m auxMedia) GetInputSettings(u string) (map[string]any, error) {
	r, err := m.a.GetInputSettings(u)
	if err != nil {
		return nil, err
	}
	return r.InputSettings, nil
}

func (m auxMedia) SetInputSettings(u string, s map[string]any, overlay bool) error {
	_, err := m.a.SetInputSettings(u, s, overlay)
	return err
}

// MediaWatch は、OBS を先に起動して止まったメディアソースを見つけ、設定を入れ直す。
//
// goroutine の持ち主（すべて wg に数え、Close が戻ったときには全部終わっている）:
//   - 見張りの goroutine: Watch の sources の1つにつき1本。待ち（時計）と補助の接続の要求を
//     順に行い、終わったら自分で消える。ctx は Watch の世代ごとに作り、同じカメラの次の Watch、
//     CancelAll、Close で取り消す。
//
// Watch、CancelAll、Warnings は mu を短く持つだけで、補助の接続の要求も待ちも mu の外で行うので、
// 呼び出し側（core）を止めない。
type MediaWatch struct {
	aux   MediaAux
	log   *slog.Logger
	clock auxClock

	mu       sync.Mutex
	closed   bool
	watches  map[string]*mediaWatchGen // カメラ ID → 今の世代
	warnings map[string][]i18n.Msg     // カメラ ID → 警告（次の Watch まで残す）
	wg       sync.WaitGroup
}

type mediaWatchGen struct {
	cancel context.CancelFunc
}

// NewMediaWatch は MediaWatch を作る。clock が nil なら実時間。
func NewMediaWatch(aux MediaAux, log *slog.Logger, clock auxClock) *MediaWatch {
	if log == nil {
		log = slog.Default()
	}
	if clock == nil {
		clock = realClock{}
	}
	return &MediaWatch{
		aux: aux, log: log, clock: clock,
		watches:  map[string]*mediaWatchGen{},
		warnings: map[string][]i18n.Msg{},
	}
}

// Watch は camera のメディアソースの見張りを始める。同じカメラの前の見張りは取り消し、そのカメラの
// 警告は消す（新しい方だけ）。止まらずに戻る。Close のあとは何もしない。
func (m *MediaWatch) Watch(camera string, sources []MediaSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	if old := m.watches[camera]; old != nil {
		old.cancel()
		delete(m.watches, camera)
	}
	delete(m.warnings, camera)
	if len(sources) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	g := &mediaWatchGen{cancel: cancel}
	m.watches[camera] = g
	srcs := append([]MediaSource(nil), sources...)
	sort.SliceStable(srcs, func(i, j int) bool { return srcs[i].Name < srcs[j].Name })
	for _, s := range srcs {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.watchSource(ctx, g, camera, s)
		}()
	}
}

// CancelAll は進行中の見張りをすべて取り消す（OBS から切れたとき）。警告は残す。
func (m *MediaWatch) CancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, g := range m.watches {
		g.cancel()
		delete(m.watches, k)
	}
}

// Warnings は今出す警告（カメラの設定の順ではなく、カメラ ID の辞書順）。
func (m *MediaWatch) Warnings() []string { return i18n.Strings(i18n.JA, m.WarningMessages()) }

func (m *MediaWatch) WarningMessages() []i18n.Msg {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.warnings))
	for id := range m.warnings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []i18n.Msg
	for _, id := range ids {
		out = append(out, m.warnings[id]...)
	}
	return out
}

// Close は見張りをすべて取り消し、goroutine が終わってから戻る。何度呼んでもよい。
// 補助の接続の要求の途中なら、その要求の期限までかかりうる。
func (m *MediaWatch) Close() {
	m.mu.Lock()
	m.closed = true
	for k, g := range m.watches {
		g.cancel()
		delete(m.watches, k)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// sleep は d 待つ。ctx が取り消されたら false。時計が先に起きても、取り消されていれば false。
func (m *MediaWatch) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-m.clock.After(d):
		return ctx.Err() == nil
	}
}

// watchSource はソース1つの見張り。補助の接続が使えないなどで要求が失敗したら、何もせず終わる
// （次の OBSConnected か Online で見張り直す）。
func (m *MediaWatch) watchSource(ctx context.Context, g *mediaWatchGen, camera string, src MediaSource) {
	log := m.log.With("camera", camera, "source", src.Name)
	if !m.sleep(ctx, mediaWatchFirstWait) {
		return
	}
	for refresh := 0; ; refresh++ {
		ok, err := m.healthy(ctx, src)
		if err != nil {
			m.logErr(log, "stopping media source watch", err)
			return
		}
		if ok {
			return
		}
		if refresh == 2 {
			m.warn(g, camera, src)
			return
		}
		if err := m.refresh(src); err != nil {
			m.logErr(log, "cannot refresh media source settings", err)
			return
		}
		log.Info("media source refreshed")
		if !m.sleep(ctx, mediaWatchRecheck) {
			return
		}
	}
}

// healthy は、1秒あけて2回読み、PLAYING で位置が進んでいれば true。取り消されたら ctx のエラー。
func (m *MediaWatch) healthy(ctx context.Context, src MediaSource) (bool, error) {
	a, err := m.aux.GetMediaInputStatus(src.UUID)
	if err != nil {
		return false, err
	}
	if !m.sleep(ctx, mediaWatchReadGap) {
		return false, context.Canceled
	}
	b, err := m.aux.GetMediaInputStatus(src.UUID)
	if err != nil {
		return false, err
	}
	if ctx.Err() != nil {
		return false, context.Canceled
	}
	return b.State == mediaStatePlaying && b.Cursor > a.Cursor, nil
}

// refresh は今の input を読み、同じ値を overlay で入れ直す。
func (m *MediaWatch) refresh(src MediaSource) error {
	cur, err := m.aux.GetInputSettings(src.UUID)
	if err != nil {
		return err
	}
	in, ok := cur["input"]
	if !ok {
		return errors.New("設定に input がない")
	}
	return m.aux.SetInputSettings(src.UUID, map[string]any{"input": in}, true)
}

func (m *MediaWatch) warn(g *mediaWatchGen, camera string, src MediaSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.watches[camera] != g || m.closed {
		return // 取り消された世代は警告を足さない
	}
	m.warnings[camera] = append(m.warnings[camera], i18n.New("obs.media", "name", src.Name))
}

func (m *MediaWatch) logErr(log *slog.Logger, msg string, err error) {
	switch {
	case errors.Is(err, context.Canceled):
	case errors.Is(err, ErrAuxUnavailable), errors.Is(err, ErrAuxClosed):
		log.Debug(msg, "error", err)
	default:
		log.Warn(msg, "error", err)
	}
}
