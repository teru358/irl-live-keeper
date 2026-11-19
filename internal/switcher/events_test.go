package switcher

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/teru358/irl-live-keeper/internal/ingest"
)

const evMs = 50_000 // 1つの Event を流すケースの時刻

func link(l ingest.LinkState) func(*state) { return func(s *state) { s.Link = l } }

func setLink(l ingest.LinkState) func(*state) { return link(l) }

func all(fs ...func(*state)) func(*state) {
	return func(s *state) {
		for _, f := range fs {
			if f != nil {
				f(s)
			}
		}
	}
}

func roleIs(r Role) func(*state) { return func(s *state) { setRole(s, r) } }

func TestLinkChanged(t *testing.T) {
	rows := []evRow{
		{name: "LC-1", setup: link(OK), ev: lc(evMs, LOW), out: PurposeAutoFail, change: setLink(LOW)},
		{name: "LC-2", setup: link(OK), ev: lc(evMs, DOWN), out: PurposeAutoFail, change: setLink(DOWN)},
		{name: "LC-3", setup: all(link(LOW), func(s *state) { pend(s, 1, PurposeAutoFail) }),
			ev: lc(evMs, DOWN), change: setLink(DOWN)},
		{name: "LC-4", setup: all(link(LOW), func(s *state) {
			s.Expectations = []Expectation{responded(1, RoleFail, RequestOK, evMs-1000, evMs+2000)}
		}), ev: lc(evMs, DOWN), change: setLink(DOWN)},
		{name: "LC-5", setup: all(link(LOW), func(s *state) {
			s.LastAutoSent[PurposeAutoFail] = at(evMs - 1000)
			s.LastError = &ErrorInfo{At: at(evMs - 900), Msg: "前の失敗"}
		}), ev: lc(evMs, DOWN), out: PurposeAutoFail, change: setLink(DOWN)},
	}
	for _, from := range []ingest.LinkState{LOW, DOWN} {
		rows = append(rows, evRow{name: "LC-6/" + from.String() + "→OK", setup: all(roleIs(RoleFail), link(from)),
			ev: lc(evMs, OK), out: PurposeAutoRecover, change: setLink(OK)})
	}
	rows = append(rows,
		evRow{name: "LC-7/pending", setup: all(roleIs(RoleFail), link(LOW), func(s *state) { pend(s, 1, PurposeAutoRecover) }),
			ev: lc(evMs, OK), change: setLink(OK)},
		evRow{name: "LC-7/expectation", setup: all(roleIs(RoleFail), link(DOWN), func(s *state) {
			s.Expectations = []Expectation{expect(1, RoleLive)}
		}), ev: lc(evMs, OK), change: setLink(OK)},
	)
	for _, c := range [][2]ingest.LinkState{{DOWN, LOW}, {OK, LOW}, {OK, DOWN}} {
		rows = append(rows, evRow{name: fmt.Sprintf("LC-8/Fail %v→%v", c[0], c[1]),
			setup: all(roleIs(RoleFail), link(c[0])), ev: lc(evMs, c[1]), change: setLink(c[1])})
	}
	for _, from := range []ingest.LinkState{LOW, DOWN} {
		rows = append(rows, evRow{name: "LC-9/" + from.String() + "→OK",
			setup: link(from), ev: lc(evMs, OK), change: setLink(OK)})
	}
	changes := [][2]ingest.LinkState{{OK, LOW}, {OK, DOWN}, {LOW, DOWN}, {DOWN, LOW}, {LOW, OK}, {DOWN, OK}}
	for _, r := range []Role{RoleIntro, RolePause, RoleOther} {
		for _, c := range changes {
			rows = append(rows, evRow{name: fmt.Sprintf("LC-10/%v %v→%v", r, c[0], c[1]),
				setup: all(roleIs(r), link(c[0]), func(s *state) {
					if s.Role == RolePause {
						s.PausePrev = RoleLive
					}
				}), ev: lc(evMs, c[1]), change: setLink(c[1])})
		}
	}
	notStreaming := func(s *state) { s.Streaming = false }
	rows = append(rows,
		evRow{name: "LC-11/Live OK→LOW", setup: all(notStreaming, link(OK)), ev: lc(evMs, LOW), change: setLink(LOW)},
		evRow{name: "LC-11/Fail LOW→OK", setup: all(notStreaming, roleIs(RoleFail), link(LOW)), ev: lc(evMs, OK), change: setLink(OK)},
	)
	disconnected := func(s *state) { s.Connected = false }
	for _, r := range []Role{RoleLive, RoleFail, RoleIntro} {
		for _, c := range changes {
			rows = append(rows, evRow{name: fmt.Sprintf("LC-12/%v %v→%v", r, c[0], c[1]),
				setup: all(disconnected, roleIs(r), link(c[0])), ev: lc(evMs, c[1]), change: setLink(c[1])})
		}
	}
	held := func(s *state) { hold(s, evMs-5000) }
	for _, to := range []ingest.LinkState{LOW, DOWN} {
		rows = append(rows, evRow{name: "LC-13/OK→" + to.String(), setup: all(held, link(OK)),
			ev: lc(evMs, to), out: PurposeAutoFail, change: all(setLink(to), unhold)})
	}
	for _, from := range []ingest.LinkState{LOW, DOWN} {
		rows = append(rows,
			evRow{name: "LC-14/" + from.String() + "→OK", setup: all(held, link(from)),
				ev: lc(evMs, OK), change: all(setLink(OK), unhold)},
			evRow{name: "LC-15/" + from.String() + "→OK", setup: all(held, roleIs(RoleFail), link(from)),
				ev: lc(evMs, OK), out: PurposeAutoRecover, change: all(setLink(OK), unhold)},
			evRow{name: "LC-16/OK→" + from.String(), setup: all(held, roleIs(RoleFail), link(OK)),
				ev: lc(evMs, from), change: all(setLink(from), unhold)},
		)
	}
	for _, r := range []Role{RoleLive, RoleFail} {
		for _, c := range [][2]ingest.LinkState{{LOW, DOWN}, {DOWN, LOW}} {
			rows = append(rows, evRow{name: fmt.Sprintf("LC-17/%v %v→%v", r, c[0], c[1]),
				setup: all(held, roleIs(r), link(c[0])), ev: lc(evMs, c[1]), change: setLink(c[1])})
		}
	}
	for _, c := range [][2]ingest.LinkState{{OK, LOW}, {OK, DOWN}, {LOW, OK}, {DOWN, OK}} {
		rows = append(rows, evRow{name: fmt.Sprintf("LC-18/%v→%v", c[0], c[1]),
			setup: all(held, disconnected, link(c[0])), ev: lc(evMs, c[1]), change: all(setLink(c[1]), unhold)})
	}
	for _, l := range []ingest.LinkState{OK, LOW, DOWN} {
		rows = append(rows, evRow{name: "LC-19/held " + l.String(), setup: all(held, link(l)), ev: lc(evMs, l)})
	}
	rows = append(rows, evRow{name: "LC-19/Live LOW no hold", setup: link(LOW), ev: lc(evMs, LOW)})
	runEvRows(t, rows)
}

