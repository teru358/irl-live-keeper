package switcher

import (
	"testing"

	"github.com/teru358/irl-live-keeper/internal/scenemap"
)

// 表 0c 2（内部状態と整理）FB-ST、1.3 の SW 行（FB-CFG-16/17）、4.6 FB-CF。

func TestFBST(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-ST-01", func(t *testing.T) {
			m := newMachine()
			if m.st.StandbyOwner != OwnerNone || len(m.st.Stack) != 0 || m.st.ReturnScene != "" {
				t.Fatalf("新規 Machine: %+v", m.st)
			}
			h := newHB(t, hbNoConn())
			h.wantStack("")
		}},
		{"FB-ST-02", func(t *testing.T) {
			// 切断: 接続比較用に履歴を保持（地図なしを削除扱いしない）。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.ev(OBSDisconnected{At: at(2000)})
			h.wantStack("A,D")
			h.wantOwner(OwnerAuto, camera)
		}},
		{"FB-ST-03", func(t *testing.T) {
			// 再接続の4条件がすべて同一: 新しい地図で整理したあと、残る履歴と持ち主を保って判断する。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.ev(OBSDisconnected{At: at(2000)})
			h.outP(h.connect(2100, true, standby, false), "")
			h.wantStack("A,D")
			// 整理: 再接続の地図で D が消えていれば、途中の D だけ外れる。
			h2 := newHB(t, fbAD)
			h2.toSAD(1000)
			h2.ev(OBSDisconnected{At: at(2000)})
			h2.mp = editMap(h2.mp, func(m *scenemapMap) { delete(m.Scenes, deskScene) })
			h2.connect(2100, true, standby, false)
			h2.wantStack("A")
		}},
		{"FB-ST-14/reconnect", func(t *testing.T) {
			// S が引けない間に作った auto は、同じ4条件の再接続（S はまだない）で、古い待機の UUID と比べて C にならない。
			h := newHB(t, fbAD, hbNoStandby())
			bs := h.cams(1000, DOWN, OK)
			h.outP(bs, "F(D;+A)")
			h.settle(bs[0], 1100)
			h.wantStack("A")
			h.ev(OBSDisconnected{At: at(2000)})
			h.connect(2100, true, deskScene, false)
			h.wantStack("A")
		}},
		{"FB-ST-04", func(t *testing.T) {
			// 4条件のどれかが違う: C。過去履歴を再利用しない。
			for name, f := range map[string]func(h *hb){
				"scene":      func(h *hb) { h.connect(2100, true, camera2, false) },
				"streaming":  func(h *hb) { h.connect(2100, false, standby, false) },
				"collection": func(h *hb) { h.mp.Collection = "collection-y"; h.connect(2100, true, standby, false) },
			} {
				h := newHB(t, fbAD)
				h.toSAD(1000)
				h.ev(OBSDisconnected{At: at(2000)})
				f(h)
				h.wantStack("")
				h.wantOwner(OwnerNone, "")
				_ = name
			}
		}},
		{"FB-ST-05", func(t *testing.T) {
			// 配信停止: C、全シーン期待・候補時刻を消す。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.cams(2000, OK, OK) // R(A)
			h.ev(StreamStateChanged{At: at(2100), Streaming: false})
			h.wantStack("")
			if len(h.m.st.Expectations) != 0 || len(h.m.st.LastCandidateSent) != 0 {
				t.Fatalf("期待・時刻が残る: %+v %+v", h.m.st.Expectations, h.m.st.LastCandidateSent)
			}
		}},
		{"FB-ST-06", func(t *testing.T) {
			// fallbacks だけの変更: 到達可能性を見ず保持、ConfigEpoch+1。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			ce := h.m.st.ConfigEpoch
			c := h.cfg
			c.At = at(2000)
			c.SceneFallbacks = []FallbackRow{fbRow(talk, camera)}
			h.ev(c)
			h.wantStack("A,D")
			if h.m.st.ConfigEpoch != ce+1 {
				t.Fatalf("ConfigEpoch %d→%d", ce, h.m.st.ConfigEpoch)
			}
		}},
		{"FB-ST-07", func(t *testing.T) {
			// auto_enabled=false: C、auto の生成・承認なし。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			c := h.cfg
			c.At = at(2000)
			c.AutoEnabled = false
			c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene)}
			h.ev(c)
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
			h.outP(h.cams(3000, OK, OK), "") // 回復しても戻らない
		}},
		{"FB-ST-08", func(t *testing.T) {
			// auto_enabled=false、manual/A: manual/A 保持、K=[]。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			c := h.cfg
			c.At = at(2000)
			c.AutoEnabled = false
			c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene)}
			h.ev(c)
			h.wantOwner(OwnerManual, camera)
			h.wantStack("")
		}},
		{"FB-ST-09", func(t *testing.T) {
			// 人の SceneChanged（履歴内を選んだ場合も）: 手動共通解除。そのシーンが悪ければ新規退避。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			e := h.m.st.SceneEpoch
			h.outP(h.scene(2000, camera), "F(S;+A,D)")
			if h.m.st.SceneEpoch != e+1 || h.m.st.StandbyOwner != OwnerNone {
				t.Fatalf("手動共通解除でない")
			}
			h.wantStack("")
		}},
		{"FB-ST-10", func(t *testing.T) {
			// 下 A が削除: 手動共通解除（SceneEpoch+1）。抑止印を作らない。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			e := h.m.st.SceneEpoch
			h.smc(2000, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, camera) }))
			if h.m.st.SceneEpoch != e+1 {
				t.Fatalf("SceneEpoch %d→%d", e, h.m.st.SceneEpoch)
			}
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
		}},
		{"FB-ST-11", func(t *testing.T) {
			// [A,D,B] の途中 D が削除: [A,B]。SceneEpoch 不変。
			h := newHB(t, fbAD)
			h.setAuto(talk, camera, deskScene, camera2)
			e := h.m.st.SceneEpoch
			h.smc(2000, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, deskScene) }))
			h.wantStack("A,B")
			if h.m.st.SceneEpoch != e {
				t.Fatalf("SceneEpoch が進んだ")
			}
		}},
		{"FB-ST-12", func(t *testing.T) {
			// A が存在するが管理外化: A だけ外し [D]。下が D になる。空になれば none。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.smc(2000, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[camera]
				c.Items = nil
				m.Scenes[camera] = c
			}))
			h.wantStack("D")
			h.smc(2100, editMap(h.mp, func(m *scenemapMap) {
				for _, u := range []string{camera, deskScene} {
					c := m.Scenes[u]
					c.Items = nil
					m.Scenes[u] = c
				}
			}))
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
		}},
		{"FB-ST-13", func(t *testing.T) {
			// manual/A、A は存在し管理外化: manual/A 維持。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			h.smc(2000, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[camera]
				c.Items = nil
				m.Scenes[camera] = c
			}))
			h.wantOwner(OwnerManual, camera)
		}},
		{"FB-ST-14", func(t *testing.T) {
			// 待機の解決結果が変わった入口で、auto/manual とも C。旧期待は照合だけ（作用無効）。以後に作った auto は有効。
			h := newHB(t, fbAD)
			bs := h.cams(1000, DOWN, OK)
			h.outP(bs, "F(D;+A)")
			h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: deskScene}, OverrideNone{}, at(1100))
			h.smc(1200, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, standby) })) // S の消失
			if !h.m.st.Expectations[0].Plan.EffectsDisabled {
				t.Fatalf("旧期待の作用が無効になっていない")
			}
			h.outP(h.scene(1300, deskScene), "")
			h.wantOwner(OwnerNone, "") // 旧期待は履歴を足さない
			// S がない間に作った auto は有効。
			h2 := newHB(t, fbAD, hbNoStandby())
			bs = h2.cams(1000, DOWN, OK)
			h2.outP(bs, "F(D;+A)")
			h2.settle(bs[0], 1100)
			h2.wantStack("A")
		}},
		{"FB-ST-15", func(t *testing.T) {
			// 改名のみ（UUID 不変）: 履歴維持、status は新名。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.smc(2000, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[camera]
				c.Name = "Camera-renamed"
				m.Scenes[camera] = c
			}))
			h.wantStack("A,D")
			if n := h.m.Snapshot().Standby.ReturnSceneName; n != "Camera-renamed" {
				t.Fatalf("戻り先の名前: %q", n)
			}
		}},
		{"FB-ST-16", func(t *testing.T) {
			// 整理時 K に現在 UUID が混入: 現在要素とそれより上を捨てる。K=[A,D]、現在 A なら [] で none。
			h := newHB(t, fbAD)
			h.setAuto(camera, camera, deskScene)
			h.m.refine()
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
			h.setAuto(deskScene, camera, deskScene)
			h.m.refine()
			h.wantStack("A")
			// 同じ復帰の永久再送がない。
			h.setAuto(camera, camera)
			h.m.refine()
			h.outP(h.tick(5000), "")
		}},
	})
}

