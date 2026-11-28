package switcher

import (
	"slices"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// S2-68: 消したカメラの hidden 記録（spec 7章「hidden.json」）。
// カメラを消す保存は新しい世代（設定に cam2 がない Machine）を作り、hidden.json の記録を HiddenRestored で渡す。
// 照合できる記録は「設定にないカメラ」として戻して消し、照合できない記録は触らずに残す。
// あとで同じ番号を作ると、残った記録は新しいカメラのものとして扱う。削除時の別の書き込みはしない（HiddenRevision が動かない）。

// hbDelSub は cam2 を消した後の世代の設定（cam1 だけ）。
func hbDelSub() hbOpt {
	return func(h *hb) { h.cfg.Cameras = h.cfg.Cameras[:1] }
}

// allRecords は Hidden と Retained の記録をまとめて複製する（次の世代へ hidden.json 経由で渡す記録の代わり）。
func (h *hb) allRecords() []HiddenEntry {
	return cloneHidden(append(slices.Clone(h.m.st.Hidden), h.m.st.Retained...))
}

// delGen は cam2 を消した世代を作り、cam2 の項目 x の記録（隠した）を渡す。まだ OBS につながない。
func delGen(t *testing.T, mod func(e *HiddenEntry)) (*hb, HiddenEntry) {
	t.Helper()
	h := newHB(t, hbNoConn(), hbDelSub())
	ex := h.entryOf(kx, HiddenH)
	if mod != nil {
		mod(&ex)
	}
	h.out(h.ev(HiddenRestored{At: at(20), Entries: []HiddenEntry{ex}}), "")
	return h, ex
}

func TestS268消したカメラの記録の照合(t *testing.T) {
	// (a) 照合できる記録は、設定にないカメラの項目として戻して消す（core のログの理由は restore）。
	t.Run("a 接続して地図が合えば戻して消す", func(t *testing.T) {
		for _, streaming := range []bool{false, true} {
			name := "非配信"
			if streaming {
				name = "配信中"
			}
			t.Run(name, func(t *testing.T) {
				h, _ := delGen(t, nil)
				rev := h.m.Snapshot().HiddenRevision
				setItem(h.mp, kx.ContainerUUID, kx.ItemID, false)
				bs := h.connect(1000, streaming, camera, false)
				h.out(bs, "Sc(s)[x]") // s = cam2
				if bs[0].CameraID != camSub {
					t.Fatalf("バッチのカメラ = %q, want %q（設定にないカメラの記録）", bs[0].CameraID, camSub)
				}
				h.wantEntries("x:H") // 戻し終わるまでは記録がある
				h.sendTrue(bs[0], 1100, kx)
				h.out(h.bdone(1200, bs[0].BatchID, BatchCompleted, nil), "")
				h.wantEntries("")
				h.wantRetained("")
				if h.m.Snapshot().HiddenRevision == rev {
					t.Errorf("記録を消したのに HiddenRevision が動かない（hidden.json が更新されない）")
				}
			})
		}
	})

	// (b) 照合できない記録は触らずに残す。出す要求もない。
	t.Run("b 照合できない記録は残す", func(t *testing.T) {
		type tc struct {
			name string
			mod  func(e *HiddenEntry)
			// connect は OBS への接続の仕方。nil なら接続しない。
			connect func(h *hb) []Batch
		}
		hidMap := func(h *hb) { setItem(h.mp, kx.ContainerUUID, kx.ItemID, false) }
		cases := []tc{
			{"OBS 未接続", nil, nil},
			{"空の地図", nil, func(h *hb) []Batch {
				return h.ev(OBSConnected{At: at(1000), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300),
					Map: &scenemap.Map{Collection: collectionX}})
			}},
			{"別のシーンコレクション", func(e *HiddenEntry) { e.Collection = "collection-other" }, func(h *hb) []Batch {
				hidMap(h)
				return h.connect(1000, true, camera, false)
			}},
			{"別の接続先", func(e *HiddenEntry) { e.Host = "other.local" }, func(h *hb) []Batch {
				hidMap(h)
				return h.connect(1000, true, camera, false)
			}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				h, ex := delGen(t, c.mod)
				rev := h.m.Snapshot().HiddenRevision
				if c.connect != nil {
					h.out(c.connect(h), "")
				}
				h.wantRetainedOrHidden("x:H")
				got := h.allRecords()
				if len(got) != 1 || !got[0].WrittenAt.Equal(ex.WrittenAt) || got[0].State != HiddenH || got[0].CameraID != camSub ||
					got[0].Key() != ex.Key() || got[0].SourceUUID != ex.SourceUUID || got[0].Collection != ex.Collection || got[0].Host != ex.Host {
					t.Errorf("記録が変わった: %+v, want %+v", got, ex)
				}
				// 空の地図では、最初の接続で記録が Retained から Hidden へ移るだけ（中身は同じ）。それ以外は動かない。
				if c.name != "空の地図" && h.m.Snapshot().HiddenRevision != rev {
					t.Errorf("記録の変更（hidden.json の書き込み）が起きた")
				}
			})
		}
	})
}