func TestOBSConnected(t *testing.T) {
	// 再接続の前提: 切断中。切断の直前の役割と配信状態は引数のとおり。
	disconnectedFrom := func(r Role, streaming bool) func(*state) {
		return func(s *state) {
			setRole(s, r)
			s.Streaming = streaming
			s.Connected = false
			s.PrevConnScene = s.Scene
			s.PrevConnStreaming = streaming
			s.TransitionMs = ip(300)
		}
	}
	// 共通の変化
	connected := func(streaming bool, r Role, tr *int) func(*state) {
		return func(s *state) {
			s.Connected = true
			s.Streaming = streaming
			setRole(s, r)
			s.PausePrev = RoleUnknown
			s.TransitionMs = tr
		}
	}
	streamUp := func(s *state) { s.StreamEpoch++ }
	held := func(s *state) { hold(s, evMs-5000) }
	fresh := func(s *state) {
		m := newMachine()
		*s = m.st
	}
	rows := []evRow{
		{name: "OC-1", setup: fresh, ev: conn(evMs, false, "Live", ip(300)), change: connected(false, RoleLive, ip(300))},
		{name: "OC-2/Live DOWN", setup: fresh, ev: conn(evMs, true, "Live", ip(300)),
			out: PurposeAutoFail, change: all(connected(true, RoleLive, ip(300)), streamUp)},
		{name: "OC-2/Intro", setup: fresh, ev: conn(evMs, true, "Intro", ip(300)),
			change: all(connected(true, RoleIntro, ip(300)), streamUp)},
	}
	for _, l := range []ingest.LinkState{LOW, DOWN} {
		rows = append(rows, evRow{name: "OC-3/" + l.String(), setup: all(disconnectedFrom(RoleLive, true), link(l)),
			ev: conn(evMs, true, "Live", ip(300)), out: PurposeAutoFail, change: connected(true, RoleLive, ip(300))})
	}
	rows = append(rows, evRow{name: "OC-4", setup: all(disconnectedFrom(RoleFail, true), link(OK)),
		ev: conn(evMs, true, "Fail", ip(300)), out: PurposeAutoRecover, change: connected(true, RoleFail, ip(300))})
	for _, c := range []struct {
		r Role
		l ingest.LinkState
	}{{RoleLive, OK}, {RoleFail, LOW}, {RoleFail, DOWN}, {RoleIntro, LOW}, {RolePause, OK}, {RolePause, LOW}, {RoleOther, LOW}} {
		rows = append(rows, evRow{name: fmt.Sprintf("OC-5/%v %v", c.r, c.l), setup: all(disconnectedFrom(c.r, true), link(c.l)),
			ev: conn(evMs, true, sceneOf(c.r), ip(300)), change: connected(true, c.r, ip(300))})
	}
	rows = append(rows,
		evRow{name: "OC-6/Live LOW", setup: all(disconnectedFrom(RoleLive, false), link(LOW)),
			ev: conn(evMs, false, "Live", ip(300)), change: connected(false, RoleLive, ip(300))},
		evRow{name: "OC-6/Fail OK", setup: all(disconnectedFrom(RoleFail, false), link(OK)),
			ev: conn(evMs, false, "Fail", ip(300)), change: connected(false, RoleFail, ip(300))},
		evRow{name: "OC-7/Live LOW", setup: all(disconnectedFrom(RoleLive, true), link(LOW), held),
			ev: conn(evMs, true, "Live", ip(300)), change: connected(true, RoleLive, ip(300))},
		evRow{name: "OC-7/Fail OK", setup: all(disconnectedFrom(RoleFail, true), link(OK), held),
			ev: conn(evMs, true, "Fail", ip(300)), change: connected(true, RoleFail, ip(300))},
		evRow{name: "OC-8/Live→Fail OK", setup: all(disconnectedFrom(RoleLive, true), link(OK), held),
			ev: conn(evMs, true, "Fail", ip(300)), out: PurposeAutoRecover, change: all(connected(true, RoleFail, ip(300)), unhold)},
		evRow{name: "OC-8/Fail→Intro", setup: all(disconnectedFrom(RoleFail, true), link(OK), held),
			ev: conn(evMs, true, "Intro", ip(300)), change: all(connected(true, RoleIntro, ip(300)), unhold)},
		evRow{name: "OC-9", setup: all(disconnectedFrom(RoleLive, true), link(LOW), held),
			ev: conn(evMs, false, "Live", ip(300)), change: all(connected(false, RoleLive, ip(300)), unhold, streamUp)},
		evRow{name: "OC-10/Live LOW", setup: all(disconnectedFrom(RoleLive, false), link(LOW)),
			ev: conn(evMs, true, "Live", ip(300)), out: PurposeAutoFail, change: all(connected(true, RoleLive, ip(300)), streamUp)},
		evRow{name: "OC-10/Intro", setup: all(disconnectedFrom(RoleIntro, false), link(LOW)),
			ev: conn(evMs, true, "Intro", ip(300)), change: all(connected(true, RoleIntro, ip(300)), streamUp)},
		evRow{name: "OC-11", setup: all(disconnectedFrom(RolePause, true), func(s *state) { s.PausePrev = RoleLive }),
			ev: conn(evMs, true, "Pause", ip(300)), change: connected(true, RolePause, ip(300))},
		evRow{name: "OC-12/Live OK", setup: all(disconnectedFrom(RoleLive, true), link(OK)),
			ev: conn(evMs, true, "Live", nil), change: connected(true, RoleLive, nil)},
		evRow{name: "OC-12/Live LOW", setup: all(disconnectedFrom(RoleLive, true), link(LOW)),
			ev: conn(evMs, true, "Live", nil), out: PurposeAutoFail, change: connected(true, RoleLive, nil)},
		evRow{name: "OC-13", setup: all(func(s *state) { pend(s, 1, PurposeAutoFail) }, disconnectedFrom(RoleLive, true), link(LOW),
			func(s *state) { s.ConnEpoch++ }),
			ev: conn(evMs, true, "Live", ip(300)), change: connected(true, RoleLive, ip(300))},
	)
	runEvRows(t, rows)
}