func TestFBCFG(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-CFG-16", func(t *testing.T) {
			// 対応の並べ替えのみ: 同値、ConfigEpoch 不変、Jc なし、再送時刻も不変。
			h := newHB(t, hbFB(fbRow(camera, deskScene), fbRow(camera2, talk)))
			h.toSAD(1000)
			ce, sent := h.m.st.ConfigEpoch, len(h.m.st.LastCandidateSent)
			c := h.cfg
			c.At = at(2000)
			c.SceneFallbacks = []FallbackRow{fbRow(camera2, talk), fbRow(camera, deskScene)}
			h.outP(h.ev(c), "")
			if h.m.st.ConfigEpoch != ce || len(h.m.st.LastCandidateSent) != sent {
				t.Fatalf("並べ替えで変わった")
			}
		}},
		{"FB-CFG-17", func(t *testing.T) {
			// 対応の先を別名に変更 / 最初の設定: ConfigEpoch+1。
			h := newHB(t, fbAD)
			ce := h.m.st.ConfigEpoch
			c := h.cfg
			c.At = at(2000)
			c.SceneFallbacks = []FallbackRow{fbRow(camera, talk)}
			h.ev(c)
			if h.m.st.ConfigEpoch != ce+1 {
				t.Fatalf("先の変更: %d→%d", ce, h.m.st.ConfigEpoch)
			}
			h2 := newHB(t)
			ce = h2.m.st.ConfigEpoch
			c = h2.cfg
			c.At = at(2000)
			c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene)}
			h2.ev(c)
			if h2.m.st.ConfigEpoch != ce+1 {
				t.Fatalf("最初の設定: %d→%d", ce, h2.m.st.ConfigEpoch)
			}
		}},
	})
}

