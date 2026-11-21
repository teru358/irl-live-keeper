package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 3台（cam1 = RTMP、cam2 と cam3 = SRT）の一覧。ほかに設定にない path "other" がある。
const multiDir = "testdata/multi-camera"

func loadMulti(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(multiDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// listWith は paths_list_3cams.json の各 path の inboundBytes と source.id を差し替えた写し。
// 値が nil の path はそのまま。drop の path は一覧から外す。
type pathEdit struct {
	bytes int64
	id    string
	drop  bool
}

func listWith(t *testing.T, edits map[string]pathEdit) reply {
	t.Helper()
	var m struct {
		ItemCount int              `json:"itemCount"`
		PageCount int              `json:"pageCount"`
		Items     []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(loadMulti(t, "paths_list_3cams.json"), &m); err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	for _, it := range m.Items {
		e, has := edits[it["name"].(string)]
		if has && e.drop {
			continue
		}
		if has {
			it["inboundBytes"] = e.bytes
			if e.id != "" {
				it["source"].(map[string]any)["id"] = e.id
			}
		}
		items = append(items, it)
	}
	m.Items, m.ItemCount = items, len(items)
	b, _ := json.Marshal(m)
	return ok(b)
}

func newMulti(t *testing.T, paths []reply, conns reply, ids ...string) (*fakeServer, *Poller) {
	f := newFake(t, paths, conns)
	return f, NewPoller(f.URL, "api", "secret", ids, f.Client())
}

var cams3 = []string{"cam1", "cam2", "cam3"}

// 1回の読み取りから、3台分が設定の順で、同じ At で出る。1回の path 一覧と1回の SRT 一覧だけを読む。
func TestPollerMultiOrderAndOneRead(t *testing.T) {
	f, p := newMulti(t, []reply{ok(loadMulti(t, "paths_list_3cams.json"))}, ok(loadMulti(t, "srt_conns_list.json")), "cam3", "cam1", "cam2")
	set := p.Poll(nil2ctx(), at(0))
	if !set.At.Equal(at(0)) || len(set.Cameras) != 3 {
		t.Fatalf("set = %+v", set)
	}
	for i, id := range []string{"cam3", "cam1", "cam2"} {
		s := set.Cameras[i]
		if s.Camera != id || !s.At.Equal(set.At) || !s.Online || s.Err != nil || s.Interval != 0 || s.Kbps != 0 {
			t.Errorf("cameras[%d] = %+v, want camera %s", i, s, id)
		}
	}
	if f.pathReqs != 1 || f.connReqs != 1 {
		t.Errorf("requests: paths=%d conns=%d", f.pathReqs, f.connReqs)
	}
}

// SRT の振り分け: path ごとに自分の接続の値。RTMP の camera は nil。
func TestPollerMultiSRTRouting(t *testing.T) {
	_, p := newMulti(t, []reply{ok(loadMulti(t, "paths_list_3cams.json"))}, ok(loadMulti(t, "srt_conns_list.json")), cams3...)
	set := p.Poll(nil2ctx(), at(0))
	c1, c2, c3 := set.Cameras[0], set.Cameras[1], set.Cameras[2]
	if c1.Protocol != "rtmp" || c1.SRT != nil {
		t.Errorf("cam1 = %+v", c1)
	}
	if c2.Protocol != "srt" || c2.SRT == nil || c2.SRT.RTTMs != 20 || c2.SRT.RecvMbps != 2.0 {
		t.Errorf("cam2 = %+v %+v", c2, c2.SRT)
	}
	if c3.Protocol != "srt" || c3.SRT == nil || c3.SRT.RTTMs != 30 || c3.SRT.LossRate != 0.5 {
		t.Errorf("cam3 = %+v %+v", c3, c3.SRT)
	}
}

// SRT の一覧の失敗は SRT の値だけを nil にする。
func TestPollerMultiSRTListFailure(t *testing.T) {
	_, p := newMulti(t, []reply{
		ok(loadMulti(t, "paths_list_3cams.json")),
		listWith(t, map[string]pathEdit{"cam1": {bytes: 1000000 + 125000}, "cam2": {bytes: 2000000 + 250000}, "cam3": {bytes: 3000000 + 250000}}),
	}, reply{status: 500}, cams3...)
	p.Poll(nil2ctx(), at(0))
	set := p.Poll(nil2ctx(), at(1))
	for i, want := range []float64{1000, 2000, 2000} {
		s := set.Cameras[i]
		if s.Err != nil || s.Kbps != want || (i > 0 && (s.SRT != nil || s.Protocol != "srt")) {
			t.Errorf("cameras[%d] = %+v", i, s)
		}
	}
}

// SRT のカメラがいない（RTMP だけ）なら SRT の一覧を読まない。
func TestPollerMultiNoSRTNoConnsRead(t *testing.T) {
	f, p := newMulti(t, []reply{ok(loadMulti(t, "paths_list_3cams.json"))}, ok(loadMulti(t, "srt_conns_list.json")), "cam1")
	p.Poll(nil2ctx(), at(0))
	if f.connReqs != 0 {
		t.Errorf("conns requests = %d", f.connReqs)
	}
}

// 基準はカメラごと。cam2 の識別子が変わっても cam1・cam3 は続き、cam3 だけ減って取り直す。
func TestPollerMultiPerCameraBase(t *testing.T) {
	_, p := newMulti(t, []reply{
		ok(loadMulti(t, "paths_list_3cams.json")),
		listWith(t, map[string]pathEdit{"cam1": {bytes: 1000000 + 125000}, "cam2": {bytes: 2000000 + 125000, id: "srt-cam2-new"}, "cam3": {bytes: 100}}),
		listWith(t, map[string]pathEdit{"cam1": {bytes: 1000000 + 250000}, "cam2": {bytes: 2000000 + 250000, id: "srt-cam2-new"}, "cam3": {bytes: 100 + 125000}}),
	}, ok(loadMulti(t, "srt_conns_list.json")), cams3...)
	var sets []SampleSet
	for _, s := range []int{0, 1, 2} {
		sets = append(sets, p.Poll(nil2ctx(), at(s)))
	}
	kb := func(i, cam int) float64 { return sets[i].Cameras[cam].Kbps }
	if kb(0, 0) != 0 || kb(0, 1) != 0 || kb(0, 2) != 0 {
		t.Errorf("最初: %v %v %v", kb(0, 0), kb(0, 1), kb(0, 2))
	}
	if kb(1, 0) != 1000 || kb(1, 1) != 0 || kb(1, 2) != 0 {
		t.Errorf("1秒後: %v %v %v", kb(1, 0), kb(1, 1), kb(1, 2))
	}
	if kb(2, 0) != 1000 || kb(2, 1) != 1000 || kb(2, 2) != 1000 {
		t.Errorf("2秒後: %v %v %v", kb(2, 0), kb(2, 1), kb(2, 2))
	}
}

// path の一覧の失敗: 全カメラに Err。基準は変えず、次の回は2秒分を2秒で割る。Interval は前回からの経過。
func TestPollerMultiListFailure(t *testing.T) {
	_, p := newMulti(t, []reply{
		ok(loadMulti(t, "paths_list_3cams.json")),
		{status: 500, body: []byte("SECRET-BODY")},
		listWith(t, map[string]pathEdit{"cam1": {bytes: 1000000 + 250000}, "cam2": {bytes: 2000000 + 500000}, "cam3": {bytes: 3000000 + 250000}}),
	}, ok(loadMulti(t, "srt_conns_list.json")), cams3...)
	p.Poll(nil2ctx(), at(0))
	bad := p.Poll(nil2ctx(), at(1))
	for i, s := range bad.Cameras {
		if s.Err == nil || s.Kbps != 0 || s.Online || s.Protocol != "" || s.SRT != nil || s.Interval != time.Second || s.Camera != cams3[i] {
			t.Errorf("失敗の回 cameras[%d] = %+v", i, s)
		}
		if contains(s.Err.Error(), "SECRET-BODY") {
			t.Errorf("Err に本文: %v", s.Err)
		}
	}
	good := p.Poll(nil2ctx(), at(2))
	for i, want := range []float64{1000, 2000, 1000} {
		s := good.Cameras[i]
		if s.Err != nil || s.Kbps != want || s.Interval != time.Second {
			t.Errorf("次の回 cameras[%d] = %+v, want %v kbps", i, s, want)
		}
	}
}

// 一覧にない path のカメラは Online false、Kbps 0、Err なし。ほかのカメラに影響しない。
func TestPollerMultiMissingPath(t *testing.T) {
	_, p := newMulti(t, []reply{
		ok(loadMulti(t, "paths_list_3cams.json")),
		listWith(t, map[string]pathEdit{"cam1": {bytes: 1000000 + 125000}, "cam2": {drop: true}, "cam3": {bytes: 3000000 + 125000}}),
	}, ok(loadMulti(t, "srt_conns_list.json")), cams3...)
	p.Poll(nil2ctx(), at(0))
	set := p.Poll(nil2ctx(), at(1))
	c2 := set.Cameras[1]
	if c2.Online || c2.Kbps != 0 || c2.Err != nil || c2.Protocol != "" || c2.SRT != nil || c2.Camera != "cam2" {
		t.Errorf("cam2 = %+v", c2)
	}
	if set.Cameras[0].Kbps != 1000 || set.Cameras[2].Kbps != 1000 {
		t.Errorf("他: %v %v", set.Cameras[0].Kbps, set.Cameras[2].Kbps)
	}
}

// 一覧に出ていなかった path が出てきたら、その回は基準を取るだけ。
func TestPollerMultiPathAppears(t *testing.T) {
	_, p := newMulti(t, []reply{
		listWith(t, map[string]pathEdit{"cam2": {drop: true}}),
		ok(loadMulti(t, "paths_list_3cams.json")),
	}, ok(loadMulti(t, "srt_conns_list.json")), cams3...)
	p.Poll(nil2ctx(), at(0))
	s := p.Poll(nil2ctx(), at(1)).Cameras[1]
	if !s.Online || s.Kbps != 0 {
		t.Errorf("cam2 = %+v", s)
	}
}

// 接続の id が path の source.id と合わない（または source.id が空）ときは、path だけで選ぶ。
func TestPollerMultiSRTPicksByPathWhenIDMismatch(t *testing.T) {
	var m struct {
		ItemCount int              `json:"itemCount"`
		PageCount int              `json:"pageCount"`
		Items     []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(loadMulti(t, "srt_conns_list.json"), &m); err != nil {
		t.Fatal(err)
	}
	for _, it := range m.Items {
		it["id"] = "mismatch-" + it["id"].(string) // どの接続も source.id と当たらない
	}
	body, _ := json.Marshal(m)
	_, p := newMulti(t, []reply{ok(loadMulti(t, "paths_list_3cams.json"))}, ok(body), cams3...)
	set := p.Poll(nil2ctx(), at(0))
	if s := set.Cameras[1]; s.SRT == nil || s.SRT.RTTMs != 20 {
		t.Errorf("cam2 = %+v", s.SRT)
	}
	if s := set.Cameras[2]; s.SRT == nil || s.SRT.RTTMs != 30 {
		t.Errorf("cam3 = %+v", s.SRT)
	}
}

// 一覧から消えて戻ったカメラは、同じ source.id でも基準を取り直す（戻った回は Kbps 0）。
func TestPollerMultiRebaseAfterMissing(t *testing.T) {
	_, p := newMulti(t, []reply{
		ok(loadMulti(t, "paths_list_3cams.json")),
		listWith(t, map[string]pathEdit{"cam1": {drop: true}}),
		listWith(t, map[string]pathEdit{"cam1": {bytes: 1000000 + 125000}}),
		listWith(t, map[string]pathEdit{"cam1": {bytes: 1000000 + 250000}}),
	}, ok(loadMulti(t, "srt_conns_list.json")), "cam1")
	p.Poll(nil2ctx(), at(0))
	p.Poll(nil2ctx(), at(1))
	if s := p.Poll(nil2ctx(), at(2)).Cameras[0]; !s.Online || s.Kbps != 0 {
		t.Errorf("戻った回 = %+v", s)
	}
	if s := p.Poll(nil2ctx(), at(3)).Cameras[0]; s.Kbps != 1000 {
		t.Errorf("次の回 = %+v", s)
	}
}
