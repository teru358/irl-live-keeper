package switcher

import (
	"reflect"
	"testing"
)

// 0b で足した Event（StudioModeChanged、ItemRequestDone）は、M-11 までは Machine が何もしないで捨てる。
// 状態もバッチも変えない。
func TestM10新しいEventはMachineが捨てる(t *testing.T) {
	build := func() *seq {
		q := newSeq(t)
		q.prep(camera, true, OK, ip(300))
		return q
	}
	for _, ev := range []Event{
		StudioModeChanged{At: at(100), Enabled: true},
		StudioModeChanged{At: at(100), Enabled: false},
		ItemRequestDone{At: at(100), ReqID: 7, Result: ItemOK},
		ItemRequestDone{At: at(100), ReqID: 7, Result: ItemTimeout},
		ItemRequestDone{At: at(100), ReqID: 7, Result: ItemError},
		ItemRequestDone{At: at(100), ReqID: 7, Result: ItemNotFound},
	} {
		q, ref := build(), build()
		b, err := q.m.Handle(ev)
		if b != nil || err != nil {
			t.Fatalf("%T: batch=%s err=%v, want none", ev, showBatch(b), err)
		}
		if !reflect.DeepEqual(q.m.st, ref.m.st) {
			t.Errorf("%T で状態が変わった:\n got %+v\nwant %+v", ev, q.m.st, ref.m.st)
		}
		if !reflect.DeepEqual(q.m.Snapshot(), ref.m.Snapshot()) {
			t.Errorf("%T で Snapshot が変わった", ev)
		}
	}
}

// OBSConnected に載せる StudioMode は、今は Machine の動きを変えない。
func TestM10OBSConnectedのStudioModeは動きを変えない(t *testing.T) {
	run := func(studio bool) (*Batch, Snapshot, state) {
		q := newSeq(t)
		q.none(cfg(0))
		q.none(lc(0, OK))
		c := conn(0, true, camera, ip(300)).(OBSConnected)
		c.StudioMode = studio
		b, err := q.m.Handle(c)
		if err != nil {
			t.Fatal(err)
		}
		return b, q.m.Snapshot(), q.m.st
	}
	b1, s1, st1 := run(false)
	b2, s2, st2 := run(true)
	if showBatch(b1) != showBatch(b2) || !reflect.DeepEqual(s1, s2) || !reflect.DeepEqual(st1, st2) {
		t.Fatalf("StudioMode で動きが変わった: %s / %s", showBatch(b1), showBatch(b2))
	}
}

func TestM10ItemResultのString(t *testing.T) {
	for r, want := range map[ItemResult]string{ItemOK: "OK", ItemTimeout: "Timeout", ItemError: "Error", ItemNotFound: "NotFound", ItemResult(0): "ItemResult?"} {
		if got := r.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(r), got, want)
		}
	}
	// 0 値は有効な結果にしない
	if ItemOK == 0 {
		t.Error("ItemOK が 0 値")
	}
}

func TestM10SetItemEnabledはActionの型(t *testing.T) {
	var a Action = SetItemEnabled{SceneUUID: "u", ItemID: 3, Enabled: false}
	if _, ok := a.(SetItemEnabled); !ok {
		t.Fatal("SetItemEnabled が Action でない")
	}
}
