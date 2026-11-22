# S-1〜S-5 の代理の確認（Studio Mode を切るときの一瞬の切替、F-9。スマホ2台。Windows の Claude Code 向けの指示書）

あなたは、irl-live-keeper（以下「本体」）の確認を、ユーザーと一緒に進める。この指示書のフォルダ（試験のフォルダ）で作業する。**ユーザーへの文は日本語で書く。**

**前提・守ること・道具・準備・片づけは、同じフォルダの `hide-agent-brief.md` のとおりにする。** 違うのは次の点だけ:

- 記録のファイルは `studio-result.md`、見張りのファイルは `status-studio.txt` と `obs-studio.txt`
- 本体は**新しい `irl-live-keeper.exe`（2026-11-22 14:35 の版、14,135,808 バイト）**。`ctl.exe` も新しい版（`preview` の命令が増えた）。準備 1 で、ユーザーに exe を入れ替えて起動し直したことを確かめてもらう
- `ctl.exe` の命令に `preview [名前]` が増えた（Studio Mode の preview のシーンを変える。名前なしで今の値を出す）。OBS の設定を変えてよい命令に、`preview` を加える。`preview` は Studio Mode がオンのときだけ使う
- `config.yaml` は前回のまま（`main` は `on_bad: standby`、`sub` は `on_bad: hide`、`studio_mode_hide` は書かない）。ユーザーが `sub` を `standby` に戻していても、この確認には響かない（そのまま進めてよい。結果に書く）

## 背景（F-9）

OBS は Studio Mode を切ると、program を一瞬 preview のシーンにしてから元へ戻す（`SceneChanged <preview>` → `SceneChanged <元>` → Studio Mode のオフが、数ミリ秒の間に届く）。前の版の本体は、これを人の切替と受け取り、待機の持ち主と戻り先を消していた（カメラが戻っても待機から戻らない、または preview のシーンへ戻る）。新しい版は、この一瞬の切替を捨て、ログ `studio mode off bounce ignored`（`scenes` に捨てたシーンの名前）を出す。また、Studio Mode がオンの間の `warnings` の文の頭に「屋外配信では Studio Mode を勧めません。」が付いた。

## 確かめること

各項目の始まりは、2台が OK、配信中（`streaming=True`）、`scene=T-A`、`owner=none`、Studio Mode オフ。`main`（Android）を止めて送り直してもらうのが、各項目の操作になる（約5分の制限は、ここで送り直すので気にしなくてよい）。

| # | 手順 | 期待 |
|---|---|---|
| S-1 | `.\ctl.exe studio on` → `-Once` で `warnings` を確かめる → main を止めてもらう → 待機へ（最大15秒。`owner=auto return=T-A`）→ `.\ctl.exe preview T-Sub`（main が映らない、健康の良いシーン）→ `.\ctl.exe preview` で `T-Sub` を確かめる → `.\ctl.exe studio off` → 3秒の status → main を送り直してもらう → 最大40秒 | `warnings` が「屋外配信では Studio Mode を勧めません。Studio Mode のため、隠す動きの代わりに待機シーンへ送っています」。`studio off` のあとも `scene=⚠回線待機中 owner=auto return=T-A` のまま。本体のログに `studio mode off bounce ignored`（`scenes` に T-Sub と ⚠回線待機中）が出て、`scene changed by user` は出ない。main が OK になると T-A へ戻る（`AutoRecover`） |
| S-2 | S-1 と同じ。ただし preview は `.\ctl.exe preview ⚠回線待機中`（今の program と同じ） | S-1 と同じ（`scenes` は ⚠回線待機中 が2つ） |
| S-3 | S-1 と同じ。ただし preview は `.\ctl.exe preview T-B`（main が映る、悪いシーン） | S-1 と同じ。戻り先は `T-A` のままで、main が戻ると **T-B でなく T-A** へ戻る |
| S-4 | Studio Mode の中の人の切替: `.\ctl.exe studio on` → `.\ctl.exe scene T-B` → 2秒 → `.\ctl.exe scene T-A` → `.\ctl.exe studio off` | 本体のログに `scene changed by user` が `T-A→T-B` と `T-B→T-A` の2回出る（人の切替は、ふだんどおり届く）。`studio off` では `bounce ignored` が出て、`scene changed by user` は増えない |
| S-5 | 落ちたあとの起動のログ: `scene T-A`。sub を止めてもらい、隠れたら（`hidden=2`）、ユーザーにタスクマネージャーで本体を終了してもらう → ユーザーに本体を起動し直してもらう → sub を送り直してもらう | 起動し直した本体のログの最初のほうに `隠した項目の記録を読んだ（OBS につないだら照合する）` と `items=2` が出る。sub が OK になると `camera shown items=2 reason=ok` |

## 記録

`studio-result.md` に、`hide-agent-brief.md` の「記録」と同じ形で書く。項目ごとに、`studio off` の前後の status の行、本体のログの `studio mode off bounce ignored` と `scene changed by user` の行（あれば）、OBS のイベント（`obs-studio.txt` の `SceneChanged` と `StudioModeStateChanged` の時刻）を入れる。sub の回線（LAN / Tailscale）も書く。最後に、ユーザーに `studio-result.md` ができたことを伝える。
