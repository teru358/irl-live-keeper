---
id: integ-overload-failures
title: 結合のテストが強い過負荷（32本×2回）で INV-03・INV-04・入場の逆順で落ちる
status: 是正済
priority: 低
kind: 保守
opened: 2026-11-23
closed: 2026-11-28
related: []
---
# [integ-overload-failures] 結合のテストが強い過負荷（32本×2回）で INV-03・INV-04・入場の逆順で落ちる

**状態**: 是正済 / **優先**: 低 / **種別**: 保守

## 現象

2026-11-23: core のテストのバイナリ（-race）を32本×-test.count=2 で並べ、TestIntegrationHiddenInvariants と FallbackInvariants を回すと（nproc 32）、status の食い違いを直したあとも、INV-03（seed 1132: 人の true の印のあとの次の false の要求）、INV-04（seed 1047: 回復のあとも hidden.json に未決着の記録が残る）、入場の逆順（seed 25002: Nest の false が mode=3 で未適用のまま Nest へ切り替えた）が出る。16本×2回では0回。実時間の待ち（StopTimeout 300ms、HiddenRetry 5ms、settle の静かさ）が過負荷で崩れる見立てで、本体の不具合かは未確定（Codex の記録の読み: scratchpad の out/fbmis.md）

## 原因

## 処置案・裁定

## 修正内容

- 2026-11-28: 試験だけ直した。(1) 偽の OBS の SetSceneUUID・StartStream・StopStream と、人の切替・配信の切替（humanScene、fbHumanScene、fbDelete、streamToggle、recoverAll）で、世界を変えてからイベントを送る・積むまでを sendMu の下で続けて行う（ロックの順は sendMu→mu）ので、SceneChanged の順が世界の変わった順と入れ替わらない。人の操作が pending から取り出されて本体へ渡る途中も未着と数える（humanTransit）。(2) settle・fbSync・INV-04 の30秒の待ちの sleep をやめ、observe のフックで数えた「積んだバッチ・BatchDone」と要求の出入りと hidden.json の書き込みの終わりの知らせで待つ（waitHiddenWritten は persistRev と writtenRev を比べる）。(3) hidden.json の書き込みを本体の世代で区切り、落ちた世代の persister の worker が新しい世代の hidden.json を古い中身で上書きする試験の側の隙間を塞いだ（INV-01・INV-04 の残りの原因）。(4) 食い違いの診断（世界の program の履歴と渡した SceneChanged・OBSConnected の列、実時間つき）を失敗の出力の頭に、世界の記録の末尾を続けて出す。(5) internal/app の waitFor の上限を10秒から60秒に。負荷の下（16本並べ、-race の core のバイナリ、HiddenInvariants と FallbackInvariants を各 count=6、ほかの作業で load average 60〜80）: 直す前 6/96 回が落ちた（INV-01・02・03・04・status の食い違い）、直したあと 0/192 回（途中の版は 3/96）。settle の打ち切り 0。

## 経緯

- 2026-11-23: 起票。
- 2026-11-23: 16本×TestIntegration でもまれに出る。S2-58 の確かめで、ブランチで 1/48（FB-INV-06 seed 20006: 改名で owner auto→none）、main で 1/48（INV-04 seed 30001: 回復から30秒で hidden.json が空でない）。seed 20006 だけを16本×4回回すと、ブランチも main も 0/64。S2-58 の変更によるものではない
- 2026-11-23 夜: ふだんの負荷の `go test -race ./...` でも1回、`TestIntegrationFallbackInvariants/fault/seed25002`（入場: 戻り先 Nest に悪い hide のカメラ [sub] が表示されたままなのに本体が戻した）が落ちた。8本×3回で回すと、main でも直しのブランチでも 0/24。過負荷だけの揺れではないので、優先を上げて調べる候補
- 2026-11-28: 状態: 裁定待ち — 2026-11-28: ふつうの go test ./... で FallbackInvariants/normal/seed20011 が1回落ちた（世界 program=Dual、status scene=SubOnly、静かな時点）。調べ（sonnet、約400回、load avg 27 まで、2コアの閉じ込めも）では 20011 は0回で未再現。本体の Machine は最後の SceneChanged だけで今のシーンを決め、順が入れ替わらない限り間違う道は見つからない。試験の偽の OBS の SetSceneUUID が mu を放してから sendNow するので、走者の humanScene と重なると SceneChanged の順が入れ替わりうる（未検証の候補）。負荷の下では別に HiddenInvariants seed1008・FallbackInvariants late seed30001 の INV-04（約110回に1〜2回）、HiddenInvariants fault seed5034 の INV-01（2コアで6回に1回）、internal/app の TestRun の10秒の待ち（43回に1回）が出た。どれも実時間の待ち（settle の 80µs、fbSync の 100µs、second() の 1ms、HiddenRetry 5ms）に寄る。判断は (c)、(a) 寄り。直し方の案: 偽の OBS の順を sendMu の下で守る、sleep のポーリングをやめて中核ループのフックで待つ、INV-04 は hidden.json の書き込みの数えに追いつくまで待つ、食い違いのときに世界の program の履歴と渡した SceneChanged の列を出す診断
- 2026-11-28: 是正内容 — 試験だけ直した。(1) 偽の OBS の SetSceneUUID・StartStream・StopStream と、人の切替・配信の切替（humanScene、fbHumanScene、fbDelete、streamToggle、recoverAll）で、世界を変えてからイベントを送る・積むまでを sendMu の下で続けて行う（ロックの順は sendMu→mu）ので、SceneChanged の順が世界の変わった順と入れ替わらない。人の操作が pending から取り出されて本体へ渡る途中も未着と数える（humanTransit）。(2) settle・fbSync・INV-04 の30秒の待ちの sleep をやめ、observe のフックで数えた「積んだバッチ・BatchDone」と要求の出入りと hidden.json の書き込みの終わりの知らせで待つ（waitHiddenWritten は persistRev と writtenRev を比べる）。(3) hidden.json の書き込みを本体の世代で区切り、落ちた世代の persister の worker が新しい世代の hidden.json を古い中身で上書きする試験の側の隙間を塞いだ（INV-01・INV-04 の残りの原因）。(4) 食い違いの診断（世界の program の履歴と渡した SceneChanged・OBSConnected の列、実時間つき）を失敗の出力の頭に、世界の記録の末尾を続けて出す。(5) internal/app の waitFor の上限を10秒から60秒に。負荷の下（16本並べ、-race の core のバイナリ、HiddenInvariants と FallbackInvariants を各 count=6、ほかの作業で load average 60〜80）: 直す前 6/96 回が落ちた（INV-01・02・03・04・status の食い違い）、直したあと 0/192 回（途中の版は 3/96）。settle の打ち切り 0。

