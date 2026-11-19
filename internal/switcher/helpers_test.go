package switcher

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

// テストの時刻はすべて t0 からのミリ秒で書く。
var t0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

// W は RestartMedia.Wait。テストでは2秒とする。
const W = 2 * time.Second

const wMs = int(W / time.Millisecond)

const otherScene = "Other のシーン"

var testScenes = map[Role]string{
	RoleIntro: "Intro",
	RoleLive:  "Live",
	RoleFail:  "Fail",
	RolePause: "Pause",
}

const (
	OK   = ingest.LinkOK
	LOW  = ingest.LinkLow
	DOWN = ingest.LinkDown
)

func newMachine() *Machine { return New(Config{Scenes: testScenes, RestartWait: W}) }

func ip(v int) *int { return &v }

func sceneOf(r Role) string {
	if r == RoleOther {
		return otherScene
	}
	return testScenes[r]
}

// actsFor は Purpose ごとの Action の列（期待値）。
func actsFor(p Purpose) []Action {
	switch p {
	case PurposeAutoFail, PurposeGoFail, PurposePauseOffFail:
		return []Action{SetScene{Role: RoleFail}}
	case PurposeAutoRecover, PurposeGoLive, PurposePauseOffLive:
		return []Action{RestartMedia{Wait: W}, SetScene{Role: RoleLive}}
	case PurposePauseOffIntro, PurposeStartIntro:
		return []Action{SetScene{Role: RoleIntro}}
	case PurposePauseOn:
		return []Action{SetScene{Role: RolePause}}
	case PurposeStartStream:
		return []Action{StartStream{}}
	case PurposeStopStream:
		return []Action{StopStream{}}
	}
	panic("unknown purpose")
}

// ---- Event の短い書き方 ----

func lc(ms int, s ingest.LinkState) Event { return LinkChanged{At: at(ms), State: s} }
func sc(ms int, scene string) Event       { return SceneChanged{At: at(ms), Scene: scene} }
func tick(ms int) Event                   { return Tick{At: at(ms)} }
func ssc(ms int, v bool) Event            { return StreamStateChanged{At: at(ms), Streaming: v} }
func disc(ms int) Event                   { return OBSDisconnected{At: at(ms)} }
func cmd(ms int, k CommandKind) Event     { return Command{At: at(ms), Kind: k} }
func conn(ms int, streaming bool, scene string, tr *int) Event {
	return OBSConnected{At: at(ms), Streaming: streaming, Scene: scene, TransitionMs: tr}
}
func srd(ms, req int, r RequestResult) Event {
	return SceneRequestDone{At: at(ms), ReqID: req, Result: r}
}
func bd(ms, id int, r BatchResult) Event { return BatchDone{At: at(ms), BatchID: id, Result: r} }

// ---- 状態の複製と比較 ----

func cloneState(s state) state {
	c := s
	if s.Expectations != nil {
		c.Expectations = append([]Expectation(nil), s.Expectations...)
	}
	c.Pending = make(map[int]PendingBatch, len(s.Pending))
	for k, v := range s.Pending {
		c.Pending[k] = v
	}
	c.LastAutoSent = make(map[Purpose]time.Time, len(s.LastAutoSent))
	for k, v := range s.LastAutoSent {
		c.LastAutoSent[k] = v
	}
	if s.StartWait != nil {
		w := *s.StartWait
		c.StartWait = &w
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
	case *StartWait:
		if x == nil {
			return "nil"
		}
		return fmt.Sprintf("&%+v", *x)
	case *ErrorInfo:
		if x == nil {
			return "nil"
		}
		return fmt.Sprintf("&%+v", *x)
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
		return "{" + strings.Join(parts, " ") + "}"
	}
	return fmt.Sprintf("%+v", v)
}

func showExp(e Expectation) string {
	return fmt.Sprintf("{r%d %v reg=%s ov=%T%+v tr=%s resp=%v at=%s res=%v dl=%s}",
		e.ReqID, e.Role, show(e.RegisteredAt), e.Override, e.Override, show(e.TransitionMs),
		e.Responded, show(e.RespondedAt), e.Result, show(e.Deadline))
}

// ---- 入口ごとの表の行を流す仕組み ----

// baseState は表の「特記がなければ」の状態: 接続、配信中、Live、OK、保持なし、
// 未完了なし、期待なし。Epoch と通し番号は 0 / 1 以外にしておき、動いたかがわかるようにする。
func baseState(s *state) {
	s.Connected = true
	s.Streaming = true
	setRole(s, RoleLive)
	s.PrevConnScene = "Intro"
	s.PrevConnStreaming = false
	s.Link = OK
	s.TransitionMs = ip(300)
	s.SceneEpoch = 3
	s.ConnEpoch = 2
	s.StreamEpoch = 4
	s.NextBatchID = 10
	s.NextReqID = 20
	s.NextStartID = 5
}

func setRole(s *state, r Role) {
	s.Role = r
	s.Scene = sceneOf(r)
	if r != RolePause {
		s.PausePrev = RoleUnknown
	}
}

