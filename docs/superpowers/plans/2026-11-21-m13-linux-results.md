# M-13（Linux の部分）0b の実機の確認 — 結果（2026-11-21）

- 対象: main の 38d15c0（`go build ./cmd/irl-live-keeper`）。0b（`on_bad: hide`、`hidden.json`、Studio Mode、すべて表示に戻す）
- 環境: 画面のない Linux。OBS Studio 33.0.0（obs-websocket 5.7.6）を Xvfb :77 の上で動かした。設定は専用のディレクトリ（M-9 の `obs-env` を `m13/obs-env/` に `cp -a` で写したもの）に閉じ込めた。写した後、`global.ini`（4か所）と `basic.ini`（3か所）、`env.sh`、`start-sink.sh` に残っていた m9 のパスを m13 のものに直した（grep で 0 件を確認）。M-9 のディレクトリのファイルは変わっていない（mtime で確認）。MediaMTX は M-9 のもの。本体の待ち受けは別のポート（RTMP 11935、SRT 18890、RTSP 18554、API 19997、管理 18787、obs-websocket 14455）。配信先は試験だけの MediaMTX（RTMP 21935）。1935 番（システムの nginx）は使っていない
- OBS の「シーンを複製」は切ってある（`SceneDuplicationMode=false`。M-0 の設定）
- カメラ: ffmpeg の testsrc2（x264、約 2,500kbps）。RTMP を、カメラごとの TCP の中継（31935 main、31936 sub、31937 hand → 11935）に通し、500kbps に絞る（LOW の判定は 800kbps 未満）
- config.yaml: `main`（`on_bad: standby`）、`sub`（`on_bad: hide`）、`hand`（`on_bad: hide`、`recover_sec: 3`）。待機シーン `Standby`。`obs.studio_mode_hide` は項目9で変えた
- OBS のシーン（コレクション `m13`。websocket で作成、G のグループだけ JSON を編集）: `Standby`、`A`（main_src、sub_src）、`B`（main_src）、`H`（sub_src、hand_src）、`N`（A を入れ子に持つ）、`G`（main_src と、グループ `GrpSub`〔中に sub_src〕）、`Talk`、項目12のために `Hand`（hand_src だけ）を追加
- 人の操作は `tools/obsctl`（`item`、`scene`）と、標準ライブラリだけの Python の websocket のクライアント（`SetStudioModeEnabled`、`SetCurrentSceneCollection` など）で行った。時刻は 22:00〜22:15（JST）

## 結果

