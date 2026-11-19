package switcher

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// テストの時刻はすべて t0 からのミリ秒で書く。
var t0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

// W は RestartMedia.Wait。テストでは 0.5秒とする。
const W = 500 * time.Millisecond

const (
	OK   = ingest.LinkOK
	LOW  = ingest.LinkLow
	DOWN = ingest.LinkDown
)

// 例に使うシーン名。待機シーン、監視対象2つ、管理外2つ。
const (
	standby = "Standby"
	camera  = "Camera"
	camera2 = "Camera2"
	talk    = "Talk"
	opening = "Opening"
)

func newMachine() *Machine { return New(Config{RestartWait: W}) }

func ip(v int) *int       { return &v }
func sp(v string) *string { return &v }

// actsFor は Purpose と切替先ごとの Action の列（期待値）。
func actsFor(p Purpose, to string) []Action {
	switch p {
	case PurposeAutoFail, PurposeStandbyOn, PurposeStandbyOffOther, PurposeStartScene:
		return []Action{SetScene{Scene: to}}
	case PurposeAutoRecover, PurposeStandbyOffMonitored:
		return []Action{RestartMedia{Wait: W}, SetScene{Scene: to}}
	case PurposeStartStream:
		return []Action{StartStream{}}
	case PurposeStopStream:
		return []Action{StopStream{}}
	}
	panic("unknown purpose")
}

func set(scene string) Action { return SetScene{Scene: scene} }

var restart Action = RestartMedia{Wait: W}

// ---- Event の短い書き方 ----

func lc(ms int, s ingest.LinkState) Event { return LinkChanged{At: at(ms), State: s} }
func sc(ms int, scene string) Event       { return SceneChanged{At: at(ms), Scene: scene} }
func tick(ms int) Event                   { return Tick{At: at(ms)} }
func ssc(ms int, v bool) Event            { return StreamStateChanged{At: at(ms), Streaming: v} }
func disc(ms int) Event                   { return OBSDisconnected{At: at(ms)} }
func cmd(ms int, k CommandKind) Event     { return Command{At: at(ms), Kind: k} }
func tc(ms int, tr *int) Event            { return TransitionChanged{At: at(ms), TransitionMs: tr} }
func conn(ms int, streaming bool, scene string, tr *int) Event {
	return OBSConnected{At: at(ms), Streaming: streaming, Scene: scene, TransitionMs: tr}
}
func srd(ms, req int, r RequestResult) Event {
	return SceneRequestDone{At: at(ms), ReqID: req, Result: r}
}
func bd(ms, id int, r BatchResult) Event { return BatchDone{At: at(ms), BatchID: id, Result: r} }

// cfgMod は設定 CFG の1項目を変える。
type cfgMod func(*ScenesConfigured)

func withAuto(v bool) cfgMod           { return func(c *ScenesConfigured) { c.AutoEnabled = v } }
func withStart(s string) cfgMod        { return func(c *ScenesConfigured) { c.StartScene = sp(s) } }
func withStandby(s string) cfgMod      { return func(c *ScenesConfigured) { c.Standby = s } }
func withMonitored(s ...string) cfgMod { return func(c *ScenesConfigured) { c.Monitored = s } }
func cfgOf(ms int, mods ...cfgMod) ScenesConfigured {
	c := ScenesConfigured{At: at(ms), Standby: standby, Monitored: []string{camera, camera2}, AutoEnabled: true}
	for _, f := range mods {
		f(&c)
	}
	return c
}
func cfg(ms int, mods ...cfgMod) Event { return cfgOf(ms, mods...) }

// ---- 状態の複製と比較 ----

func cloneState(s state) state {
	c := s
	c.Monitored = slices.Clone(s.Monitored)
	if s.StartScene != nil {
		c.StartScene = sp(*s.StartScene)
	}
	if s.TransitionMs != nil {
		c.TransitionMs = ip(*s.TransitionMs)
	}
	c.Expectations = slices.Clone(s.Expectations)
	c.Pending = make(map[int]PendingBatch, len(s.Pending))
	for k, v := range s.Pending {
		c.Pending[k] = v
	}
	c.LastAutoSent = make(map[Purpose]time.Time, len(s.LastAutoSent))
	for k, v := range s.LastAutoSent {
		c.LastAutoSent[k] = v
	}
	return c
}