func TestOBSDisconnected(t *testing.T) {
	common := func(s *state) {
		s.Connected = false
		s.ConnEpoch++
		s.Expectations = nil
		s.PrevConnScene = s.Scene
		s.PrevConnStreaming = s.Streaming
	}
	rows := []evRow{
		{name: "OD-1", setup: all(link(LOW), func(s *state) {
			pend(s, 1, PurposeAutoFail)
			s.Expectations = []Expectation{expect(1, RoleFail), responded(2, RolePause, RequestOK, evMs-100, evMs+3000)}
		}), ev: disc(evMs), change: common},
		{name: "OD-1/Pause keeps PausePrev", setup: all(roleIs(RolePause), func(s *state) {
			s.PausePrev = RoleFail
			s.Expectations = []Expectation{expect(1, RoleLive)}
		}), ev: disc(evMs), change: common},
		{name: "OD-2", setup: all(roleIs(RoleFail), func(s *state) { hold(s, evMs-3000) }), ev: disc(evMs), change: common},
	}
	runEvRows(t, rows)
}

func TestSceneChangedExpected(t *testing.T) {
	scene := func(r Role) func(*state) { return func(s *state) { setRole(s, r) } }
	clearExp := func(s *state) { s.Expectations = nil }
	expsOf := func(es ...Expectation) func(*state) {
		return func(s *state) { s.Expectations = es }
	}
	var rows []evRow
	for _, e := range []struct {
		name string
		e    Expectation
	}{
		{"before response", expect(1, RoleFail)},
		{"after OK", responded(1, RoleFail, RequestOK, evMs-1000, evMs+2300)},
		{"after Timeout", responded(1, RoleFail, RequestTimeout, evMs-1000, evMs+2300)},
		{"at deadline", responded(1, RoleFail, RequestOK, evMs-3300, evMs)},
	} {
		rows = append(rows, evRow{name: "SCE-1/" + e.name, setup: all(link(LOW), expsOf(e.e)),
			ev: sc(evMs, "Fail"), change: all(scene(RoleFail), clearExp)})
	}
	rows = append(rows,
		evRow{name: "SCE-2", setup: all(link(OK), expsOf(expect(1, RoleFail))),
			ev: sc(evMs, "Fail"), out: PurposeAutoRecover, change: all(scene(RoleFail), clearExp)},
		evRow{name: "SCE-3", setup: all(roleIs(RoleFail), link(LOW), expsOf(expect(1, RoleLive))),
			ev: sc(evMs, "Live"), out: PurposeAutoFail, change: all(scene(RoleLive), clearExp)},
		evRow{name: "SCE-4", setup: all(roleIs(RoleFail), link(OK), expsOf(expect(1, RoleLive))),
			ev: sc(evMs, "Live"), change: all(scene(RoleLive), clearExp)},
		evRow{name: "SCE-5", setup: all(link(LOW), expsOf(expect(1, RoleFail), expect(2, RolePause))),
			ev: sc(evMs, "Pause"), change: all(scene(RolePause), clearExp, func(s *state) { s.PausePrev = RoleLive })},
		evRow{name: "SCE-6", setup: all(link(OK), expsOf(expect(1, RoleFail), expect(2, RolePause))),
			ev: sc(evMs, "Fail"), change: all(scene(RoleFail), expsOf(expect(2, RolePause)))},
		evRow{name: "SCE-7", setup: all(link(LOW), expsOf(expect(1, RolePause), expect(2, RoleFail))),
			ev: sc(evMs, "Pause"), change: all(scene(RolePause), expsOf(expect(2, RoleFail)), func(s *state) { s.PausePrev = RoleLive })},
	)
	for _, r := range []Role{RoleIntro, RoleLive, RoleFail} {
		rows = append(rows, evRow{name: "SCE-8/" + r.String(), setup: all(roleIs(r), link(OK), expsOf(expect(1, RolePause))),
			ev: sc(evMs, "Pause"), change: all(scene(RolePause), clearExp, func(s *state) { s.PausePrev = r })})
	}
	runEvRows(t, rows)
}