| # | 項目 | 結果 | 記録 |
|---|---|---|---|
| 1 | 隠す・戻す | 合格 | A で sub を 500kbps に絞って 7秒で LOW、同時に `camera hidden camera=sub items=3 reason=low`（A のまま、待機へ行かない）。status は `sub: LOW hidden=3`、OBS の `A/sub_src`、`H/sub_src`、グループ内の `sub_src` がすべて OFF、`hidden.json` に3件。戻して 10秒で OK、1秒後に `camera shown items=3 reason=ok`、3か所とも ON、`hidden.json` の `entries` は空 |
| 2 | 全部 hide で全部悪い | 合格（気になる動きあり） | H（sub と hand）で両方絞る: LOW と同時に `scene switch from=H to=Standby reason=low camera=sub purpose=AutoFail`、その 84ms 後に `camera hidden camera=sub items=3`、6秒後に `camera hidden camera=hand items=1`（待機シーンへ届いてから隠れた。H は待機のあと空になったが映らない）。両方戻す: hand が先に `camera shown`（recover_sec 3）と **同じ瞬間**（ログの時刻は同じ 22:01:33.591）に `scene switch Standby→H reason=ok camera=hand purpose=AutoRecover`。隠れていた hand が表示に戻ってから H へ戻った（H は sub が隠れたまま hand だけが映る）。sub は 5秒後に表示に戻った。→ 気になる動き 2 |
| 3 | 混ざったシーン | 合格 | A で main（standby）を絞る → 6秒で待機へ（持ち主 `auto`、戻り先 A、`hidden` は 0）。戻すと 11秒で A へ。A で sub だけを絞る → 項目1と同じく隠れるだけで A のまま（待機へ行かない） |
| 4 | 入れ子とグループ | 合格 | N（A を入れ子に持つ）を program にして sub を絞る → 7秒で LOW、`A/sub_src`、`H/sub_src`、グループ内の `sub_src` が OFF、N のまま。戻すと 11秒で全部 ON。G（main_src とグループ）でも同じ（隠れて戻る、G のまま）。G が sub のグループだけのとき（main なし）は、隠さずに待機へ送った（映るカメラがなくなるため。項目2と同じ規則） |
| 5 | 人が途中で表示を変える | 合格 | 本体が sub を隠している間に、人が `A/sub_src` を表示に戻す → 3秒後の status は `sub: hidden=2 user_shown=true`（H とグループ内は隠したまま）、`hidden item dropped camera=sub scene=A item=2 state=H` のログ。sub がそのまま LOW の 12秒間、本体は隠し直さなかった。sub が OK になると、残りの2件が表示に戻り `user_shown=false`。もう一度絞ると、また隠した（`hidden=3`） |
| 6 | 配信の停止 | 合格 | 隠している間に `POST stream/stop` → `camera shown items=3 reason=stream_stop`（約1秒）、`hidden.json` 空。配信を始め直し、sub がまだ LOW なら、約2秒後にまた隠した |
| 6 | `auto_enabled` を切る | 合格（API はなく、設定のみ） | 実行中に切り替える API はなく（設定で起動時に決まる。ホットリロードなし）、`kill -9` で落としてから `auto_enabled: false` で起動し直す形で確かめた: 起動して OBS につながった直後に `camera shown camera=sub items=3 reason=auto_off`（カメラは DOWN のまま。`hidden.json` から戻した）。以後は隠さない |
| 7 | SIGINT | 合格 | 隠している間に SIGINT → 0.5秒で止まり、`camera shown items=3 reason=shutdown`、`hidden.json` は空、OBS の項目は全部 ON。子の MediaMTX も止まった（2秒以内） |
| 7 | `kill -9` | 合格（理由の名前に注意） | `kill -9` で本体を落とすと、隠したまま残り（OBS 側3件 OFF、`hidden.json` 3件）、MediaMTX も一緒に落ちた（カメラの ffmpeg も接続が切れて終わる）。起動し直すと、OBS につながった直後に照合して戻した。ただし理由は `reason=ok`（`restore` ではない）。起動直後のカメラは DOWN から OK になるまで10秒かかるため、OK になった時点（hand は recover_sec 3 で3秒、sub は10秒）で `camera shown reason=ok`。カメラがすでに OK のときに OBS へつなぐ形（OBS を SIGSTOP で20秒止めて、その間に本体とカメラを起動）でも `reason=ok`。`reason=restore` は、設定にないカメラ（`hand` を設定から外して起動し直した）の記録を戻すときに出た: `camera shown camera=hand items=2 reason=restore`。コードの `itemReason` もそう（`cam == nil` のとき。そのほかの `restore` は、OK でも配信中でもない…の最後の既定値）。→ 気になる動き 3 |
| 8 | すべて表示に戻す | 合格 | 人が隠した `B/main_src` と `H/hand_src`、本体が隠した sub を同時に OFF にして `POST cameras/show-all`（ヘッダーあり）→ 200、全部 ON（`reason=manual_all`、カメラごとに1行）。`X-IRL-Live-Keeper` なしは 403 `forbidden`。OBS を止めているときは 503「OBS に未接続」。show-all のあと、sub がまだ LOW なら8秒後にまた隠した（`user_shown` ではないため。設計どおり） |
| 9 | Studio Mode `standby`（既定） | 合格 | Studio Mode をオン → `warnings=['Studio Mode のため、隠す動きの代わりに待機シーンへ送っています']`。A で sub を絞ると、隠さずに 6秒で待機へ（`scene switch reason=low camera=sub`、`hidden` 0、OBS の項目は ON のまま）。戻すと A へ |
| 9 | `ignore` | 合格 | `warnings=['Studio Mode のため、隠す動きを止めています']`。A で sub を絞っても何も起きない（隠さず、待機へも行かない）。Studio Mode をオフにして sub を絞り、隠したあとでオンにする → 隠したまま（`hidden=3`、warnings は同じ）。sub が OK になると、Studio Mode の間でも表示に戻った（`camera shown reason=studio_mode`。spec v0.15） |
| 9 | `hide` | 合格 | `warnings=['Studio Mode でも隠しています（OBS の『シーンを複製』を切っていないと配信に効きません）']`。sub を絞ると隠し、OK で戻した（`reason=ok`）。この環境は「シーンを複製」が切れているので、program への効きは OBS の設定どおり |
| 10 | コレクションの切替 | 合格 | 隠している間（`hidden.json` 3件、collection `m13`）に `SetCurrentSceneCollection m9` → 本体のログに切替の始まりと終わり、status の `warnings=['別の OBS かシーンコレクションで隠した項目の記録が 3 件残っています（その OBS とコレクションにつなぐと戻します）']`、`hidden.json` は m13 の3件のまま。m13 へ戻すと `warnings` は空、sub は隠れたまま（`hidden=3`）で、OK になると戻る。m9 に滞在した14秒の間に、m9 の A の sub も隠した（`collection: m9` の記録が増えた。5秒ほどあとから） |
| 11 | OBS を先に閉じる | 合格 | 隠している間に OBS を止める → 本体は動いたまま（`connected: false`、つなぎ直しのログ。`cameras` の状態は更新される）。OBS を起動し直すと、起動の約6秒後に「OBS に接続した」→ 直後に `camera shown items=3 reason=stream_stop`（OBS の再起動で配信が止まるため）。`hidden.json` の m13 の3件は空 |
| 12 | 使わない日のカメラの運用 | 合格（気になる動きあり） | A（hand を含まない）にいて hand の送信を止める → 待機へは行かない。ただし hand の項目は、今のシーンにないのに隠した（`camera hidden camera=hand items=2 reason=down`。`H/hand_src` と `Hand/hand_src`）。→ 気になる動き 1。そのあと `Hand`（hand_src だけ）へ切り替える → 約1秒で待機へ（`reason=down camera=hand`、戻り先 Hand）。hand を送り始めると、DOWN → LOW → OK（recover_sec 3）、同じ瞬間に `camera shown`（2件）と `scene switch Standby→Hand reason=ok` |