func normState(s state) state {
	c := cloneState(s)
	if len(c.Expectations) == 0 {
		c.Expectations = nil
	}
	return c
}

// diffState は2つの状態の違うフィールドを並べる。同じなら空文字列。
func diffState(got, want state) string {
	g, w := normState(got), normState(want)
	gv, wv := reflect.ValueOf(g), reflect.ValueOf(w)
	var b strings.Builder
	for i := 0; i < gv.NumField(); i++ {
		if !reflect.DeepEqual(gv.Field(i).Interface(), wv.Field(i).Interface()) {
			fmt.Fprintf(&b, "\n  %s: got %s, want %s", gv.Type().Field(i).Name,
				show(gv.Field(i).Interface()), show(wv.Field(i).Interface()))
		}
	}
	return b.String()
}

func show(v any) string {
	switch x := v.(type) {
	case *int:
		if x == nil {
			return "nil"
		}
		return fmt.Sprintf("&%d", *x)
	case *string:
		if x == nil {
			return "nil"
		}
		return fmt.Sprintf("&%q", *x)
	case *ErrorInfo:
		if x == nil {
			return "nil"
		}
		return fmt.Sprintf("&{%s %q}", show(x.At), x.Msg)
	case time.Time:
		if x.IsZero() {
			return "zero"
		}
		return fmt.Sprintf("t0+%v", x.Sub(t0))
	case []Expectation:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = showExp(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[Purpose]time.Time:
		parts := []string{}
		for k, t := range x {
			parts = append(parts, fmt.Sprintf("%v:%s", k, show(t)))
		}
		slices.Sort(parts)
		return "{" + strings.Join(parts, " ") + "}"
	}
	return fmt.Sprintf("%+v", v)
}

func showExp(e Expectation) string {
	return fmt.Sprintf("{r%d %q %v ret=%q cfg=%d reg=%s ov=%T%+v tr=%s resp=%v at=%s res=%v dl=%s}",
		e.ReqID, e.Scene, e.Purpose, e.ReturnScene, e.ConfigEpoch, show(e.RegisteredAt), e.Override, e.Override,
		show(e.TransitionMs), e.Responded, show(e.RespondedAt), e.Result, show(e.Deadline))
}

// ---- 状態を直接作る仕組み ----

// baseState は既定の前提の状態: 設定 CFG、接続、配信中、Camera（Monitored）、OK、持ち主 none、
// 未完了なし、期待なし。Epoch と通し番号は 0 / 1 以外にしておき、動いたかがわかるようにする。
func baseState(s *state) {
	s.Standby = standby
	s.Monitored = []string{camera, camera2}
	s.AutoEnabled = true
	s.StartScene = nil
	s.ConfigEpoch = 5
	s.Connected = true
	s.Streaming = true
	s.Scene = camera
	s.PrevConnScene = talk
	s.PrevConnStreaming = false
	s.Link = OK
	s.TransitionMs = ip(300)
	s.SceneEpoch = 3
	s.ConnEpoch = 2
	s.StreamEpoch = 4
	s.NextBatchID = 10
	s.NextReqID = 20
}

// inStandby は待機シーンにいて、持ち主 o、戻り先 ret の状態にする。
func inStandby(s *state, o Owner, ret string) {
	s.Scene = s.Standby
	s.StandbyOwner = o
	s.ReturnScene = ret
}

// clearOwner は持ち主と戻り先を消す。
func clearOwner(s *state) {
	s.StandbyOwner = OwnerNone
	s.ReturnScene = ""
}

// pend は今の Epoch で未完了のバッチを足す。ret はバッチが持つ戻り先。
func pend(s *state, id int, p Purpose, ret string) {
	s.Pending[id] = PendingBatch{BatchID: id, Purpose: p, SceneEpoch: s.SceneEpoch,
		ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch, ReturnScene: ret}
}

// waiting は今の設定の世代で登録した、応答待ちの期待を返す。
func waiting(s *state, req int, scene string, p Purpose, ret string) Expectation {
	return Expectation{ReqID: req, Scene: scene, Purpose: p, ReturnScene: ret, ConfigEpoch: s.ConfigEpoch,
		RegisteredAt: at(0), Override: OverrideNone{}, TransitionMs: ip(300)}
}

// answered は x を応答済みにする。
func answered(x Expectation, res RequestResult, respMs, deadlineMs int) Expectation {
	x.Responded = true
	x.RespondedAt = at(respMs)
	x.Result = res
	x.Deadline = at(deadlineMs)
	return x
}

// addExp は応答済み（期限は遠い先）の期待を足す。
func addExp(s *state, req int, scene string, p Purpose, ret string) {
	s.Expectations = append(s.Expectations, answered(waiting(s, req, scene, p, ret), RequestOK, 0, 60000))
}

// manualChange は手動の SceneChanged の共通の変化。
func manualChange(s *state, scene string) {
	s.Scene = scene
	s.SceneEpoch++
	s.Expectations = nil
	s.LastAutoSent = map[Purpose]time.Time{}
	clearOwner(s)
}

// wantB はバッチを出したときの共通の変化を期待の状態に当てはめ、返るべきバッチを返す。
func wantB(s *state, p Purpose, to, ret string, atT time.Time) *Batch {
	id := s.NextBatchID
	s.NextBatchID++
	s.Pending[id] = PendingBatch{BatchID: id, Purpose: p, SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch,
		StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch, ReturnScene: ret}
	if p.IsAuto() {
		s.LastAutoSent[p] = atT
	}
	return &Batch{BatchID: id, Purpose: p, SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch,
		StreamEpoch: s.StreamEpoch, ConfigEpoch: s.ConfigEpoch, ReturnScene: ret, Actions: actsFor(p, to)}
}

// defaultTarget は、行が切替先と戻り先を書かないときの既定値。
func defaultTarget(s *state, p Purpose, to, ret string) (string, string) {
	switch p {
	case PurposeAutoFail, PurposeStandbyOn:
		if to == "" {
			to = s.Standby
		}
		if ret == "" {
			ret = s.Scene
		}
	case PurposeAutoRecover, PurposeStandbyOffMonitored, PurposeStandbyOffOther:
		if to == "" {
			to = s.ReturnScene
		}
	case PurposeStartScene:
		if to == "" {
			to = *s.StartScene
		}
	}
	return to, ret
}

// ---- 入口ごとに、1つの Event の前後の状態を比べる仕組み ----

type evRow struct {
	name   string
	setup  func(s *state)
	ev     Event
	err    error   // 返るべき cmdErr（型だけ比べる）
	reason string  // 返るべき理由の文（空なら比べない）
	out    Purpose // 0 ならバッチなし
	to     string  // バッチの SetScene の切替先（空なら既定値）
	ret    string  // バッチの戻り先（空なら既定値）
	change func(s *state)
	recErr bool // LastError を記録する
}

func runEvRows(t *testing.T, rows []evRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			m := newMachine()
			baseState(&m.st)
			if r.setup != nil {
				r.setup(&m.st)
			}
			before := cloneState(m.st)
			got, err := m.Handle(r.ev)
			checkErr(t, err, r.err, r.reason)
			want := cloneState(before)
			if r.change != nil {
				r.change(&want)
			}
			if r.out != 0 {
				to, ret := defaultTarget(&want, r.out, r.to, r.ret)
				wb := wantB(&want, r.out, to, ret, r.ev.eventAt())
				if !reflect.DeepEqual(got, wb) {
					t.Errorf("batch: got %+v, want %+v", got, wb)
				}
			} else if got != nil {
				t.Errorf("batch: got %+v, want none", got)
			}
			checkRecErr(t, m, &want, before.LastError, r.recErr, r.ev.eventAt())
			if d := diffState(m.st, want); d != "" {
				t.Errorf("state differs:%s", d)
			}
		})
	}
}