func TestSceneChangedManual(t *testing.T) {
	manual := func(r Role) func(*state) {
		return func(s *state) {
			setRole(s, r)
			s.SceneEpoch++
			s.Expectations = nil
		}
	}
	held := func(s *state) { hold(s, evMs-5000) }
	enter := func(s *state) { hold(s, evMs) }
	prev := func(r Role) func(*state) { return func(s *state) { s.PausePrev = r } }
	var rows []evRow
	for _, from := range []Role{RoleIntro, RoleFail, RoleOther} {
		rows = append(rows, evRow{name: "SCM-1/" + from.String() + "→Live LOW", setup: all(roleIs(from), link(LOW)),
			ev: sc(evMs, "Live"), change: all(manual(RoleLive), enter)})
	}
	rows = append(rows,
		evRow{name: "SCM-1/Fail→Live OK", setup: all(roleIs(RoleFail), link(OK)), ev: sc(evMs, "Live"), change: all(manual(RoleLive), enter)},
		evRow{name: "SCM-2/Live→Fail OK", setup: link(OK), ev: sc(evMs, "Fail"), change: all(manual(RoleFail), enter)},
		evRow{name: "SCM-2/Intro→Fail LOW", setup: all(roleIs(RoleIntro), link(LOW)), ev: sc(evMs, "Fail"), change: all(manual(RoleFail), enter)},
	)
	for _, to := range []Role{RoleIntro, RoleOther} {
		rows = append(rows, evRow{name: "SCM-3/→" + to.String(), setup: all(held, link(LOW)),
			ev: sc(evMs, sceneOf(to)), change: all(manual(to), unhold)})
	}
	for _, from := range []Role{RoleIntro, RoleLive, RoleFail} {
		rows = append(rows, evRow{name: "SCM-4/" + from.String() + "→Pause", setup: all(roleIs(from), link(LOW)),
			ev: sc(evMs, "Pause"), change: all(manual(RolePause), prev(from))})
	}
	for _, from := range []Role{RoleLive, RoleFail} {
		rows = append(rows, evRow{name: "SCM-4/held " + from.String() + "→Pause", setup: all(roleIs(from), held),
			ev: sc(evMs, "Pause"), change: all(manual(RolePause), prev(from), unhold)})
	}
	rows = append(rows,
		evRow{name: "SCM-5", setup: roleIs(RoleOther), ev: sc(evMs, "Pause"), change: all(manual(RolePause), prev(RoleUnknown))},
		evRow{name: "SCM-5/not streaming", setup: all(roleIs(RoleOther), func(s *state) { s.Streaming = false }),
			ev: sc(evMs, "Pause"), change: all(manual(RolePause), prev(RoleUnknown))},
	)
	for _, to := range []Role{RoleLive, RoleFail} {
		rows = append(rows, evRow{name: "SCM-6/→" + to.String(), setup: all(roleIs(RolePause), prev(RoleIntro)),
			ev: sc(evMs, sceneOf(to)), change: all(manual(to), enter)})
	}
	for _, to := range []Role{RoleIntro, RoleOther} {
		rows = append(rows, evRow{name: "SCM-7/→" + to.String(), setup: all(roleIs(RolePause), prev(RoleLive)),
			ev: sc(evMs, sceneOf(to)), change: manual(to)})
	}
	notStreaming := func(s *state) { s.Streaming = false }
	for _, to := range []Role{RoleLive, RoleFail} {
		rows = append(rows, evRow{name: "SCM-8/→" + to.String(), setup: all(roleIs(RoleIntro), notStreaming, link(LOW)),
			ev: sc(evMs, sceneOf(to)), change: manual(to)})
	}
	rows = append(rows,
		evRow{name: "SCM-9", setup: func(s *state) { s.Expectations = []Expectation{expect(1, RoleFail)} },
			ev: sc(evMs, "Pause"), change: all(manual(RolePause), prev(RoleLive))},
		evRow{name: "SCM-10/Fail", setup: all(link(LOW), func(s *state) {
			s.Expectations = []Expectation{responded(1, RoleFail, RequestOK, evMs-3400, evMs-1)}
		}), ev: sc(evMs, "Fail"), change: all(manual(RoleFail), enter)},
		evRow{name: "SCM-10/Pause", setup: all(link(LOW), func(s *state) {
			s.Expectations = []Expectation{responded(1, RolePause, RequestTimeout, evMs-3400, evMs-1)}
		}), ev: sc(evMs, "Pause"), change: all(manual(RolePause), prev(RoleLive))},
		evRow{name: "SCM-11/Live→Fail", setup: all(held, link(LOW)), ev: sc(evMs, "Fail"), change: manual(RoleFail)},
		evRow{name: "SCM-11/Fail→Live", setup: all(roleIs(RoleFail), held, link(OK)), ev: sc(evMs, "Live"), change: manual(RoleLive)},
		evRow{name: "SCM-13/AutoFail", setup: all(link(LOW), func(s *state) { pend(s, 1, PurposeAutoFail) }),
			ev: sc(evMs, "Pause"), change: all(manual(RolePause), prev(RoleLive))},
		evRow{name: "SCM-13/AutoRecover", setup: all(roleIs(RoleFail), link(OK), func(s *state) { pend(s, 1, PurposeAutoRecover) }),
			ev: sc(evMs, "Live"), change: all(manual(RoleLive), enter)},
	)
	runEvRows(t, rows)
}