// pend は今の Epoch で未完了のバッチを足す。
func pend(s *state, id int, p Purpose) {
	s.Pending[id] = PendingBatch{BatchID: id, Purpose: p, SceneEpoch: s.SceneEpoch,
		ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch}
}

// expect は応答待ちの期待を返す。
func expect(req int, r Role) Expectation {
	return Expectation{ReqID: req, Role: r, RegisteredAt: at(0), Override: OverrideNone{},
		TransitionMs: ip(300)}
}

// responded は応答済みの期待を返す。
func responded(req int, r Role, res RequestResult, respMs, deadlineMs int) Expectation {
	e := expect(req, r)
	e.Responded = true
	e.RespondedAt = at(respMs)
	e.Result = res
	e.Deadline = at(deadlineMs)
	return e
}

func hold(s *state, sinceMs int) {
	s.Hold = true
	s.HoldSince = at(sinceMs)
}

func unhold(s *state) {
	s.Hold = false
	s.HoldSince = time.Time{}
}

// wantB は表の B をそのまま期待の状態に当てはめ、返るべきバッチを返す。
func wantB(s *state, p Purpose, atT time.Time) *Batch {
	id := s.NextBatchID
	s.NextBatchID++
	pb := PendingBatch{BatchID: id, Purpose: p, SceneEpoch: s.SceneEpoch,
		ConnEpoch: s.ConnEpoch, StreamEpoch: s.StreamEpoch}
	if p == PurposeStartIntro || p == PurposeStartStream {
		pb.StartID = s.StartWait.ID
	}
	s.Pending[id] = pb
	if p.IsAuto() {
		s.LastAutoSent[p] = atT
	}
	return &Batch{BatchID: id, Purpose: p, SceneEpoch: s.SceneEpoch, ConnEpoch: s.ConnEpoch,
		StreamEpoch: s.StreamEpoch, Actions: actsFor(p)}
}

type evRow struct {
	name   string
	setup  func(s *state)
	ev     Event
	err    error   // 返るべき cmdErr（型だけ比べる）
	out    Purpose // 0 ならバッチなし
	change func(s *state)
	recErr bool                           // LastError を記録する
	after  func(t *testing.T, m *Machine) // 追加の確かめ
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
			checkErr(t, err, r.err)
			want := cloneState(before)
			if r.change != nil {
				r.change(&want)
			}
			if r.out != 0 {
				wb := wantB(&want, r.out, r.ev.eventAt())
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
			if r.after != nil {
				r.after(t, m)
			}
		})
	}
}