func TestFBCF(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-CF-01", func(t *testing.T) {
			// auto/[A,B]、C 不良、対応を C→B→D へ変更、A/B 不可: [F(D;+C)]。ConfigEpoch+1、既存 [A,B] 維持。
			h := newHB(t, hbMapEdit(addSceneC), hbAt(sceneC))
			h.m.st.CameraStates = map[string]scenemap.CameraState{camMain: DOWN, camSub: OK}
			h.setAuto(sceneC, camera, camera2)
			ce := h.m.st.ConfigEpoch
			c := h.cfg
			c.At = at(1000)
			c.SceneFallbacks = []FallbackRow{{Scene: "C", Fallback: "Camera2"}, fbRow(camera2, deskScene)}
			bs := h.ev(c)
			h.outP(bs, "F(D;+C)")
			if h.m.st.ConfigEpoch != ce+1 {
				t.Fatalf("ConfigEpoch %d→%d", ce, h.m.st.ConfigEpoch)
			}
			h.wantStack("A,B")
			h.settle(bs[0], 1100)
			h.wantStack("A,B,C")
		}},
		{"FB-CF-02", func(t *testing.T) {
			// S auto/[A,D]、m/s 不良・x/y H、対応から A/D 到達不能へ: 履歴維持、ConfigEpoch+1。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			ce := h.m.st.ConfigEpoch
			c := h.cfg
			c.At = at(2000)
			c.SceneFallbacks = nil
			h.outP(h.ev(c), "")
			h.wantStack("A,D")
			if h.m.st.ConfigEpoch != ce+1 {
				t.Fatalf("ConfigEpoch")
			}
		}},
		{"FB-CF-03", func(t *testing.T) {
			// 並べ替えだけ: 同値（FB-CFG-16 と同じ）。再送時刻も不変。
			h := newHB(t, hbFB(fbRow(camera, deskScene), fbRow(deskScene, talk)))
			h.cams(1000, DOWN, OK)
			sent := h.m.st.LastCandidateSent
			c := h.cfg
			c.At = at(2000)
			c.SceneFallbacks = []FallbackRow{fbRow(deskScene, talk), fbRow(camera, deskScene)}
			h.outP(h.ev(c), "")
			if len(h.m.st.LastCandidateSent) != len(sent) {
				t.Fatalf("時刻が変わった")
			}
		}},
		{"FB-CF-04", func(t *testing.T) {
			// S auto/[A,D]、m/s OK、記録なし、auto=false へ: C、旧自動期待は照合だけ（世代で作用しない）。
			h := newHB(t, fbAD)
			h.setAuto(standby, camera, deskScene)
			bs := h.tick(2000)
			h.outP(bs, "R(A)")
			h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: camera}, OverrideNone{}, at(2100))
			c := h.cfg
			c.At = at(2200)
			c.AutoEnabled = false
			c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene)}
			h.ev(c)
			h.wantStack("")
			h.outP(h.scene(2300, camera), "") // 照合だけ。履歴は触らない
			h.wantOwner(OwnerNone, "")
		}},
		{"FB-CF-05", func(t *testing.T) {
			// S manual/A、全 OK、auto=false へ: manual 保持、旧 OFF は世代で Reject。
			h := newHB(t, fbAD)
			h.standbyOnManual(t)
			bs, _ := h.evErr(Command{At: at(1000), Kind: CmdStandbyOff})
			c := h.cfg
			c.At = at(1100)
			c.AutoEnabled = false
			c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene)}
			h.ev(c)
			h.wantOwner(OwnerManual, camera)
			if r := h.m.Approve(bs[0].BatchID, SetScene{SceneUUID: camera}, OverrideNone{}, at(1200)); r != (Reject{}) {
				t.Fatalf("旧 OFF の承認 = %#v", r)
			}
		}},
		{"FB-CF-06", func(t *testing.T) {
			// S auto/[A,D]、m/s 不良・x/y H、standby 名変更: C、旧期待は作用しない。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			c := h.cfg
			c.At = at(2000)
			c.Standby = "Standby2"
			c.SceneFallbacks = []FallbackRow{fbRow(camera, deskScene)}
			h.ev(c)
			h.wantStack("")
			h.wantOwner(OwnerNone, "")
		}},
		{"FB-CF-07", func(t *testing.T) {
			// 切断中の設定変更: 未取得の地図で削除・管理外化としない。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.ev(OBSDisconnected{At: at(2000)})
			c := h.cfg
			c.At = at(2100)
			c.SceneFallbacks = []FallbackRow{fbRow(camera, talk)}
			h.ev(c)
			h.wantStack("A,D")
		}},
		{"FB-CF-08", func(t *testing.T) {
			// S auto/[A,D]、A のカメラ削除で管理外化: K=[D]、auto。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.smc(2000, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[camera]
				c.Items = nil
				m.Scenes[camera] = c
			}))
			h.wantStack("D")
		}},
	})
}

var _ = scenemap.StateOK