func TestTransitionChanged(t *testing.T) {
	rows := []evRow{
		{name: "TC-1", ev: TransitionChanged{At: at(evMs), TransitionMs: ip(5000)},
			change: func(s *state) { s.TransitionMs = ip(5000) }},
		{name: "TC-2", ev: TransitionChanged{At: at(evMs), TransitionMs: nil},
			change: func(s *state) { s.TransitionMs = nil }},
		{name: "TC-3", setup: func(s *state) { s.Expectations = []Expectation{expect(1, RoleFail)} },
			ev:     TransitionChanged{At: at(evMs), TransitionMs: ip(5000)},
			change: func(s *state) { s.TransitionMs = ip(5000) },
			after: func(t *testing.T, m *Machine) {
				if _, err := m.Handle(srd(evMs+100, 1, RequestOK)); err != nil {
					t.Fatal(err)
				}
				if got, want := m.st.Expectations[0].Deadline, at(evMs+100+300+3000); !got.Equal(want) {
					t.Errorf("deadline: got %s, want %s", show(got), show(want))
				}
			}},
	}
	runEvRows(t, rows)
}

func TestStreamStateChanged(t *testing.T) {
	notStreaming := func(s *state) { s.Streaming = false }
	up := func(s *state) {
		s.Streaming = true
		s.StreamEpoch++
	}
	down := func(s *state) {
		s.Streaming = false
		s.StreamEpoch++
		s.Expectations = nil
		s.LastAutoSent = map[Purpose]time.Time{}
	}
	rows := []evRow{
		{name: "SST-1/LOW", setup: all(notStreaming, link(LOW)), ev: ssc(evMs, true), out: PurposeAutoFail, change: up},
		{name: "SST-1/DOWN", setup: all(notStreaming, link(DOWN)), ev: ssc(evMs, true), out: PurposeAutoFail, change: up},
		{name: "SST-2", setup: all(notStreaming, roleIs(RoleFail), link(OK)), ev: ssc(evMs, true), out: PurposeAutoRecover, change: up},
	}
	for _, c := range []struct {
		r Role
		l ingest.LinkState
	}{{RoleLive, OK}, {RoleFail, LOW}, {RoleFail, DOWN}, {RoleIntro, LOW}, {RolePause, LOW}, {RoleOther, OK}} {
		rows = append(rows, evRow{name: fmt.Sprintf("SST-3/%v %v", c.r, c.l), setup: all(notStreaming, roleIs(c.r), link(c.l)),
			ev: ssc(evMs, true), change: up})
	}
	rows = append(rows,
		evRow{name: "SST-6", setup: link(LOW), ev: ssc(evMs, true)},
		evRow{name: "SSF-1", setup: all(link(LOW), func(s *state) {
			s.Expectations = []Expectation{expect(1, RoleFail), responded(2, RolePause, RequestOK, evMs-100, evMs+3000)}
			s.LastAutoSent[PurposeAutoFail] = at(evMs - 2000)
			s.LastAutoSent[PurposeAutoRecover] = at(evMs - 9000)
		}), ev: ssc(evMs, false), change: down},
		evRow{name: "SSF-2", setup: all(roleIs(RoleFail), func(s *state) { hold(s, evMs-4000) }),
			ev: ssc(evMs, false), change: all(down, unhold)},
		evRow{name: "SSF-3", setup: all(roleIs(RolePause), func(s *state) { s.PausePrev = RoleLive }),
			ev: ssc(evMs, false), change: all(down, func(s *state) { s.PausePrev = RoleUnknown })},
		evRow{name: "SSF-4", setup: all(link(LOW), func(s *state) {
			pend(s, 1, PurposeAutoFail)
			pend(s, 2, PurposeStopStream)
		}), ev: ssc(evMs, false), change: down},
		evRow{name: "SSF-5", setup: all(notStreaming, func(s *state) {
			s.Expectations = []Expectation{expect(1, RoleIntro)}
			s.LastAutoSent[PurposeAutoFail] = at(evMs - 2000)
		}), ev: ssc(evMs, false)},
	)
	runEvRows(t, rows)
}

