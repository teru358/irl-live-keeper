# switcher「入口 × 状態」の表（T5 / T6 の期待値）

- 対象 spec: `docs/superpowers/specs/2026-11-18-core-design.md` v1.4 の 6章、7章、9章、11章
- 対象 task: `docs/superpowers/plans/2026-11-19-core-tasks.md` の T5、T6（実装の前にこの表をレビューする）
- 状態: 不明点は spec v1.5 の 6章「細部の規則」で決めた（すべて 4章の案どおり）。`U-xx` を引いている行は、その案を期待値とする
- 読み方: 表の行番号（`LC-1`、`CMD-START-3`、`AP-AUTOFAIL-2`、`SQ-05a` など）をテスト名かテーブルの行の名前に入れ、どの行を確かめているかを辿れるようにする

## 0. 表記

- **T 列:** その行を緑にする task（T5 か T6）
- **書いていない欄は変わらない。** 「—」は、出力なし、または状態の変化なし
- **バッチの書き方:** `AutoFail[SetScene(fail)]` は Purpose と Action の列。`W` は `RestartMedia.Wait`（spike ② で決める定数。この文書では記号のまま扱う）
- **不良:** 受信状態が LOW か DOWN
- **本体の切替 / 手動:** `SceneChanged` が期待と一致した / しなかった（6章「見分け方」）
- **決定:** 4章の不明点に対する決定（spec v1.5 の 6章「細部の規則」）

### 自動判定 J

6章「自動切替」と「再送の歯止め」をまとめたもの。表で「J」と書いたら、その Event を処理し終えた状態で次を上から確かめ、すべて満たすときだけバッチを1本返す。

1. `Connected` かつ `Streaming` かつ `Hold` でない
2. `Role == Live` かつ `Link` が不良 → Purpose は AutoFail、`[SetScene(fail)]`。`Role == Fail` かつ `Link == OK` → AutoRecover、`[RestartMedia{W}, SetScene(live)]`。どちらでもなければ出さない
3. `Pending` に同じ Purpose のバッチがない
4. `Expectations` に切替先の役割（AutoFail なら Fail、AutoRecover なら Live）への期待がない
5. `Expectations` に Live / Fail 以外の役割（Pause、Intro）への期待がない（U-12）
6. `Tick` のときだけ: `At - LastAutoSent[Purpose] >= 5秒`（起点と、送出の記録がないときの扱いは U-07）

### バッチを出したときの共通の変化 B

`Handle` か `Approve`（`Replace`）がバッチを返したら、必ず次が起きる。表では「B」と書く。

- `NextBatchID` が1つ進み、返したバッチの `BatchID` になる
- バッチはその時点の `SceneEpoch`、`ConnEpoch`、`StreamEpoch` を持つ
- `Pending[BatchID]` に Purpose と3つの Epoch を入れる
- Purpose が AutoFail / AutoRecover なら `LastAutoSent[Purpose] = At`（U-07）

`Handle` が1回で返すバッチは多くても1本である（`Handle(Event) (batch *Batch, cmdErr error)`）。下の表に、2本を同時に出す必要のある行はない。

## 1. Machine の内部状態

テストが読む語彙。フィールド名は実装とそろえる（テストから読めるよう、`Snapshot` か、パッケージ内テストの直接参照で見えるようにする）。

### 設定（生成時に受け取り、変わらない）

| 名前 | 型 | 意味 |
|---|---|---|
| `Scenes` | `map[Role]string` | 役割 → OBS のシーン名（config の `obs.scenes`）。逆引きで役割を出し、どれにも当たらなければ `RoleOther` |
| `RestartWait` | `time.Duration` | `RestartMedia.Wait` に入れる値（`W`） |
| `ExpectMargin` | `time.Duration` | 期待の期限の余裕。3秒 |
| `ExpectFloor` | `time.Duration` | 上書きあり・長さ不明のときの下限。10秒 |
| `RetryInterval` | `time.Duration` | `Tick` の再送の間隔。5秒 |

### 状態

| 名前 | 型 | 意味 |
|---|---|---|
| `Connected` | `bool` | OBS に接続しているか。`OBSConnected` で true、`OBSDisconnected` で false |
| `Streaming` | `bool` | 配信中か（7章の写像を通った値）。切断中は切断の直前の値を持ち続ける |
| `Scene` | `string` | program の現在のシーン名。切断中は切断の直前の値を持ち続ける |
| `Role` | `Role` | `Scene` を `Scenes` で変換した役割。`RoleIntro` / `RoleLive` / `RoleFail` / `RolePause` / `RoleOther` |
| `PrevConnScene` | `string` | 切断の直前のシーン名。`OBSDisconnected` で `Scene` を写す。次の `OBSConnected` との比較に使う |
| `PrevConnStreaming` | `bool` | 切断の直前の配信状態。同上（最初の接続のときの値は U-15） |
| `Link` | `ingest.LinkState` | 受信状態。`LinkOK` / `LinkLow` / `LinkDown`（初期値は U-15） |
| `Hold` | `bool` | 手動保持中か |
| `HoldSince` | `time.Time` | 保持に入った `SceneChanged` の `At`。`Hold` が false の間はゼロ値 |
| `Expectations` | `[]Expectation` | 期待の列。登録順（先頭が古い） |
| `PausePrev` | `Role` | Pause の直前の役割。`RoleIntro` / `RoleLive` / `RoleFail` か、`RoleUnknown`（不明）。`Role != Pause` の間は必ず `RoleUnknown` |
| `StartWait` | `*StartWait` | 配信開始の待ち。なければ nil |
| `Pending` | `map[int]PendingBatch` | 未完了のバッチ。`BatchDone` が来るまで持つ |
| `LastAutoSent` | `map[Purpose]time.Time` | AutoFail / AutoRecover のバッチを最後に出した時刻。再送の5秒の起点（U-07） |
| `SceneEpoch` | `int` | 手動とみなした `SceneChanged` のたびに1つ進む |
| `ConnEpoch` | `int` | `OBSDisconnected` のたびに1つ進む |
| `StreamEpoch` | `int` | 配信状態が変わるたびに1つ進む |
| `TransitionMs` | `*int` | 全体のトランジションの長さ。固定長で取れなければ nil |
| `LastError` | `*ErrorInfo` | 最後に記録したエラー（時刻と文）。status の `last_error` になる。消し方は U-05 |
| `NextBatchID` | `int` | 次に振る `BatchID`。通し番号（1 から） |
| `NextReqID` | `int` | 次に振る `ReqID`。通し番号（1 から） |
| `NextStartID` | `int` | 次に振る配信開始の待ちの通し番号（1 から） |

`Expectation` の中身:

| 名前 | 型 | 意味 |
|---|---|---|
| `ReqID` | `int` | `Approve` が `Proceed` で返した番号。`SceneRequestDone` と1対1 |
| `Role` | `Role` | 送った `SetScene` の切替先の役割 |
| `RegisteredAt` | `time.Time` | `Approve` の `at` |
| `Override` | `Override` | `Approve` に渡された上書きの状態（`None` / `Known{Ms}` / `Unknown`） |
| `TransitionMs` | `*int` | 期限に使う全体の長さ。どの時点の値を写すかは U-10 |
| `Responded` | `bool` | `SceneRequestDone` を受けたか。false の間は期限切れにならない |
| `RespondedAt` | `time.Time` | `SceneRequestDone` の `At` |
| `Result` | `OK / Timeout` | 応答の結果（`Error` は期待ごと消すので残らない） |
| `Deadline` | `time.Time` | `Responded` のときだけ有効。式は 2.8 の表 |

`StartWait` の中身: `ID int`（StreamStart ごとの通し番号）、`Stage`（`StageWaitIntro` = Intro の確認待ち / `StageIntroConfirmed` = Intro 確認済み）。

`PendingBatch` の中身: `BatchID`、`Purpose`、`SceneEpoch`、`ConnEpoch`、`StreamEpoch`、`StartID int`（StartIntro / StartStream だけ。どの待ちのためのバッチか）、`Replaced bool`（`Approve` が `Replace` を返した元のバッチ。却下を `LastError` に記録しない印）。

Purpose の一覧: `AutoFail`、`AutoRecover`、`GoLive`、`GoFail`、`PauseOffLive`、`PauseOffFail`、`PauseOffIntro`、`PauseOn`、`StartIntro`、`StartStream`、`StopStream`。「Command 由来」は Auto の2つ以外すべて。

## 2. 入口ごとの表

各表の上に「特記がなければ」の前提を書く。行の「前提」欄には、そこから変えた点だけを書く。時間に依存する値（期限の具体的な時刻、5秒）は表に入れず、条件の成否だけを書く。具体的な時刻は 3章で確かめる。

### 2.1 LinkChanged{State}

特記がなければ: 接続、配信中、保持なし、未完了なし、期待なし。すべての行で `Link` が新しい値になる。

| # | T | 前提（役割、変化） | 出力 | 状態の変化 |
|---|---|---|---|---|
| LC-1 | T5 | Live、OK → LOW | `AutoFail[SetScene(fail)]` | B |
| LC-2 | T5 | Live、OK → DOWN | `AutoFail[SetScene(fail)]` | B |
| LC-3 | T5 | Live、LOW → DOWN、AutoFail が未完了 | — | — |
| LC-4 | T5 | Live、LOW → DOWN、未完了はないが Fail への期待が残る | — | — |
| LC-5 | T5 | Live、LOW → DOWN、未完了も期待もない（前の AutoFail が `Error` で終わった直後。5秒たっていない） | `AutoFail[SetScene(fail)]`（イベント起点には5秒の条件が掛からない。U-07 の注） | B |
| LC-6 | T5 | Fail、不良 → OK | `AutoRecover[RestartMedia{W}, SetScene(live)]`（この順） | B |
| LC-7 | T5 | Fail、不良 → OK、AutoRecover が未完了、または Live への期待が残る | — | — |
| LC-8 | T5 | Fail、DOWN → LOW、または OK → 不良 | — | — |
| LC-9 | T5 | Live、不良 → OK | — | — |
| LC-10 | T5 | Intro / Pause / Other、どの変化でも | — | — |
| LC-11 | T5 | 配信中でない、Live、OK → LOW（Fail、不良 → OK も同じ） | — | — |
| LC-12 | T5 | 未接続、どの役割・変化でも（保持なし） | — | — |
| LC-13 | T5 | 保持中、Live、OK → 不良 | `AutoFail[SetScene(fail)]` | 保持を解除（`Hold=false`、`HoldSince` をゼロ値に）。B |
| LC-14 | T5 | 保持中、Live、不良 → OK | — | 保持を解除 |
| LC-15 | T5 | 保持中、Fail、不良 → OK | `AutoRecover[RestartMedia{W}, SetScene(live)]` | 保持を解除。B |
| LC-16 | T5 | 保持中、Fail、OK → 不良 | — | 保持を解除 |
| LC-17 | T5 | 保持中、Live または Fail、LOW ↔ DOWN | — | 保持は続く |
| LC-18 | T5 | 保持中、未接続、OK ↔ 不良 | — | U-13（提案: 保持を解除） |
| LC-19 | T5 | 今と同じ `State` が届いた | — | U-14（提案: 何も変えない。保持も解除しない） |
| LC-20 | T6 | Live、OK → 不良、Pause への期待が残る（PauseOn を送った直後で、`SceneChanged(Pause)` がまだ届いていない） | spec の文面どおりなら `AutoFail[SetScene(fail)]`。U-12（提案: 出さない） | U-12 |

### 2.2 OBSConnected{Streaming, Scene, TransitionMs}

前提は未接続。すべての行で `Connected=true`、`Streaming`、`Scene`、`Role`、`TransitionMs` を Event の値にし、`PausePrev=RoleUnknown` にする。「同じ / 違う」は `PrevConnScene`、`PrevConnStreaming` との比較。