func checkErr(t *testing.T, got, want error, reason string) {
	t.Helper()
	switch want.(type) {
	case nil:
		if got != nil {
			t.Fatalf("cmdErr: got %v, want nil", got)
		}
	case ErrNotConnected:
		var e ErrNotConnected
		if !errors.As(got, &e) {
			t.Fatalf("cmdErr: got %v, want ErrNotConnected", got)
		}
	case ErrInvalidState:
		var e ErrInvalidState
		if !errors.As(got, &e) || e.Reason == "" {
			t.Fatalf("cmdErr: got %v, want ErrInvalidState with reason", got)
		}
		if reason != "" && e.Reason != reason {
			t.Fatalf("reason: got %q, want %q", e.Reason, reason)
		}
	}
}

// checkRecErr は LastError の記録を確かめ、記録したならその値を期待の状態に写す。
func checkRecErr(t *testing.T, m *Machine, want *state, before *ErrorInfo, rec bool, atT time.Time) {
	t.Helper()
	if !rec {
		return
	}
	got := m.st.LastError
	if got == nil || got == before || !got.At.Equal(atT) || got.Msg == "" {
		t.Errorf("LastError: got %s, want a new record at %s", show(got), show(atT))
		return
	}
	want.LastError = got
}

// ---- Approve の前後の状態を比べる仕組み ----