## 2026-11-24 調査（integ-flake）

### seed 25002 の判断

保存ログ `/tmp/fbmis-logs/overload-14.log:310–330` では、12:00:42 の
Camera/2 の true が modeHang(7)、12:00:47 に適用、12:00:50 に
Nest/2 の false が modeError(3)、直後に Nest 復帰で sub 表示の違反となっている。
重要なのは Nest/2 は hand で、sub は Camera/2 経由であること。
Nest/2 の失敗だけでは sub の逆待違反の説明にならない。

テスト側 (b) の具体的な不整合を確認した。`integration_client_test.go` の旧 modeHang は
`applyLocked` の返すイベントを捨てていた。`integration_world_test.go:243` の
releaseHungDue は偽時計の5秒で待ちを解除するだけで接続は切らない。
したがって本体には false のまま、世界には true の sub が残る。
`switcher/fallback_machine.go:89` の confirmedShown は確認済み H を非表示と数え、
`switcher/items.go:666` は受け取ったイベントで地図を更新する。
実クライアントは `obs/wssession.go:789` で表示変更イベントを転送する。
この事象は「未確認 hide を本体が確認済みと判断した」という証拠ではない。

ただし fault はそもそもイベント欠落を許すプロファイルであり、実 OBS における
タイムアウトとイベント欠落の併発まで不可能と証明したわけではない。
本体の不具合 (a) を示す証拠は得られず、本体は変更しない。

### 修正

modeHang の適用イベントを既存のキューへ残す（切断時には enqueueLocked が破棄する）。
意図的な欠落の modeDropEvent は維持する。
`integration_hung_test.go` に、要求開始の条件を待って偽時計を5秒進め、
接続中の適用にイベントが伴うことを確認する固定テストを追加した。
旧実装で失敗し、修正後に成功する。sleep の延長や検査の無効化はしていない。

### 実時間への依存の棚卸し

- integration_test.go:91: HiddenRetry 5ms（hidden.go:271 の実タイマー）、StopTimeout 300ms。
- integration_test.go:145–172: 停止・Tick の5秒期限、停止の1.5秒判定。
- integration_test.go:178–192: settle は80µs sleep、3回静か、最大600回。
  faulty では inflight=0 を要求しない。処理完了の厳密な障壁ではない。
- integration_test.go:220–239: Sample 5秒期限、保存の最大200回×1msポーリング。
- integration_test.go:617: 回復の偽時計30秒と各1msの保存待ち。
- integration_fb_test.go:413,798: コマンド等3秒期限。
- integration_fb_test.go:539–549: fbSync は inflight を100µsごとに最大5秒待ち、Tick を渡す。
- integration_test.go:706 / integration_fb_test.go:1079: goroutine cleanup の20msポーリング、5秒期限。
- 遅着・hang解除・期待期限・回復時間は fakeClock だが、上記の非同期処理の進み方とは独立。

INV-03 seed1132、INV-04 seed1047/30001、FB-INV-06 seed20006 は
同じ原因と断定できない。特に INV-04 の保存待ち、settle の静かさには依然として
スケジューリング依存があり、別途条件同期を設計する余地がある。


### 今回の検証結果

- 修正前 seed25002: -race -count=3 -cpu=1,2,8、0/9失敗。元の失敗は今回再現できなかった。
- 固定テスト: 旧実装でイベント欠落の失敗。修正後 -race -count=16 -cpu=1,2,8、0/48失敗。
- 修正後 seed25002: GOMAXPROCS=1、-race -count=16 -cpu=1,2,8、0/48失敗（140.309秒）。
- go test -race ./internal/core -run '^TestIntegration' -count=1: 成功（49.192秒）。Hidden 210 seed、Fallback 48 seed と固定テストを含む。
- seed1132/1047/20006/30001: -race -count=4 -cpu=1,8、各8回、計0/32失敗（25.471秒）。
- git diff --check: 成功。16プロセス同時の外部 stress は未実施。
- core の結合テストは偽 OBS を直接注入し、ソケットを使わず実行できた。全パッケージのソケットを使うテスト・実 OBS は未検証。
- ログ: /tmp/flake-before.log、/tmp/flake-regression-after.log、/tmp/flake-seed-after.log、/tmp/flake-integration-after.log、/tmp/flake-other-seeds.log。
- 元のseed失敗を今回再現できていないため、完全解消の実証ではない。チケットは未完了。本体変更・コミットなし。
- 2026-11-24: S2-28 の Codex の環境で、ふだんの負荷の `go test -race ./...` で `TestIntegrationHiddenInvariants/normal/seed1008`（INV-04）が1回落ちた。単独では通る。S2-28 は core を変えていない
