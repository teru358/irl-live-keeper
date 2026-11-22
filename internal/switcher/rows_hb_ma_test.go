package switcher

import "testing"

// 0b 表 HB-MA-07 / HB-MA-08（0c の結合のテストで見つかった MAConfirm の残り）。
func TestHB手動のすべて表示に戻すのMAConfirmの残り(t *testing.T) {
	// 人が隠して記録のない x を、MA が Show し、true のイベントで確かめる。
	setup := func(t *testing.T) *hb {
		h := newHB(t)
		h.yOff()
		h.setItemMapOnly(kx, false) // 人が隠した。記録なし
		bs, err := h.evErr(Command{At: at(1000), Kind: CmdShowAllCameras})
		if err != nil || len(bs) != 1 {
			t.Fatalf("MA: %v %s", err, descs(bs))
		}
		b := bs[0]
		req := h.proceed(b, 1100, kx, true).ItemReqID
		h.itemEv(1200, kx, true)
		h.itemDone(1300, req, ItemOK)
		h.bdone(1400, b.BatchID, BatchCompleted, nil)
		setItem(h.mp, kx.ContainerUUID, kx.ItemID, true)
		return h
	}
	t.Run("HB-MA-07 MA の true が確かめられたら、記録のない鍵の MAConfirm も消える", func(t *testing.T) {
		h := setup(t)
		if h.m.st.MAConfirm[kx] {
			t.Fatalf("true が確かめられたのに MAConfirm[x] が残っている")
		}
	})
	t.Run("HB-MA-08 あとの Hide の記録は、Hide が当たる前の地図の読み直しで消えない", func(t *testing.T) {
		h := setup(t)
		b := h.out(h.sIs(20000, DOWN), "Hc(s)[x]")[0]
		h.proceed(b, 20100, kx, false)
		h.wantEntries("x:P")
		h.mp.Scenes["uuid-extra"] = scenemapContainer("Extra") // 別の変化を含む読み直し（x はまだ地図で有効）
		h.smc(20200, h.mp)
		h.wantEntries("x:P")
	})
	t.Run("HB-MA-08/b MA の true が確かめられる前でも、新しい Hide の記録を作るときに古い MAConfirm は消える", func(t *testing.T) {
		h := newHB(t)
		h.yOff()
		h.setItemMapOnly(kx, false)
		bs, _ := h.evErr(Command{At: at(1000), Kind: CmdShowAllCameras})
		req := h.proceed(bs[0], 1100, kx, true).ItemReqID
		h.itemDone(1300, req, ItemOK) // true のイベントは来ない（地図は読み直しで true になる）
		h.bdone(1400, bs[0].BatchID, BatchCompleted, nil)
		setItem(h.mp, kx.ContainerUUID, kx.ItemID, true)
		h.smc(1500, h.mp)
		b := h.out(h.sIs(30000, DOWN), "Hc(s)[x]")[0]
		h.proceed(b, 30100, kx, false)
		if h.m.st.MAConfirm[kx] {
			t.Fatalf("新しい Hide の記録を作ったのに MAConfirm[x] が残っている")
		}
	})
}