func TestSceneRequestDone(t *testing.T) {
	two := func(s *state) {
		s.Expectations = []Expectation{expect(1, RoleFail), expect(2, RolePause)}
	}
	respond := func(i int, res RequestResult, deadlineMs int) func(*state) {
		return func(s *state) {
			e := &s.Expectations[i]
			e.Responded = true
			e.RespondedAt = at(evMs)
			e.Result = res
			e.Deadline = at(deadlineMs)
		}
	}
	rows := []evRow{
		{name: "SRD-OK-1", setup: two, ev: srd(evMs, 1, RequestOK), change: respond(0, RequestOK, evMs+3300)},
		{name: "SRD-OK-1/second", setup: two, ev: srd(evMs, 2, RequestOK), change: respond(1, RequestOK, evMs+3300)},
		{name: "SRD-OK-2/unknown", setup: two, ev: srd(evMs, 7, RequestOK)},
		{name: "SRD-OK-2/empty", ev: srd(evMs, 1, RequestOK)},
		{name: "SRD-TO-1", setup: two, ev: srd(evMs, 1, RequestTimeout), change: respond(0, RequestTimeout, evMs+3300)},
		{name: "SRD-ERR-1", setup: two, ev: srd(evMs, 1, RequestError),
			change: func(s *state) { s.Expectations = []Expectation{expect(2, RolePause)} }},
		{name: "SRD-ERR-1/second", setup: two, ev: srd(evMs, 2, RequestError),
			change: func(s *state) { s.Expectations = []Expectation{expect(1, RoleFail)} }},
		{name: "SRD-ERR-3", setup: two, ev: srd(evMs, 9, RequestError)},
	}
	dl := []struct {
		name string
		ov   Override
		tr   *int
		add  int // 応答の時刻からの期限（ミリ秒）
	}{
		{"DL-1/300", OverrideNone{}, ip(300), 3300},
		{"DL-1/5000", OverrideNone{}, ip(5000), 8000},
		{"DL-2", OverrideNone{}, nil, 10000},
		{"DL-3/500 with G", OverrideKnown{Ms: 500}, ip(300), 10000},
		{"DL-3/8000 with G", OverrideKnown{Ms: 8000}, ip(300), 11000},
		{"DL-3/8000 nil", OverrideKnown{Ms: 8000}, nil, 11000},
		{"DL-3/7000 nil", OverrideKnown{Ms: 7000}, nil, 10000},
		{"DL-4/300", OverrideUnknown{}, ip(300), 10000},
		{"DL-4/8000", OverrideUnknown{}, ip(8000), 11000},
		{"DL-5", OverrideUnknown{}, nil, 10000},
	}
	for _, d := range dl {
		rows = append(rows, evRow{name: d.name, setup: func(s *state) {
			e := expect(1, RoleFail)
			e.Override = d.ov
			e.TransitionMs = d.tr
			s.TransitionMs = ip(1) // 今の全体の長さは使わない
			s.Expectations = []Expectation{e}
		}, ev: srd(evMs, 1, RequestOK), change: respond(0, RequestOK, evMs+d.add)})
	}
	runEvRows(t, rows)
}