// (c)(d) 残った記録のあとで、同じ番号（cam2）のカメラをまた作る。
func TestS268同じ番号を作ると記録は新しいカメラのもの(t *testing.T) {
	// 消した世代で照合できずに残った記録（空の地図）を、次の世代へ hidden.json 経由で渡した形を作る。
	kept := func(t *testing.T) []HiddenEntry {
		h, _ := delGen(t, nil)
		h.ev(OBSConnected{At: at(1000), Streaming: true, Scene: nameOf(camera), SceneUUID: camera, TransitionMs: ip(300),
			Map: &scenemap.Map{Collection: collectionX}})
		h.wantRetainedOrHidden("x:H")
		return h.allRecords()
	}
	// 作り直した世代（cam1 と cam2）。cam2 は DOWN で配信中、x は OBS で非表示のまま。
	recreated := func(t *testing.T) (*hb, []Batch) {
		es := kept(t)
		h := newHB(t, hbNoConn())
		h.yOff()
		h.sIs(10, DOWN)
		h.out(h.ev(HiddenRestored{At: at(20), Entries: es}), "")
		setItem(h.mp, kx.ContainerUUID, kx.ItemID, false)
		return h, h.connect(1000, true, camera, false)
	}

	t.Run("c 悪いあいだは隠したまま、OK になれば通常の ShowCamera で戻す", func(t *testing.T) {
		h, bs := recreated(t)
		h.out(bs, "")
		h.wantEntries("x:H")
		h.wantRetained("")
		if e := h.entry(kx); e == nil || e.CameraID != camSub {
			t.Fatalf("記録 = %+v, want 新しい cam2 の記録", e)
		}
		bs = h.sIs(2000, OK)
		h.out(bs, "Sc(s)[R(is),x]") // 設定にあるカメラ。core の理由は ok（restore ではない）
		if bs[0].CameraID != camSub || bs[0].Purpose != PurposeShowCamera {
			t.Fatalf("batch = %+v", bs[0])
		}
		h.sendTrue(bs[0], 2100, kx)
		h.out(h.bdone(2200, bs[0].BatchID, BatchCompleted, nil), "")
		h.wantEntries("")
	})

	t.Run("d すべて表示に戻すでも消える", func(t *testing.T) {
		h, bs := recreated(t)
		h.out(bs, "")
		h.wantEntries("x:H")
		bs = h.ev(Command{At: at(2000), Kind: CmdShowAllCameras})
		h.out(bs, "MA[x,y]") // y は一覧にない人が隠した項目。x が残した記録
		h.sendTrue(bs[0], 2100, kx)
		h.sendTrue(bs[0], 2100, ky)
		h.out(h.bdone(2200, bs[0].BatchID, BatchCompleted, nil), "")
		h.wantEntries("")
		h.wantRetained("")
	})
}