| # | T | 前提 | 出力 | 状態の変化 |
|---|---|---|---|---|
| OC-1 | T5 | 起動後の最初の接続。`Streaming=false` | — | `StreamEpoch` は進まない |
| OC-2 | T5 | 起動後の最初の接続。`Streaming=true` | J | `StreamEpoch` は U-15（提案: 初期値 false と違うので +1） |
| OC-3 | T5 | 再接続。シーンも配信状態も同じ、配信中、保持なし、Live、不良 | `AutoFail[SetScene(fail)]` | B。`StreamEpoch` は進まない |
| OC-4 | T5 | 同上、Fail、OK | `AutoRecover[RestartMedia{W}, SetScene(live)]` | B |
| OC-5 | T5 | 同上、Live かつ OK、Fail かつ不良、Intro / Pause / Other | — | — |
| OC-6 | T5 | 再接続。配信中でない（切断前も配信中でない） | — | — |
| OC-7 | T5 | 再接続。保持中、シーンも配信状態も同じ（Live かつ不良、または Fail かつ OK） | —（保持中は判定しない） | 保持は続く（`HoldSince` も同じ） |
| OC-8 | T5 | 再接続。保持中、シーンが違う | J（解除したあとの状態で） | 保持を解除。`SceneEpoch` は進まない（U-23） |
| OC-9 | T5 | 再接続。保持中、配信状態が true → false | — | 保持を解除。`StreamEpoch+1` |
| OC-10 | T5 | 再接続。配信状態が false → true（保持なし） | J | `StreamEpoch+1` |
| OC-11 | T5 | 再接続。Pause で切断し Pause のまま（切断前の `PausePrev` は Live） | — | `PausePrev=RoleUnknown` |
| OC-12 | T5 | `TransitionMs` が nil | 他の行と同じ | `TransitionMs=nil` |
| OC-13 | T5 | 再接続。Live、不良、切断前の AutoFail がまだ `Pending` にある（`BatchDone` が未着） | —（J の 3） | — |

### 2.3 OBSDisconnected{}

前提は接続。すべての行で出力はなく、`Connected=false`、`ConnEpoch+1`、`Expectations` を空に、`PrevConnScene=Scene`、`PrevConnStreaming=Streaming` にする。`Streaming`、`Scene`、`Role`、`Pending`、`PausePrev`（`OBSConnected` で戻す）は変えない。

| # | T | 前提 | 出力 | 状態の変化（上の共通に加えて） |
|---|---|---|---|---|
| OD-1 | T5 | 期待あり、未完了あり | — | 期待は全部消える。`Pending` は残る |
| OD-2 | T5 | 保持中 | — | 保持は続く |
| OD-3 | T6 | 配信開始の待ちあり（どちらの段階でも） | — | `StartWait=nil`、`LastError` を記録 |

### 2.4 SceneChanged{Scene}（期待どおり = 本体の切替）

前提: 接続。`Expectations` に、Event のシーンの役割と一致する期待がある（応答の前でも後でも、`Timeout` の応答のあとでも、期限の前なら同じ）。すべての行で `Scene`、`Role` を更新し、一致した期待とそれより前の期待を消す。`SceneEpoch` は進めず、保持には入らない。役割が Pause から出たら `PausePrev=RoleUnknown`。

| # | T | 前提 | 出力 | 状態の変化 |
|---|---|---|---|---|
| SCE-1 | T5 | 期待 [Fail]、Live → Fail、配信中、不良 | — | 期待が空になる |
| SCE-2 | T5 | 期待 [Fail]、Live → Fail、配信中、その間に OK に戻っていた | `AutoRecover[RestartMedia{W}, SetScene(live)]` | B |
| SCE-3 | T5 | 期待 [Live]、Fail → Live、配信中、その間に不良になっていた | `AutoFail[SetScene(fail)]` | B |
| SCE-4 | T5 | 期待 [Live]、Fail → Live、配信中、OK | — | — |
| SCE-5 | T5 | 期待 [Fail, Pause]、Live で `SceneChanged(Pause)` | — | 期待は両方消える。`PausePrev=RoleLive`（Event の直前の役割） |
| SCE-6 | T5 | 期待 [Fail, Pause]、Live で `SceneChanged(Fail)` | —（Pause への期待が残っているので J の 5 を満たさない。U-12） | Fail だけ消え、Pause は残る |
| SCE-7 | T5 | 期待 [Pause, Fail]、Live で `SceneChanged(Pause)` | — | Pause だけ消え、後ろの Fail は残る。`PausePrev=RoleLive` |
| SCE-8 | T5 | 期待 [Pause]、直前の役割が Intro / Live / Fail | — | `PausePrev` はその役割 |
| SCE-9 | T6 | 配信中でない、期待 [Intro]、`StartWait` が `StageWaitIntro` で、期待はその StreamStart の StartIntro が登録したもの | `StartStream[StartStream]` | `Stage=StageIntroConfirmed`。B（`StartID` は待ちの `ID`） |
| SCE-10 | T6 | 配信中でない、期待 [Intro]、`StartWait` なし（待ちを抜けたあとの遅い Intro） | — | 役割が Intro になるだけ |
| SCE-11 | T6 | 配信中でない、期待 [Intro] は前の StreamStart のもの、`StartWait` は新しい `ID` で `StageWaitIntro` | U-03（提案: `StartStream[StartStream]`） | U-03（提案: `StageIntroConfirmed` に進む） |
| SCE-12 | T6 | 配信中、期待 [Intro]（PauseOffIntro）、Pause → Intro | — | `PausePrev=RoleUnknown` |
| SCE-13 | T6 | 配信中、期待 [Live]（PauseOffLive / GoLive）、その間に不良になっていた | `AutoFail[SetScene(fail)]` | `PausePrev=RoleUnknown`。B |
| SCE-14 | T6 | 配信中、期待 [Fail]（PauseOffFail / GoFail）、その間に OK になっていた | `AutoRecover[RestartMedia{W}, SetScene(live)]` | `PausePrev=RoleUnknown`。B |

保持中に期待どおりの `SceneChanged` が届く組み合わせはない（保持に入るとき期待は全部消え、保持中に `Proceed` になるのは Command のバッチだけで、Command の受け付けで保持は解ける）。

### 2.5 SceneChanged{Scene}（手動 = どの期待とも一致しない）

前提: 接続。すべての行で出力はなく、`Scene`、`Role` を更新し、`SceneEpoch+1`、`Expectations` を空にする。J は走らない（配信中の Live / Fail は保持に入るため。それ以外は自動切替の対象外）。

| # | T | 前提（新しい役割） | 出力 | 状態の変化（上の共通に加えて） |
|---|---|---|---|---|
| SCM-1 | T5 | 配信中、→ Live（不良でも） | — | 保持に入る（`Hold=true`、`HoldSince=At`） |
| SCM-2 | T5 | 配信中、→ Fail（OK でも） | — | 保持に入る |
| SCM-3 | T5 | 配信中、保持中、→ Intro / Other | — | 保持を解除 |
| SCM-4 | T5 | 配信中、Intro / Live / Fail → Pause | — | `PausePrev` は直前の役割。保持中なら解除 |
| SCM-5 | T5 | Other → Pause | — | `PausePrev=RoleUnknown` |
| SCM-6 | T5 | 配信中、Pause → Live / Fail | — | `PausePrev=RoleUnknown`。保持に入る |
| SCM-7 | T5 | Pause → Intro / Other | — | `PausePrev=RoleUnknown` |
| SCM-8 | T5 | 配信中でない、→ Live / Fail | — | 保持に入らない |
| SCM-9 | T5 | 期待 [Fail] が残る、`SceneChanged(Pause)`（期待にない別のシーン） | — | 期待は全部消える。`PausePrev` は直前の役割 |
| SCM-10 | T5 | 期待が期限切れで消えたあとに、その期待のシーンが届いた | — | 手動として扱う（配信中の Live / Fail なら保持に入る） |
| SCM-11 | T5 | 保持中、Live → Fail（または Fail → Live） | — | 保持は続く。`HoldSince` は U-20（提案: 変えない） |
| SCM-12 | T6 | `StartWait` あり（どちらの段階でも。新しい役割が Intro でも） | — | `StartWait=nil`、`LastError` を記録 |
| SCM-13 | T5 | AutoFail / AutoRecover が未完了（承認の前） | — | `Pending` は残る（承認で `SceneEpoch` により却下される。AP-COM-5） |

### 2.6 TransitionChanged{TransitionMs}

| # | T | 前提 | 出力 | 状態の変化 |
|---|---|---|---|---|
| TC-1 | T5 | 接続、値あり（例: 5000） | — | `TransitionMs=5000` |
| TC-2 | T5 | 接続、nil | — | `TransitionMs=nil` |
| TC-3 | T5 | 承認済みで応答待ちの期待がある | — | その期待の期限に効くかは U-10（提案: 効かない） |

### 2.7 StreamStateChanged{Streaming}

前提: 接続。

| # | T | 前提 | 出力 | 状態の変化 |
|---|---|---|---|---|
| SST-1 | T5 | `true`。配信中でない、Live、不良 | `AutoFail[SetScene(fail)]` | `Streaming=true`、`StreamEpoch+1`。B |
| SST-2 | T5 | `true`。配信中でない、Fail、OK | `AutoRecover[RestartMedia{W}, SetScene(live)]` | 同上 |
| SST-3 | T5 | `true`。配信中でない、Live かつ OK、Fail かつ不良、Intro / Pause / Other | — | `Streaming=true`、`StreamEpoch+1` |
| SST-4 | T6 | `true`。`StartWait` が `StageIntroConfirmed` | J（Intro なので出ない） | `Streaming=true`、`StreamEpoch+1`、`StartWait=nil`。`LastError` は U-17（提案: 記録しない） |
| SST-5 | T6 | `true`。`StartWait` が `StageWaitIntro`（待ちの間に OBS の画面で配信を始めた） | J | `Streaming=true`、`StreamEpoch+1`、`StartWait=nil`。`LastError` は U-17 |
| SST-6 | T5 | `true`。すでに配信中 | — | U-14（提案: 何も変えない。`StreamEpoch` も進めない） |
| SSF-1 | T5 | `false`。配信中、期待あり | — | `Streaming=false`、`StreamEpoch+1`、`Expectations` を空に。`LastAutoSent` は U-16 |
| SSF-2 | T5 | `false`。配信中、保持中 | — | SSF-1 に加えて保持を解除 |
| SSF-3 | T5 | `false`。配信中、Pause（`PausePrev` は Live） | — | SSF-1 に加えて `PausePrev=RoleUnknown` |
| SSF-4 | T5 | `false`。配信中、未完了あり | — | `Pending` は残る（承認で `StreamEpoch` により却下される） |
| SSF-5 | T5 | `false`。すでに配信中でない | — | U-14（提案: 何も変えない） |

### 2.8 SceneRequestDone{ReqID, Result}

出力はどの行もなし（J も走らない。期待が消えたあとの再送は `Tick` が行う）。

| # | T | 前提 | 状態の変化 |
|---|---|---|---|
| SRD-OK-1 | T5 | `OK`。その `ReqID` の期待が応答待ち | `Responded=true`、`RespondedAt=At`、`Result=OK`、`Deadline` を下の式で決める。期待は残る |
| SRD-OK-2 | T5 | `OK`。その `ReqID` の期待がない（先に `SceneChanged` で消えた、切断や配信停止で消えた、未知の番号） | — |
| SRD-TO-1 | T5 | `Timeout`。期待が応答待ち | SRD-OK-1 と同じ（`Result=Timeout`）。期待は期限まで残る |
| SRD-TO-2 | T6 | `Timeout`。Intro への期待はすでに `SceneChanged` で消えていて、`StartWait` は `StageIntroConfirmed` | —（待ちは消えない） |
| SRD-ERR-1 | T5 | `Error`。期待が応答待ち | その期待だけをすぐ消す（列の他の期待は残る） |
| SRD-ERR-2 | T6 | `Error`。Intro への期待、`StartWait` が `StageWaitIntro` | 期待を消す。`StartWait=nil`、`LastError` を記録 |
| SRD-ERR-3 | T5 | `Error`。その `ReqID` の期待がない | — |

`Deadline` の式（`G` は全体の長さ。どの時点の `G` かは U-10）:

| # | T | `Override` | 全体の長さ | `Deadline` |
|---|---|---|---|---|
| DL-1 | T5 | `None` | `G` あり | `RespondedAt + G + 3秒` |
| DL-2 | T5 | `None` | nil | `RespondedAt + 10秒` |
| DL-3 | T5 | `Known{Ms}` | どちらでも | `RespondedAt + max(Ms + 3秒, 10秒)` |
| DL-4 | T5 | `Unknown` | `G` あり | U-09（提案: `RespondedAt + max(G + 3秒, 10秒)`） |
| DL-5 | T5 | `Unknown` | nil | `RespondedAt + 10秒` |

### 2.9 BatchDone{BatchID, Result, Err}

すべての行で `Pending` からその `BatchID` を消す。出力は BD-C-4 を除いてなし（J は走らない）。