## ログの抜き書き（パスワードは含まれない）

```
camera changed camera=sub from=OK to=LOW kbps=638.9
camera hidden camera=sub items=3 reason=low
camera changed camera=sub from=LOW to=OK kbps=2594.4
camera shown camera=sub items=3 reason=ok
scene switch from=H to=Standby reason=low camera=sub kbps=502.7 purpose=AutoFail
camera hidden camera=sub items=3 reason=low          (84ms 後)
camera hidden camera=hand items=1 reason=low         (6秒後)
camera shown camera=hand items=1 reason=ok
scene switch from=Standby to=H reason=ok camera=hand kbps=2751.3 purpose=AutoRecover   (shown と同じ時刻)
hidden item dropped camera=sub scene=A item=2 state=H
camera shown camera=sub items=3 reason=stream_stop
camera shown camera=sub items=3 reason=shutdown
camera shown camera=hand items=1 reason=manual_all
camera shown camera=sub items=3 reason=auto_off
camera shown camera=sub items=3 reason=studio_mode
camera shown camera=hand items=2 reason=restore      (hand を設定から外して起動したとき)
```

ログファイル（`logs/*.log`）には、送信・読み出し・管理・obs-websocket のどのパスワードも出ていない（grep 0 件）。標準出力の起動の表示（バナー）には、送信用 URL と OBS 用 URL が出る（設計どおり）。

## `hidden.json` の例（sub を A、H、グループ内で隠している間）

```json
{
  "version": 1,
  "written_at": "2026-11-21T12:58:52.631779237Z",
  "entries": [
    {"camera": "sub", "host": "127.0.0.1", "port": 14455, "collection": "m13",
     "container": "01dafe2a-…(シーン A)", "item": 2, "source": "d11e1ba3-…",
     "state": "hidden", "settled_at": null, "written_at": "2026-11-21T12:58:52.590…Z"},
    {"camera": "sub", …, "container": "3023274d-…(シーン H)", "item": 1, "settled_at": null},
    {"camera": "sub", …, "container": "9b557e2b-…(グループ GrpSub)", "item": 1,
     "settled_at": "2026-11-21T12:58:52.591…Z"}
  ]
}
```

