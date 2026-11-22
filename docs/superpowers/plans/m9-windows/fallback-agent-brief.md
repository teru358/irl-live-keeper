# W0c-1〜W0c-9 の代理の確認（シーンごとの逃がし先、0c。スマホ2台。Windows の Claude Code 向けの指示書）

あなたは、irl-live-keeper（以下「本体」）の確認を、ユーザーと一緒に進める。この指示書のフォルダ（試験のフォルダ）で作業する。**ユーザーへの文は日本語で書く。**

**前提・守ること・道具・準備・片づけは、同じフォルダの `hide-agent-brief.md` のとおりにする。** 違うのは次の点だけ:

- 記録のファイルは `fallback-result.md`、見張りのファイルは `status-fb.txt` と `obs-fb.txt`
- 本体は**新しい `irl-live-keeper.exe`（2026-11-22 20:16 の版、14,224,384 バイト）**、`ctl.exe` も新しい版（11,415,040 バイト。`test setup` が T-A の cam-main を左上、cam-sub を右下に半分の大きさで置き、T-B と T-Sub は全画面にする。重ならないので、どちらが映っているかを目で見分けやすい）。準備 1 で、ユーザーに exe を入れ替えたことを確かめてもらう
- 道具に `standby.ps1` が増えた: `powershell -ExecutionPolicy Bypass -File .\standby.ps1 on`（または `off`）で、手動の待機の ON / OFF を押す（パスワードは `config.yaml` から読み、表示しない）
- **`config.yaml` は、ユーザーに手で書き足してもらう**（あなたは書き換えない）。`obs:` の下に次の2行を足し、本体を起動し直してもらう:
  ```yaml
    scene_fallbacks:
      - {scene: T-A, fallback: T-Sub}
  ```
  `main` は `on_bad: standby`、`sub` は `on_bad: hide` のまま。**`test setup` の前に本体を起動すると、T-A と T-Sub がまだないので、status の `warnings` に「scene_fallbacks の 1 行目: scene「T-A」が OBS にない…」が出る。`test setup` のあとで消えることも確かめる**（W0c-0）
- **送り方と回線**: 2台とも **RTMP** で送る（It's My Live の SRT は映像の track を宣言しないので映らない。T-15 の記録）。`main`（Android）は **LAN**、`sub`（iOS）は **キャリアのモバイル回線から Tailscale** で送る（`hide-agent-brief.md` の「回線」のとおり。It's My Live の URL のホストだけを Windows の Tailscale のアドレスに替える）。項目の結果に、sub の回線を書く
- 片づけの最後に、ユーザーに `scene_fallbacks` の2行を消してもらうかどうかを聞く（残しても、T-A がなければ警告が出るだけで害はない）

## 背景（0c）

本体は、シーンごとに逃がし先を持てるようになった。この確認の形では、`T-A`（main と sub）で main が悪くなると、待機ではなく `T-Sub`（sub だけ）へ逃がす。`T-Sub` の sub も悪くなると、共通の待機（`⚠回線待機中`）へ。戻るときは、逃げてきた段を逆にたどり、いちばん元に近い、入ってよいシーンへ戻る。status の `obs.standby.stack` に、逃げてきたシーンの名前の並び（スタック）が出る。

`watch-status.ps1` の1行の形には stack が出ないので、**stack は各項目で `watch-status.ps1 -Once` の `obs.standby.stack` を読む**。

## 確かめること

各項目の始まりは、2台が OK、配信中、`scene=T-A`、`owner=none`、stack が `[]`。main（Android、LAN）の約5分の制限に気をつけ、長くなったら項目の前に送り直してもらう。カメラを悪くするのは、ユーザーにアプリの送信を止めてもらうこと（DOWN）。

| # | 手順 | 期待 |
|---|---|---|
| W0c-0 | 準備の前と、`test setup` のあとで `-Once` の `warnings` | 前は `scene_fallbacks の 1 行目` の警告、あとは消える |
| W0c-1 | T-A で main を止めてもらう（最大15秒） | **T-Sub へ**（本体のログ `scene switch from=T-A to=T-Sub reason=down camera=main purpose=AutoFail`）。owner `auto`、stack `["T-A"]`。sub は隠さない |
| W0c-2 | 続けて sub も止めてもらう | 待機へ（`from=T-Sub to=⚠回線待機中`）、stack `["T-A","T-Sub"]`。待機に着いてから sub を隠す（`camera hidden camera=sub`） |
| W0c-3 | sub だけ送り直してもらう（main は止めたまま。最大40秒） | sub を表示に戻してから **T-Sub へ**（`AutoRecover`）、stack `["T-A"]` |
| W0c-4 | main も送り直してもらう | **T-A へ**、owner `none`、stack `[]` |
| W0c-5 | 人の切替: W0c-1 の形（T-Sub、`["T-A"]`）にしてから `.\ctl.exe scene T-B`（main が止まったまま） | stack が消えて owner `none` になったあと、T-B の main が悪いので待機へ（戻り先 `T-B`、stack `["T-B"]`）。main を送り直すと T-B へ |
| W0c-6 | 手動の待機: W0c-1 の形にしてから `standby.ps1 on` → 5秒 → `standby.ps1 off`（main は止めたまま） | ON で owner `manual`、戻り先 `T-A`、stack `[]`。OFF で、T-A には入れない（main が悪い）ので **T-Sub へ**（`StandbyOffMonitored`）、owner `auto`、stack `["T-A"]`。main を送り直すと T-A へ |
| W0c-7 | 待機の自動の ON の特例: W0c-2 の形（待機、`["T-A","T-Sub"]`）で `standby.ps1 on` → 2台を送り直してもらう | ON の応答は 200、切替なしで owner `manual`、戻り先 `T-A`、stack `[]`。2台が OK になっても**待機のまま**（manual）。最後に `standby.ps1 off` で T-A へ |
| W0c-8 | sub だけ悪い: T-A で sub を止めてもらう | シーンは変えず、sub を隠す（0b のまま）。送り直すと戻す |
| W0c-9 | 同時に悪くなる（記録のみ）: T-A で、ユーザーに2台をできるだけ同時に止めてもらう | 待機へ、stack `["T-A","T-Sub"]`。2台の止まる時刻がずれると、いったん T-Sub を経ることがある（どちらでも記録する） |

各項目で、本体のログの `scene switch`（from、to、reason、camera、kbps、purpose）、`standby changed`、`camera hidden/shown`、`-Once` の `obs.standby`（owner、return_scene、stack）を記録する。期待と違ったら、その場の `-Once`（`urls` を除く）、`.\ctl.exe items T-A`、`.\ctl.exe items T-Sub`、本体のログの直近の `time=` の行を残して次へ進む。

## 記録

`fallback-result.md` に、`hide-agent-brief.md` の「記録」と同じ形で書く。項目ごとに stack の値と、sub の回線（LAN / Tailscale）も書く。最後に、ユーザーに `fallback-result.md` ができたことを伝える。