func checkErr(t *testing.T, got, want error) {
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

// ---- Approve の表の行を流す仕組み ----

type apRow struct {
	name     string
	setup    func(s *state) // 未完了のバッチは id=1 で足す
	action   Action
	override Override
	atMs     int
	res      ApproveResult // Proceed / Skip / Reject / Replace（Replace は Purpose だけ見る）
	register Role          // Proceed で期待を登録するなら、その役割
	replace  Purpose
	change   func(s *state)
}

func runApRows(t *testing.T, rows []apRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			m := newMachine()
			baseState(&m.st)
			r.setup(&m.st)
			before := cloneState(m.st)
			ov := r.override
			if ov == nil {
				ov = OverrideNone{}
			}
			atT := at(r.atMs)
			got := m.Approve(1, r.action, ov, atT)
			want := cloneState(before)
			if r.change != nil {
				r.change(&want)
			}
			switch exp := r.res.(type) {
			case Proceed:
				p, ok := got.(Proceed)
				if !ok {
					t.Fatalf("result: got %#v, want Proceed", got)
				}
				if r.register != RoleUnknown {
					if p.ReqID != before.NextReqID {
						t.Errorf("ReqID: got %d, want %d", p.ReqID, before.NextReqID)
					}
					want.Expectations = append(want.Expectations, Expectation{
						ReqID: before.NextReqID, Role: r.register, RegisteredAt: atT,
						Override: ov, TransitionMs: before.TransitionMs})
					want.NextReqID++
				} else if p.ReqID != exp.ReqID {
					t.Errorf("ReqID: got %d, want %d", p.ReqID, exp.ReqID)
				}
			case Replace:
				rp, ok := got.(Replace)
				if !ok {
					t.Fatalf("result: got %#v, want Replace", got)
				}
				pb := want.Pending[1]
				pb.Replaced = true
				want.Pending[1] = pb
				wb := wantB(&want, r.replace, atT)
				if !reflect.DeepEqual(rp.Batch, wb) {
					t.Errorf("replace batch: got %+v, want %+v", rp.Batch, wb)
				}
			default:
				if !reflect.DeepEqual(got, r.res) {
					t.Fatalf("result: got %#v, want %#v", got, r.res)
				}
			}
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

// prep は準備 P(シーン, 配信, 受信状態) を行う。tr は全体のトランジションの長さ。
func (q *seq) prep(scene string, streaming bool, link ingest.LinkState, tr *int) *seq {
	q.t.Helper()
	q.none(lc(0, link))
	q.none(conn(0, streaming, scene, tr))
	return q
}

func (q *seq) p(scene string, streaming bool, link ingest.LinkState) *seq {
	q.t.Helper()
	return q.prep(scene, streaming, link, ip(300))
}

// none は Event を流し、バッチも cmdErr も返らないことを確かめる。
func (q *seq) none(ev Event) {
	q.t.Helper()
	b, err := q.m.Handle(ev)
	if err != nil || b != nil {
		q.t.Fatalf("%T at %s: got batch %+v err %v, want none", ev, show(ev.eventAt()), b, err)
	}
}

// batch は Event を流し、id と Purpose のバッチが返ることを確かめる。
func (q *seq) batch(ev Event, id int, p Purpose) *Batch {
	q.t.Helper()
	b, err := q.m.Handle(ev)
	if err != nil {
		q.t.Fatalf("%T at %s: cmdErr %v", ev, show(ev.eventAt()), err)
	}
	q.checkBatch(b, id, p)
	return b
}

func (q *seq) checkBatch(b *Batch, id int, p Purpose) {
	q.t.Helper()
	if b == nil {
		q.t.Fatalf("got no batch, want %v#%d", p, id)
	}
	if b.BatchID != id || b.Purpose != p || !reflect.DeepEqual(b.Actions, actsFor(p)) {
		q.t.Fatalf("got %v#%d %+v, want %v#%d %+v", b.Purpose, b.BatchID, b.Actions, p, id, actsFor(p))
	}
	if b.SceneEpoch != q.m.st.SceneEpoch || b.ConnEpoch != q.m.st.ConnEpoch || b.StreamEpoch != q.m.st.StreamEpoch {
		q.t.Fatalf("batch epochs %d/%d/%d, state %d/%d/%d", b.SceneEpoch, b.ConnEpoch, b.StreamEpoch,
			q.m.st.SceneEpoch, q.m.st.ConnEpoch, q.m.st.StreamEpoch)
	}
}

// cmdErr は Command を流し、その型の cmdErr が返り状態が変わらないことを確かめる。
func (q *seq) cmdErr(ev Event, want error) {
	q.t.Helper()
	before := cloneState(q.m.st)
	b, err := q.m.Handle(ev)
	if b != nil {
		q.t.Fatalf("got batch %+v, want none", b)
	}
	checkErr(q.t, err, want)
	if d := diffState(q.m.st, before); d != "" {
		q.t.Fatalf("state changed on cmdErr:%s", d)
	}
}

// approve は Approve を呼び、結果が want と一致することを確かめる。
func (q *seq) approve(id int, a Action, ms int, want ApproveResult) ApproveResult {
	q.t.Helper()
	return q.approveOv(id, a, OverrideNone{}, ms, want)
}

func (q *seq) approveOv(id int, a Action, ov Override, ms int, want ApproveResult) ApproveResult {
	q.t.Helper()
	got := q.m.Approve(id, a, ov, at(ms))
	if _, isRep := want.(Replace); isRep {
		if _, ok := got.(Replace); !ok {
			q.t.Fatalf("Approve(%d, %+v) at %dms: got %#v, want Replace", id, a, ms, got)
		}
		return got
	}
	if !reflect.DeepEqual(got, want) {
		q.t.Fatalf("Approve(%d, %+v) at %dms: got %#v, want %#v", id, a, ms, got, want)
	}
	return got
}

// expRoles は期待の列の役割と ReqID を確かめる。
func (q *seq) expRoles(want ...any) {
	q.t.Helper()
	var got []any
	for _, e := range q.m.st.Expectations {
		got = append(got, e.Role, e.ReqID)
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

func (q *seq) hold(active bool, sinceMs int) {
	q.t.Helper()
	s := q.m.Snapshot().Hold
	if s.Active != active {
		q.t.Fatalf("hold: got %v, want %v", s.Active, active)
	}
	if active && !s.Since.Equal(at(sinceMs)) {
		q.t.Fatalf("hold since: got %s, want %s", show(s.Since), show(at(sinceMs)))
	}
	if !active && !s.Since.IsZero() {
		q.t.Fatalf("hold since: got %s, want zero", show(s.Since))
	}
}

func (q *seq) role(r Role) {
	q.t.Helper()
	if got := q.m.Snapshot().Role; got != r {
		q.t.Fatalf("role: got %v, want %v", got, r)
	}
}

func (q *seq) pausePrev(r Role) {
	q.t.Helper()
	if q.m.st.PausePrev != r {
		q.t.Fatalf("PausePrev: got %v, want %v", q.m.st.PausePrev, r)
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

func (q *seq) lastErr(set bool) {
	q.t.Helper()
	if got := q.m.Snapshot().LastError; (got != nil) != set {
		q.t.Fatalf("LastError: got %s, want set=%v", show(got), set)
	}
}

// epochs は3つの Epoch を返す。
func (q *seq) epochs() (scene, conn, stream int) {
	return q.m.st.SceneEpoch, q.m.st.ConnEpoch, q.m.st.StreamEpoch
}

func (q *seq) ticks(fromMs, toMs, stepMs int) {
	q.t.Helper()
	for ms := fromMs; ms <= toMs; ms += stepMs {
		q.none(tick(ms))
	}
}