| # | T | 前提（Purpose、結果） | 状態の変化（`Pending` から消すのに加えて） |
|---|---|---|---|
| BD-C-1 | T5 | `Completed`。Auto、または StartIntro / StartStream 以外の Command 由来 | —（`LastError` は記録しない。`Err` が nil でないときは U-18） |
| BD-C-2 | T6 | `Completed`。StartStream、`StartWait` はそのバッチの `StartID` | `StartWait=nil`。`LastError` は記録しない |
| BD-C-3 | T6 | `Completed`。StartIntro、`StageWaitIntro`、役割はまだ Intro でない（イベント待ち） | 待ちは残る |
| BD-C-4 | T6 | `Completed`。StartIntro（`SetScene` が `Skip` された）、`StageWaitIntro`、役割はすでに Intro | U-02（提案: `StageIntroConfirmed` に進み、`StartStream[StartStream]` を返す） |
| BD-F-1 | T5 | `Failed`。Auto | `LastError` は U-05（提案: 記録する）。再送は `Tick` |
| BD-F-2 | T6 | `Failed`。GoLive / GoFail / PauseOn / PauseOff* / StopStream | `LastError` を記録 |
| BD-F-3 | T6 | `Failed`。StartIntro、`StageWaitIntro` | `StartWait=nil`、`LastError` を記録 |
| BD-F-4 | T6 | `Failed`。StartIntro、`StageIntroConfirmed`（確認後の応答のタイムアウト） | 待ちは残る。`LastError` は U-17（提案: 記録しない） |
| BD-F-5 | T6 | `Failed`。StartStream | `StartWait=nil`、`LastError` を記録 |
| BD-R-1 | T5 | `Rejected`。Auto | —（`LastError` は記録しない） |
| BD-R-2 | T6 | `Rejected`。Command 由来、`Replaced` でない | `LastError` を記録 |
| BD-R-3 | T6 | `Rejected`。`Replaced` の GoLive / PauseOffLive | —（`LastError` は記録しない） |
| BD-R-4 | T6 | `Rejected`。StartIntro / StartStream | `StartWait=nil`（承認の時点で消えている）、`LastError` を記録 |
| BD-X-1 | T5 | 未知の `BatchID` | U-22（提案: 何もしない） |
| BD-X-2 | T6 | StartIntro / StartStream で、`StartWait` の `ID` がバッチの `StartID` と違う | 到達しない。前の StartIntro / StartStream が未完了の間は次の StreamStart を受け付けない（U-04）ので、番号の違う待ちとバッチは同時に存在しない。テストは書かず、CMD-START-6 で確かめる |

### 2.10 Tick{}

すべての行で、まず期限を過ぎた期待を消し（境界は U-11）、次に J を確かめる。

| # | T | 前提 | 出力 | 状態の変化 |
|---|---|---|---|---|
| TK-1 | T5 | 接続、配信中、Live、不良、保持なし、未完了なし、期待なし、前回の AutoFail の送出から5秒以上 | `AutoFail[SetScene(fail)]` | B |
| TK-2 | T5 | TK-1 で、前回の送出から5秒未満 | — | — |
| TK-3 | T5 | TK-1 で、AutoFail が未完了 | — | — |
| TK-4 | T5 | TK-1 で、Fail への期待が残る（応答待ち、または期限の前） | — | 期待は残る |
| TK-5 | T5 | TK-1 で、保持中 | — | — |
| TK-6 | T5 | Fail、OK、ほかは TK-1 と同じ（AutoRecover の送出から5秒以上） | `AutoRecover[RestartMedia{W}, SetScene(live)]` | B |
| TK-7 | T5 | Live かつ OK、Fail かつ不良、Intro / Pause / Other、配信中でない、未接続 | — | — |
| TK-8 | T5 | 期限を過ぎた期待あり | J（消したあとの状態で） | その期待を消す。`SceneEpoch` は進めず、保持にも入らない（U-11） |
| TK-9 | T5 | 応答待ち（`Responded=false`）の期待。登録からどれだけたっていても | — | 期待は残る |
| TK-10 | T6 | 期限を過ぎた Intro への期待、`StartWait` が `StageWaitIntro` | — | 期待を消す。`StartWait=nil`、`LastError` を記録 |
| TK-11 | T5 | J の 1〜4 を満たすが、その Purpose を一度も出していない（保持が Command で解けたあと、など） | U-07（提案: 出す） | U-07 |

### 2.11 Command

共通: `cmdErr` を返す行では状態を何も変えない（保持も解除しない）。受け付けた行では保持を解除する（範囲は U-08）。未接続の判定が最初（`ErrNotConnected`）。

**StreamStart**

| # | T | 前提 | `cmdErr` | 出力 | 状態の変化 |
|---|---|---|---|---|---|
| CMD-START-1 | T6 | 未接続 | `ErrNotConnected` | — | — |
| CMD-START-2 | T6 | 接続、配信中 | `ErrInvalidState` | — | — |
| CMD-START-3 | T6 | 接続、配信中でない、`StartWait` あり（どちらの段階でも） | `ErrInvalidState` | — | — |
| CMD-START-4 | T6 | 接続、配信中でない、待ちなし、Intro | nil | `StartStream[StartStream]` | `StartWait={ID, StageIntroConfirmed}`、`NextStartID+1`。B |
| CMD-START-5 | T6 | 接続、配信中でない、待ちなし、Live / Fail / Pause / Other | nil | `StartIntro[SetScene(intro)]` | `StartWait={ID, StageWaitIntro}`、`NextStartID+1`。B |
| CMD-START-6 | T6 | CMD-START-4 / 5 の前提で、前の StreamStart の StartStream / StartIntro がまだ `Pending` にある | U-04（提案: `ErrInvalidState`） | U-04 | U-04 |

**StreamStop**

| # | T | 前提 | `cmdErr` | 出力 | 状態の変化 |
|---|---|---|---|---|---|
| CMD-STOP-1 | T6 | 未接続 | `ErrNotConnected` | — | — |
| CMD-STOP-2 | T6 | 接続、配信中でない | `ErrInvalidState` | — | — |
| CMD-STOP-3 | T6 | 接続、配信中（役割、受信状態は何でも） | nil | `StopStream[StopStream]` | B |
| CMD-STOP-4 | T6 | CMD-STOP-3 で保持中 | nil | `StopStream[StopStream]` | U-08（提案: 保持を解除）。B |
| CMD-STOP-5 | T6 | CMD-STOP-3 で、StopStream が未完了 | U-04（提案: `ErrInvalidState`） | U-04 | U-04 |

**GoLive**

| # | T | 前提 | `cmdErr` | 出力 | 状態の変化 |
|---|---|---|---|---|---|
| CMD-GOLIVE-1 | T6 | 未接続 | `ErrNotConnected` | — | — |
| CMD-GOLIVE-2 | T6 | 接続、配信中でない（役割は何でも） | `ErrInvalidState` | — | — |
| CMD-GOLIVE-3 | T6 | 接続、配信中、Live / Fail / Pause / Other | `ErrInvalidState` | — | — |
| CMD-GOLIVE-4 | T6 | 接続、配信中、Intro、OK | nil | `GoLive[RestartMedia{W}, SetScene(live)]` | B |
| CMD-GOLIVE-5 | T6 | 接続、配信中、Intro、LOW / DOWN | nil | `GoFail[SetScene(fail)]` | B |
| CMD-GOLIVE-6 | T6 | CMD-GOLIVE-4 / 5 で、GoLive か GoFail が未完了 | U-04（提案: `ErrInvalidState`） | U-04 | U-04 |

**PauseOn**

| # | T | 前提 | `cmdErr` | 出力 | 状態の変化 |
|---|---|---|---|---|---|
| CMD-PON-1 | T6 | 未接続 | `ErrNotConnected` | — | — |
| CMD-PON-2 | T6 | 接続、配信中でない | `ErrInvalidState` | — | — |
| CMD-PON-3 | T6 | 接続、配信中、Pause / Other | `ErrInvalidState` | — | — |
| CMD-PON-4 | T6 | 接続、配信中、Intro / Live / Fail（受信状態は何でも） | nil | `PauseOn[SetScene(pause)]` | B。`PausePrev` はまだ変えない（`SceneChanged` で決まる） |
| CMD-PON-5 | T6 | CMD-PON-4 で保持中（Live / Fail） | nil | `PauseOn[SetScene(pause)]` | U-08（提案: 保持を解除）。B |
| CMD-PON-6 | T6 | CMD-PON-4 で、PauseOn が未完了 | U-04（提案: `ErrInvalidState`） | U-04 | U-04 |

**PauseOff**

| # | T | 前提 | `cmdErr` | 出力 | 状態の変化 |
|---|---|---|---|---|---|
| CMD-POFF-1 | T6 | 未接続 | `ErrNotConnected` | — | — |
| CMD-POFF-2 | T6 | 接続、配信中でない | `ErrInvalidState` | — | — |
| CMD-POFF-3 | T6 | 接続、配信中、Intro / Live / Fail / Other | `ErrInvalidState` | — | — |
| CMD-POFF-4 | T6 | 接続、配信中、Pause、`PausePrev=Intro`（受信状態は何でも） | nil | `PauseOffIntro[SetScene(intro)]` | B |
| CMD-POFF-5 | T6 | 接続、配信中、Pause、`PausePrev` が Live / Fail / 不明、OK | nil | `PauseOffLive[RestartMedia{W}, SetScene(live)]` | B |
| CMD-POFF-6 | T6 | 接続、配信中、Pause、`PausePrev` が Live / Fail / 不明、LOW / DOWN | nil | `PauseOffFail[SetScene(fail)]` | B |
| CMD-POFF-7 | T6 | CMD-POFF-4〜6 で、PauseOff* のどれかが未完了 | U-04（提案: `ErrInvalidState`） | U-04 | U-04 |

### 2.12 Approve(batchID, action, override, at)

確かめる順（Skip と有効条件の前後は U-01）: 共通の条件 → Purpose の有効条件。`Proceed` を `SetScene` に返したときは、`Expectations` の末尾に `{ReqID, Role, RegisteredAt=at, Override, Responded=false}` を足し、`NextReqID` を進める（表では「期待を登録」と書く）。`SetScene` 以外の `Proceed` が返す `ReqID` は U-21。`Approve` は `Pending` から消さない（消すのは `BatchDone`）。

**共通の条件（どの Purpose でも）**

| # | T | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|
| AP-COM-1 | T6 | 未接続（`OBSDisconnected` のあと、`OBSConnected` の前） | `Reject` | — |
| AP-COM-2 | T6 | 接続しているが、バッチを作ったあとに切断と再接続があった（`ConnEpoch` が違う） | `Reject` | — |
| AP-COM-3 | T6 | 配信状態がバッチを作ったあとに変わった（`StreamEpoch` が違う。止まったまま、または止まって始め直した） | `Reject` | — |
| AP-COM-4 | T6 | 未知の `batchID`（`Pending` にない） | U-22（提案: `Reject`） | — |
| AP-COM-5 | T5 | シーンを切り替えるバッチ（StartStream と StopStream 以外）で、作ったあとに手動の `SceneChanged` があった（`SceneEpoch` が違う）。`RestartMedia` の承認も同じ | `Reject` | — |

各 Purpose の「配信中」「配信中でない」「手動保持でない」は、バッチを作ったあとに崩れると必ず `StreamEpoch` か `SceneEpoch` が進むので、AP-COM-3 と AP-COM-5 で確かめたことになる（U-14 の重複の Event を除く）。下の表には、共通の条件を満たしたうえで結果が分かれる行だけを書く。