func TestBatchDoneAuto(t *testing.T) {
	removed := func(s *state) { delete(s.Pending, 1) }
	var rows []evRow
	for _, p := range []Purpose{PurposeAutoFail, PurposeAutoRecover, PurposeGoLive, PurposeGoFail, PurposePauseOn,
		PurposePauseOffLive, PurposePauseOffFail, PurposePauseOffIntro, PurposeStopStream} {
		rows = append(rows, evRow{name: "BD-C-1/" + p.String(), setup: func(s *state) {
			pend(s, 1, p)
			pend(s, 2, PurposeStopStream)
		}, ev: bd(evMs, 1, BatchCompleted), change: removed})
	}
	rows = append(rows,
		evRow{name: "BD-C-1/Err AutoRecover", setup: func(s *state) { pend(s, 1, PurposeAutoRecover) },
			ev:     BatchDone{At: at(evMs), BatchID: 1, Result: BatchCompleted, Err: fmt.Errorf("RESTART 失敗")},
			change: removed, recErr: true},
		evRow{name: "BD-C-1/Err GoLive", setup: func(s *state) { pend(s, 1, PurposeGoLive) },
			ev:     BatchDone{At: at(evMs), BatchID: 1, Result: BatchCompleted, Err: fmt.Errorf("RESTART 失敗")},
			change: removed, recErr: true},
	)
	for _, p := range []Purpose{PurposeAutoFail, PurposeAutoRecover} {
		rows = append(rows,
			evRow{name: "BD-F-1/" + p.String(), setup: func(s *state) { pend(s, 1, p) },
				ev:     BatchDone{At: at(evMs), BatchID: 1, Result: BatchFailed, Err: fmt.Errorf("SetScene 失敗")},
				change: removed, recErr: true},
			evRow{name: "BD-R-1/" + p.String(), setup: func(s *state) { pend(s, 1, p) },
				ev: bd(evMs, 1, BatchRejected), change: removed},
		)
	}
	rows = append(rows,
		evRow{name: "BD-F-1/nil Err", setup: func(s *state) { pend(s, 1, PurposeAutoFail) },
			ev: bd(evMs, 1, BatchFailed), change: removed, recErr: true},
		evRow{name: "BD-X-1", setup: func(s *state) { pend(s, 1, PurposeAutoFail) }, ev: bd(evMs, 5, BatchFailed)},
	)
	runEvRows(t, rows)
}

func TestTick(t *testing.T) {
	ready := link(LOW)
	sent := func(p Purpose, agoMs int) func(*state) {
		return func(s *state) { s.LastAutoSent[p] = at(evMs - agoMs) }
	}
	rows := []evRow{
		{name: "TK-1/5s exactly", setup: all(ready, sent(PurposeAutoFail, 5000)), ev: tick(evMs), out: PurposeAutoFail},
		{name: "TK-1/9s", setup: all(link(DOWN), sent(PurposeAutoFail, 9000)), ev: tick(evMs), out: PurposeAutoFail},
		{name: "TK-2", setup: all(ready, sent(PurposeAutoFail, 4999)), ev: tick(evMs)},
		{name: "TK-3", setup: all(ready, sent(PurposeAutoFail, 9000), func(s *state) { pend(s, 1, PurposeAutoFail) }), ev: tick(evMs)},
		{name: "TK-4/before response", setup: all(ready, sent(PurposeAutoFail, 9000), func(s *state) {
			s.Expectations = []Expectation{expect(1, RoleFail)}
		}), ev: tick(evMs)},
		{name: "TK-4/before deadline", setup: all(ready, sent(PurposeAutoFail, 9000), func(s *state) {
			s.Expectations = []Expectation{responded(1, RoleFail, RequestTimeout, evMs-2000, evMs+1300)}
		}), ev: tick(evMs)},
		{name: "TK-5", setup: all(ready, sent(PurposeAutoFail, 9000), func(s *state) { hold(s, evMs-8000) }), ev: tick(evMs)},
		{name: "TK-6", setup: all(roleIs(RoleFail), link(OK), sent(PurposeAutoRecover, 5000), sent(PurposeAutoFail, 100)),
			ev: tick(evMs), out: PurposeAutoRecover},
	}
	for _, c := range []struct {
		name  string
		setup func(*state)
	}{
		{"Live OK", link(OK)},
		{"Fail LOW", all(roleIs(RoleFail), link(LOW))},
		{"Intro LOW", all(roleIs(RoleIntro), link(LOW))},
		{"Pause LOW", all(roleIs(RolePause), link(LOW))},
		{"Other DOWN", all(roleIs(RoleOther), link(DOWN))},
		{"not streaming", all(link(LOW), func(s *state) { s.Streaming = false })},
		{"not connected", all(link(LOW), func(s *state) { s.Connected = false })},
	} {
		rows = append(rows, evRow{name: "TK-7/" + c.name, setup: c.setup, ev: tick(evMs)})
	}
	rows = append(rows,
		evRow{name: "TK-8", setup: all(ready, sent(PurposeAutoFail, 9000), func(s *state) {
			s.Expectations = []Expectation{responded(1, RoleFail, RequestOK, evMs-3400, evMs-100)}
		}), ev: tick(evMs), out: PurposeAutoFail, change: func(s *state) { s.Expectations = nil }},
		evRow{name: "TK-8/only expired removed", setup: all(roleIs(RoleIntro), func(s *state) {
			s.Expectations = []Expectation{
				responded(1, RoleFail, RequestOK, evMs-3400, evMs-100),
				responded(2, RoleLive, RequestOK, evMs-1000, evMs+2300),
				expect(3, RolePause),
			}
		}), ev: tick(evMs), change: func(s *state) { s.Expectations = s.Expectations[1:] }},
		evRow{name: "TK-8/at deadline kept", setup: all(ready, sent(PurposeAutoFail, 9000), func(s *state) {
			s.Expectations = []Expectation{responded(1, RoleFail, RequestOK, evMs-3300, evMs)}
		}), ev: tick(evMs)},
		evRow{name: "TK-9", setup: all(ready, sent(PurposeAutoFail, 900_000), func(s *state) {
			e := expect(1, RoleFail)
			e.RegisteredAt = at(evMs - 3_600_000)
			s.Expectations = []Expectation{e}
		}), ev: tick(evMs)},
		evRow{name: "TK-11/AutoFail", setup: ready, ev: tick(evMs), out: PurposeAutoFail},
		evRow{name: "TK-11/AutoRecover", setup: all(roleIs(RoleFail), link(OK), sent(PurposeAutoFail, 1000)),
			ev: tick(evMs), out: PurposeAutoRecover},
	)
	runEvRows(t, rows)
}