type apRow struct {
	name     string
	setup    func(s *state) // 未完了のバッチは id=1 で足す
	action   Action
	override Override
	atMs     int
	res      ApproveResult // Proceed（ReqID は見ない）/ Skip / Reject / Replace
	register bool          // Proceed で期待を登録する
	change   func(s *state)
	recErr   bool
}

func runApRows(t *testing.T, rows []apRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			m := newMachine()
			baseState(&m.st)
			r.setup(&m.st)
			before := cloneState(m.st)
			atT := at(r.atMs)
			got := m.Approve(1, r.action, r.override, atT)
			want := cloneState(before)
			if r.change != nil {
				r.change(&want)
			}
			switch r.res.(type) {
			case Proceed:
				p, ok := got.(Proceed)
				if !ok {
					t.Fatalf("result: got %#v, want Proceed", got)
				}
				if r.register {
					if p.ReqID != before.NextReqID {
						t.Errorf("ReqID: got %d, want %d", p.ReqID, before.NextReqID)
					}
					ov := r.override
					if ov == nil {
						ov = OverrideNone{}
					}
					pb := before.Pending[1]
					want.Expectations = append(want.Expectations, Expectation{
						ReqID: before.NextReqID, Scene: r.action.(SetScene).Scene, Purpose: pb.Purpose,
						ReturnScene: pb.ReturnScene, ConfigEpoch: before.ConfigEpoch, RegisteredAt: atT,
						Override: ov, TransitionMs: before.TransitionMs})
					want.NextReqID++
				} else if p.ReqID != 0 {
					t.Errorf("ReqID: got %d, want 0", p.ReqID)
				}
			default:
				if !reflect.DeepEqual(got, r.res) {
					t.Fatalf("result: got %#v, want %#v", got, r.res)
				}
			}
			checkRecErr(t, m, &want, before.LastError, r.recErr, atT)
			if d := diffState(m.st, want); d != "" {
				t.Errorf("state differs:%s", d)
			}
		})
	}
}

// ---- Event の列を流す仕組み ----

type seq struct {
	t *testing.T
	m *Machine
}

func newSeq(t *testing.T) *seq { return &seq{t: t, m: newMachine()} }

// prep は準備 P(シーン, 配信, 受信状態) を、全体の長さ tr で行う。
func (q *seq) prep(scene string, streaming bool, link ingest.LinkState, tr *int, mods ...cfgMod) *seq {
	q.t.Helper()
	q.none(cfg(0, mods...))
	q.none(lc(0, link))
	q.none(conn(0, streaming, scene, tr))
	return q
}

// P は準備 P(シーン, 配信, 受信状態)。
func (q *seq) P(scene string, streaming bool, link ingest.LinkState, mods ...cfgMod) *seq {
	q.t.Helper()
	return q.prep(scene, streaming, link, ip(300), mods...)
}

// PA は準備 PA(R): 自動の待機。結果は待機シーン、持ち主 auto、戻り先 R、LOW。
func (q *seq) PA(ret string) *seq {
	q.t.Helper()
	q.P(ret, true, OK)
	q.batch(lc(100, LOW), 1, PurposeAutoFail, standby, ret)
	q.approve(1, set(standby), 100, Proceed{ReqID: 1})
	q.none(srd(100, 1, RequestOK))
	q.none(bd(100, 1, BatchCompleted))
	q.none(sc(500, standby))
	q.owner(OwnerAuto, ret)
	return q
}

