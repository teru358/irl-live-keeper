package switcher

import (
	"testing"
)

// 表 0c 1.2 FB-NM（名前の解決）。純粋な関数の行は fallback_test.go。ここは Machine を通した行。

func TestFBNM(t *testing.T) {
	runHB(t, []hbRow{
		{"FB-NM-01", func(t *testing.T) {
			// 最初の設定、地図なし: 解決結果は不明。画像と推定せず切替なし。
			h := newHB(t, fbAD, hbNoConn())
			h.cams(0, DOWN, OK)
			if h.m.st.StandbyUUID != "" || h.m.resolved().Known {
				t.Fatalf("地図なしで解決している")
			}
			if got := h.m.Snapshot().Class; got != ClassUnknown {
				t.Fatalf("Class = %v", got)
			}
		}},
		{"FB-NM-02/03", func(t *testing.T) {
			// A→D が両方ある: 解決。D の名前が消えると、その行は実行時だけ共通待機（設定の文字列は保持）。
			h := newHB(t, fbAD)
			if got := h.m.resolved().Next[camera]; got != deskScene {
				t.Fatalf("A→D: %q", got)
			}
			h.smc(100, editMap(h.mp, func(m *scenemapMap) { delete(m.Scenes, deskScene) }))
			r := h.m.resolved()
			if _, ok := r.Next[camera]; ok || r.NextOf(camera) != standby {
				t.Fatalf("D がないとき A→S になっていない: %+v", r)
			}
			if len(r.Warnings) == 0 || h.m.st.Fallbacks[0].Fallback != "Desk" {
				t.Fatalf("警告か設定の保持: %+v %+v", r.Warnings, h.m.st.Fallbacks)
			}
			// FB-NM-05: 復活すれば A→D に戻る。ConfigEpoch は進めない。
			ce := h.m.st.ConfigEpoch
			h.smc(200, h.mp)
			if got := h.m.resolved().Next[camera]; got != deskScene || h.m.st.ConfigEpoch != ce {
				t.Fatalf("復活: %q epoch %d→%d", got, ce, h.m.st.ConfigEpoch)
			}
		}},
		{"FB-NM-04", func(t *testing.T) {
			// 元の A の名前がない: 適用元なし、別シーンに適用しない。
			h := newHB(t, hbFB(FallbackRow{Scene: "Gone", Fallback: "Desk"}))
			r := h.m.resolved()
			if len(r.Next) != 0 || len(r.Warnings) == 0 {
				t.Fatalf("%+v", r)
			}
		}},
		{"FB-NM-06", func(t *testing.T) {
			// S なし、A不良→D、D は入れる、s OK: F(D;+A)。着いたら auto/[A]。m 回復で R(A)、着いたら none。
			h := newHB(t, fbAD, hbNoStandby())
			bs := h.cams(1000, DOWN, OK)
			h.outP(bs, "F(D;+A)")
			h.settle(bs[0], 1100)
			h.wantScene(deskScene)
			h.wantStack("A")
			bs = h.cams(2000, OK, OK)
			h.outP(bs, "R(A)")
			h.settle(bs[0], 2100)
			h.wantScene(camera)
			h.wantStack("")
		}},
		{"FB-NM-07", func(t *testing.T) {
			// S なし、A→D、D も入れない: 履歴不変で、その地点で探索終了（警告）。
			h := newHB(t, fbAD, hbNoStandby())
			h.outP(h.cams(1000, DOWN, DOWN), "Hc(s)[x,y]") // F は出ない（s の Hide だけ）
			h.wantStack("")
			h.wantScene(camera)
		}},
		{"FB-NM-08", func(t *testing.T) {
			// D を同 UUID で改名: 設定の行は未解決扱い、履歴の D は維持（名前追従しない）。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			h.smc(2000, editMap(h.mp, func(m *scenemapMap) {
				c := m.Scenes[deskScene]
				c.Name = "Desk-renamed"
				m.Scenes[deskScene] = c
			}))
			h.wantStack("A,D")
			if h.m.resolved().Next[camera] != "" {
				t.Fatalf("改名で行が追従した")
			}
			if names := h.m.Snapshot().Standby.StackNames; len(names) != 2 || names[1] != "Desk-renamed" {
				t.Fatalf("status は新名: %v", names)
			}
		}},
		{"FB-NM-09", func(t *testing.T) {
			// 同名の D が別 UUID で復活: 対応は新 UUID、旧履歴を名前で置換しない。
			h := newHB(t, fbAD)
			h.toSAD(1000)
			const d2 = "uuid-desk-2"
			h.smc(2000, editMap(h.mp, func(m *scenemapMap) {
				delete(m.Scenes, deskScene)
				m.Scenes[d2] = scenemapContainer("Desk")
			}))
			if h.m.resolved().Next[camera] != d2 {
				t.Fatalf("対応が新 UUID でない: %v", h.m.resolved().Next)
			}
			// 旧 D は消えたので外れる（途中の削除）。新 D は履歴に入らない。
			h.wantStack("A")
		}},
		{"FB-NM-10", func(t *testing.T) {
			// 行なし、S あり: 今のシーンの次は S（空設定も同じ計算経路）。
			h := newHB(t)
			h.outP(h.cams(1000, DOWN, OK), "F(S;+A)")
		}},
	})
}