**AutoFail** `[SetScene(fail)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-AUTOFAIL-1 | T5 | `SetScene(fail)` | Live、LOW | `Proceed{ReqID}` | 期待を登録（Fail） |
| AP-AUTOFAIL-2 | T5 | `SetScene(fail)` | Live、DOWN | `Proceed{ReqID}` | 期待を登録（Fail） |
| AP-AUTOFAIL-3 | T6 | `SetScene(fail)` | Live、OK に戻った | `Reject` | — |
| AP-AUTOFAIL-4 | T6 | `SetScene(fail)` | 本体の切替で Pause / Intro になった（`SceneEpoch` は同じ）、不良 | `Reject` | — |
| AP-AUTOFAIL-5 | T6 | `SetScene(fail)` | 本体の切替ですでに Fail | U-01（提案: `Skip`） | — |
| AP-AUTOFAIL-6 | T6 | `SetScene(fail)` | Live、不良、Pause か Intro への期待が残っている | `Reject`（U-12） | — |

**AutoRecover** `[RestartMedia{W}, SetScene(live)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-AUTOREC-1 | T5 | `RestartMedia` | Fail、OK | `Proceed` | —（期待は登録しない） |
| AP-AUTOREC-2 | T5 | `SetScene(live)` | Fail、OK | `Proceed{ReqID}` | 期待を登録（Live） |
| AP-AUTOREC-3 | T6 | どちらでも | Fail、LOW / DOWN になった | `Reject` | — |
| AP-AUTOREC-4 | T6 | どちらでも | 本体の切替で Pause になった（PauseOn が先に通った）、OK | `Reject` | — |
| AP-AUTOREC-5 | T6 | `SetScene(live)` | 本体の切替ですでに Live | U-01（提案: `Skip`） | — |
| AP-AUTOREC-6 | T6 | `RestartMedia` | 本体の切替ですでに Live | `Reject`（`Skip` は `SetScene` だけ） | — |
| AP-AUTOREC-7 | T6 | どちらでも | Fail、OK、Pause か Intro への期待が残っている | `Reject`（U-12） | — |

**GoLive** `[RestartMedia{W}, SetScene(live)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-GOLIVE-1 | T6 | `RestartMedia` | Intro、OK | `Proceed` | — |
| AP-GOLIVE-2 | T6 | `SetScene(live)` | Intro、OK | `Proceed{ReqID}` | 期待を登録（Live） |
| AP-GOLIVE-3 | T6 | `RestartMedia` | Intro、LOW / DOWN（受信状態だけが悪くなった） | `Replace{GoFail[SetScene(fail)]}` | 元のバッチに `Replaced=true`。代わりのバッチに B（新しい `BatchID`） |
| AP-GOLIVE-4 | T6 | `SetScene(live)` | 同上（`RestartMedia` を送ったあとに悪くなった） | `Replace{GoFail[SetScene(fail)]}` | 同上。期待は登録しない |
| AP-GOLIVE-5 | T6 | どちらでも | 本体の切替で Pause になった、OK | `Reject` | — |
| AP-GOLIVE-6 | T6 | どちらでも | 本体の切替で Pause になった、不良（受信状態以外も崩れた） | `Reject`（`Replace` にしない） | — |
| AP-GOLIVE-7 | T6 | `SetScene(live)` | 本体の切替ですでに Live | U-01（提案: `Skip`） | — |

**PauseOffLive** `[RestartMedia{W}, SetScene(live)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-POFFLIVE-1 | T6 | `RestartMedia` | Pause、OK | `Proceed` | — |
| AP-POFFLIVE-2 | T6 | `SetScene(live)` | Pause、OK | `Proceed{ReqID}` | 期待を登録（Live） |
| AP-POFFLIVE-3 | T6 | どちらでも | Pause、LOW / DOWN | `Replace{PauseOffFail[SetScene(fail)]}` | 元のバッチに `Replaced=true`。代わりのバッチに B |
| AP-POFFLIVE-4 | T6 | どちらでも | 本体の切替で Pause 以外（Intro / Fail）になった | `Reject`（受信状態が不良でも） | — |
| AP-POFFLIVE-5 | T6 | `SetScene(live)` | 本体の切替ですでに Live | U-01（提案: `Skip`） | — |

**GoFail** `[SetScene(fail)]`（Command で直接出たものも、`Replace` で出たものも同じ）

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-GOFAIL-1 | T6 | `SetScene(fail)` | Intro、受信状態は OK / LOW / DOWN のどれでも | `Proceed{ReqID}`（OK に戻っていても `Replace` にしない） | 期待を登録（Fail） |
| AP-GOFAIL-2 | T6 | `SetScene(fail)` | 本体の切替で Pause になった | U-06（提案: `Reject`） | — |
| AP-GOFAIL-3 | T6 | `SetScene(fail)` | 本体の切替で Live になった | `Reject` | — |
| AP-GOFAIL-4 | T6 | `SetScene(fail)` | 本体の切替ですでに Fail | U-01（提案: `Skip`） | — |

**PauseOffFail** `[SetScene(fail)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-POFFFAIL-1 | T6 | `SetScene(fail)` | Pause、受信状態は何でも | `Proceed{ReqID}` | 期待を登録（Fail） |
| AP-POFFFAIL-2 | T6 | `SetScene(fail)` | 本体の切替で Intro になった | U-06（提案: `Reject`） | — |
| AP-POFFFAIL-3 | T6 | `SetScene(fail)` | 本体の切替で Live になった | `Reject` | — |
| AP-POFFFAIL-4 | T6 | `SetScene(fail)` | 本体の切替ですでに Fail | U-01（提案: `Skip`） | — |

**PauseOffIntro** `[SetScene(intro)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-POFFINTRO-1 | T6 | `SetScene(intro)` | Pause、受信状態は何でも | `Proceed{ReqID}` | 期待を登録（Intro） |
| AP-POFFINTRO-2 | T6 | `SetScene(intro)` | 本体の切替で Live / Fail になった | `Reject` | — |
| AP-POFFINTRO-3 | T6 | `SetScene(intro)` | 本体の切替ですでに Intro | U-01（提案: `Skip`） | — |

**PauseOn** `[SetScene(pause)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-PON-1 | T6 | `SetScene(pause)` | Intro / Live / Fail、受信状態は何でも（作ったあとに OK ↔ 不良、LOW ↔ DOWN と変わっていても） | `Proceed{ReqID}` | 期待を登録（Pause） |
| AP-PON-2 | T6 | `SetScene(pause)` | 作ったときは Live、本体の切替で Fail になった（AutoFail が先に通った） | `Proceed{ReqID}` | 期待を登録（Pause） |
| AP-PON-3 | T6 | `SetScene(pause)` | 本体の切替ですでに Pause（前の PauseOn の遅い `SceneChanged`） | U-01（提案: `Skip`） | — |

**StartIntro** `[SetScene(intro)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-STARTINTRO-1 | T6 | `SetScene(intro)` | 配信中でない、`StartWait` がこのバッチの `StartID`、役割は Intro 以外 | `Proceed{ReqID}` | 期待を登録（Intro） |
| AP-STARTINTRO-2 | T6 | `SetScene(intro)` | `StartWait` がない（手動の `SceneChanged`、切断、`StreamStateChanged{true}` で消えた。このとき共通の条件も崩れている） | `Reject` | —（`StreamStateChanged{true}` で消えた場合は、待ちを消した時点では記録しない（U-17）。手動の `SceneChanged` と切断で消えた場合は、待ちを消した時点で記録する。どの場合も、残ったバッチの `BatchDone{Rejected}` は Command 由来なので記録する） |
| AP-STARTINTRO-3 | T6 | `SetScene(intro)` | `StartWait` はあるが `ID` が違う（次の StreamStart の待ち） | 到達しない（BD-X-2 と同じ理由。U-04） | テストは書かない |
| AP-STARTINTRO-4 | T6 | `SetScene(intro)` | 待ちあり、前の StreamStart の期待で役割がすでに Intro | `Skip`（期待を登録しない） | 待ちの段階は U-02 / U-03 |

「却下（待ちも消す）」について: 待ちを残したまま StartIntro / StartStream が却下される列は、AP-STARTINTRO-3 を除いて作れない（共通の条件を崩す Event は、どれも先に待ちを消す）。実装は却下のときに自分の `StartID` の待ちを消すが、表の行としては AP-STARTINTRO-3 だけになる。

**StartStream** `[StartStream]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-STARTSTREAM-1 | T6 | `StartStream` | 配信中でない、Intro、`StartWait` がこのバッチの `StartID` で `StageIntroConfirmed` | `Proceed` | —（待ちは `BatchDone` か `StreamStateChanged{true}` まで残る） |
| AP-STARTSTREAM-2 | T6 | `StartStream` | `StartWait` がない（手動の `SceneChanged` で消えた。役割は Intro でも Intro 以外でも） | `Reject` | — |
| AP-STARTSTREAM-3 | T6 | `StartStream` | 未接続（`OBSDisconnected` で待ちは消えている） | `Reject` | — |

**StopStream** `[StopStream]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-STOP-1 | T6 | `StopStream` | 配信中（役割、受信状態は何でも。作ったあとに何度変わっていても） | `Proceed` | — |
| AP-STOP-2 | T6 | `StopStream` | 配信中、作ったあとに手動の `SceneChanged` があった（`SceneEpoch` が違う） | `Proceed`（`SceneEpoch` は見ない） | — |
| AP-STOP-3 | T6 | `StopStream` | 作ったあとに配信が止まり、始め直された（今は配信中、`StreamEpoch` が違う） | `Reject` | —（`LastError` は `BatchDone{Rejected}` で記録。BD-R-2） |

## 3. Event の列のケース

### 共通の決まり

- 時刻 `t` は秒。Event の `At` と `Approve` の `at` に入れる。`Tick` は書いた時刻のものだけを流す（書いていない整数秒の `Tick` を足しても結果は変わらない）
- 既定: `TransitionMs=300`（全体の長さ 0.3秒）、`Override` は `None`、期限 = 応答の時刻 + 0.3 + 3 = 応答の時刻 + 3.3秒。上書きありか長さ不明なら10秒以上。再送は5秒
- **準備 P(シーン, 配信, 受信状態):** t=0 に `LinkChanged{受信状態}`（未接続なので何も出ない）、続けて `OBSConnected{Streaming: 配信, Scene: シーン, TransitionMs: 300}`。準備では、`OBSConnected` でバッチが出ない組み合わせだけを使う
- `#n` は `BatchID`、`r1` は `ReqID=1`。「本体」は本体の切替とみなしたこと（`SceneEpoch` は進まず、保持に入らない）、「手動」は手動とみなしたこと（`SceneEpoch+1`、期待を全部消す）
- 実行役の動き（`Approve`、`SceneRequestDone`、`BatchDone` の順）は、テストが実行役の代わりに呼ぶ。1つのバッチでは `SceneRequestDone` のあとに `BatchDone` を送る（9章）

### 必須ケースとの対応（spec 11章 switcher の箇条書き）

| spec 11章の項目 | ケース | T |
|---|---|---|
| 自動切替は、配信中かつ Live / Fail のときだけ | SQ-01 | T5 |
| `RestartMedia` が `SetScene(live)` より先 | SQ-02 | T5 |
| PauseOff の戻り先 | SQ-03 | T6 |
| OBS 側の手動の Pause でも直前の役割を覚える | SQ-04 | T5 |
| 再送が5秒以上あく | SQ-05a〜c | T5 |
| 各 Command の 409 になる条件 | SQ-06 | T6 |
| 手動保持（見分け方、入る、再送の取り消し、解除、`OBSConnected`） | SQ-07、SQ-08、SQ-09a〜e | T5（SQ-09a は T6） |
| 期待（登録、応答待ち、期限、タイムアウト、エラー、前の期待、一致しない変化） | SQ-10〜SQ-15 | T5（SQ-14、SQ-15 は T6） |
| 有効条件（復帰の待ち時間中、StopStream と PauseOn、GoLive の決め直し） | SQ-16a〜e、SQ-17、SQ-18 | T6 |
| 承認の `Skip` | SQ-19a、SQ-19b | T6 |
| 重複の歯止め | SQ-20 | T5 |
| 配信開始（失敗、却下、期限切れ、遅い Intro、応答より先の Intro） | SQ-21a〜g | T6 |
| 置き換え | SQ-18 | T6 |
| 上書きの状態 | SQ-22 | T5 |
| 配信の区切り | SQ-23 | T6 |
| 配信状態（`RECONNECTING` → `RECONNECTED`） | SQ-24 | T5 |
| 配信していない間の手動の Live では保持に入らない | SQ-25 | T5 |
| バッチ内の失敗 | SQ-26 | T6 |
| （必須ではない。U-12 の確認用） | SQ-27 | T6 |

### SQ-01 自動切替の範囲（T5）

それぞれ別の列。どの列も、書いた出力のほかにバッチは出ない。

| # | 準備 | 入力 | 期待する出力 |
|---|---|---|---|
| a | P(Live, 配信中, OK) | t=1 `LinkChanged{LOW}`、t=2 `LinkChanged{DOWN}`、t=3 `LinkChanged{OK}` | t=1 に `AutoFail#1[SetScene(fail)]`。t=2 は未完了があるので出ない。t=3 は Live かつ OK で出ない |
| b | P(Fail, 配信中, LOW) | t=1 `LinkChanged{DOWN}`、t=2 `LinkChanged{LOW}`、t=3 `LinkChanged{OK}` | t=3 に `AutoRecover#1[RestartMedia{W}, SetScene(live)]` だけ |
| c | P(Intro, 配信中, OK)、P(Pause, …)、P(Other のシーン, …) | t=1 `LinkChanged{LOW}`、t=2 `LinkChanged{OK}`、t=10 `Tick` | なし |
| d | P(Live, 配信中でない, OK) | t=1 `LinkChanged{LOW}`、t=10 `Tick` | なし |
| e | P(Live, 配信中, OK) | t=1 `OBSDisconnected`、t=2 `LinkChanged{LOW}`、t=10 `Tick`、t=11 `OBSConnected{true, "Live", 300}` | t=2、t=10 はなし。t=11 に `AutoFail#1`（OC-3） |

### SQ-02 Fail → Live の順序（T5）

準備 P(Fail, 配信中, LOW)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LinkChanged{OK}` | `AutoRecover#1`。Action は `[RestartMedia{Wait: W}, SetScene{Live}]` の順 | `Pending` に #1。期待はまだない |
| 1.0 | `Approve(1, RestartMedia, None)` | `Proceed` | 期待は登録されない |
| 1.0+W | `Approve(1, SetScene(live), None)` | `Proceed{r1}` | 期待 [Live r1] |
| 1.1+W | `SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}` | — | 期限 = 4.4+W。`Pending` が空 |
| 1.4+W | `SceneChanged{"Live"}` | —（本体。Live かつ OK） | 役割 Live、期待が空、`SceneEpoch` は同じ、保持なし |

### SQ-03 PauseOff の戻り先（T6）

作り方: 直前が Intro / Live / Fail の行は、P(その役割, 配信中, その役割で自動切替が出ない受信状態) から `PauseOn` → `Approve` → `SceneRequestDone{OK}` → `BatchDone{Completed}` → `SceneChanged{"Pause"}`（本体）で Pause にし、そのあと `LinkChanged` で受信状態を表の値にする（Pause なので何も出ない）。直前が不明の行は、P(Other のシーン, 配信中, OK) から手動の `SceneChanged{"Pause"}`、または P(Pause, 配信中, OK)（`OBSConnected` の時点で Pause）。最後に `PauseOff` を送る。

| # | `PausePrev` | 受信状態 | 期待する出力（`cmdErr` は nil） |
|---|---|---|---|
| a | Intro | OK / LOW / DOWN | `PauseOffIntro[SetScene(intro)]` |
| b | Live | OK | `PauseOffLive[RestartMedia{W}, SetScene(live)]` |
| c | Live | LOW / DOWN | `PauseOffFail[SetScene(fail)]` |
| d | Fail | OK | `PauseOffLive[RestartMedia{W}, SetScene(live)]` |
| e | Fail | LOW / DOWN | `PauseOffFail[SetScene(fail)]` |
| f | 不明（Other から、または接続時に Pause） | OK | `PauseOffLive[RestartMedia{W}, SetScene(live)]` |
| g | 不明（同上） | LOW / DOWN | `PauseOffFail[SetScene(fail)]` |

### SQ-04 OBS 側の手動の Pause（T5。最後の PauseOff は T6）

準備 P(Live, 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `SceneChanged{"Pause"}`（期待なし） | — | 手動。`PausePrev=RoleLive`、保持なし |
| 2.0 | `LinkChanged{LOW}` | — | — |
| 3.0 | `Command{PauseOff}` | `PauseOffFail#1[SetScene(fail)]` | — |
| 4.0 | `SceneChanged{"Live"}`（期待なし。#1 は未承認） | — | 手動。`PausePrev=RoleUnknown`、保持に入る（`HoldSince=4.0`） |

別の列: P(Other のシーン, 配信中, OK) で t=1.0 に `SceneChanged{"Pause"}` → `PausePrev=RoleUnknown`。

### SQ-05 再送の間隔（T5）

**a. `Error` の応答のあと。** 準備 P(Live, 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LinkChanged{LOW}` | `AutoFail#1` | `LastAutoSent[AutoFail]=1.0` |
| 1.0 | `Approve(1, SetScene(fail), None)` | `Proceed{r1}` | 期待 [Fail r1] |
| 1.1 | `SceneRequestDone{r1, Error}`、`BatchDone{1, Failed}` | — | 期待が空、`Pending` が空。`LastError` は U-05 |
| 2.0〜5.0 | `Tick` ×4 | —（経過 1〜4秒） | — |
| 6.0 | `Tick` | `AutoFail#2`（経過 5.0秒ちょうど） | `LastAutoSent[AutoFail]=6.0` |
| 7.0 | `Tick` | —（#2 が未完了） | — |

**b. `Timeout` の応答のあと。** 準備は同じ。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LinkChanged{LOW}`、`Approve(1, SetScene(fail), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Fail r1] |
| 6.0 | `SceneRequestDone{r1, Timeout}`、`BatchDone{1, Failed}` | — | 期待は残る。期限 = 9.3 |
| 7.0〜9.0 | `Tick` ×3 | —（5秒はたったが、Fail への期待が残る） | 期待は残る |
| 10.0 | `Tick` | `AutoFail#2` | 期待が期限切れで消える。`SceneEpoch` は同じ |

**c. 実行役が詰まっている間。** 準備は同じ。t=1.0 に `LinkChanged{LOW}` → `AutoFail#1`。`Approve` も `BatchDone` も呼ばずに t=2.0〜30.0 の `Tick` を流す → バッチは出ない（#1 が未完了）。

### SQ-06 各 Command の 409（T6）

1本の列。`ErrInvalidState` は 409、`ErrNotConnected` は 503 に当たる。

| t | 入力 | 期待する `cmdErr` / 出力 | 理由 |
|---|---|---|---|
| 0.0 | 5つの Command を順に（接続の前） | すべて `ErrNotConnected` | 未接続 |
| 0.5 | `LinkChanged{OK}`、`OBSConnected{false, "Live", 300}` | — | — |
| 1.0 | `StreamStop`、`GoLive`、`PauseOn`、`PauseOff` | すべて `ErrInvalidState` | 配信中でない |
| 2.0 | `StreamStart` | nil、`StartIntro#1` | CMD-START-5 |
| 2.1 | `StreamStart` | `ErrInvalidState` | 配信開始の待ちがある |
| 2.2〜2.6 | `Approve(1, SetScene(intro))` → `Proceed{r1}`、`SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}`、`SceneChanged{"Intro"}` → `StartStream#2`、`Approve(2, StartStream)` → `Proceed`、`BatchDone{2, Completed}`、`StreamStateChanged{true}` | — | 配信中、Intro |
| 3.0 | `StreamStart` | `ErrInvalidState` | 配信中 |
| 3.1 | `PauseOff` | `ErrInvalidState` | 役割が Pause でない |
| 3.2 | `GoLive` | nil、`GoLive#3` | CMD-GOLIVE-4 |
| 3.3〜3.9 | #3 を最後まで実行し、`SceneChanged{"Live"}`（本体） | — | Live |
| 4.0 | `GoLive` | `ErrInvalidState` | 役割が Intro でない |
| 4.1 | `PauseOn` | nil、`PauseOn#4` | CMD-PON-4 |
| 4.2〜4.6 | #4 を最後まで実行し、`SceneChanged{"Pause"}`（本体） | — | Pause |
| 5.0 | `PauseOn`、`GoLive` | どちらも `ErrInvalidState` | 役割が Pause |
| 5.1 | `SceneChanged{"Other のシーン"}`（手動） | — | Other |
| 5.2 | `PauseOn`、`PauseOff`、`GoLive` | すべて `ErrInvalidState` | 役割が Other |
| 5.3 | `StreamStop` | nil、`StopStream#5` | Other でも受け付ける |

### SQ-07 手動保持: 入る、再送の取り消し、解除（T5）

準備 P(Live, 配信中, OK)。spec の例（回線が悪いときに手動で Live にした）そのもの。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LinkChanged{LOW}`、`Approve(1, SetScene(fail), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Fail r1] |
| 1.1 | `SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}` | — | 期限 = 4.4 |
| 1.4 | `SceneChanged{"Fail"}` | — | 本体。保持なし、`SceneEpoch` は同じ |
| 3.0 | `SceneChanged{"Live"}`（期待なし） | —（Live かつ LOW でも出さない） | 手動。保持に入る（`HoldSince=3.0`） |
| 4.0〜12.0 | `Tick` ×9 | —（保持がなければ t=6.0 で再送の条件を満たす） | 保持は続く |
| 13.0 | `LinkChanged{DOWN}` | — | 保持は続く |
| 14.0 | `LinkChanged{LOW}` | — | 保持は続く |
| 15.0 | `LinkChanged{OK}` | —（Live かつ OK） | 保持を解除 |
| 20.0 | `LinkChanged{LOW}` | `AutoFail#2` | — |

### SQ-08 期限切れのあと、別のシーンへの変化は手動（T5）

準備 P(Live, 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LinkChanged{LOW}`、`Approve(1, SetScene(fail), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Fail r1] |
| 1.1 | `SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}` | — | 期限 = 4.4 |
| 2.0〜4.0 | `Tick` ×3 | — | 期待は残る |
| 5.0 | `Tick` | —（期待は消えたが、送出から 4.0秒） | 期待が空。`SceneEpoch` は同じ、保持なし |
| 5.5 | `SceneChanged{"Fail"}` | — | 手動（SCM-10）。保持に入る（`HoldSince=5.5`） |
| 6.0 | `Tick` | — | — |

別の列（別のシーン）: t=1.1 までは同じ。t=1.2 に `SceneChanged{"Intro"}` → 手動。期待が空になり、`SceneEpoch+1`、保持には入らない（Intro）。

### SQ-09 保持の解除: Command、配信の停止、OBSConnected（a は T6、ほかは T5）

共通の準備: P(Live, 配信中, OK)、t=1.0 に `SceneChanged{"Fail"}`（手動）→ 保持に入る（`HoldSince=1.0`）。Fail かつ OK だが AutoRecover は出ない。

| # | 続き | 期待する出力 | 期待する状態 |
|---|---|---|---|
| a | t=2.0 `Command{PauseOn}` | `PauseOn#1[SetScene(pause)]` | 保持を解除（U-08） |
| b | t=2.0 `StreamStateChanged{false}` | — | 保持を解除。期待が空、`StreamEpoch+1` |
| c | t=2.0 `OBSDisconnected`、t=4.0 `OBSConnected{true, "Fail", 300}` | どちらもなし（保持中は判定しない） | 保持は続く（`HoldSince=1.0`）。`ConnEpoch+1` |
| d | t=2.0 `OBSDisconnected`、t=4.0 `OBSConnected{true, "Live", 300}` | —（Live かつ OK） | 保持を解除 |
| e | t=2.0 `OBSDisconnected`、t=4.0 `OBSConnected{false, "Fail", 300}` | — | 保持を解除。`StreamEpoch+1` |
| f | a の代わりに t=2.0 `Command{GoLive}` | `ErrInvalidState` | 保持は続く（受け付けなかった Command では解除しない） |

### SQ-10 期待の登録と、長いトランジション（T5）

準備 P(Live, 配信中, OK) を `TransitionMs: 5000` で行う。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LinkChanged{LOW}` | `AutoFail#1` | 期待は空（決めた時点では登録しない） |
| 2.0 | `Approve(1, SetScene(fail), None)` | `Proceed{r1}` | 期待 [Fail r1]、`RegisteredAt=2.0` |
| 2.1 | `SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}` | — | 期限 = 2.1 + 5 + 3 = 10.1 |
| 3.0〜7.0 | `Tick` ×5 | — | 期待は残る（3秒固定なら t=5.1 で切れていた） |
| 7.1 | `SceneChanged{"Fail"}`（5秒のフェードの終わり） | — | 本体。保持なし、`SceneEpoch` は同じ |

### SQ-11 応答を待つ間は期限切れにしない（T5）

準備 P(Live, 配信中, OK)。t=1.0 `LinkChanged{LOW}` → `AutoFail#1`、`Approve(1, SetScene(fail), None)` → `Proceed{r1}`。`SceneRequestDone` を送らずに t=2.0〜30.0 の `Tick` を流す → 期待 [Fail r1] は残り、バッチは出ない。t=30.5 に `SceneChanged{"Fail"}` → 本体（保持に入らない）。

### SQ-12 タイムアウトのあとに届いた本体の SceneChanged（T5）

準備 P(Live, 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LinkChanged{LOW}`、`Approve(1, SetScene(fail), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Fail r1] |
| 6.0 | `SceneRequestDone{r1, Timeout}`、`BatchDone{1, Failed}` | — | 期待は残る。期限 = 9.3 |
| 7.5 | `SceneChanged{"Fail"}` | — | 本体。保持なし、`SceneEpoch` は同じ、期待が空 |
| 8.0〜12.0 | `Tick` ×5 | —（Fail かつ LOW） | — |

### SQ-13 エラーの応答ではすぐ消える（T5）

準備 P(Live, 配信中, OK)。t=1.0 `LinkChanged{LOW}`、`Approve` → `Proceed{r1}`。t=1.1 `SceneRequestDone{r1, Error}` → 期待が空。t=1.5 `SceneChanged{"Fail"}` → 手動（保持に入る、`SceneEpoch+1`）。

### SQ-14 打ち切られたトランジション: Fail の直後に Pause、Pause のイベントだけ（T6）

準備 P(Live, 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.00 | `LinkChanged{LOW}`、`Approve(1, SetScene(fail), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Fail r1] |
| 1.05 | `SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}` | — | r1 の期限 = 4.35 |
| 1.10 | `Command{PauseOn}`（役割はまだ Live） | nil、`PauseOn#2` | — |
| 1.10 | `Approve(2, SetScene(pause), None)` | `Proceed{r2}` | 期待 [Fail r1, Pause r2] |
| 1.15 | `SceneRequestDone{r2, OK}`、`BatchDone{2, Completed}` | — | r2 の期限 = 4.45 |
| 1.45 | `SceneChanged{"Pause"}`（`SceneChanged{"Fail"}` は届かない） | — | 本体。期待が空（r1 も消える）。`PausePrev=RoleLive`、`SceneEpoch` は同じ、保持なし |
| 2.0〜10.0 | `Tick` | — | — |

### SQ-15 一致しない SceneChanged で期待が全部消える（T6）

SQ-14 の t=1.15 までと同じ（期待 [Fail r1, Pause r2]）。t=1.30 に `SceneChanged{"Intro"}` → 手動。期待が空、`SceneEpoch+1`、保持なし。t=1.45 に `SceneChanged{"Pause"}` → これも手動（期待がない）。`SceneEpoch+1`、`PausePrev=RoleIntro`。

### SQ-16 有効条件: 復帰の待ち時間中の変化（T6）

準備 P(Fail, 配信中, LOW)。t=1.0 `LinkChanged{OK}` → `AutoRecover#1[RestartMedia{W}, SetScene(live)]`。t=1.0 `Approve(1, RestartMedia, None)` → `Proceed`。待ち時間の途中（t=1.0+W/2）に下の Event を1つ流し、t=1.0+W に `Approve(1, SetScene(live), None)` を呼ぶ。

| # | 待ち時間中の Event | `Approve` の結果 | そのあと |
|---|---|---|---|
| a | `LinkChanged{LOW}` | `Reject`（AP-AUTOREC-3） | `BatchDone{1, Rejected}` → `LastError` は記録しない。期待は空 |
| b | `SceneChanged{"Pause"}`（手動。`PausePrev=RoleFail`） | `Reject`（AP-COM-5） | 同上 |
| c | `StreamStateChanged{false}` | `Reject`（AP-COM-3） | 同上 |
| d | `OBSDisconnected` | `Reject`（AP-COM-1） | 同上 |
| e | なし（対照） | `Proceed{r1}` | 期待 [Live r1] |

### SQ-17 StopStream と PauseOn は受信状態の変化で却下されない（T6）

準備 P(Intro, 配信中, OK)（Intro なので自動切替は出ない）。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StreamStop}` | `StopStream#1` | — |
| 1.1 | `Command{PauseOn}` | `PauseOn#2` | — |
| 1.2〜1.6 | `LinkChanged{LOW}`、`{DOWN}`、`{LOW}`、`{DOWN}`、`{LOW}` | — | — |
| 2.0 | `Approve(1, StopStream, None)` | `Proceed` | — |
| 2.1 | `BatchDone{1, Completed}` | — | StopStream#1 が未完了の一覧から消える（STOPPED はまだ届いていないので配信中のまま） |
| 2.2 | `Approve(2, SetScene(pause), None)` | `Proceed{r1}` | 期待 [Pause r1] |

実行役は `BatchDone` を送るまで次のバッチを受け取らない（9章）ので、承認は #1、#2 の順に来る。

### SQ-18 GoLive の決め直し（Replace）（T6）

準備 P(Intro, 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{GoLive}` | nil、`GoLive#1[RestartMedia{W}, SetScene(live)]` | — |
| 1.0 | `Approve(1, RestartMedia, None)` | `Proceed` | — |
| 1.0+W/2 | `LinkChanged{LOW}` | —（Intro） | — |
| 1.0+W | `Approve(1, SetScene(live), None)` | `Replace{GoFail#2[SetScene(fail)]}` | #1 に `Replaced=true`、`Pending` に #1 と #2。期待は空 |
| 1.0+W | `BatchDone{1, Rejected}` | — | `LastError` は nil のまま。`Pending` は #2 だけ |
| 1.1+W | `LinkChanged{OK}` | —（Intro） | — |
| 1.2+W | `Approve(2, SetScene(fail), None)` | `Proceed{r1}`（OK に戻っていても置き換えない） | 期待 [Fail r1] |
| 1.3+W | `SceneRequestDone{r1, OK}`、`BatchDone{2, Completed}` | — | `LastError` は nil |
| 1.6+W | `SceneChanged{"Fail"}` | `AutoRecover#3[RestartMedia{W}, SetScene(live)]`（本体の切替のあとの判定。Fail かつ OK。SCE-14） | 本体 |

別の列: t=1.0 `Command{GoLive}` → `GoLive#1`、t=1.05 `LinkChanged{LOW}`（`RestartMedia` の承認の前）、t=1.1 `Approve(1, RestartMedia, None)` → `Replace{GoFail#2}`（AP-GOLIVE-3）。

### SQ-19 承認の Skip（T6）

**a. StartIntro。** 準備 P(Live, 配信中でない, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StreamStart}`、`Approve(1, SetScene(intro), None)` | `StartIntro#1`、`Proceed{r1}` | 待ち {1, `StageWaitIntro`}、期待 [Intro r1] |
| 6.0 | `SceneRequestDone{r1, Timeout}`、`BatchDone{1, Failed}` | — | 期待は残る（期限 9.3）。待ちが消える。`LastError` を記録 |
| 6.5 | `Command{StreamStart}` | nil、`StartIntro#2` | 待ち {2, `StageWaitIntro`} |
| 7.0 | `SceneChanged{"Intro"}`（r1 に一致） | U-03（提案: `StartStream#3`） | 本体。役割 Intro。待ちの段階は U-03 |
| 7.1 | `Approve(2, SetScene(intro), None)` | `Skip` | 期待は登録されない |
| 7.1 | `BatchDone{2, Completed}` | U-02 | U-02 |

**b. PauseOn。** 準備 P(Live, 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{PauseOn}`、`Approve(1, SetScene(pause), None)` | `PauseOn#1`、`Proceed{r1}` | 期待 [Pause r1] |
| 6.0 | `SceneRequestDone{r1, Timeout}`、`BatchDone{1, Failed}` | — | 期待は残る（期限 9.3）。`LastError` を記録 |
| 6.5 | `Command{PauseOn}`（役割はまだ Live） | nil、`PauseOn#2` | — |
| 7.0 | `SceneChanged{"Pause"}`（r1 に一致） | — | 本体。役割 Pause、`PausePrev=RoleLive` |
| 7.1 | `Approve(2, SetScene(pause), None)` | U-01（提案: `Skip`） | 期待は登録されない |

### SQ-20 重複の歯止め（T5）

準備 P(Live, 配信中, OK)。全体で AutoFail のバッチは1本だけ。

| t | 入力 | 期待する出力 | 理由 |
|---|---|---|---|
| 1.0 | `LinkChanged{LOW}` | `AutoFail#1` | — |
| 2.0 | `LinkChanged{DOWN}` | — | #1 が未完了 |
| 2.5 | `Approve(1, SetScene(fail), None)` | `Proceed{r1}` | DOWN でも条件を満たす |
| 2.6 | `SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}` | — | 期限 = 5.9 |
| 2.7 | `LinkChanged{LOW}` | — | 未完了はないが、Fail への期待が残る |
| 2.9 | `SceneChanged{"Fail"}` | — | 本体。Fail かつ LOW |

### SQ-21 配信開始（T6）

準備は、特記がなければ P(Live, 配信中でない, OK)。

**a. 通常（2段階）。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StreamStart}` | nil、`StartIntro#1[SetScene(intro)]` | 待ち {1, `StageWaitIntro`} |
| 1.0 | `Approve(1, SetScene(intro), None)` | `Proceed{r1}` | 期待 [Intro r1] |
| 1.1 | `SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}` | — | 待ちは残る（BD-C-3） |
| 1.2 | `Command{StreamStart}` | `ErrInvalidState` | — |
| 1.4 | `SceneChanged{"Intro"}` | `StartStream#2[StartStream]` | 本体。待ち {1, `StageIntroConfirmed`} |
| 1.4 | `Approve(2, StartStream, None)` | `Proceed` | — |
| 1.5 | `BatchDone{2, Completed}` | — | 待ちが消える。`LastError` は nil |
| 1.6 | `StreamStateChanged{true}` | —（Intro） | 配信中、`StreamEpoch+1` |

**b. すでに Intro。** 準備 P(Intro, 配信中でない, OK)。t=1.0 `Command{StreamStart}` → `StartStream#1[StartStream]`、待ち {1, `StageIntroConfirmed`}（StartIntro は出ない）。`Approve(1, StartStream, None)` → `Proceed`。

**c. `SceneChanged(Intro)` が応答より先に届き、応答がタイムアウトする。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StreamStart}`、`Approve(1, SetScene(intro), None)` | `StartIntro#1`、`Proceed{r1}` | 待ち {1, `StageWaitIntro`} |
| 1.3 | `SceneChanged{"Intro"}` | `StartStream#2`（実行役は #1 の応答待ちなので、列で待つ） | 本体。期待が空、待ち {1, `StageIntroConfirmed`} |
| 6.0 | `SceneRequestDone{r1, Timeout}` | — | 何も変わらない（SRD-TO-2） |
| 6.0 | `BatchDone{1, Failed}` | — | 待ちは残る。`LastError` は U-17 |
| 6.0 | `Approve(2, StartStream, None)` | `Proceed`（配信は始まる） | — |
| 6.1 | `BatchDone{2, Completed}`、`StreamStateChanged{true}` | — | 待ちが消える。配信中 |

**d. Intro への切替が失敗（`Error`）。** t=1.0 `StreamStart`、`Approve` → `Proceed{r1}`。t=1.1 `SceneRequestDone{r1, Error}` → 期待が空、待ちが消える、`LastError` を記録。`BatchDone{1, Failed}`。`StartStream` のバッチは出ない。t=2.0 `Command{StreamStart}` → nil、`StartIntro#2`（次の StreamStart を受け付ける）。

**e. 却下。** t=1.0 `StreamStart` → `StartIntro#1`。t=1.2 `SceneChanged{"Fail"}`（手動。配信していないので保持に入らない）→ 待ちが消える、`LastError` を記録、`SceneEpoch+1`。t=1.3 `Approve(1, SetScene(intro), None)` → `Reject`。`BatchDone{1, Rejected}` → `LastError` を記録。`StartStream` は出ない。t=2.0 `Command{StreamStart}` → nil、`StartIntro#2`。

**f. 期限切れと、そのあとの遅い Intro。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StreamStart}`、`Approve(1, SetScene(intro), None)` | `StartIntro#1`、`Proceed{r1}` | 待ち {1, `StageWaitIntro`} |
| 1.1 | `SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}` | — | 期限 = 4.4。待ちは残る |
| 2.0〜4.0 | `Tick` ×3 | — | 待ちは残る |
| 5.0 | `Tick` | — | 期待が消える。待ちが消える。`LastError` を記録（TK-10） |
| 5.5 | `SceneChanged{"Intro"}` | —（`StartStream` を出さない） | 手動（期待がない）。保持なし |
| 6.0 | `Command{StreamStart}` | nil、`StartStream#2`（すでに Intro） | 待ち {2, `StageIntroConfirmed`} |

**g. タイムアウトで待ちを抜けたあとの遅い Intro。** t=1.0 `StreamStart`、`Approve` → `Proceed{r1}`。t=6.0 `SceneRequestDone{r1, Timeout}`（期限 9.3）、`BatchDone{1, Failed}` → 待ちが消える、`LastError` を記録（BD-F-3）。t=7.0 `SceneChanged{"Intro"}` → 本体（r1 に一致）だが、待ちがないので `StartStream` は出ない（SCE-10）。

### SQ-22 上書きの状態と期限（T5）

準備 P(Live, 配信中, OK) を表の `TransitionMs` で行う。t=1.0 `LinkChanged{LOW}` → `AutoFail#1`、`Approve(1, SetScene(fail), 表の Override)` → `Proceed{r1}`、t=1.1 `SceneRequestDone{r1, OK}`。`BatchDone` は送らない（#1 を未完了のままにして、下の `Tick` で再送が出ないようにする）。期限の前後で `SceneChanged{"Fail"}` を流す2本の列で確かめる: 「期限 − 0.1」に流すと本体。「期限 + 0.05」に `Tick` を1回流し（出力なし）、「期限 + 0.1」に流すと手動（期限ちょうどの扱いと、`Tick` なしで消えるかは U-11）。

| # | `TransitionMs` | `Override` | 期限 | 式 |
|---|---|---|---|---|
| a | 300 | `None` | 4.4 | 1.1 + 0.3 + 3 |
| b | 5000 | `None` | 9.1 | 1.1 + 5 + 3 |
| c | nil | `None` | 11.1 | 1.1 + 10 |
| d | 300 | `Known{500}` | 11.1 | 1.1 + max(0.5 + 3, 10) |
| e | 300 | `Known{8000}` | 12.1 | 1.1 + max(8 + 3, 10) |
| f | nil | `Known{8000}` | 12.1 | 同上（全体の長さは使わない） |
| g | nil | `Unknown` | 11.1 | 1.1 + 10 |
| h | 300 | `Unknown` | U-09（提案: 11.1） | 提案: 1.1 + max(0.3 + 3, 10) |
| i | 8000 | `Unknown` | U-09（提案: 12.1） | 提案: 1.1 + max(8 + 3, 10) |

### SQ-23 配信の区切り: StopStream が配信の始め直しをまたぐ（T6）

準備 P(Intro, 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StreamStop}` | nil、`StopStream#1` | #1 の `StreamEpoch` = e |
| 1.5 | `StreamStateChanged{false}` | — | `StreamEpoch` = e+1 |
| 2.0 | `StreamStateChanged{true}` | —（Intro） | `StreamEpoch` = e+2、配信中 |
| 2.5 | `Approve(1, StopStream, None)` | `Reject`（AP-STOP-3） | 配信中のまま |
| 2.5 | `BatchDone{1, Rejected}` | — | `LastError` を記録 |

対照: t=1.5 と t=2.0 を流さなければ `Proceed`。

### SQ-24 RECONNECTING → RECONNECTED（T5）

7章の写像により、`RECONNECTING` と `RECONNECTED` では switcher に Event が届かない（確かめ方は U-19）。switcher のテストは、その間に届きうる Event（`Tick` だけ）で状態が変わらないことを確かめる。

| # | 準備 | 入力 | 期待する状態 |
|---|---|---|---|
| a | P(Live, 配信中, OK)、t=1.0 `SceneChanged{"Fail"}`（手動、保持に入る） | t=2.0〜12.0 `Tick` ×11（この間、OBS の出力は `RECONNECTING` → `RECONNECTED`） | 保持は続く（`HoldSince=1.0`）。`Streaming=true`、`StreamEpoch` は同じ。バッチは出ない（Fail かつ OK でも） |
| b | P(Live, 配信中, OK)、t=0.5 `LinkChanged{LOW}` → `AutoFail#1`（未承認のまま）、t=1.0 `SceneChanged{"Pause"}`（手動。`PausePrev=RoleLive`） | t=2.0〜12.0 `Tick` ×11 | `PausePrev=RoleLive`、`Pending` に #1、`Streaming=true`、`StreamEpoch` は同じ。バッチは出ない |

### SQ-25 配信していない間の手動の Live（T5）

準備 P(Intro, 配信中でない, LOW)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `SceneChanged{"Live"}`（期待なし） | — | 手動。`SceneEpoch+1`。保持に入らない |
| 2.0 | `StreamStateChanged{true}` | `AutoFail#1`（保持していないので、Live かつ LOW で切り替える） | 配信中、`StreamEpoch+1` |

### SQ-26 バッチ内の失敗（T6）

失敗したあと続けるか捨てるかを決めるのは実行役（core、T9）で、Machine には失敗そのものを知らせる入口がない（U-18）。Machine の側で確かめるのは次の2つ。

| # | 列 | 期待 |
|---|---|---|
| a | 準備 P(Fail, 配信中, LOW)、t=1.0 `LinkChanged{OK}` → `AutoRecover#1`。`Approve(1, RestartMedia, None)` → `Proceed`。（実行役の側で RESTART が失敗したとする。）t=1.0+W `Approve(1, SetScene(live), None)` | `Proceed{r1}`、期待 [Live r1]（`RestartMedia` の失敗は承認に影響しない）。続く `BatchDone` の `Result` と `LastError` は U-18 |
| b | 準備 P(Intro, 配信中, OK)、t=1.0 `Command{GoLive}` → `GoLive#1`。`Approve(1, RestartMedia, None)` → `Proceed`、`Approve(1, SetScene(live), None)` → `Proceed{r1}`、t=1.1+W `SceneRequestDone{r1, Error}`、`BatchDone{1, Failed}` | 期待が空、`Pending` が空、`LastError` を記録（BD-F-2）。そのあと #1 への `Approve` は来ない |

### SQ-27 PauseOn の直後の回線の悪化（T6。必須ではない。U-12 の確認用）

準備 P(Live, 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{PauseOn}`、`Approve(1, SetScene(pause), None)` | `PauseOn#1`、`Proceed{r1}` | 期待 [Pause r1] |
| 1.1 | `SceneRequestDone{r1, OK}`、`BatchDone{1, Completed}` | — | — |
| 1.2 | `LinkChanged{LOW}`（`SceneChanged{"Pause"}` はまだ届かない。役割は Live） | spec の文面どおりなら `AutoFail#2`。U-12（提案: 出さない） | — |
| 1.2 | （出た場合）`Approve(2, SetScene(fail), None)` | spec の文面どおりなら `Proceed{r2}`（Live、LOW、`SceneEpoch` は同じ）。U-12 | Pause の直後に Fail を送ることになる |

## 4. spec の不明点

spec（v1.4）が期待値を1つに決めていない箇所と、2つの規則がぶつかる箇所。表と Event の列では、ここの番号を引くだけにして、どれかを選んだ値を黙って書いていない。**提案** は裁定の案で、決まったら spec を直して変更履歴に足し、この文書の該当行から `U-xx` を外す。

### 結果が大きく変わるもの

**U-01 `Skip` と有効条件のどちらを先に確かめるか**
- spec: 「`SetScene` の承認で、現在の役割がすでに目的の役割なら `Skip`（送らず、期待も登録しない）」（6章）と、「目的と有効条件」の表の各行の役割の条件（AutoFail は「役割が Live」、PauseOn は「役割が Intro / Live / Fail」など）
- 不明な点: 役割がすでに目的の役割のとき、StartIntro を除くどの Purpose でも、役割の条件が同時に崩れている。`Skip`（`BatchDone{Completed}`）か却下（`BatchDone{Rejected}`、Command 由来なら `last_error`）かが決まらない。実際に起きる列は SQ-19b（PauseOn の応答がタイムアウトし、利用者がもう一度 PauseOn を押したあとに、最初の切替の `SceneChanged` が届く）
- 決定（案どおり）: 順序を「共通の条件（接続、3つの Epoch）→ `SetScene` で役割がすでに目的の役割なら `Skip` → Purpose の有効条件」とする。目的は達しているので `last_error` に残さない。`RestartMedia` には `Skip` がないので、役割がすでに Live なら有効条件で却下する（AP-AUTOREC-6）
- 引いている行: AP-AUTOFAIL-5、AP-AUTOREC-5、AP-GOLIVE-7、AP-POFFLIVE-5、AP-GOFAIL-4、AP-POFFFAIL-4、AP-POFFINTRO-3、AP-PON-3、SQ-19b

**U-02 StartIntro が `Skip` されたときの配信開始の待ち**
- spec: 「Intro への期待どおりの `SceneChanged` が来た時点で「Intro 確認済み」に進み、StartStream のバッチを出す」（6章）
- 不明な点: StartIntro の `SetScene` が `Skip` されると期待が登録されないので、期待どおりの `SceneChanged` は来ない。待ちは「Intro の確認待ち」のまま残り、消す条件（StartIntro の却下か失敗、期待の期限切れか `Error`）にも当たらない。以後の StreamStart は「待ちがある」で 409 になり続ける
- 決定（案どおり）: StartIntro の `BatchDone{Completed}` を受けた時点で、待ちが「Intro の確認待ち」で役割が Intro なら、「Intro 確認済み」に進めて StartStream のバッチを返す。通常の列（`BatchDone` が `SceneChanged` より先）では役割がまだ Intro でないので、何も変わらない
- 引いている行: BD-C-4、AP-STARTINTRO-4、SQ-19a

**U-03 配信開始の待ちの通し番号と、期待・バッチの対応**
- spec: 「「配信開始の待ち」（StreamStart ごとの通し番号を持つ）」「この StreamStart の「配信開始の待ち」が残っている」「却下（待ちも消す）」「StartStream のバッチの `BatchDone`（結果を問わない）」（6章）
- 不明な点: (1) 前の StreamStart が登録した Intro への期待（タイムアウト後も期限まで残る）に一致する `SceneChanged` が、次の StreamStart の待ちを「Intro 確認済み」に進めるか。(2) 前の StreamStart のバッチの却下や `BatchDone` が、次の StreamStart の待ちを消すか
- 決定（案どおり）: (1) 進める。待ちが確かめたいのは「OBS が Intro になった」ことで、どの要求によるかは問わない（進めないと U-02 と同じ行き止まりになる）。(2) 消さない。バッチは自分の `StartID` と同じ `ID` の待ちにだけ作用する
- 引いている行: SCE-11、BD-X-2、AP-STARTINTRO-3、AP-STARTINTRO-4、SQ-19a

**U-04 同じ Purpose の未完了のバッチがある間の Command**
- spec: 「同じ Purpose の未完了のバッチがある間は、同じ Purpose の新しいバッチを出さない（イベント起点の判定にも再送にも掛かる）」（6章）と、「手動操作」の表の受け付ける条件
- 不明な点: 受け付ける条件を満たす Command が、未完了のバッチのために出せないとき、`cmdErr` が何になるかが書かれていない（nil でバッチなしなら、API は 200 を返すのに何も起きない）。また GoLive と GoFail、PauseOff の3つは Purpose が別なので、文面どおりなら GoFail が未完了の間に GoLive のバッチを重ねて出せる（先のバッチが通ると、あとのバッチは役割の条件で却下され `last_error` に残る）。StreamStart では、前の待ちが消えたあとも前の StartIntro が未完了のことがあり、待ちだけ作ってバッチを出せない状態になりうる
- 決定（案どおり）: `ErrInvalidState`（理由: 同じ操作が実行待ち）を返し、状態を変えない。未完了の判定は Command の単位で行う（GoLive は GoLive / GoFail、PauseOff は PauseOffLive / PauseOffFail / PauseOffIntro、StreamStart は StartIntro / StartStream のどれかが未完了なら受け付けない）
- 引いている行: CMD-START-6、CMD-STOP-5、CMD-GOLIVE-6、CMD-PON-6、CMD-POFF-7

**U-05 Auto のバッチの失敗を `last_error` に記録するか。`last_error` をいつ消すか**
- spec: 「`BatchDone{Rejected}` または `{Failed}` が Command 由来のバッチ（Auto 以外）なら、`last_error` に記録する」（6章）と、「OBS の操作が失敗した | ログと `last_error` に記録。条件が続く間、最短5秒あけて再送」（10章）
- 不明な点: Auto のバッチの `Failed` は、6章では記録の対象外、10章では記録する。また `last_error` を消す条件はどこにも書かれていない
- 決定（案どおり）: `Failed` は Auto でも記録する（OBS の操作の失敗は利用者に見せる）。`Rejected` は Command 由来だけ記録する（Auto の却下は状況が変わっただけで、エラーではない）。第1段階では Machine は `last_error` を消さず、新しい記録で上書きするだけとする
- 引いている行: BD-F-1、SQ-05a

**U-06 GoFail / PauseOffFail の役割の条件**
- spec: 表の「GoFail / PauseOffFail | `[SetScene(fail)]` | 配信中、役割が Intro または Pause」（6章）。上の行は「役割が Intro（GoLive）または Pause（PauseOffLive）」と対応を書いている
- 不明な点: GoFail は役割が Pause でも有効か（PauseOffFail は Intro でも有効か）。GoLive の実行待ちの間に PauseOn が先に通って Pause になった場合などに結果が分かれる
- 決定（案どおり）: 上の行と同じに読む。GoFail は Intro だけ、PauseOffFail は Pause だけ。それ以外は却下
- 引いている行: AP-GOFAIL-2、AP-POFFFAIL-2

**U-07 `Tick` で判定する範囲と、5秒の起点**
- spec: 「条件が続く場合は、`Tick` で再送する」「`Tick` の再送なら、同じ Purpose の前回の送出から5秒以上たった」（6章「再送の歯止め」）、「`LinkChanged` で変わったときだけでなく、`SceneChanged`（本体の切替によるもの）、`StreamStateChanged`、`OBSConnected` のあとにも同じ判定をする」（6章「自動切替」）
- 不明な点: (1) `Tick` は「前に出したバッチの再送」だけか、その時点の状態での判定そのものか。判定を起こす Event の一覧にない変化（Command による保持の解除、`BatchDone`、期待の期限切れ）のあと、次の Event が来るまで誰も判定しない。(2) 「前回の送出」は `Handle` がバッチを返した時刻か、`Approve` が `Proceed` を返した時刻か。(3) その Purpose を一度も出していないときの扱い
- 決定（案どおり）: (1) `Tick` はイベント起点と同じ判定（J）を毎回行い、5秒の条件だけを足す。(2) 起点は `Handle` がバッチを返した Event の `At`。(3) 記録がなければ5秒の条件は満たしたものとする
- 注（不明点ではない）: 文面どおり、イベント起点の判定には5秒の条件が掛からない。`Error` で失敗した直後に LOW ↔ DOWN の `LinkChanged` が来ると、5秒を待たずに出し直す（LC-5）。これを避けたいなら spec の変更になる
- 引いている行: J の 5、B、LC-5、TK-11

**U-12 期待が残っている間の自動判定に使う役割**
- spec: 「自動切替は、OBS に接続していて、配信中で、現在の役割が Live か Fail のときだけ行う」「現在のシーンは OBS のイベントで追う」（6章）、AutoFail の有効条件「役割が Live」
- 不明な点: `SetScene(pause)` を送ってから `SceneChanged(Pause)` が届くまで（トランジションの長さの分）、役割は Live のままである。この間に受信状態が悪くなると、文面どおりなら AutoFail が出て承認も通り、Pause の直後に Fail を送る（SQ-27）。利用者が Pause を押したのに Fail になる。PauseOffIntro や GoLive の直後でも同じ形になる
- 決定（案どおり）: `Expectations` に Live / Fail 以外の役割（Pause、Intro）への期待が残っている間は、J を満たさないものとし、Auto の承認も却下する。期待が消えた（一致した、期限切れ）あとの判定は通常どおり
- 引いている行: LC-20、SCE-6、SQ-27

### 境界と時刻

**U-09 `Override` が `Unknown` のときの「長さ」**
- spec: 「次のときは「長さ + 3秒」と「10秒」の長いほう（長さがなければ10秒）: … `Override` が `Unknown` …」（6章）
- 不明な点: `Unknown`（上書きあり・長さ不明）で、全体の長さが取れているとき、「長さ」が全体の長さを指すのか、長さなし（常に10秒）なのか
- 決定（案どおり）: 全体の長さがあれば `max(全体の長さ + 3秒, 10秒)`、なければ10秒。長いほうに倒す（早く切れると、本体の切替を手動とみなして保持に入るため）
- 引いている行: DL-4、SQ-22h、SQ-22i

**U-10 期限に使う全体の長さを読む時点**
- spec: 「期限は `SceneRequestDone` を受けた時点から数え始め、「その時点 + 実際に使われるトランジションの長さ + 3秒」」「`Override` が `None` なら全体の長さ」（6章）
- 不明な点: `Approve` と `SceneRequestDone` の間に `TransitionChanged` が届いたとき、どちらの時点の全体の長さを使うか
- 決定（案どおり）: `Approve` の時点の値を期待に写す（`Override` も同じ時点で読んでいる。OBS が使うのは要求を受けた時点のトランジション）
- 引いている行: `Expectation.TransitionMs`、TC-3、2.8 の式

**U-11 期限の境界と、期限切れを確かめる時点**
- spec: 「期限を過ぎた期待は消す」（6章）、「`Tick{}` // 1秒ごと。再送と期待の期限の判定に使う」、「期限切れや別のシーンへの変化は手動とみなす」（11章）
- 不明な点: (1) `At` が期限とちょうど同じとき、切れているか。(2) 期限切れを消すのは `Tick` だけか、すべての Event と `Approve` の入口か（`Tick` だけだと、期限と次の `Tick` の間に届いた `SceneChanged` の扱いが `Tick` の位相で変わる）。(3) 「期限切れ…は手動とみなす」は、期限切れそのものを手動の切替として扱う（`SceneEpoch` を進める）意味か
- 決定（案どおり）: (1) `At > Deadline` で切れる（ちょうどは残る）。(2) すべての `Handle` と `Approve` の入口で、その時刻を使って消す。(3) 期限切れそのものでは `SceneEpoch` を進めず、保持にも入らない。手動とみなすのは、そのあとに届いた `SceneChanged`
- 引いている行: 2.10 の前書き、TK-8、SQ-22

**U-13 未接続の間の `LinkChanged` による保持の解除**
- spec: 「受信状態が OK と不良（LOW / DOWN）の間で変わったら解除する」と、「`OBSConnected` で…どちらも同じなら保持を続ける（websocket の瞬断だけで人の選択を打ち消さないため）」（6章）
- 不明な点: 切断中に受信状態が OK と不良の間で変わったとき、保持を解くか
- 決定（案どおり）: 解く（文面どおり。保持の解除は受信状態の変化だけで決まり、接続の有無を見ない）。再接続後の `OBSConnected` で通常どおり判定する
- 引いている行: LC-18

**U-14 今と同じ値の Event（重複）**
- spec: 「`StreamEpoch` は配信状態が変わるたび（`StreamStateChanged` と、…）に1つ進む」（6章）
- 不明な点: すでに配信中のときの `StreamStateChanged{true}`、配信中でないときの `{false}`、今と同じ `LinkChanged` が届いたとき、Epoch を進めたり、期待や保持を消したりするか
- 決定（案どおり）: 値が変わらない Event は何もしない（Epoch も進めない）。これで「配信中」の条件は `StreamEpoch` の一致だけで確かめられる
- 引いている行: LC-19、SST-6、SSF-5

**U-15 初期値と、最初の `OBSConnected`**
- spec: 「`OBSConnected` で配信状態が切断前と違っていたとき」「シーンか配信状態が切断の直前と違っていたとき」（6章）。Machine の初期の受信状態は書かれていない
- 不明な点: 起動後の最初の `OBSConnected` には「切断前」がない。Machine の `Link` の初期値もない
- 決定（案どおり）: 初期値は `Link=LinkDown`（Judge の起動直後と同じ）、`PrevConnStreaming=false`、`PrevConnScene=""`、3つの Epoch は 0。最初の `OBSConnected{Streaming: true}` では `StreamEpoch` が1つ進む（未完了のバッチがないので害はない）
- 引いている行: `PrevConnStreaming`、`Link`、OC-2

**U-16 配信の停止で消す「再送待ち」の実体**
- spec: 「`StreamStateChanged{Streaming: false}`（配信の停止）を受けたとき。あわせて期待と再送待ちも消す」（6章）
- 不明な点: 再送は `Tick` のたびに条件から決まるので、消すべき状態が何かが決まらない
- 決定（案どおり）: `LastAutoSent` を空にする（次の配信で、前の配信の送出時刻が5秒の条件に効かないようにする）
- 引いている行: SSF-1

### `last_error` とテストの置き場所

**U-17 配信開始の待ちを消したときの「成功」の範囲**
- spec: 「成功以外で消したときは `last_error` に記録し、配信は始めない」「「Intro 確認済み」のあとに StartIntro の応答がタイムアウトしても、待ちは消さない」「`BatchDone{Rejected}` または `{Failed}` が Command 由来のバッチ…なら、`last_error` に記録する」（6章）
- 不明な点: (1) `StreamStateChanged{true}` で待ちを消すのは成功か。「Intro の確認待ち」の間（OBS の画面で配信を始めた）でも、「Intro 確認済み」の間（StartStream の `BatchDone` より先に届いた、または OBS の画面で始めた）でも起きる。(2) 「Intro 確認済み」のあとの StartIntro の `BatchDone{Failed}` は、配信が始まるのに `last_error` に残るか
- 決定（案どおり）: (1) どちらの段階でも、待ちを消すこと自体は記録しない（配信は始まっている）。残った StartIntro / StartStream は承認で却下され、その `BatchDone{Rejected}` は通常どおり記録する。(2) 記録しない（その待ちが「Intro 確認済み」に進んでいれば、StartIntro の目的は達している）
- 引いている行: SST-4、SST-5、BD-F-4、SQ-21c

**U-18 バッチ内の失敗を Machine でどう確かめるか**
- spec: 11章は switcher の必須ケースに「バッチ内の失敗: `RestartMedia` の失敗では続け、それ以外の失敗では残りを捨てる」を挙げるが、6章「バッチ内の失敗」と 7章は「実行役が行う」としている。`BatchDone{BatchID, Result: Completed | Failed | Rejected, Err}`
- 不明な点: Machine には Action の失敗を受ける入口が `SceneRequestDone` と `BatchDone` しかなく、続けるか捨てるかを Machine は決めない。また `RestartMedia` が失敗し `SetScene` が成功したバッチの `Result` が `Completed` か `Failed` かが決まらない
- 決定（案どおり）: 規則そのもののテストは core（T9）に置く。switcher（T6）では SQ-26 の2つだけを確かめる。`RestartMedia` だけが失敗したバッチは `Result: Completed`、`Err` に RESTART のエラーを入れ、Machine は `Err != nil` なら `last_error` に記録する（Auto でも。10章「OBS の操作が失敗した」）
- 引いている行: BD-C-1、SQ-26

**U-19 `RECONNECTING` → `RECONNECTED` を switcher でどう確かめるか**
- spec: 11章の switcher の必須ケース「`RECONNECTING` → `RECONNECTED` の列では、手動保持、Pause の直前の役割、未完了のバッチがどれも変わらない」と、7章「`STARTING`、`STOPPING`、`RECONNECTING`、`RECONNECTED` では何も流さない」
- 不明な点: 写像は obs（T7）にあり、switcher にはこの2つに当たる Event が届かない。switcher のテストで流す Event がない
- 決定（案どおり）: 写像の全値は T7 の変換関数のテストで確かめる。switcher（T5）は SQ-24（その間の `Tick` だけで状態が変わらない）を確かめる。obs の偽物から core まで通す確認は T9 に足す
- 引いている行: SQ-24

### 細部

**U-08 保持を解除する Command の範囲**
- spec: 「本体の Command（GoLive、PauseOff など）を受け付けたとき」（6章）
- 不明な点: 「など」の範囲。保持中は役割が Live か Fail で配信中なので、受け付けられる Command は StreamStop と PauseOn だけである（例に挙がっている GoLive と PauseOff は、保持中には受け付けられない）
- 決定（案どおり）: 受け付けた Command（`cmdErr` が nil）はどれでも解除する。`cmdErr` を返した Command では解除しない。解除した `Handle` では Command のバッチだけを返し、自動判定は次の Event か `Tick` で行う
- 引いている行: 2.11 の共通、CMD-STOP-4、CMD-PON-5、SQ-09a

**U-20 保持中の手動の Live ↔ Fail と `since`**
- spec: 「手動保持中かどうかと、いつから保持しているかを status に出す」（6章）
- 不明な点: 保持中に手動で Live から Fail（またはその逆）に切り替えたとき、`since` を更新するか
- 決定（案どおり）: 更新しない（保持に入った時刻のまま）
- 引いている行: SCM-11

**U-21 `SetScene` 以外の `Proceed` が返す `ReqID`**
- spec: 「`Proceed{ReqID}`: 送る。`SetScene` なら期待を登録して ReqID を返す」（6章）
- 不明な点: `RestartMedia`、`StartStream`、`StopStream` の `Proceed` の `ReqID`
- 決定（案どおり）: 0（`NextReqID` も進めない）
- 引いている行: 2.12 の前書き

**U-22 未知の `BatchID` / `ReqID`**
- spec: 書かれていない
- 不明な点: `Pending` にない `BatchID` の `Approve` と `BatchDone`、期待にない `ReqID` の `SceneRequestDone`
- 決定（案どおり）: `Approve` は `Reject`、`BatchDone` と `SceneRequestDone` は何もしない（`SceneRequestDone` は、期待が先に `SceneChanged` で消える通常の列で実際に起きる。SRD-OK-2）
- 引いている行: AP-COM-4、BD-X-1

**U-23 `OBSConnected` でシーンが切断前と違っていたとき**
- spec: 「`SceneEpoch` は手動とみなした `SceneChanged` のたびに」「`OBSConnected` で、シーンか配信状態が切断の直前と違っていたとき（保持を解除する）」（6章）
- 不明な点: 切断中にシーンが変わっていたことを手動の切替として扱う（`SceneEpoch` を進める、配信中で Live / Fail なら保持に入る）か
- 決定（案どおり）: 扱わない。`SceneEpoch` は進めず、保持にも入らない（切断前のバッチは `ConnEpoch` で失効する。保持は解除だけ）
- 引いている行: OC-8