// PM は準備 PM(R, 受信状態): 手動の待機。結果は持ち主 manual、戻り先 R。
func (q *seq) PM(ret string, link ingest.LinkState, mods ...cfgMod) *seq {
	q.t.Helper()
	q.P(ret, true, OK, mods...)
	q.standbyOnFrom(ret)
	if link != OK {
		q.none(lc(600, link))
	}
	return q
}

// standbyOnFrom は t=0.1 に StandbyOn#1 を受け付けて通し、t=0.5 に待機シーンへ届ける。
func (q *seq) standbyOnFrom(ret string) {
	q.t.Helper()
	q.batch(cmd(100, CmdStandbyOn), 1, PurposeStandbyOn, standby, ret)
	q.approve(1, set(standby), 100, Proceed{ReqID: 1})
	q.none(srd(100, 1, RequestOK))
	q.none(bd(100, 1, BatchCompleted))
	q.none(sc(500, standby))
	q.owner(OwnerManual, ret)
}

// none は Event を流し、バッチも cmdErr も返らないことを確かめる。
func (q *seq) none(ev Event) {
	q.t.Helper()
	b, err := q.m.Handle(ev)
	if err != nil || b != nil {
		q.t.Fatalf("%T at %s: got batch %s err %v, want none", ev, show(ev.eventAt()), showBatch(b), err)
	}
}

func showBatch(b *Batch) string {
	if b == nil {
		return "nil"
	}
	return fmt.Sprintf("%v#%d%+v ret=%q", b.Purpose, b.BatchID, b.Actions, b.ReturnScene)
}

// batch は Event を流し、id と Purpose、切替先 to、戻り先 ret のバッチが返ることを確かめる。
func (q *seq) batch(ev Event, id int, p Purpose, to, ret string) *Batch {
	q.t.Helper()
	b, err := q.m.Handle(ev)
	if err != nil {
		q.t.Fatalf("%T at %s: cmdErr %v", ev, show(ev.eventAt()), err)
	}
	q.checkBatch(b, id, p, to, ret)
	return b
}

func (q *seq) checkBatch(b *Batch, id int, p Purpose, to, ret string) {
	q.t.Helper()
	if b == nil {
		q.t.Fatalf("got no batch, want %v#%d", p, id)
	}
	want := actsFor(p, to)
	if b.BatchID != id || b.Purpose != p || !reflect.DeepEqual(b.Actions, want) || b.ReturnScene != ret {
		q.t.Fatalf("got %s, want %v#%d%+v ret=%q", showBatch(b), p, id, want, ret)
	}
	s := &q.m.st
	if b.SceneEpoch != s.SceneEpoch || b.ConnEpoch != s.ConnEpoch || b.StreamEpoch != s.StreamEpoch || b.ConfigEpoch != s.ConfigEpoch {
		q.t.Fatalf("batch epochs %d/%d/%d/%d, state %d/%d/%d/%d", b.SceneEpoch, b.ConnEpoch, b.StreamEpoch, b.ConfigEpoch,
			s.SceneEpoch, s.ConnEpoch, s.StreamEpoch, s.ConfigEpoch)
	}
}

// cmdErr は Command を流し、その型の cmdErr が返り状態が変わらないことを確かめる。
func (q *seq) cmdErr(ev Event, want error) {
	q.t.Helper()
	before := cloneState(q.m.st)
	b, err := q.m.Handle(ev)
	if b != nil {
		q.t.Fatalf("got batch %s, want none", showBatch(b))
	}
	checkErr(q.t, err, want, "")
	if d := diffState(q.m.st, before); d != "" {
		q.t.Fatalf("state changed on cmdErr:%s", d)
	}
}

// approve は Approve を呼び、結果が want と一致することを確かめる。
func (q *seq) approve(id int, a Action, ms int, want ApproveResult) {
	q.t.Helper()
	q.approveOv(id, a, OverrideNone{}, ms, want)
}

func (q *seq) approveOv(id int, a Action, ov Override, ms int, want ApproveResult) {
	q.t.Helper()
	got := q.m.Approve(id, a, ov, at(ms))
	if !reflect.DeepEqual(got, want) {
		q.t.Fatalf("Approve(%d, %+v) at %dms: got %#v, want %#v", id, a, ms, got, want)
	}
}