戻すと `"entries": []`。`settled_at` は、シーンの項目の2件が `null`、グループの中の1件だけが時刻だった（意味は未確認。`state` は3件とも `hidden`）。

## 見つけた不具合・気になる動き（直していない）

不具合と言えるものは見つからなかった。気になる動きが3つ。

1. **使わない日のカメラの項目を、そのシーンにいなくても隠す。** ブリーフには「今のシーンにない間は何もしない」とあったが、実際は、A（hand を含まない）にいて hand の送信を止めると、`H` と `Hand` の `hand_src` を隠した（`camera hidden camera=hand items=2 reason=down`）。README の「OBS のどのシーンにあるそのカメラの項目も隠します」とは合う。待機へ送る判定（「今のシーンだけで決める」）とは別の規則なので、README に「手元のシーンの項目が、送っていない間は隠れる」と書くか、仕様の意図を確かめる。害はない（隠れた hand は、届いて OK になれば先に表示に戻ってからシーンへ戻る）
   - 再現: 3台とも OK、配信中、A にいる。`cam_stop hand`（hand の ffmpeg を止める）→ 約4秒で hand が DOWN、`hidden` が 2 に。`items` で `H/hand_src` と `Hand/hand_src` が OFF
2. **全部隠れるシーンで待機へ送ったあとの、2台目の隠す動きが遅い（約6秒）。** H で sub と hand の両方が LOW のとき、sub は待機への切替の84ms後に隠したが、hand は6秒後（`camera hidden camera=hand reason=low` が 22:01:08、切替は 22:01:02）。待機シーンにいる間なので画面には影響しない。意図した待ち（`recover_sec` や `low_sec` 相当）か未確認
   - 再現: H にいて、sub と hand を同時に 500kbps に絞る
3. **`reason=restore` の意味がブリーフの書き方と違う。** ブリーフでは「`kill -9` のあと、起動し直すと照合して OK なら表示に戻す（`reason=restore`）」だが、実際は OK になった時点の `reason=ok`、`restore` は設定にないカメラの記録を戻すときだけ（`itemReason` の `cam == nil`）。ほかに、`reason=auto_off`（`auto_enabled: false` で起動）、`stream_stop`（OBS の再起動のあと配信中でない）も出る。仕様か README の「照合して戻す」の理由の書き方を、実装に合わせて確かめる
   - 再現: `kill -9` → 起動し直す → `camera shown ... reason=ok`（カメラが OK になったあと）

そのほか（本体の問題ではないもの、または軽いもの）:

- `switch.auto_enabled` を実行中に切り替える手段がない（API なし、ホットリロードなし）。項目6の `auto_off` は、設定を変えて起動し直す形でしか確かめられなかった
- Studio Mode のオン・オフの切り替えで、ログに `scene changed by user from=A to=A` が2回出る（無害だが、紛らわしい）
- 本体を `kill -9` すると、子の MediaMTX も一緒に落ちる（確認済み）。残ったポートの衝突はなかった
- M-9 の OBS 側の動きと同じで、OBS を再起動すると配信は止まるので、OBS を閉じたあとの復旧は `reason=stream_stop` になる

## 試験の道具（記録用。リポジトリの外）

`/tmp/claude-1000/-home-teru358-project-irl-live-keeper/9ba12fa0-d1e3-40a3-86be-c9d0f6c7da54/scratchpad/m13/`（`bin/env.sh`〔`cs`、`items`、`pl`、`krestart` などを追加〕、`bin/mkscenes13.py`、`obsctl`、`logs/keeper.out`〔最後の起動の分だけ〕、`keeper/`）。終了時に、自分で起動したプロセス（本体、MediaMTX〔本体の子と配信先〕、ffmpeg 3つ、中継3つ、OBS、Xvfb :77）をすべて PID で止めた。ほかのプロセス（Xvfb :99、システムの nginx 1935）には触れていない。

## まだ確かめていないこと

- Windows 版 OBS のメディアソース、IRL Link との併用、実際の「シーンを複製」がオンの Studio Mode の見え方（目視）
- 管理ページ・小さな窓の「すべて表示に戻す」ボタン（第2段階）
