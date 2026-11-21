package switcher

import (
	"testing"
)

// 0b で足した Event の動き（StudioModeChanged、ItemRequestDone、OBSConnected.StudioMode）は M-11 で実装し、
// 旧 M-10 の「Machine が捨てる」テスト2件は、動きを持つ 0b の行（HB-SM-*、HB-IR-*、HB-OC-*）に置き換えた。

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