// exps は期待の列のシーン名と ReqID を、"Standby", 1, ... の形で確かめる。
func (q *seq) exps(want ...any) {
	q.t.Helper()
	var got []any
	for _, e := range q.m.st.Expectations {
		got = append(got, e.Scene, e.ReqID)
	}
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		q.t.Fatalf("expectations: got %v, want %v", got, want)
	}
}

func (q *seq) deadline(req, ms int) {
	q.t.Helper()
	for _, e := range q.m.st.Expectations {
		if e.ReqID == req {
			if !e.Responded || !e.Deadline.Equal(at(ms)) {
				q.t.Fatalf("r%d deadline: got %s (responded=%v), want %s", req, show(e.Deadline), e.Responded, show(at(ms)))
			}
			return
		}
	}
	q.t.Fatalf("r%d not found", req)
}

// owner は Snapshot の持ち主と戻り先を確かめる。
func (q *seq) owner(o Owner, ret string) {
	q.t.Helper()
	s := q.m.Snapshot().Standby
	if s.Owner != o || s.ReturnScene != ret {
		q.t.Fatalf("standby: got %v %q, want %v %q", s.Owner, s.ReturnScene, o, ret)
	}
}

func (q *seq) scene(name string) {
	q.t.Helper()
	if got := q.m.Snapshot().Scene; got != name {
		q.t.Fatalf("scene: got %q, want %q", got, name)
	}
}

func (q *seq) pending(ids ...int) {
	q.t.Helper()
	if len(q.m.st.Pending) != len(ids) {
		q.t.Fatalf("pending: got %v, want %v", q.m.st.Pending, ids)
	}
	for _, id := range ids {
		if _, ok := q.m.st.Pending[id]; !ok {
			q.t.Fatalf("pending: %d missing in %v", id, q.m.st.Pending)
		}
	}
}

// err は今の LastError を返す。
func (q *seq) err() *ErrorInfo { return q.m.st.LastError }

// errAt は LastError が時刻 ms の記録であることを確かめる。
func (q *seq) errAt(ms int) {
	q.t.Helper()
	got := q.m.Snapshot().LastError
	if got == nil || !got.At.Equal(at(ms)) || got.Msg == "" {
		q.t.Fatalf("LastError: got %s, want a record at %s", show(got), show(at(ms)))
	}
}

// noErr は LastError が一度も記録されていないことを確かめる。
func (q *seq) noErr() {
	q.t.Helper()
	if got := q.m.Snapshot().LastError; got != nil {
		q.t.Fatalf("LastError: got %s, want none", show(got))
	}
}

// sameErr は LastError が prev から変わっていないことを確かめる。
func (q *seq) sameErr(prev *ErrorInfo) {
	q.t.Helper()
	if got := q.m.st.LastError; got != prev {
		q.t.Fatalf("LastError: got %s, want unchanged %s", show(got), show(prev))
	}
}

// epochs は4つの Epoch を返す。
func (q *seq) epochs() [4]int {
	s := &q.m.st
	return [4]int{s.SceneEpoch, s.ConnEpoch, s.StreamEpoch, s.ConfigEpoch}
}

// epochDelta は4つの Epoch が before から scene、conn、stream、config だけ進んだことを確かめる。
func (q *seq) epochDelta(before [4]int, scene, conn, stream, config int) {
	q.t.Helper()
	now := q.epochs()
	want := [4]int{before[0] + scene, before[1] + conn, before[2] + stream, before[3] + config}
	if now != want {
		q.t.Fatalf("epochs (scene/conn/stream/config): got %v, want %v", now, want)
	}
}

func (q *seq) lastAuto(p Purpose, ms int) {
	q.t.Helper()
	got, ok := q.m.st.LastAutoSent[p]
	if !ok || !got.Equal(at(ms)) {
		q.t.Fatalf("LastAutoSent[%v]: got %s (ok=%v), want %s", p, show(got), ok, show(at(ms)))
	}
}

func (q *seq) noLastAuto() {
	q.t.Helper()
	if len(q.m.st.LastAutoSent) != 0 {
		q.t.Fatalf("LastAutoSent: got %s, want empty", show(q.m.st.LastAutoSent))
	}
}

func (q *seq) ticks(fromMs, toMs, stepMs int) {
	q.t.Helper()
	for ms := fromMs; ms <= toMs; ms += stepMs {
		q.none(tick(ms))
	}
}