func TestApproveAuto(t *testing.T) {
	pendAt := func(p Purpose) func(*state) { return func(s *state) { pend(s, 1, p) } }
	rows := []apRow{
		{name: "AP-COM-5/AutoFail", setup: all(link(LOW), pendAt(PurposeAutoFail), func(s *state) { s.SceneEpoch++ }),
			action: SetScene{Role: RoleFail}, atMs: evMs, res: Reject{}},
		{name: "AP-COM-5/AutoRecover RestartMedia", setup: all(roleIs(RoleFail), pendAt(PurposeAutoRecover), func(s *state) { s.SceneEpoch++ }),
			action: RestartMedia{Wait: W}, atMs: evMs, res: Reject{}},
		{name: "AP-COM-5/AutoRecover SetScene", setup: all(roleIs(RoleFail), pendAt(PurposeAutoRecover), func(s *state) { s.SceneEpoch++ }),
			action: SetScene{Role: RoleLive}, atMs: evMs, res: Reject{}},
		{name: "AP-AUTOFAIL-1", setup: all(link(LOW), pendAt(PurposeAutoFail)),
			action: SetScene{Role: RoleFail}, atMs: evMs, res: Proceed{}, register: RoleFail},
		{name: "AP-AUTOFAIL-1/Known override", setup: all(link(LOW), pendAt(PurposeAutoFail), func(s *state) { s.TransitionMs = nil }),
			action: SetScene{Role: RoleFail}, override: OverrideKnown{Ms: 700}, atMs: evMs, res: Proceed{}, register: RoleFail},
		{name: "AP-AUTOFAIL-2", setup: all(link(DOWN), pendAt(PurposeAutoFail),
			func(s *state) { s.Expectations = []Expectation{expect(5, RoleLive)} }),
			action: SetScene{Role: RoleFail}, override: OverrideUnknown{}, atMs: evMs, res: Proceed{}, register: RoleFail},
		{name: "AP-AUTOREC-1", setup: all(roleIs(RoleFail), pendAt(PurposeAutoRecover)),
			action: RestartMedia{Wait: W}, atMs: evMs, res: Proceed{ReqID: 0}},
		{name: "AP-AUTOREC-2", setup: all(roleIs(RoleFail), pendAt(PurposeAutoRecover)),
			action: SetScene{Role: RoleLive}, atMs: evMs, res: Proceed{}, register: RoleLive},
	}
	runApRows(t, rows)
}

func TestSnapshot(t *testing.T) {
	m := newMachine()
	baseState(&m.st)
	setRole(&m.st, RoleFail)
	m.st.Link = LOW
	hold(&m.st, 1234)
	m.st.LastError = &ErrorInfo{At: at(5), Msg: "x"}
	got := m.Snapshot()
	want := Snapshot{Connected: true, Streaming: true, Scene: "Fail", Role: RoleFail, Link: LOW,
		Hold: HoldStatus{Active: true, Since: at(1234)}, LastError: &ErrorInfo{At: at(5), Msg: "x"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("snapshot: got %+v, want %+v", got, want)
	}
	got.LastError.Msg = "changed"
	if m.st.LastError.Msg != "x" {
		t.Errorf("snapshot shares LastError with the machine")
	}
}
