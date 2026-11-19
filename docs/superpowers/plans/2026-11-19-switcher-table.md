# switcher「入口 × 状態」の表（spec v2.2 の期待値）

- 対象 spec: `docs/superpowers/specs/2026-11-18-core-design.md` v2.2 の 6章（「細部の規則」「実装前に決めたこと」を含む）、7章、9章、11章
- 対象: `internal/switcher/` の書き直し（タグは `SW` の1つ。実装の前にこの表をレビューする）
- 状態: v1.x の表（4役割、手動保持、配信開始の待ち）を捨て、待機シーンと監視対象シーンのモデルで作り直した。4章の不明点 `V-xx` は spec v2.2 の 6章「細部の規則（v2.1 の表を作る中で決めたこと）」で、すべて案どおりに決めた。`V-xx` を引いている行は、その案を期待値とする。v1.x の不明点の番号 `U-xx` は、spec 6章「細部の規則」が引いている番号で、決定済みの規則を指す
- 読み方: 表の行番号（`LC-1`、`CMD-SON-6`、`AP-SOFFM-3`、`SQ-05b` など）をテスト名かテーブルの行の名前に入れ、どの行を確かめているかを辿れるようにする

## 0. 表記

- **T 列:** その行を緑にする実装のまとまり。この版ではすべて `SW`
- **書いていない欄は変わらない。** 「—」は、出力なし、または状態の変化なし
- **例に使うシーン名:** 待機シーン `"Standby"`、監視対象 `"Camera"` と `"Camera2"`、管理外 `"Talk"` と `"Opening"`。設定を変える行では、変えた点だけを書く
- **分類:** 現在のシーンを、その時点の設定で Monitored / 待機シーン / Other に分ける。未接続の間は「不明」で、どの条件にも当てはまらない
- **不良:** 受信状態が LOW か DOWN
- **持ち主 / 戻り先:** `StandbyOwner`（`none` / `auto` / `manual`）と `ReturnScene`
- **本体の切替 / 手動:** `SceneChanged` が期待と一致した / しなかった（6章「見分け方」）
- **バッチの書き方:** `AutoFail[SetScene("Standby")]` は Purpose と Action の列。`W` は `RestartMedia.Wait`（spike ② で決める定数）。待機シーンへ切り替えるバッチ（AutoFail、StandbyOn）は「戻り先=…」でバッチが持つ `ReturnScene` を書く
- **シーン切替のバッチ:** AutoFail、AutoRecover、StandbyOn、StandbyOffMonitored、StandbyOffOther、StartScene（`RestartMedia` の Action も含む）。StartStream と StopStream は含まない
- **Command 由来:** StandbyOn、StandbyOffMonitored、StandbyOffOther、StartStream、StopStream

### 持ち主と戻り先を消す C

表で「C」と書いたら、`StandbyOwner=none`、`ReturnScene` をなしにする。現在のシーンは変えない。

### 入口の期限切れ E

すべての `Handle` と `Approve` は、最初に、その `At`（`Approve` は `at`）が `Deadline` を過ぎた（`At > Deadline`）応答済みの期待を消す（U-11）。消しても `SceneEpoch` は進めず、持ち主と戻り先も変えない。消した期待の Purpose が StartScene なら `LastError` を記録する（StandbyOn、StandbyOffMonitored、StandbyOffOther の期待の期限切れを記録するかは V-18）。`cmdErr` を返す Command でも行う。表の各行は、E を済ませたあとの状態を前提に書く。

### 切替中の抑止 X

次のどちらかなら「抑止中」（6章「切替中の抑止」、U-12）。

- `Pending` に StartScene、StandbyOn、StandbyOffMonitored、StandbyOffOther のどれかのバッチがある
- `Expectations` に、その4つの Purpose のどれかが登録した期待がある（設定の世代が古い期待も数える）

### 自動判定 J

6章「自動切替」と「再送の歯止め」をまとめたもの。表で「J」と書いたら、その Event を処理し終えた状態で次を上から確かめ、すべて満たすときだけバッチを1本返す。J を行う入口は `LinkChanged`、`SceneChanged`（本体でも手動でも）、`StreamStateChanged`、`OBSConnected`、`ScenesConfigured`、`Tick` だけで、`Command`、`SceneRequestDone`、`BatchDone`、`TransitionChanged` では行わない。

1. `Connected` かつ `Streaming` かつ `AutoEnabled`
2. 抑止中（X）でない
3. 次のどちらかに当たる。どちらでもなければ出さない
   - 分類が Monitored、`Link` が不良、持ち主が `none` → Purpose は AutoFail、`[SetScene(待機シーン)]`、戻り先=現在のシーン名
   - 持ち主が `auto`、現在のシーンが待機シーン、`Link == OK` → AutoRecover、`[RestartMedia{W}, SetScene(戻り先)]`
4. `Pending` に同じ Purpose のバッチがない
5. `Expectations` に、切替先のシーン名（AutoFail なら今の待機シーン、AutoRecover なら戻り先）と同じ名前への期待がない（登録した Purpose を問わず、シーン名で比べる。V-17）
6. `Tick` のときだけ: `At - LastAutoSent[Purpose] >= 5秒`。その Purpose の記録がなければ満たしたものとする（U-07）

### バッチを出したときの共通の変化 B

`Handle` がバッチを返したら、必ず次が起きる。表では「B」と書く。

- `NextBatchID` が1つ進み、返したバッチの `BatchID` になる
- バッチはその時点の `SceneEpoch`、`ConnEpoch`、`StreamEpoch`、`ConfigEpoch` を持つ。`SetScene` の切替先は、その時点のシーン名で固定する
- `Pending[BatchID]` に Purpose、4つの Epoch、`ReturnScene`（AutoFail と StandbyOn だけ）を入れる
- Purpose が AutoFail / AutoRecover なら `LastAutoSent[Purpose] = At`（U-07）
- 持ち主と戻り先は変えない（入るのは、期待どおりの `SceneChanged` が届いた時点）

`Handle` が1回で返すバッチは多くても1本である。v2.1 の `Replace` は代わりのバッチを持たない（`Replace{nil}`）ので、`Approve` が B を起こすことはない。

## 1. Machine の内部状態

テストが読む語彙。フィールド名は実装とそろえる（テストから読めるよう、`Snapshot` か、パッケージ内テストの直接参照で見えるようにする）。

### 定数（生成時に受け取り、変わらない）

| 名前 | 型 | 意味 |
|---|---|---|
| `RestartWait` | `time.Duration` | `RestartMedia.Wait` に入れる値（`W`）。3章の列のテストは 0.5秒で Machine を作る |
| `ExpectMargin` | `time.Duration` | 期待の期限の余裕。3秒 |
| `ExpectFloor` | `time.Duration` | 上書きあり・長さ不明のときの下限。10秒 |
| `RetryInterval` | `time.Duration` | `Tick` の再送の間隔。5秒 |

### シーンの設定（`ScenesConfigured` で差し替える）

| 名前 | 型 | 意味 |
|---|---|---|
| `Standby` | `string` | 待機シーンの名前 |
| `Monitored` | `[]string` | 監視対象のシーン名の列 |
| `AutoEnabled` | `bool` | 自動切替が有効か |
| `StartScene` | `*string` | 配信開始時に切り替えるシーン。無効なら nil |
| `ConfigEpoch` | `int` | 設定の世代。内容が違う `ScenesConfigured` のたびに1つ進む（最初の1回は V-10） |

### 状態

| 名前 | 型 | 意味 |
|---|---|---|
| `Connected` | `bool` | OBS に接続しているか |
| `Streaming` | `bool` | 配信中か（7章の写像を通った値）。切断中は切断の直前の値を持ち続ける |
| `Scene` | `string` | program の現在のシーン名。切断中は切断の直前の値を持ち続ける |
| `Class` | `SceneClass` | `Scene` を今の設定で分類した値。`ClassMonitored` / `ClassStandby` / `ClassOther` / `ClassUnknown`（未接続）。持たずに毎回計算してもよい |
| `PrevConnScene` | `string` | 切断の直前のシーン名。初期値は空（U-15） |
| `PrevConnStreaming` | `bool` | 切断の直前の配信状態。初期値は false（U-15） |
| `Link` | `ingest.LinkState` | 受信状態。初期値は `LinkDown`（U-15） |
| `StandbyOwner` | `Owner` | 待機の持ち主。`OwnerNone` / `OwnerAuto` / `OwnerManual`。初期値は `OwnerNone` |
| `ReturnScene` | `string` | 戻り先のシーン名。なしは空。持ち主が `none` の間は必ず空 |
| `Expectations` | `[]Expectation` | 期待の列。登録順（先頭が古い） |
| `Pending` | `map[int]PendingBatch` | 未完了のバッチ。`BatchDone` が来るまで持つ |
| `LastAutoSent` | `map[Purpose]time.Time` | AutoFail / AutoRecover のバッチを最後に出した時刻。再送の5秒の起点（U-07）。配信の停止で消す（U-16） |
| `SceneEpoch` | `int` | 手動とみなした `SceneChanged` のたびに1つ進む |
| `ConnEpoch` | `int` | `OBSDisconnected` のたびに1つ進む |
| `StreamEpoch` | `int` | 配信状態が変わるたびに1つ進む（`StreamStateChanged` と、`OBSConnected` で切断前と違ったとき） |
| `TransitionMs` | `*int` | 全体のトランジションの長さ。固定長で取れなければ nil |
| `LastError` | `*ErrorInfo` | 最後に記録したエラー（時刻と文）。消さず、新しい記録で上書きする（U-05） |
| `NextBatchID` | `int` | 次に振る `BatchID`。通し番号（1 から） |
| `NextReqID` | `int` | 次に振る `ReqID`。通し番号（1 から） |

OBS に接続している間は、持ち主が `none` 以外なら現在のシーンは待機シーンである（6章。これが崩れる列は V-01）。配信中でない間は、持ち主は `none`。

`Expectation` の中身:

| 名前 | 型 | 意味 |
|---|---|---|
| `ReqID` | `int` | `Approve` が `Proceed` で返した番号。`SceneRequestDone` と1対1 |
| `Scene` | `string` | 送った `SetScene` の切替先のシーン名 |
| `Purpose` | `Purpose` | 登録したバッチの Purpose |
| `ReturnScene` | `string` | バッチが持つ戻り先（AutoFail、StandbyOn だけ。ほかは空） |
| `ConfigEpoch` | `int` | 登録した時点の設定の世代 |
| `RegisteredAt` | `time.Time` | `Approve` の `at` |
| `Override` | `Override` | `Approve` に渡された上書きの状態（`None` / `Known{Ms}` / `Unknown`） |
| `TransitionMs` | `*int` | `Approve` の時点の全体の長さ（U-10） |
| `Responded` | `bool` | `SceneRequestDone` を受けたか。false の間は期限切れにならない |
| `RespondedAt` | `time.Time` | `SceneRequestDone` の `At` |
| `Result` | `OK / Timeout` | 応答の結果（`Error` は期待ごと消すので残らない） |
| `Deadline` | `time.Time` | `Responded` のときだけ有効。式は 2.9 の表 |

`PendingBatch` の中身: `BatchID`、`Purpose`、`SceneEpoch`、`ConnEpoch`、`StreamEpoch`、`ConfigEpoch`、`ReturnScene`、`Replaced bool`（`Approve` が `Replace` を返したバッチ。却下を `LastError` に記録しない印）、`ManualRejected bool`（StartScene だけ。手動の切替による却下の印。決め方は V-05）。

Purpose の一覧: `AutoFail`、`AutoRecover`、`StandbyOn`、`StandbyOffMonitored`、`StandbyOffOther`、`StartScene`、`StartStream`、`StopStream`。Command は `StreamStart`、`StreamStop`、`StandbyOn`、`StandbyOff` の4つ。

## 2. 入口ごとの表

各表の上に「特記がなければ」の前提を書く。行の「前提」欄には、そこから変えた点だけを書く。時間に依存する値（期限の具体的な時刻、5秒）は表に入れず、条件の成否だけを書く。具体的な時刻は 3章で確かめる。ありえない組み合わせ（未接続で期待あり、持ち主が `none` 以外で現在のシーンが待機シーンでない、配信中でないのに持ち主がある）は行にしない。

### 2.1 ScenesConfigured{Standby, Monitored, AutoEnabled, StartScene}

特記がなければ: 設定済み、接続、配信中、`auto_enabled` は true、持ち主 `none`、未完了なし、期待なし、今の設定と内容が違う。内容が違う行ではすべて、設定を差し替え、`ConfigEpoch+1`、分類をやり直す。`Expectations` と `Pending` は変えない。持ち主と戻り先は「消す条件」（待機シーンの名前が変わった / 持ち主が `auto` で戻り先が新しい Monitored にない / 持ち主が `auto` で `auto_enabled` が false になった）に当たれば C、当たらなければ保つ。そのあと J。StartScene のバッチは、この Event では出さない。

| # | T | 前提 | 出力 | 状態の変化（上の共通に加えて） |
|---|---|---|---|---|
| SCF-1 | SW | 最初の `ScenesConfigured`（未設定、未接続） | — | 設定が入る。`ConfigEpoch` は V-10（提案: 1 になる） |
| SCF-2 | SW | 今と同じ内容 | — | 何も変えない（`ConfigEpoch` も。U-14）。`Monitored` の順序だけが違うときは V-10 |
| SCF-3 | SW | 現在のシーンは Other。それを `Monitored` に足す。不良 | `AutoFail[SetScene(待機シーン)]`、戻り先=現在のシーン | B |
| SCF-4 | SW | SCF-3 で OK | — | 以後 Monitored として判定する |
| SCF-5 | SW | 現在のシーンは Monitored。それを `Monitored` から外す。不良 | — | 以後 Other。不良でも出ない |
| SCF-6 | SW | 持ち主 `auto`（待機シーン）。待機シーンの名前を変える | —（持ち主が消え、今いる旧待機シーンが Other になる） | C |
| SCF-7 | SW | 持ち主 `manual`（待機シーン）。待機シーンの名前を変える | — | C |
| SCF-8 | SW | Monitored、不良、AutoFail の未完了も期待もない（前の AutoFail が `Error` で終わった直後）。待機シーンの名前を変える | `AutoFail[SetScene(新しい待機シーン)]`、戻り先=現在のシーン | B |
| SCF-9 | SW | 持ち主 `auto`。戻り先を `Monitored` から外す | — | C |
| SCF-10 | SW | 持ち主 `manual`。戻り先を `Monitored` から外す | — | 保つ（以後の StandbyOff は CMD-SOFF-7） |
| SCF-11 | SW | 持ち主 `auto`。`auto_enabled` を false にする | — | C（待機シーンに残る） |
| SCF-12 | SW | 持ち主 `manual`。`auto_enabled` を false にする | — | 保つ |
| SCF-13 | SW | 持ち主 `auto` か `manual`。消す条件に当たらない変更（別の監視対象の追加、`StartScene` だけの変更） | J（`auto` で OK、AutoRecover の未完了も戻り先への期待もなければ `AutoRecover`。ほかは —） | 保つ |
| SCF-14 | SW | `auto_enabled` を false → true。Monitored、不良 | `AutoFail[SetScene(待機シーン)]`、戻り先=現在のシーン | B |
| SCF-15 | SW | `auto_enabled` を false → true。待機シーン（持ち主 `none`。SCF-11 のあと） | — | 持ち主は `none` のまま（戻らない） |
| SCF-16 | SW | Monitored、不良、AutoFail が未完了（承認の前）。`auto_enabled` を false にする | — | `Pending` は残る（承認で却下。AP-COM-6） |
| SCF-17 | SW | 未完了のバッチと期待がある。どの変更でも | J | `Pending` も `Expectations` も残る（シーン切替のバッチは AP-COM-6 で却下、StartStream / StopStream は AP-COM-7。期待は SCE-13 / SCE-14） |
| SCF-18 | SW | 抑止中（StandbyOn の未完了か期待など）。現在の Other のシーンを `Monitored` に足す。不良 | —（J の 2） | — |
| SCF-19 | SW | Monitored、不良、AutoFail は完了済みで旧待機シーンへの期待だけが残る。待機シーンの名前を変える | `AutoFail[SetScene(新しい待機シーン)]`（新しい名前への期待はない。J の 5。V-17） | B。旧待機シーンへの期待は残る（SQ-15a、SQ-20） |
| SCF-20 | SW | SCF-19 で、変更が待機シーンの名前以外（別の監視対象の追加など） | —（今の待機シーンへの期待が残る。J の 5。V-17） | 期待は残る |
| SCF-21 | SW | 未接続（持ち主 `auto` か `manual` を持ったまま切断中） | — | V-11（提案: 接続中と同じ規則で持ち主を確かめる。J は未接続なので出ない） |
| SCF-22 | SW | 配信中でない。どの変更でも | — | 持ち主は `none` のまま |

### 2.2 LinkChanged{State}

特記がなければ: 接続、配信中、`auto_enabled` は true、持ち主 `none`、未完了なし、期待なし。LC-19 を除くすべての行で `Link` が新しい値になり、そのあと J。

| # | T | 前提（分類、持ち主、変化） | 出力 | 状態の変化 |
|---|---|---|---|---|
| LC-1 | SW | Monitored、OK → LOW | `AutoFail[SetScene(待機シーン)]`、戻り先=現在のシーン | B。持ち主は `none` のまま |
| LC-2 | SW | Monitored、OK → DOWN | LC-1 と同じ | B |
| LC-3 | SW | Monitored、LOW → DOWN、AutoFail が未完了 | — | — |
| LC-4 | SW | Monitored、LOW → DOWN、未完了はないが待機シーンへの期待が残る | — | — |
| LC-5 | SW | Monitored、LOW → DOWN、未完了も期待もない（前の AutoFail が `Error` で終わった直後。5秒たっていない） | `AutoFail[…]`（イベント起点には5秒の条件が掛からない。U-07） | B |
| LC-6 | SW | Monitored、不良 → OK | — | — |
| LC-7 | SW | Monitored、不良 → OK、AutoFail が未完了（承認の前） | — | `Pending` は残る（承認で却下。AP-AUTOFAIL-2） |
| LC-8 | SW | 待機シーン、持ち主 `auto`、不良 → OK | `AutoRecover[RestartMedia{W}, SetScene(戻り先)]`（この順） | B。持ち主は `auto` のまま |
| LC-9 | SW | 待機シーン、持ち主 `auto`、不良 → OK、AutoRecover が未完了、または戻り先への期待が残る | — | — |
| LC-10 | SW | 待機シーン、持ち主 `auto`、LOW ↔ DOWN、または OK → 不良 | — | — |
| LC-11 | SW | 待機シーン、持ち主 `none`（人が手動で選んだ）、どの変化でも | — | — |
| LC-12 | SW | 待機シーン、持ち主 `manual`、どの変化でも | — | — |
| LC-13 | SW | Other、どの変化でも | — | — |
| LC-14 | SW | 配信中でない、Monitored、OK → 不良 | — | — |
| LC-15 | SW | 未接続、どの変化でも | — | `Link` だけ変わる |
| LC-16 | SW | `auto_enabled` が false、Monitored、OK → 不良 | — | — |
| LC-17 | SW | 抑止中（StandbyOn が未完了、またはその期待が残る）、Monitored、OK → 不良 | —（J の 2） | — |
| LC-18 | SW | 抑止中（StartScene が未完了、またはその期待が残る）、Monitored、OK → 不良 | — | — |
| LC-19 | SW | 今と同じ `State` が届いた | — | 何も変えない（U-14） |
| LC-20 | SW | 抑止中（StandbyOffMonitored / StandbyOffOther の期待どおりの `SceneChanged` で Monitored に戻ったが、その `BatchDone` が未着）、OK → 不良 | — | —（`BatchDone` のあとの `Tick` で出る。TK-5） |
| LC-21 | SW | StopStream か StartStream だけが未完了（抑止ではない）、Monitored、OK → 不良 | `AutoFail[…]` | B |

### 2.3 OBSConnected{Streaming, Scene, TransitionMs}

前提は未接続。すべての行で `Connected=true`、`Streaming`、`Scene`、`TransitionMs` を Event の値にし、分類をやり直す。「同じ / 違う」は `PrevConnScene`、`PrevConnStreaming` との比較。シーンか配信状態が違えば C、配信状態が違えば `StreamEpoch+1`。`SceneEpoch` は進めない（U-23）。StartScene のバッチは出さない。そのあと J。

| # | T | 前提 | 出力 | 状態の変化 |
|---|---|---|---|---|
| OC-1 | SW | 起動後の最初の接続。`Streaming=false` | — | `StreamEpoch` は進まない |
| OC-2 | SW | 起動後の最初の接続。`Streaming=true` | J | `StreamEpoch+1`（初期値 false と違う。U-15） |
| OC-3 | SW | 再接続。シーンも配信状態も同じ、配信中、持ち主 `none`、Monitored、不良 | `AutoFail[…]`、戻り先=現在のシーン | B。`StreamEpoch` は進まない |
| OC-4 | SW | 再接続。同じ、持ち主 `auto`（待機シーン）、OK | `AutoRecover[RestartMedia{W}, SetScene(戻り先)]` | 持ち主と戻り先を保つ。B |
| OC-5 | SW | 再接続。同じ、持ち主 `auto` で不良 / 持ち主 `manual` / 待機シーンで持ち主 `none` / Other / Monitored で OK | — | 持ち主と戻り先を保つ |
| OC-6 | SW | 再接続。シーンが違う、持ち主 `auto` か `manual` | J（新しいシーンが Monitored で不良なら `AutoFail`、戻り先=新しいシーン） | C。`SceneEpoch` は進まない |
| OC-7 | SW | 再接続。配信状態が true → false（シーンは同じ）、持ち主 `auto` か `manual` | — | C。`StreamEpoch+1`。`LastAutoSent` は V-15（提案: 消さない） |
| OC-8 | SW | 再接続。配信状態が false → true、開始時の切替が有効 | J（StartScene は出ない） | `StreamEpoch+1` |
| OC-9 | SW | 再接続。同じ、Monitored、不良、切断前の AutoFail がまだ `Pending` にある | —（J の 4） | — |
| OC-10 | SW | 再接続。同じ、Monitored、不良、切断前の StandbyOn がまだ `Pending` にある | —（J の 2） | — |
| OC-11 | SW | `TransitionMs` が nil | 他の行と同じ | `TransitionMs=nil` |
| OC-12 | SW | 再接続。同じ、Monitored、不良、`auto_enabled` が false | — | — |

### 2.4 OBSDisconnected{}

前提は接続。すべての行で出力はなく、`Connected=false`、`ConnEpoch+1`、`Expectations` を空に、`PrevConnScene=Scene`、`PrevConnStreaming=Streaming` にする。分類は不明になる。`Streaming`、`Scene`、`Pending`、持ち主、戻り先、`LastAutoSent` は変えない。J は未接続なので出ない。

| # | T | 前提 | 出力 | 状態の変化（上の共通に加えて） |
|---|---|---|---|---|
| OD-1 | SW | 期待あり、未完了あり | — | 期待は全部消える。`Pending` は残る（承認で却下。AP-COM-2 / AP-COM-3） |
| OD-2 | SW | 持ち主 `auto` か `manual` | — | 持ち主と戻り先を保つ（`OBSConnected` で確かめる） |
| OD-3 | SW | StartScene の期待あり | — | 期待は消える。`LastError` は V-14（提案: 記録しない） |

### 2.5 SceneChanged{Scene}（期待どおり = 本体の切替）

前提: 接続、配信中。`Expectations` に、Event のシーン名と同じ名前の期待がある（応答の前でも後でも、`Timeout` の応答のあとでも、期限の前なら同じ）。先頭から見て最初に一致した期待を「一致した期待」と呼ぶ。すべての行で `Scene` を更新し、一致した期待とそれより前の期待を消す。`SceneEpoch` は進めない。持ち主と戻り先は、一致した期待の Purpose と設定の世代で決める（前にあって一緒に消した期待は作用しない）。そのあと J。特記がなければ、期待の `ConfigEpoch` は今と同じ、`auto_enabled` は true。

| # | T | 前提（一致した期待、受信状態） | 出力 | 状態の変化 |
|---|---|---|---|---|
| SCE-1 | SW | 待機シーンへの AutoFail の期待（戻り先 R）、不良 | — | 持ち主 `auto`、戻り先 R |
| SCE-2 | SW | SCE-1 で、その間に OK に戻っていた | `AutoRecover[RestartMedia{W}, SetScene(R)]` | 持ち主 `auto`、戻り先 R。B |
| SCE-3 | SW | 待機シーンへの StandbyOn の期待（戻り先 R。R の分類と受信状態は何でも） | — | 持ち主 `manual`、戻り先 R |
| SCE-4 | SW | 戻り先への AutoRecover の期待、持ち主 `auto`、OK | — | C |
| SCE-5 | SW | SCE-4 で、最後の承認のあとに不良になっていた（戻り先は Monitored） | `AutoFail[SetScene(待機シーン)]`、戻り先=届いたシーン | C のあと B |
| SCE-6 | SW | 戻り先（Monitored）への StandbyOffMonitored の期待、持ち主 `manual`、OK | — | C |
| SCE-7 | SW | SCE-6 で、最後の承認のあとに不良になっていた（`auto_enabled` は true）。StandbyOffMonitored の `BatchDone` は受け取り済み | `AutoFail[…]`、戻り先=届いたシーン | C のあと B |
| SCE-8 | SW | SCE-7 で、`BatchDone` が未着 | —（J の 2。`BatchDone` のあとの `Tick` で出る） | C |
| SCE-9 | SW | 戻り先（Monitored でない）への StandbyOffOther の期待、持ち主 `manual`、受信状態は何でも | — | C |
| SCE-10 | SW | 開始時のシーン（Other）への StartScene の期待 | — | 持ち主は `none` のまま |
| SCE-11 | SW | 開始時のシーン（Monitored）への StartScene の期待、不良、StartScene の `BatchDone` は受け取り済み | `AutoFail[…]`、戻り先=開始時のシーン | B |
| SCE-12 | SW | SCE-11 で、`BatchDone` が未着 | —（J の 2。`BatchDone` のあとの `Tick` で出る） | — |
| SCE-13 | SW | 期待の `ConfigEpoch` が今と違う。AutoFail か StandbyOn の期待 | J | 本体の切替とみなす（`SceneEpoch` は同じ）。持ち主と戻り先は入れない（`none` のまま）。設定の変更が消す条件に当たらなかった場合の結果は V-07 |
| SCE-14 | SW | 期待の `ConfigEpoch` が今と違う。AutoRecover / StandbyOffMonitored / StandbyOffOther の期待で、持ち主は設定の変更のあとも保たれている | V-01 | V-01（文面どおりなら持ち主と戻り先が残り、現在のシーンは待機シーンでなくなる。提案: C） |
| SCE-15 | SW | 期待 [A, B]（別のシーン）で `SceneChanged(B)` | J | 両方消える。持ち主と戻り先は B の期待だけで決まる |
| SCE-16 | SW | 期待 [A, B] で `SceneChanged(A)` | J | A だけ消え、後ろの B は残る |
| SCE-17 | SW | 期待 [X, X]（同じシーン名が2つ） | J | 持ち主と戻り先は先頭の期待で決まる。後ろの期待は V-02（6章の文面なら残る。11章と提案は「消える」） |
| SCE-18 | SW | `auto_enabled` が false（StandbyOn、StandbyOff の期待） | —（J の 1） | 持ち主と戻り先は SCE-3 / SCE-6 / SCE-9 と同じ |

配信中でない間に期待が残ることはない（配信の停止で全部消える）ので、その行はない。

### 2.6 SceneChanged{Scene}（手動 = どの期待とも一致しない）

前提: 接続。すべての行で `Scene` を更新し、`SceneEpoch+1`、`Expectations` を空に、C。`Pending` は変えない（シーン切替のバッチは承認で却下される。AP-COM-5）。そのあと J。特記がなければ: 配信中、`auto_enabled` は true、未完了なし。

| # | T | 前提（直前の状態 → 新しいシーン、受信状態） | 出力 | 状態の変化（上の共通に加えて） |
|---|---|---|---|---|
| SCM-1 | SW | → Monitored、OK | — | — |
| SCM-2 | SW | Other（持ち主 `none`） → Monitored、不良 | `AutoFail[SetScene(待機シーン)]`、戻り先=新しいシーン | B |
| SCM-3 | SW | 待機シーン（持ち主 `auto`） → 別の Monitored、不良 | `AutoFail[…]`、戻り先=新しいシーン（前の戻り先ではない） | 持ち主と戻り先が消えたうえで B |
| SCM-4 | SW | 待機シーン（持ち主 `manual`） → Monitored、不良 | `AutoFail[…]`、戻り先=新しいシーン | 同上 |
| SCM-5 | SW | 待機シーン（持ち主 `none`） → Monitored、不良 | `AutoFail[…]`、戻り先=新しいシーン | B |
| SCM-6 | SW | → 待機シーン（人が手動で選んだ）、受信状態は何でも | — | 持ち主は `none`（回復しても戻さない） |
| SCM-7 | SW | 待機シーン（持ち主 `auto` か `manual`） → Other | — | 持ち主と戻り先が消える |
| SCM-8 | SW | → Monitored、不良、AutoFail が未完了（承認の前） | —（J の 4） | `Pending` は残る。却下のあとの再送の時刻は V-06 |
| SCM-9 | SW | → Monitored、不良、抑止の Purpose（StandbyOn、StartScene など）が未完了 | —（J の 2） | `Pending` は残る（`BatchDone` のあとの `Tick` で出る。TK-5） |
| SCM-10 | SW | → Monitored、不良、`auto_enabled` が false | — | — |
| SCM-11 | SW | 配信中でない。どのシーンへでも | — | 持ち主は `none` のまま |
| SCM-12 | SW | 待機シーンへの期待が残る間に、別のシーンが届いた | J（新しいシーンで） | 期待は全部消える |
| SCM-13 | SW | 期待が期限切れで消えたあとに、その期待のシーンが届いた | J | 手動として扱う |
| SCM-14 | SW | 現在のシーンと同じ名前が届いた（期待なし） | V-09 | V-09（提案: 手動として扱う） |
| SCM-15 | SW | StartScene の期待が残る間の手動の切替 | J | 期待は消える。`LastError` は V-14（提案: 記録しない） |
| SCM-16 | SW | 本体が送った `SetScene` の `SceneChanged` が、手動の切替で期待が消えたあとに届いた | J | 手動として扱う（V-08。待機シーンなら持ち主 `none` で残る） |

### 2.7 TransitionChanged{TransitionMs}

J は行わない。

| # | T | 前提 | 出力 | 状態の変化 |
|---|---|---|---|---|
| TC-1 | SW | 接続、値あり（例: 5000） | — | `TransitionMs=5000` |
| TC-2 | SW | 接続、nil | — | `TransitionMs=nil` |
| TC-3 | SW | 承認済みで応答待ちの期待がある | — | その期待の期限には効かない（`Approve` の時点の値を使う。U-10） |

### 2.8 StreamStateChanged{Streaming}

前提: 接続。特記がなければ `auto_enabled` は true、未完了なし。`true` の行では `Streaming=true`、`StreamEpoch+1`。開始時の切替が有効なら StartScene のバッチを返し（J は行わない）、無効なら J。

| # | T | 前提 | 出力 | 状態の変化 |
|---|---|---|---|---|
| SST-1 | SW | `true`。開始時の切替は無効。Monitored、不良 | `AutoFail[…]`、戻り先=現在のシーン | B |
| SST-2 | SW | `true`。無効。Monitored で OK / Other / 待機シーン | — | — |
| SST-3 | SW | `true`。有効。現在のシーンは開始時のシーンでない（分類と受信状態は何でも。Monitored で不良でも） | `StartScene[SetScene(開始時のシーン)]`（AutoFail は出ない） | B（`ReturnScene` なし、`LastAutoSent` は変えない） |
| SST-4 | SW | `true`。有効。`auto_enabled` が false | `StartScene[…]` | B |
| SST-5 | SW | `true`。有効。現在のシーンがすでに開始時のシーン | V-04（提案: `StartScene[…]` を出し、承認で `Skip`） | V-04 |
| SST-6 | SW | `true`。有効。前の配信の StartScene がまだ `Pending` にある | V-03（提案: 出さない。J も抑止で出ない） | V-03 |
| SST-7 | SW | `true`。無効。`auto_enabled` が false、Monitored、不良 | — | — |
| SST-8 | SW | `true`。すでに配信中 | — | 何も変えない（`StreamEpoch` も。U-14） |
| SSF-1 | SW | `false`。配信中、期待あり、持ち主 `auto` か `manual` | — | `Streaming=false`、`StreamEpoch+1`、C、`Expectations` を空に、`LastAutoSent` を空に（U-16） |
| SSF-2 | SW | `false`。配信中、未完了あり | — | SSF-1 に加えて、`Pending` は残る（承認で却下。AP-COM-4） |
| SSF-3 | SW | `false`。配信中、StartScene の期待あり | — | 期待は消える。`LastError` は V-14（提案: 記録しない） |
| SSF-4 | SW | `false`。すでに配信中でない | — | 何も変えない（U-14） |

### 2.9 SceneRequestDone{ReqID, Result}

出力はどの行もなし（J は行わない。期待が消えたあとの再送は `Tick` が行う）。

| # | T | 前提 | 状態の変化 |
|---|---|---|---|
| SRD-OK-1 | SW | `OK`。その `ReqID` の期待が応答待ち | `Responded=true`、`RespondedAt=At`、`Result=OK`、`Deadline` を下の式で決める。期待は残る。持ち主は変えない |
| SRD-OK-2 | SW | `OK`。その `ReqID` の期待がない（先に `SceneChanged` で消えた、切断や配信停止や手動の切替で消えた、未知の番号） | —（U-22） |
| SRD-TO-1 | SW | `Timeout`。期待が応答待ち | SRD-OK-1 と同じ（`Result=Timeout`）。期待は期限まで残る |
| SRD-ERR-1 | SW | `Error`。期待が応答待ち | その期待だけをすぐ消す（列の他の期待は残る） |
| SRD-ERR-2 | SW | `Error`。StartScene の期待 | 期待を消す。`LastError` はここでは記録しない（続く `BatchDone{Failed}` で記録する。BD-F-3） |
| SRD-ERR-3 | SW | `Error`。その `ReqID` の期待がない | — |

`Deadline` の式（`G` は期待に写した全体の長さ。U-09、U-10）:

| # | T | `Override` | 全体の長さ | `Deadline` |
|---|---|---|---|---|
| DL-1 | SW | `None` | `G` あり | `RespondedAt + G + 3秒` |
| DL-2 | SW | `None` | nil | `RespondedAt + 10秒` |
| DL-3 | SW | `Known{Ms}` | どちらでも | `RespondedAt + max(Ms + 3秒, 10秒)` |
| DL-4 | SW | `Unknown` | `G` あり | `RespondedAt + max(G + 3秒, 10秒)` |
| DL-5 | SW | `Unknown` | nil | `RespondedAt + 10秒` |

### 2.10 BatchDone{BatchID, Result, Err}

BD-X-1 を除くすべての行で `Pending` からその `BatchID` を消す。出力はどの行もなし（J は行わない）。持ち主と戻り先はどの行でも変えない。

| # | T | 前提（Purpose、結果） | 状態の変化（`Pending` から消すのに加えて） |
|---|---|---|---|
| BD-C-1 | SW | `Completed`、`Err` は nil。どの Purpose でも（`Skip` で終わったバッチを含む） | —（`LastError` は記録しない） |
| BD-C-2 | SW | `Completed`、`Err` あり（`RestartMedia` だけが失敗した AutoRecover / StandbyOffMonitored） | `LastError` を記録（U-18） |
| BD-F-1 | SW | `Failed`。AutoFail / AutoRecover | `LastError` を記録（U-05）。再送は `Tick` |
| BD-F-2 | SW | `Failed`。StandbyOn / StandbyOffMonitored / StandbyOffOther / StartStream / StopStream | `LastError` を記録。持ち主が `manual` なら、もう一度 StandbyOff を受け付ける |
| BD-F-3 | SW | `Failed`。StartScene | `LastError` を記録。再送しない |
| BD-R-1 | SW | `Rejected`。AutoFail / AutoRecover | —（`LastError` は記録しない） |
| BD-R-2 | SW | `Rejected`。Command 由来、`Replaced` でない | `LastError` を記録 |
| BD-R-3 | SW | `Rejected`。`Replaced` の StandbyOffMonitored | —（`LastError` は記録しない） |
| BD-R-4 | SW | `Rejected`。StartScene、手動の切替（`SceneEpoch`）による却下 | —（`LastError` は記録しない。見分け方は V-05） |
| BD-R-5 | SW | `Rejected`。StartScene、それ以外の却下（配信の停止、切断、設定の変更） | `LastError` を記録。再送しない |
| BD-R-6 | SW | `Rejected`。StartScene、手動の切替と、ほかの条件（配信の停止など）の両方が起きていた | V-05（提案: 記録しない） |
| BD-J-1 | SW | 抑止の Purpose のバッチの `BatchDone` で抑止が解け、Monitored で不良、持ち主 `none` | 出力なし（次の Event か `Tick` で `AutoFail` が出る。TK-5） |
| BD-X-1 | SW | 未知の `BatchID` | 何もしない（U-22） |

### 2.11 Tick{}

すべての行で、まず E、次に J（5秒の条件つき）。StartScene はどの行でも出ない。

| # | T | 前提 | 出力 | 状態の変化 |
|---|---|---|---|---|
| TK-1 | SW | 接続、配信中、`auto_enabled`、Monitored、不良、持ち主 `none`、抑止なし、AutoFail の未完了なし、待機シーンへの期待なし、前回の AutoFail の送出から5秒以上 | `AutoFail[…]`、戻り先=現在のシーン | B |
| TK-2 | SW | TK-1 で、前回の送出から5秒未満 | — | — |
| TK-3 | SW | TK-1 で、AutoFail が未完了 | — | — |
| TK-4 | SW | TK-1 で、待機シーンへの期待が残る（応答待ち、または期限の前） | — | 期待は残る |
| TK-5 | SW | TK-1 で、AutoFail を一度も出していない（抑止が `BatchDone` で解けた直後、配信の停止で記録が消えたあと、など） | `AutoFail[…]`（U-07） | B |
| TK-6 | SW | 待機シーン、持ち主 `auto`、OK、抑止なし、AutoRecover の未完了なし、戻り先への期待なし、前回の AutoRecover の送出から5秒以上（または一度も出していない） | `AutoRecover[RestartMedia{W}, SetScene(戻り先)]` | B |
| TK-7 | SW | TK-6 で、前回の送出から5秒未満 | — | — |
| TK-8 | SW | TK-1 か TK-6 で、抑止中 | — | — |
| TK-9 | SW | `auto_enabled` が false / 配信中でない / 未接続 / Other / 待機シーンで持ち主 `none` か `manual` / Monitored で OK / 持ち主 `auto` で不良 | — | — |
| TK-10 | SW | 期限を過ぎた期待あり（`At > Deadline`） | J（消したあとの状態で） | その期待を消す。`SceneEpoch` は進めず、持ち主と戻り先も変えない（U-11）。AutoFail / AutoRecover の期待なら `LastError` は記録しない |
| TK-11 | SW | `At == Deadline` | — | 期待は残る |
| TK-12 | SW | 応答待ち（`Responded=false`）の期待。登録からどれだけたっていても | — | 期待は残る |
| TK-13 | SW | 期限を過ぎた StartScene の期待 | J（抑止が解けた状態で。StartScene は出ない） | 期待を消す。`LastError` を記録 |
| TK-14 | SW | StartScene が失敗、却下、期限切れで終わったあと（配信中） | StartScene は出ない（J だけ） | — |
| TK-15 | SW | 期限を過ぎた StandbyOn / StandbyOffMonitored / StandbyOffOther の期待（応答は `OK` で、バッチは `Completed` で終わっている） | J（抑止が解けた状態で） | 期待を消す。持ち主と戻り先は変えない（`manual` なら、もう一度 StandbyOff を受け付ける）。`LastError` は V-18（提案: 記録する） |

### 2.12 Command

共通: `cmdErr` を返す行では、E を除いて状態の変化はない。未接続の判定が最初（`ErrNotConnected`）。受け付けた `Handle` では Command のバッチだけを返し、J は行わない（`cmdErr` を返す行でも行わない）。複数の理由が同時に当たるときの理由の文の選び方は V-12（テストは `cmdErr` の型だけを確かめる）。

**StreamStart**

| # | T | 前提 | `cmdErr` | 出力 | 状態の変化 |
|---|---|---|---|---|---|
| CMD-START-1 | SW | 未接続 | `ErrNotConnected` | — | — |
| CMD-START-2 | SW | 接続、配信中 | `ErrInvalidState` | — | — |
| CMD-START-3 | SW | 接続、配信中でない（シーン、受信状態、`auto_enabled`、開始時の切替の有効・無効は何でも） | nil | `StartStream[StartStream]`（`SetScene` は出ない） | B |
| CMD-START-4 | SW | CMD-START-3 で、StartStream が未完了 | `ErrInvalidState`（同じ操作が実行待ち。U-04） | — | — |

**StreamStop**

| # | T | 前提 | `cmdErr` | 出力 | 状態の変化 |
|---|---|---|---|---|---|
| CMD-STOP-1 | SW | 未接続 | `ErrNotConnected` | — | — |
| CMD-STOP-2 | SW | 接続、配信中でない | `ErrInvalidState` | — | — |
| CMD-STOP-3 | SW | 接続、配信中（シーン、受信状態、持ち主は何でも） | nil | `StopStream[StopStream]`（`SetScene` は出ない） | B。持ち主と戻り先は変えない（消えるのは `StreamStateChanged{false}`） |
| CMD-STOP-4 | SW | CMD-STOP-3 で、StopStream が未完了 | `ErrInvalidState`（U-04） | — | — |

**StandbyOn**

| # | T | 前提 | `cmdErr` | 出力 | 状態の変化 |
|---|---|---|---|---|---|
| CMD-SON-1 | SW | 未接続 | `ErrNotConnected` | — | — |
| CMD-SON-2 | SW | 接続、配信中でない | `ErrInvalidState` | — | — |
| CMD-SON-3 | SW | 配信中、待機シーン、持ち主 `auto` | `ErrInvalidState`（すでに待機シーン） | — | — |
| CMD-SON-4 | SW | 配信中、待機シーン、持ち主 `none`（人が手動で選んだ） | `ErrInvalidState`（すでに待機シーン） | — | — |
| CMD-SON-5 | SW | 配信中、待機シーン、持ち主 `manual` | `ErrInvalidState`（すでに待機シーン） | — | — |
| CMD-SON-6 | SW | 配信中、待機シーンでない、StartScene / AutoFail / AutoRecover のどれかが未完了 | `ErrInvalidState`（切替の実行中） | — | — |
| CMD-SON-7 | SW | 配信中、待機シーンでない、未完了はないが期待が1つでも残る（どの Purpose の期待でも。`Timeout` の応答のあとの期待を含む） | `ErrInvalidState`（切替の実行中） | — | — |
| CMD-SON-8 | SW | 配信中、待機シーンでない、StandbyOn が未完了 | `ErrInvalidState`（同じ操作が実行待ち。U-04） | — | — |
| CMD-SON-9 | SW | 配信中、Monitored、持ち主 `none`、未完了なし、期待なし（受信状態は何でも） | nil | `StandbyOn[SetScene(待機シーン)]`、戻り先=現在のシーン | B。持ち主は `none` のまま |
| CMD-SON-10 | SW | CMD-SON-9 で Other | nil | `StandbyOn[…]`、戻り先=現在のシーン（Other でも） | B |
| CMD-SON-11 | SW | CMD-SON-9 で `auto_enabled` が false | nil | `StandbyOn[…]` | B |
| CMD-SON-12 | SW | CMD-SON-9 で、StopStream か、届いたあとの StandbyOffMonitored / StandbyOffOther（`BatchDone` が未着）だけが未完了 | nil | `StandbyOn[…]` | B |

**StandbyOff**

| # | T | 前提 | `cmdErr` | 出力 | 状態の変化 |
|---|---|---|---|---|---|
| CMD-SOFF-1 | SW | 未接続 | `ErrNotConnected` | — | — |
| CMD-SOFF-2 | SW | 接続、配信中でない | `ErrInvalidState` | — | — |
| CMD-SOFF-3 | SW | 配信中、持ち主 `none`（Monitored / Other / 人が手動で選んだ待機シーン） | `ErrInvalidState` | — | — |
| CMD-SOFF-4 | SW | 配信中、持ち主 `auto` | `ErrInvalidState` | — | — |
| CMD-SOFF-5 | SW | 配信中、持ち主 `manual`、戻り先が Monitored、OK、`auto_enabled` は true | nil | `StandbyOffMonitored[RestartMedia{W}, SetScene(戻り先)]` | B。持ち主は `manual` のまま |
| CMD-SOFF-6 | SW | CMD-SOFF-5 で LOW / DOWN | nil | —（バッチなし） | 持ち主 `auto`。戻り先とシーンは変えない |
| CMD-SOFF-7 | SW | 配信中、持ち主 `manual`、戻り先が Monitored でない、受信状態は何でも、`auto_enabled` は true | nil | `StandbyOffOther[SetScene(戻り先)]` | B |
| CMD-SOFF-8 | SW | 配信中、持ち主 `manual`、戻り先が Monitored、`auto_enabled` が false、受信状態は何でも（不良でも） | nil | `StandbyOffMonitored[RestartMedia{W}, SetScene(戻り先)]` | B。持ち主は `manual` のまま（`auto` にしない） |
| CMD-SOFF-9 | SW | CMD-SOFF-8 で戻り先が Monitored でない | nil | `StandbyOffOther[SetScene(戻り先)]` | B |
| CMD-SOFF-10 | SW | CMD-SOFF-5〜9 で、StandbyOffMonitored か StandbyOffOther が未完了 | `ErrInvalidState`（同じ操作が実行待ち。U-04） | — | —（CMD-SOFF-6 の前提でも持ち主を `auto` にしない） |
| CMD-SOFF-11 | SW | CMD-SOFF-5 / 7 で、前の StandbyOff の期待が残る（`Timeout` のあと。未完了はない） | nil | CMD-SOFF-5 / 7 と同じ | B（同じシーンへの期待が2つになる。SCE-17、V-02） |
| CMD-SOFF-12 | SW | CMD-SOFF-5 / 7 で、StandbyOn の `BatchDone` が未着（`SceneChanged` が先に届いた） | nil | CMD-SOFF-5 / 7 と同じ | B |

### 2.13 Approve(batchID, action, override, at)

確かめる順（U-01）: E → 共通の条件（`Pending` にある、接続、4つの Epoch）→ `SetScene` で現在のシーン名が切替先と同じなら `Skip`（StandbyOn を除く）→ Purpose の有効条件（Auto はさらに抑止）。`Proceed` を `SetScene` に返したときは、`Expectations` の末尾に `{ReqID, Scene, Purpose, ReturnScene, ConfigEpoch, RegisteredAt=at, Override, TransitionMs, Responded=false}` を足し、`NextReqID` を進める（表では「期待を登録」と書く）。`SetScene` 以外の `Proceed` の `ReqID` は 0（U-21）。`Approve` は `Pending` から消さず（消すのは `BatchDone`）、持ち主と戻り先も AP-SOFFM-3 を除いて変えない。

**共通の条件（どの Purpose でも）**

| # | T | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|
| AP-COM-1 | SW | 未知の `batchID`（`Pending` にない） | `Reject`（U-22） | — |
| AP-COM-2 | SW | 未接続（`OBSDisconnected` のあと、`OBSConnected` の前） | `Reject` | — |
| AP-COM-3 | SW | 接続しているが、バッチを作ったあとに切断と再接続があった（`ConnEpoch` が違う） | `Reject` | — |
| AP-COM-4 | SW | 配信状態がバッチを作ったあとに変わった（`StreamEpoch` が違う。止まったまま、または止まって始め直した） | `Reject` | — |
| AP-COM-5 | SW | シーン切替のバッチ（`RestartMedia` の承認も同じ）で、作ったあとに手動の `SceneChanged` があった（`SceneEpoch` が違う） | `Reject` | StartScene なら `ManualRejected` の印（BD-R-4。印の決め方は V-05） |
| AP-COM-6 | SW | シーン切替のバッチで、作ったあとに内容の違う `ScenesConfigured` があった（`ConfigEpoch` が違う。消す条件に当たらない変更でも、`auto_enabled` や `StartScene` だけの変更でも） | `Reject` | — |
| AP-COM-7 | SW | StartStream / StopStream で、`SceneEpoch` か `ConfigEpoch` が違う | `Proceed`（この2つは見ない） | — |
| AP-COM-8 | SW | `at` が期限を過ぎた期待がある | 残りの判定は消したあとの状態で行う | E |

各 Purpose の「配信中」「配信中でない」「`auto_enabled`」は、バッチを作ったあとに崩れると必ず `StreamEpoch` か `ConfigEpoch` が進むので、AP-COM-4 と AP-COM-6 で確かめたことになる。下の表には、共通の条件を満たしたうえで結果が分かれる行だけを書く。「到達しない」と書いた行は、`Handle` と `Approve` だけを通る列では作れない（理由は V-13）。

**AutoFail** `[SetScene(待機シーン)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-AUTOFAIL-1 | SW | `SetScene(待機シーン)` | Monitored、LOW か DOWN（作ったあとに LOW ↔ DOWN と変わっていても）、持ち主 `none` | `Proceed{ReqID}` | 期待を登録（待機シーン、AutoFail、戻り先）。持ち主は `none` のまま |
| AP-AUTOFAIL-2 | SW | `SetScene(待機シーン)` | Monitored、OK に戻った | `Reject` | — |
| AP-AUTOFAIL-3 | SW | `SetScene(待機シーン)` | 本体の切替ですでに待機シーン（Epoch は同じ） | `Skip`。到達しない（V-13） | — |
| AP-AUTOFAIL-4 | SW | `SetScene(待機シーン)` | 本体の切替で Monitored でなくなった（Epoch は同じ。設定が変わる前の期待で旧待機シーンへ届いた。SQ-15a） | `Reject` | —。「持ち主が `none` でない」で却下になる列には到達しない（V-13） |
| AP-AUTOFAIL-5 | SW | `SetScene(待機シーン)` | 抑止中（Epoch は同じ） | `Reject`。列では AP-COM-4 と重なる（SQ-10e。V-13） | — |

**AutoRecover** `[RestartMedia{W}, SetScene(戻り先)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-AUTOREC-1 | SW | `RestartMedia` | 待機シーン、持ち主 `auto`、OK | `Proceed`（`ReqID` は 0） | —（期待は登録しない） |
| AP-AUTOREC-2 | SW | `SetScene(戻り先)` | 待機シーン、持ち主 `auto`、OK | `Proceed{ReqID}` | 期待を登録（戻り先、AutoRecover）。持ち主は `auto` のまま |
| AP-AUTOREC-3 | SW | どちらでも | 待機シーン、持ち主 `auto`、LOW / DOWN になった | `Reject` | 持ち主と戻り先は保つ |
| AP-AUTOREC-4 | SW | `SetScene(戻り先)` | 本体の切替ですでに戻り先（Epoch は同じ） | `Skip`。到達しない（V-13） | — |
| AP-AUTOREC-5 | SW | `RestartMedia` | 現在のシーンが待機シーンでない（Epoch は同じ） | `Reject`（`RestartMedia` に `Skip` はない）。到達しない（V-13） | — |
| AP-AUTOREC-6 | SW | どちらでも | 抑止中（Epoch は同じ） | `Reject`。到達しない（V-13） | — |

**StandbyOn** `[SetScene(待機シーン)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-SON-1 | SW | `SetScene(待機シーン)` | 持ち主 `none`、待機シーンでない、受信状態は何でも（作ったあとに何度変わっていても） | `Proceed{ReqID}` | 期待を登録（待機シーン、StandbyOn、戻り先）。持ち主は `none` のまま |
| AP-SON-2 | SW | `SetScene(待機シーン)` | 現在のシーンがすでに待機シーン（Epoch は同じ） | `Reject`（`Skip` にしない）。到達しない（V-13）。列では手動の切替で AP-COM-5 になる（SQ-18g） | —（`LastError` は `BatchDone{Rejected}` で記録。BD-R-2） |
| AP-SON-3 | SW | `SetScene(待機シーン)` | 持ち主が `none` でない（Epoch は同じ） | `Reject`。到達しない（V-13） | — |

**StandbyOffMonitored** `[RestartMedia{W}, SetScene(戻り先)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-SOFFM-1 | SW | `RestartMedia` | 待機シーン、持ち主 `manual`、`auto_enabled` は true、OK | `Proceed`（`ReqID` は 0） | — |
| AP-SOFFM-2 | SW | `SetScene(戻り先)` | 同上 | `Proceed{ReqID}` | 期待を登録（戻り先、StandbyOffMonitored）。持ち主は `manual` のまま |
| AP-SOFFM-3 | SW | どちらでも | 待機シーン、持ち主 `manual`、`auto_enabled` は true、LOW / DOWN になった（受信状態だけが崩れた） | `Replace{nil}` | 持ち主 `auto`（戻り先とシーンは変えない）。`Pending` のそのバッチに `Replaced=true`。期待は登録しない。B は起きない |
| AP-SOFFM-4 | SW | どちらでも | 待機シーン、持ち主 `manual`、`auto_enabled` は false（作った時点から）、受信状態は何でも（作ったあとに何度変わっていても） | `Proceed` / `Proceed{ReqID}` | AP-SOFFM-1 / 2 と同じ。持ち主は `auto` にならない |
| AP-SOFFM-5 | SW | `SetScene(戻り先)` | `RestartMedia` の承認のあとに、前の StandbyOff の期待で戻り先へ届いた（現在のシーンが戻り先、持ち主 `none`、受信状態は何でも） | `Skip` | —（期待は登録しない） |
| AP-SOFFM-6 | SW | `RestartMedia` | `RestartMedia` の承認の前に、前の StandbyOff の期待で戻り先へ届いた（現在のシーンが待機シーンでない） | `Reject`（`RestartMedia` に `Skip` はない。U-01） | —（`BatchDone{Rejected}` で `LastError` に記録される。V-16） |

**StandbyOffOther** `[SetScene(戻り先)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-SOFFO-1 | SW | `SetScene(戻り先)` | 待機シーン、持ち主 `manual`、受信状態と `auto_enabled` は何でも | `Proceed{ReqID}` | 期待を登録（戻り先、StandbyOffOther）。持ち主は `manual` のまま |
| AP-SOFFO-2 | SW | `SetScene(戻り先)` | 前の StandbyOff の期待で戻り先へ届いた（現在のシーンが戻り先、持ち主 `none`） | `Skip` | — |
| AP-SOFFO-3 | SW | `SetScene(戻り先)` | 持ち主が `manual` でない、または現在のシーンが待機シーンでも戻り先でもない（Epoch は同じ） | `Reject`。到達しない（V-13） | — |

**StartScene** `[SetScene(開始時のシーン)]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-STARTSCENE-1 | SW | `SetScene(開始時のシーン)` | 配信中、現在のシーンは開始時のシーンでない、受信状態と `auto_enabled` は何でも | `Proceed{ReqID}` | 期待を登録（開始時のシーン、StartScene） |
| AP-STARTSCENE-2 | SW | `SetScene(開始時のシーン)` | 現在のシーンがすでに開始時のシーン | `Skip` | — |
| AP-STARTSCENE-3 | SW | `SetScene(開始時のシーン)` | 作ったあとに手動の `SceneChanged` があった（AP-COM-5） | `Reject` | `ManualRejected` の印（V-05）。`BatchDone{Rejected}` で記録しない（BD-R-4） |
| AP-STARTSCENE-4 | SW | `SetScene(開始時のシーン)` | 作ったあとに配信が止まった、切断があった、設定が変わった（AP-COM-3 / 4 / 6） | `Reject` | —（`BatchDone{Rejected}` で記録する。BD-R-5） |

**StartStream** `[StartStream]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-STARTSTREAM-1 | SW | `StartStream` | 配信中でない | `Proceed`（`ReqID` は 0） | — |
| AP-STARTSTREAM-2 | SW | `StartStream` | 作ったあとに配信が始まった（OBS の画面から。`StreamEpoch` が違う） | `Reject` | —（`BatchDone{Rejected}` で記録する。BD-R-2） |
| AP-STARTSTREAM-3 | SW | `StartStream` | 作ったあとに手動の `SceneChanged` か内容の違う `ScenesConfigured` があった | `Proceed`（AP-COM-7） | — |

**StopStream** `[StopStream]`

| # | T | Action | 前提 | 結果 | 状態の変化 |
|---|---|---|---|---|---|
| AP-STOP-1 | SW | `StopStream` | 配信中（シーン、受信状態、持ち主は何でも。作ったあとに何度変わっていても） | `Proceed`（`ReqID` は 0） | — |
| AP-STOP-2 | SW | `StopStream` | 配信中、作ったあとに手動の `SceneChanged` か内容の違う `ScenesConfigured` があった | `Proceed`（AP-COM-7） | — |
| AP-STOP-3 | SW | `StopStream` | 作ったあとに配信が止まり、始め直された（今は配信中、`StreamEpoch` が違う） | `Reject` | —（`BatchDone{Rejected}` で記録する。BD-R-2） |

## 3. Event の列のケース

### 共通の決まり

- 時刻 `t` は秒。Event の `At` と `Approve` の `at` に入れる。`Tick` は書いた時刻のものだけを流す（書いていない整数秒の `Tick` を足しても結果は変わらない）
- 略記: `LC{s}` = `LinkChanged{State: s}`、`SC{"x"}` = `SceneChanged{Scene: "x"}`、`SSC{b}` = `StreamStateChanged{Streaming: b}`、`SRD{r1, OK}` = `SceneRequestDone{ReqID: 1, Result: OK}`、`BD{1, Completed}` = `BatchDone{BatchID: 1, Result: Completed}`
- **設定 CFG:** `ScenesConfigured{Standby: "Standby", Monitored: ["Camera", "Camera2"], AutoEnabled: true, StartScene: nil}`。`CFG(auto=false)`、`CFG(start="Opening")`、`CFG(Monitored=[…])`、`CFG(Standby="Standby2")` は、その項目だけを変えたもの。`"Talk"` と `"Opening"` は、特記がなければ Other
- 既定: `TransitionMs=300`（全体の長さ 0.3秒）、`Override` は `None`、期限 = 応答の時刻 + 0.3 + 3 = 応答の時刻 + 3.3秒。上書きありか長さ不明なら10秒以上。再送は5秒。`W` は 0.5秒（Machine を `RestartWait=0.5秒` で作る）
- **準備 P(シーン, 配信, 受信状態):** t=0 に `ScenesConfigured{CFG}`（別の設定を使う列は明記する）、`LC{受信状態}`（未接続なので何も出ない。DOWN は初期値と同じなので何も起きない）、`OBSConnected{Streaming: 配信, Scene: シーン, TransitionMs: 300}`。`OBSConnected` でバッチが出ない組み合わせだけを使う
- **準備 PA(R):** 自動の待機。P(R, 配信中, OK) のあと、t=0.1 `LC{LOW}` → `AutoFail#1`、`Approve(1, SetScene("Standby"), None)` → `Proceed{r1}`、`SRD{r1, OK}`、`BD{1, Completed}`、t=0.5 `SC{"Standby"}`。結果: 現在のシーン `"Standby"`、持ち主 `auto`、戻り先 R、受信状態 LOW、`LastAutoSent[AutoFail]=0.1`。次の番号は `#2`、`r2`
- **準備 PM(R, 受信状態):** 手動の待機。P(R, 配信中, OK) のあと、t=0.1 `Command{StandbyOn}` → `StandbyOn#1`、`Approve(1, SetScene("Standby"), None)` → `Proceed{r1}`、`SRD{r1, OK}`、`BD{1, Completed}`、t=0.5 `SC{"Standby"}`、受信状態が OK でなければ t=0.6 `LC{受信状態}`（何も出ない）。結果: 持ち主 `manual`、戻り先 R。次の番号は `#2`、`r2`
- `#n` は `BatchID`、`r1` は `ReqID=1`。「本体」は本体の切替とみなしたこと（`SceneEpoch` は進まない）、「手動」は手動とみなしたこと（`SceneEpoch+1`、期待を全部消す、C）
- 実行役の動き（`Approve`、`SceneRequestDone`、`BatchDone` の順）は、テストが実行役の代わりに呼ぶ。1つのバッチでは `SceneRequestDone` のあとに `BatchDone` を送る（9章）
- `V-xx` を引いた行は、4章の裁定が出るまで期待値が決まらない。「文面」は spec の文面どおりの値、「提案」は 4章の提案の値

### 必須ケースとの対応（spec 11章 switcher の箇条書き）

| spec 11章の項目 | ケース |
|---|---|
| AutoFail は、配信中、`auto_enabled`、Monitored、持ち主 none のときだけ | SQ-01 |
| 戻り先の確定 | SQ-02 |
| AutoRecover の順序、持ち主と戻り先が消える、失敗したら5秒あけて再送 | SQ-03 |
| 手動で Monitored へ（自動の待機中から、Other から、OK なら何も出ない） | SQ-04、SQ-05 |
| 手動で Other / 待機シーンへ | SQ-06 |
| StandbyOn | SQ-07 |
| StandbyOff | SQ-08（e は V-18） |
| 置き換え | SQ-09 |
| 配信開始時の切替 | SQ-10 |
| 切替中の抑止 | SQ-07a、SQ-10a |
| `auto_enabled` を false にする `ScenesConfigured` | SQ-12 |
| `auto_enabled` が false のときの StandbyOff | SQ-08d |
| 配信の停止 | SQ-16a |
| 配信中の `ScenesConfigured` による設定の差し替え | SQ-13 |
| 配信中の `ScenesConfigured` と持ち主 | SQ-14 |
| StreamStart / StreamStop | SQ-17 |
| 再送が5秒以上あく | SQ-32、SQ-03b |
| 各 Command の 409 になる条件 | SQ-17 |
| StandbyOn の受け付け | SQ-18 |
| 期待の設定の世代 | SQ-15 |
| AutoFail の期待だけが残っている間の StandbyOn | SQ-18d |
| AutoRecover の最後の承認のあとの回線の悪化 | SQ-19 |
| 同じ待機シーンへの期待が複数ある | SQ-20（V-02。書かれた例のままでは作れない） |
| 配信を止めて始め直したあとの遅れた `SceneChanged` | SQ-16b、SQ-16c |
| StartScene の再送 | SQ-11 |
| 期待（登録、応答待ち、期限、タイムアウト、エラー、前の期待、一致しない変化、期限切れ） | SQ-21、SQ-20 |
| 有効条件（復帰の待ち時間中、StopStream と StandbyOn） | SQ-23 |
| 承認の `Skip` | SQ-24 |
| 重複の歯止め | SQ-25 |
| 上書きの状態 | SQ-22 |
| 配信の区切り | SQ-26 |
| 配信状態（`RECONNECTING` → `RECONNECTED`） | SQ-27 |
| 再接続 | SQ-28 |
| 配信していない間の手動のシーン切替 | SQ-29 |
| バッチ内の失敗 | SQ-30 |
| （必須ではない。V-06、V-08 の確認用） | SQ-05b、SQ-31 |

### SQ-01 AutoFail の範囲（SW）

それぞれ別の列。どの列も、書いた出力のほかにバッチは出ない。

| # | 準備 | 入力 | 期待する出力 |
|---|---|---|---|
| a | P("Camera", 配信中, OK) | t=1 `LC{LOW}`、t=2 `LC{DOWN}`、t=3 `LC{OK}` | t=1 に `AutoFail#1[SetScene("Standby")]`、戻り先=`"Camera"`。t=2 は未完了があるので出ない。t=3 は Monitored かつ OK で出ない |
| b | P("Talk", 配信中, OK) | t=1 `LC{LOW}`、t=2 `LC{OK}`、t=10 `Tick` | なし（Other） |
| c | P("Standby", 配信中, OK)（待機シーン、持ち主 `none`） | t=1 `LC{LOW}`、t=2 `LC{OK}`、t=10 `Tick` | なし |
| d | P("Camera", 配信中でない, OK) | t=1 `LC{LOW}`、t=10 `Tick` | なし |
| e | P("Camera", 配信中, OK) | t=1 `OBSDisconnected`、t=2 `LC{LOW}`、t=10 `Tick`、t=11 `OBSConnected{true, "Camera", 300}` | t=2、t=10 はなし。t=11 に `AutoFail#1`（OC-3） |
| f | `CFG(auto=false)` で P("Camera", 配信中, OK) | t=1 `LC{LOW}`、t=10 `Tick` | なし |

### SQ-02 戻り先の確定（SW）

準備はどれも P("Camera", 配信中, OK)。

**a. 届いた時点で入る。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}` | `AutoFail#1[SetScene("Standby")]`、戻り先=`"Camera"` | 持ち主 `none`、戻り先なし。`Pending` に #1 |
| 1.0 | `Approve(1, SetScene("Standby"), None)` | `Proceed{r1}` | 期待 [Standby r1（AutoFail、戻り先 Camera）]。持ち主 `none` |
| 1.1 | `SRD{r1, OK}`、`BD{1, Completed}` | — | 期限 = 4.4。持ち主 `none` のまま |
| 1.4 | `SC{"Standby"}` | —（本体。待機シーンで不良） | 持ち主 `auto`、戻り先 `"Camera"`。期待が空、`SceneEpoch` は同じ |

**b. 却下。** t=1.0 `LC{LOW}` → `AutoFail#1`。t=1.5 `LC{OK}` → なし。t=1.6 `Approve(1, SetScene("Standby"), None)` → `Reject`、`BD{1, Rejected}` → 持ち主 `none`、戻り先なし、`LastError` は記録されない。

**c. 失敗。** t=1.0 `LC{LOW}` → `AutoFail#1`、`Approve` → `Proceed{r1}`。t=1.1 `SRD{r1, Error}`、`BD{1, Failed}` → 期待が空、持ち主 `none`、戻り先なし、`LastError` が記録される。

**d. 期限切れ。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}`、`Approve(1, SetScene("Standby"), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Standby r1] |
| 1.7 | `SRD{r1, OK}`、`BD{1, Completed}` | — | 期限 = 5.0 |
| 2.0〜4.0 | `Tick` ×3 | —（待機シーンへの期待が残る） | 期待は残る |
| 5.0 | `Tick` | —（`At == 期限`。残る） | 期待は残る。持ち主 `none` |
| 6.0 | `Tick` | `AutoFail#2`（期待が期限切れで消えた。前回の送出から 5.0秒） | 持ち主 `none`、戻り先なし。`SceneEpoch` は同じ |

### SQ-03 AutoRecover（SW）

**a. 順序と、届いた時点で消えること。** 準備 PA("Camera")。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{OK}` | `AutoRecover#2`。Action は `[RestartMedia{Wait: W}, SetScene{"Camera"}]` の順 | `Pending` に #2。期待はまだない。`LastAutoSent[AutoRecover]=1.0` |
| 1.0 | `Approve(2, RestartMedia, None)` | `Proceed`（`ReqID` 0） | 期待は登録されない |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}` | 期待 [Camera r2（AutoRecover）]。持ち主 `auto` のまま |
| 1.6 | `SRD{r2, OK}`、`BD{2, Completed}` | — | 期限 = 4.9。持ち主 `auto` のまま |
| 1.9 | `SC{"Camera"}` | —（本体。Monitored かつ OK） | 持ち主 `none`、戻り先なし。期待が空、`SceneEpoch` は同じ |

**b. 失敗したら持ち主は `auto` のままで、5秒あけて再送。** 準備 PA("Camera")。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{OK}`、`Approve(2, RestartMedia, None)` | `AutoRecover#2`、`Proceed` | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}` | 期待 [Camera r2] |
| 1.6 | `SRD{r2, Error}`、`BD{2, Failed}` | — | 期待が空。持ち主 `auto`、戻り先 `"Camera"`。`LastError` を記録 |
| 2.0〜5.0 | `Tick` ×4 | —（経過 1〜4秒） | — |
| 6.0 | `Tick` | `AutoRecover#3[RestartMedia{W}, SetScene("Camera")]`（経過 5.0秒ちょうど） | `LastAutoSent[AutoRecover]=6.0` |

### SQ-04 手動で Monitored へ（SW）

**a. 自動の待機中に、不良のまま別の Monitored へ（IRL Link で Camera2 を選んだ）。** 準備 PA("Camera")（LOW）。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `SC{"Camera2"}`（期待なし） | `AutoFail#2[SetScene("Standby")]`、戻り先=`"Camera2"`（イベント起点なので、t=0.1 の送出から5秒たっていなくても出る） | 手動。`SceneEpoch+1`。持ち主 `none`、戻り先なし（前の戻り先 `"Camera"` は消える） |
| 1.0 | `Approve(2, SetScene("Standby"), None)` | `Proceed{r2}` | 期待 [Standby r2（AutoFail、戻り先 Camera2）] |
| 1.1 | `SRD{r2, OK}`、`BD{2, Completed}` | — | — |
| 1.4 | `SC{"Standby"}` | — | 本体。持ち主 `auto`、戻り先 `"Camera2"` |
| 2.0 | `LC{OK}` | `AutoRecover#3[RestartMedia{W}, SetScene("Camera2")]` | — |

**b. Other から、不良のまま Monitored へ（トークのシーンからカメラへ）。** 準備 P("Talk", 配信中, LOW)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `SC{"Camera"}`（期待なし） | `AutoFail#1[SetScene("Standby")]`、戻り先=`"Camera"` | 手動。持ち主 `none` |
| 1.0〜1.1 | `Approve(1, …)` → `Proceed{r1}`、`SRD{r1, OK}`、`BD{1, Completed}` | — | — |
| 1.4 | `SC{"Standby"}` | — | 本体。持ち主 `auto`、戻り先 `"Camera"` |

**c. OK なら何も出ない。** 準備 P("Talk", 配信中, OK)。t=1.0 `SC{"Camera"}` → 手動、出力なし、持ち主 `none`。

**d. 自動の待機中に回復し、AutoRecover の承認の前に人が Monitored を選んだ。** 準備 PA("Camera")。t=1.0 `LC{OK}` → `AutoRecover#2`。t=1.1 `SC{"Camera2"}` → 手動、出力なし（Monitored かつ OK）、持ち主 `none`、戻り先なし。t=1.2 `Approve(2, RestartMedia, None)` → `Reject`（`SceneEpoch`）、`BD{2, Rejected}` → `LastError` は記録されない。現在のシーンは `"Camera2"` のまま。

### SQ-05 回線が悪い間に繰り返し切り替える（SW）

**a. バッチが終わってから切り替える（通常）。** 準備 P("Camera", 配信中, OK)。イベント起点の AutoFail には5秒の条件が掛からない。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}` | `AutoFail#1`、戻り先=`"Camera"` | — |
| 1.0〜1.1 | `Approve(1, …)` → `Proceed{r1}`、`SRD{r1, OK}`、`BD{1, Completed}` | — | — |
| 1.4 | `SC{"Standby"}` | — | 持ち主 `auto`、戻り先 `"Camera"` |
| 3.0 | `SC{"Camera2"}`（人） | `AutoFail#2`、戻り先=`"Camera2"`（#1 の送出から 2.0秒） | 手動。持ち主 `none` |
| 3.0〜3.1 | `Approve(2, …)` → `Proceed{r2}`、`SRD{r2, OK}`、`BD{2, Completed}` | — | — |
| 3.4 | `SC{"Standby"}` | — | 持ち主 `auto`、戻り先 `"Camera2"` |
| 4.0 | `SC{"Camera"}`（人） | `AutoFail#3`、戻り先=`"Camera"`（#2 の送出から 1.0秒） | 手動。持ち主 `none` |
| 4.0〜4.1 | `Approve(3, …)` → `Proceed{r3}`、`SRD{r3, OK}`、`BD{3, Completed}` | — | — |
| 4.4 | `SC{"Standby"}` | — | 持ち主 `auto`、戻り先 `"Camera"` |
| 5.0〜8.0 | `Tick` ×4 | —（持ち主 `auto` で不良） | — |
| 9.0 | `LC{OK}` | `AutoRecover#4[RestartMedia{W}, SetScene("Camera")]`（最後に選んだシーンへ戻る） | — |

**b. AutoFail が実行待ちの間に切り替える（V-06）。** 準備 P("Camera", 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}` | `AutoFail#1`、戻り先=`"Camera"`（実行役が詰まっていて、承認はまだ） | `LastAutoSent[AutoFail]=1.0` |
| 2.0 | `SC{"Camera2"}`（人） | —（AutoFail が未完了。J の 4） | 手動。`SceneEpoch+1` |
| 2.5 | `Approve(1, SetScene("Standby"), None)`、`BD{1, Rejected}` | `Reject`（`SceneEpoch`）。`BatchDone` では何も出ない | `Pending` が空。`LastError` は記録されない |
| 3.0 | `Tick` | V-06。文面: —（#1 の送出から 2.0秒）。提案: `AutoFail#2`、戻り先=`"Camera2"` | V-06 |
| 6.0 | `Tick` | V-06。文面: `AutoFail#2`、戻り先=`"Camera2"`（5.0秒）。提案: —（#2 が t=3.0 に出ている） | V-06 |

### SQ-06 手動で Other / 待機シーンへ（SW）

**a. 自動の待機中に Other へ。** 準備 PA("Camera")。t=1.0 `SC{"Talk"}` → 手動、出力なし、持ち主 `none`、戻り先なし。t=2.0 `LC{OK}`、t=3.0 `LC{LOW}`、t=10.0 `Tick` → どれも出力なし。

**b. 人が手動で待機シーンを選んだ。** 準備 P("Camera", 配信中, OK)。t=1.0 `SC{"Standby"}`（期待なし） → 手動、出力なし、持ち主 `none`。t=2.0 `LC{LOW}`、t=3.0 `LC{OK}`、t=10.0 `Tick` → どれも出力なし、持ち主 `none` のまま（戻さない）。

**c. Other から Other へ、不良のまま。** 準備 P("Talk", 配信中, LOW)。t=1.0 `SC{"Opening"}` → 手動、出力なし。

### SQ-07 StandbyOn（SW）

**a. Monitored から。受け付けた直後に回線が悪化しても AutoFail は出ない（切替中の抑止）。** 準備 P("Camera", 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOn}` | `cmdErr` nil。`StandbyOn#1[SetScene("Standby")]`、戻り先=`"Camera"` | 持ち主 `none` のまま |
| 1.1 | `LC{LOW}` | —（StandbyOn が未完了。抑止） | — |
| 1.2 | `Approve(1, SetScene("Standby"), None)` | `Proceed{r1}`（受信状態は見ない） | 期待 [Standby r1（StandbyOn、戻り先 Camera）] |
| 1.3 | `SRD{r1, OK}`、`BD{1, Completed}` | — | 期限 = 4.6 |
| 2.0 | `Tick` | —（StandbyOn の期待が残る。抑止） | — |
| 2.2 | `SC{"Standby"}` | — | 本体。持ち主 `manual`、戻り先 `"Camera"` |
| 3.0、4.0、10.0 | `LC{OK}`、`LC{DOWN}`、`Tick` | —（持ち主 `manual` の間は何も出ない） | 持ち主 `manual` のまま |

**b. Other から。** 準備 P("Talk", 配信中, LOW)。t=1.0 `Command{StandbyOn}` → `StandbyOn#1[SetScene("Standby")]`、戻り先=`"Talk"`。`Approve` → `Proceed{r1}`、`SRD{r1, OK}`、`BD{1, Completed}`、t=1.4 `SC{"Standby"}` → 持ち主 `manual`、戻り先 `"Talk"`。t=2.0 `LC{OK}` → 出力なし。

### SQ-08 StandbyOff（SW）

**a. 戻り先が Monitored で OK。** 準備 PM("Camera", OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOff}` | `cmdErr` nil。`StandbyOffMonitored#2[RestartMedia{W}, SetScene("Camera")]` | 持ち主 `manual` のまま |
| 1.0 | `Approve(2, RestartMedia, None)` | `Proceed` | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}` | 期待 [Camera r2（StandbyOffMonitored）] |
| 1.6 | `SRD{r2, OK}`、`BD{2, Completed}` | — | 持ち主 `manual` のまま |
| 1.9 | `SC{"Camera"}` | — | 本体。持ち主 `none`、戻り先なし |

**b. 戻り先が Monitored で不良。バッチを出さず持ち主が `auto` になり、回復したら AutoRecover。** 準備 PM("Camera", LOW)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOff}` | `cmdErr` nil。バッチなし | 持ち主 `auto`、戻り先 `"Camera"`、現在のシーン `"Standby"` |
| 2.0 | `Tick` | —（不良） | — |
| 3.0 | `LC{OK}` | `AutoRecover#2[RestartMedia{W}, SetScene("Camera")]` | — |
| 3.0〜3.9 | `Approve(2, RestartMedia)` → `Proceed`、t=3.5 `Approve(2, SetScene("Camera"))` → `Proceed{r2}`、`SRD{r2, OK}`、`BD{2, Completed}`、t=3.9 `SC{"Camera"}` | — | 持ち主 `none`、戻り先なし |

**c. 戻り先が Monitored でない。受信状態によらず `[SetScene(戻り先)]`。** 準備 PM("Talk", LOW)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOff}` | `StandbyOffOther#2[SetScene("Talk")]`（`RestartMedia` なし） | 持ち主 `manual` のまま |
| 1.1 | `LC{DOWN}` | — | — |
| 1.2 | `Approve(2, SetScene("Talk"), None)` | `Proceed{r2}` | — |
| 1.3 | `SRD{r2, OK}`、`BD{2, Completed}` | — | — |
| 1.6 | `SC{"Talk"}` | — | 本体。持ち主 `none`、戻り先なし |

準備が PM("Talk", OK) でも、t=1.0 の出力は同じ `StandbyOffOther#2[SetScene("Talk")]`。

**d. `auto_enabled` が false。受信状態を見ない。** 設定 `CFG(auto=false)`。準備 P("Camera", 配信中, LOW)（`auto_enabled` が false なので AutoFail は出ない）のあと、PM と同じ手順で StandbyOn を通す（持ち主 `manual`、戻り先 `"Camera"`、LOW のまま）。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOff}` | `StandbyOffMonitored#2[RestartMedia{W}, SetScene("Camera")]`（不良でも出る） | 持ち主 `manual` のまま（`auto` にならない） |
| 1.0 | `Approve(2, RestartMedia, None)` | `Proceed` | — |
| 1.2、1.3、1.4 | `LC{DOWN}`、`LC{OK}`、`LC{LOW}` | — | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}`（`Replace` にも `Reject` にもならない） | 持ち主 `manual` のまま |
| 1.6 | `SRD{r2, OK}`、`BD{2, Completed}` | — | — |
| 1.9 | `SC{"Camera"}` | —（`auto_enabled` が false なので AutoFail は出ない） | 持ち主 `none`、戻り先なし。現在のシーン `"Camera"`、LOW |

同じ設定で戻り先が `"Talk"` なら、t=1.0 の出力は `StandbyOffOther#2[SetScene("Talk")]`。

**e. 戻す切替の期待が期限切れになった（V-18）。** 準備 PM("Camera", OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOff}`、`Approve(2, RestartMedia, None)` | `StandbyOffMonitored#2`、`Proceed` | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}` | 期待 [Camera r2] |
| 1.7 | `SRD{r2, OK}`、`BD{2, Completed}` | — | 期限 = 5.0。`LastError` はまだない |
| 5.0 | `Tick` | — | 期待は残る（`At == 期限`） |
| 6.0 | `Tick` | — | 期待が消える。持ち主 `manual`、戻り先 `"Camera"` のまま。`SceneEpoch` は同じ。`LastError` は V-18（提案: 記録する） |
| 6.5 | `Command{StandbyOff}` | nil、`StandbyOffMonitored#3[RestartMedia{W}, SetScene("Camera")]`（もう一度受け付ける） | — |

### SQ-09 置き換え（SW）

**a. `RestartMedia` の待ち時間中に悪化。** 準備 PM("Camera", OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOff}` | `StandbyOffMonitored#2[RestartMedia{W}, SetScene("Camera")]` | — |
| 1.0 | `Approve(2, RestartMedia, None)` | `Proceed` | — |
| 1.2 | `LC{LOW}` | —（持ち主 `manual`。抑止中でもある） | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Replace{nil}` | 持ち主 `auto`、戻り先 `"Camera"`、現在のシーン `"Standby"`。期待は登録されない。`NextBatchID` は進まない |
| 1.5 | `BD{2, Rejected}` | — | `LastError` は記録されない。`Pending` が空 |
| 2.0 | `Tick` | —（不良） | — |
| 3.0 | `LC{OK}` | `AutoRecover#3[RestartMedia{W}, SetScene("Camera")]` | — |

**b. `RestartMedia` の承認の前に悪化。** t=1.0 `Command{StandbyOff}` → #2。t=1.1 `LC{DOWN}`。t=1.2 `Approve(2, RestartMedia, None)` → `Replace{nil}`、持ち主 `auto`。`BD{2, Rejected}` → `LastError` は記録されない。

**c. 置き換えのあと、`BatchDone` より先に回復した。** a の t=1.5 の `Approve` まで同じ。t=1.6 `LC{OK}` → 出力なし（StandbyOffMonitored #2 が未完了なので抑止）。t=1.7 `BD{2, Rejected}` → 出力なし。t=2.0 `Tick` → `AutoRecover#3`（AutoRecover は一度も出していないので、5秒の条件を満たす）。

### SQ-10 配信開始時の切替（SW）

**a. 受信状態が不良でも StartScene が先。その間 Auto は出ない。** 設定 `CFG(start="Opening")`。準備 P("Camera", 配信中でない, LOW)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `SSC{true}` | `StartScene#1[SetScene("Opening")]`（Monitored で LOW だが AutoFail ではない） | `StreamEpoch+1` |
| 1.1 | `LC{DOWN}` | —（StartScene が未完了。抑止） | — |
| 1.3 | `Approve(1, SetScene("Opening"), None)` | `Proceed{r1}`（受信状態は見ない） | 期待 [Opening r1（StartScene）] |
| 1.4 | `SRD{r1, OK}`、`BD{1, Completed}` | — | 期限 = 4.7 |
| 2.0 | `Tick` | —（StartScene の期待が残る。抑止） | — |
| 2.2 | `SC{"Opening"}` | —（本体。Other） | 持ち主 `none`。期待が空 |

**b. 開始時のシーンが Monitored で不良なら、届いたあとに AutoFail。** 設定 `CFG(start="Camera")`。準備 P("Talk", 配信中でない, LOW)。応答と `SceneChanged` の順序の両方を確かめる。

b1（`BatchDone` が先）:

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `SSC{true}`、`Approve(1, SetScene("Camera"), None)` | `StartScene#1[SetScene("Camera")]`、`Proceed{r1}` | — |
| 1.1 | `SRD{r1, OK}`、`BD{1, Completed}` | — | — |
| 1.4 | `SC{"Camera"}` | `AutoFail#2[SetScene("Standby")]`、戻り先=`"Camera"`（抑止が解けた） | 本体。持ち主 `none` |
| 1.4〜1.8 | `Approve(2, …)` → `Proceed{r2}`、`SRD{r2, OK}`、`BD{2, Completed}`、t=1.8 `SC{"Standby"}` | — | 持ち主 `auto`、戻り先 `"Camera"` |

b2（`SceneChanged` が先）:

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `SSC{true}`、`Approve(1, SetScene("Camera"), None)` | `StartScene#1`、`Proceed{r1}` | — |
| 1.3 | `SC{"Camera"}` | —（期待は消えたが #1 が未完了。抑止） | 本体 |
| 1.4 | `SRD{r1, OK}`、`BD{1, Completed}` | —（r1 の期待はもうない。`BatchDone` では判定しない） | `Pending` が空 |
| 2.0 | `Tick` | `AutoFail#2`、戻り先=`"Camera"`（一度も出していないので5秒の条件を満たす） | — |

**c. 無効なら StartScene は出ず、通常の判定だけ。** 設定 CFG。準備 P("Camera", 配信中でない, LOW)。t=1.0 `SSC{true}` → `AutoFail#1[SetScene("Standby")]`、戻り先=`"Camera"`。別の列: 準備 P("Talk", 配信中でない, LOW)。t=1.0 `SSC{true}` → 出力なし。

**d. 切替の前に人がシーンを切り替えた。** 設定 `CFG(start="Opening")`。準備 P("Camera", 配信中でない, OK)。t=1.0 `SSC{true}` → `StartScene#1`。t=1.1 `SC{"Talk"}` → 手動、`SceneEpoch+1`、出力なし。t=1.2 `Approve(1, SetScene("Opening"), None)` → `Reject`、`BD{1, Rejected}` → `LastError` は記録されない。t=2.0〜20.0 `Tick` → StartScene は出ない。

**e. StartScene が未完了の間は、Auto の承認も却下される。** 設定 `CFG(start="Opening")`。準備 P("Camera", 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}` | `AutoFail#1`（実行役が詰まっていて、承認はまだ） | — |
| 1.5 | `SSC{false}` | — | `StreamEpoch+1`。`LastAutoSent` が空 |
| 2.0 | `SSC{true}` | `StartScene#2[SetScene("Opening")]` | `StreamEpoch+1` |
| 2.1 | `Approve(1, SetScene("Standby"), None)`、`BD{1, Rejected}` | `Reject`（`StreamEpoch` が違う。抑止でも却下になる。AP-AUTOFAIL-5） | `LastError` は記録されない |
| 2.2 | `Approve(2, SetScene("Opening"), None)` | `Proceed{r1}` | — |
| 2.3 | `SRD{r1, OK}`、`BD{2, Completed}` | — | — |
| 2.6 | `SC{"Opening"}` | —（Other） | 本体 |

### SQ-11 StartScene は再送しない（SW）

設定 `CFG(start="Opening")`。準備はどれも P("Camera", 配信中でない, OK)。t=1.0 `SSC{true}` → `StartScene#1[SetScene("Opening")]`。

**a. 失敗。** `Approve(1, …)` → `Proceed{r1}`。t=1.1 `SRD{r1, Error}`、`BD{1, Failed}` → 期待が空、`LastError` を記録。t=2.0〜20.0 `Tick` → StartScene は出ない（ほかのバッチも出ない）。

**b. 却下（手動の切替以外）。** t=1.1 `SSC{false}`。t=1.2 `Approve(1, SetScene("Opening"), None)` → `Reject`、`BD{1, Rejected}` → `LastError` を記録。別の列: t=1.1 `ScenesConfigured{CFG(start="Opening", Monitored=["Camera","Camera2","Camera3"])}` → `ConfigEpoch+1`。t=1.2 `Approve(1, …)` → `Reject`、`BD{1, Rejected}` → `LastError` を記録。t=2.0〜20.0 `Tick` → StartScene は出ない。

**c. 期限切れ。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Approve(1, SetScene("Opening"), None)` | `Proceed{r1}` | 期待 [Opening r1] |
| 1.7 | `SRD{r1, OK}`、`BD{1, Completed}` | — | 期限 = 5.0。`LastError` はまだない |
| 5.0 | `Tick` | — | 期待は残る（`At == 期限`） |
| 6.0 | `Tick` | — | 期待が消える。`LastError` を記録。`SceneEpoch` は同じ |
| 7.0〜20.0 | `Tick` | —（StartScene は出ない） | — |

**d. 人が先に切り替えたための却下は記録されない。** SQ-10d。

### SQ-12 `auto_enabled` を false にする ScenesConfigured（SW）

**a. 実行待ちの Auto は却下され、以後出ない。StandbyOn は受け付ける。** 準備 P("Camera", 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}` | `AutoFail#1`（承認はまだ） | — |
| 1.1 | `ScenesConfigured{CFG(auto=false)}` | — | `ConfigEpoch+1`。`Pending` に #1 が残る |
| 1.2 | `Approve(1, SetScene("Standby"), None)`、`BD{1, Rejected}` | `Reject` | `LastError` は記録されない |
| 2.0〜10.0 | `Tick` ×9、t=5.5 `LC{DOWN}` | — | — |
| 11.0 | `Command{StandbyOn}` | `cmdErr` nil。`StandbyOn#2[SetScene("Standby")]`、戻り先=`"Camera"` | — |

**b. 持ち主が `auto` なら `none` になる。true に戻しても戻らない。** 準備 PA("Camera")。t=1.0 `ScenesConfigured{CFG(auto=false)}` → 出力なし、持ち主 `none`、戻り先なし、現在のシーンは `"Standby"` のまま。t=2.0 `LC{OK}` → 出力なし。t=3.0 `ScenesConfigured{CFG}` → 出力なし（待機シーンで持ち主 `none`）、`ConfigEpoch+1`。

**c. true に戻すと判定が再開する。** 設定 `CFG(auto=false)`。準備 P("Camera", 配信中, LOW)。t=1.0 `Tick` → 出力なし。t=2.0 `ScenesConfigured{CFG}` → `AutoFail#1[SetScene("Standby")]`、戻り先=`"Camera"`。

### SQ-13 配信中の ScenesConfigured による分類の差し替え（SW）

**a. Other だった現在のシーンを監視対象に足す。** 準備 P("Talk", 配信中, LOW)。t=1.0 `Tick` → 出力なし。t=2.0 `ScenesConfigured{CFG(Monitored=["Camera","Camera2","Talk"])}` → `AutoFail#1[SetScene("Standby")]`、戻り先=`"Talk"`。

**b. 監視対象から外す。** 準備 P("Camera", 配信中, OK)。t=1.0 `ScenesConfigured{CFG(Monitored=["Camera2"])}` → 出力なし。t=2.0 `LC{LOW}`、t=10.0 `Tick` → 出力なし（`"Camera"` は Other）。

### SQ-14 配信中の ScenesConfigured と持ち主（SW）

それぞれ別の列。f を除くどの列も `ConfigEpoch+1`。現在のシーンはどの列も `"Standby"` のまま。

| # | 準備 | 入力 | 期待する出力と状態 |
|---|---|---|---|
| a | PA("Camera") | t=1.0 `ScenesConfigured{CFG(Standby="Standby2")}`、t=2.0 `LC{OK}` | t=1.0: 出力なし、持ち主 `none`、戻り先なし（`"Standby"` は Other になる）。t=2.0: 出力なし |
| b | PM("Camera", OK) | t=1.0 `ScenesConfigured{CFG(Standby="Standby2")}`、t=2.0 `Command{StandbyOff}` | t=1.0: 持ち主 `none`、戻り先なし。t=2.0: `ErrInvalidState` |
| c | PA("Camera") | t=1.0 `ScenesConfigured{CFG(Monitored=["Camera2"])}`、t=2.0 `LC{OK}` | t=1.0: 持ち主 `none`、戻り先なし。t=2.0: 出力なし |
| d | PM("Camera", LOW) | t=1.0 `ScenesConfigured{CFG(Monitored=["Camera2"])}`、t=2.0 `Command{StandbyOff}` | t=1.0: 持ち主 `manual`、戻り先 `"Camera"` を保つ。t=2.0: `StandbyOffOther#2[SetScene("Camera")]`（LOW でも出る。`RestartMedia` なし） |
| e | PA("Camera") | t=1.0 `ScenesConfigured{CFG(Monitored=["Camera","Camera2","Camera3"])}`、t=2.0 `LC{OK}` | t=1.0: 持ち主 `auto`、戻り先 `"Camera"` を保つ。t=2.0: `AutoRecover#2[RestartMedia{W}, SetScene("Camera")]` |
| f | PA("Camera") | t=1.0 `ScenesConfigured{CFG}`（今と同じ内容） | 何も変わらない。`ConfigEpoch` も進まない |

**g. 未完了のシーン切替のバッチは却下され、StopStream は却下されない。** 準備 P("Camera", 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOn}` | `StandbyOn#1`、戻り先=`"Camera"` | — |
| 1.1 | `Command{StreamStop}` | `StopStream#2[StopStream]` | — |
| 1.2 | `ScenesConfigured{CFG(Monitored=["Camera","Camera2","Camera3"])}` | —（抑止中） | `ConfigEpoch+1`。`Pending` に #1、#2 |
| 1.3 | `Approve(1, SetScene("Standby"), None)`、`BD{1, Rejected}` | `Reject` | `LastError` を記録（Command 由来） |
| 1.4 | `Approve(2, StopStream, None)` | `Proceed` | — |

**h. ほかの Purpose の却下。** 同じ変更（`"Camera3"` の追加）を、バッチを作ったあと、承認の前に入れる。

| # | 準備と入力 | 期待する出力と状態 |
|---|---|---|
| h1 | P("Camera", 配信中, OK)。t=1.0 `LC{LOW}` → `AutoFail#1`。t=1.1 `ScenesConfigured`。t=1.2 `Approve(1, SetScene("Standby"), None)`、`BD{1, Rejected}`。t=2.0〜5.0 `Tick`、t=6.0 `Tick` | t=1.1: 出力なし（AutoFail が未完了）。t=1.2: `Reject`、`LastError` は記録されない。t=2.0〜5.0: 出力なし（送出から5秒未満）。t=6.0: `AutoFail#2`、戻り先=`"Camera"` |
| h2 | PA("Camera")。t=1.0 `LC{OK}` → `AutoRecover#2`。t=1.1 `ScenesConfigured`。t=1.2 `Approve(2, RestartMedia, None)`、`BD{2, Rejected}`。t=6.0 `Tick` | t=1.1: 出力なし、持ち主 `auto` を保つ。t=1.2: `Reject`、`LastError` は記録されない。t=6.0: `AutoRecover#3` |
| h3 | PM("Camera", OK)。t=1.0 `Command{StandbyOff}` → `StandbyOffMonitored#2`。t=1.1 `ScenesConfigured`。t=1.2 `Approve(2, RestartMedia, None)`、`BD{2, Rejected}`。t=2.0 `Command{StandbyOff}` | t=1.2: `Reject`、`LastError` を記録、持ち主 `manual` を保つ。t=2.0: `StandbyOffMonitored#3`（もう一度受け付ける） |
| h4 | StartScene: SQ-11b の2つ目の列 | `Reject`、`LastError` を記録 |
| h5 | P("Camera", 配信中でない, OK)。t=1.0 `Command{StreamStart}` → `StartStream#1`。t=1.1 `ScenesConfigured`。t=1.2 `Approve(1, StartStream, None)` | t=1.2: `Proceed` |

### SQ-15 期待の設定の世代（SW）

a〜d は、準備 P("Camera", 配信中, OK)、t=1.0 `LC{LOW}` → `AutoFail#1`、`Approve(1, SetScene("Standby"), None)` → `Proceed{r1}`、t=1.1 `SRD{r1, OK}`、`BD{1, Completed}`（期限 4.4）まで同じ。そのあと、`SceneChanged` が届く前に設定を変える。

**a. 待機シーンの名前を変える。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.2 | `ScenesConfigured{CFG(Standby="Standby2")}` | `AutoFail#2[SetScene("Standby2")]`、戻り先=`"Camera"`（新しい待機シーンへの期待はない。SCF-19、V-17） | `ConfigEpoch+1`。期待 [Standby r1] は残る。持ち主 `none` |
| 1.4 | `SC{"Standby"}` | —（`"Standby"` は Other になっている） | 本体（`SceneEpoch` は同じ）。持ち主 `none`、戻り先なしのまま。期待が空 |
| 1.5 | `Approve(2, SetScene("Standby2"), None)`、`BD{2, Rejected}` | `Reject`（現在のシーンが Monitored でない。AP-AUTOFAIL-4） | `LastError` は記録されない |
| 2.0 | `LC{OK}` | — | 現在のシーン `"Standby"`、持ち主 `none` |

**b. `auto_enabled` を false にする。** t=1.2 `ScenesConfigured{CFG(auto=false)}` → 出力なし、期待は残る。t=1.4 `SC{"Standby"}` → 本体（`SceneEpoch` は同じ）、持ち主 `none`、戻り先なしのまま。t=2.0 `LC{OK}` → 出力なし。

**c. 戻り先を監視対象から外す。** t=1.2 `ScenesConfigured{CFG(Monitored=["Camera2"])}` → 出力なし（`"Camera"` は Other）。t=1.4 `SC{"Standby"}` → 本体、持ち主 `none`、戻り先なしのまま。

**d. 消す条件に当たらない変更（V-07）。** t=1.2 `ScenesConfigured{CFG(Monitored=["Camera","Camera2","Camera3"])}` → 出力なし（今の待機シーンへの期待が残る。SCF-20）。t=1.4 `SC{"Standby"}` → 本体。持ち主と戻り先は V-07（文面と提案: `none`、なし。待機シーンに持ち主 `none` で残る）。t=2.0 `LC{OK}` → V-07（文面と提案: 出力なし。自動では戻らない）。

**e. 戻す切替の期待が古い（V-01）。** 準備 PA("Camera")。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{OK}`、`Approve(2, RestartMedia, None)` | `AutoRecover#2`、`Proceed` | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}` | 期待 [Camera r2（AutoRecover）] |
| 1.6 | `SRD{r2, OK}`、`BD{2, Completed}` | — | — |
| 1.7 | `ScenesConfigured{CFG(Monitored=["Camera","Camera2","Camera3"])}` | —（戻り先への期待が残る） | `ConfigEpoch+1`。持ち主 `auto`、戻り先 `"Camera"` を保つ |
| 1.9 | `SC{"Camera"}` | — | 本体。V-01。文面: 持ち主 `auto`、戻り先 `"Camera"` のまま（現在のシーンは `"Camera"`）。提案: 持ち主 `none`、戻り先なし |
| 3.0 | `LC{LOW}` | V-01。文面: —（持ち主が `none` でないので AutoFail が出ない）。提案: `AutoFail#3`、戻り先=`"Camera"` | V-01 |

### SQ-16 配信の停止と、始め直したあとの遅れた SceneChanged（SW）

**a. 停止で持ち主、戻り先、期待、再送待ちが消える。** 準備 PA("Camera")。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{OK}`、`Approve(2, RestartMedia, None)` | `AutoRecover#2`、`Proceed` | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}` | 期待 [Camera r2] |
| 1.6 | `SSC{false}` | — | `Streaming=false`、`StreamEpoch+1`。持ち主 `none`、戻り先なし。期待が空。`LastAutoSent` が空。`Pending` に #2 は残る |
| 1.7 | `SRD{r2, OK}`、`BD{2, Completed}` | — | 応答は無視。`Pending` が空 |
| 2.0 | `Tick` | — | — |

**b. 前の配信の遅れた `SceneChanged`（監視対象）。** a の t=1.6 まで同じ。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.7 | `SRD{r2, OK}`、`BD{2, Completed}` | — | — |
| 1.8 | `LC{LOW}` | —（配信中でない） | — |
| 1.9 | `SSC{true}` | —（開始時の切替は無効。待機シーンで持ち主 `none`） | `StreamEpoch+1` |
| 2.0 | `SC{"Camera"}`（前の配信で送った切替が遅れて届いた） | `AutoFail#3[SetScene("Standby")]`、戻り先=`"Camera"` | 手動（期待は停止で消えている）。`SceneEpoch+1`。持ち主 `none` |

**c. 前の配信の遅れた `SceneChanged`（待機シーン）。** 準備 P("Camera", 配信中, OK)。t=1.0 `LC{LOW}` → `AutoFail#1`、`Approve` → `Proceed{r1}`。t=1.1 `SRD{r1, OK}`、`BD{1, Completed}`。t=1.2 `SSC{false}`。t=1.3 `LC{OK}`。t=1.4 `SSC{true}` → 出力なし。t=1.5 `SC{"Standby"}` → 手動、出力なし、持ち主 `none`（自動では戻らない）。

### SQ-17 StreamStart / StreamStop と、各 Command の 409（SW）

1本の列。`ErrInvalidState` は 409、`ErrNotConnected` は 503 に当たる。

| t | 入力 | 期待する `cmdErr` / 出力 | 理由 |
|---|---|---|---|
| 0.0 | `ScenesConfigured{CFG}`、4つの Command を順に（接続の前） | すべて `ErrNotConnected` | 未接続 |
| 0.5 | `LC{OK}`、`OBSConnected{false, "Camera", 300}` | — | — |
| 1.0 | `StreamStop`、`StandbyOn`、`StandbyOff` | すべて `ErrInvalidState` | 配信中でない |
| 2.0 | `StreamStart` | nil、`StartStream#1[StartStream]`（`SetScene` は出ない） | CMD-START-3 |
| 2.1 | `StreamStart` | `ErrInvalidState` | 同じ操作が実行待ち |
| 2.2〜2.4 | `Approve(1, StartStream, None)` → `Proceed`、t=2.3 `SSC{true}` → 出力なし、t=2.4 `BD{1, Completed}` | — | 配信中、Monitored、OK |
| 3.0 | `StreamStart` | `ErrInvalidState` | 配信中 |
| 3.1 | `StandbyOff` | `ErrInvalidState` | 持ち主が `manual` でない（`none`） |
| 3.2 | `StandbyOn` | nil、`StandbyOn#2[SetScene("Standby")]`、戻り先=`"Camera"` | CMD-SON-9 |
| 3.3 | `StandbyOn` | `ErrInvalidState` | 同じ操作が実行待ち |
| 3.4 | `StandbyOff` | `ErrInvalidState` | 持ち主はまだ `none` |
| 3.5〜3.9 | `Approve(2, SetScene("Standby"))` → `Proceed{r1}`、`SRD{r1, OK}`、`BD{2, Completed}`、t=3.9 `SC{"Standby"}` | — | 持ち主 `manual`、戻り先 `"Camera"` |
| 4.0 | `StandbyOn` | `ErrInvalidState` | すでに待機シーン |
| 4.1 | `StandbyOff` | nil、`StandbyOffMonitored#3[RestartMedia{W}, SetScene("Camera")]` | CMD-SOFF-5 |
| 4.2 | `StandbyOff` | `ErrInvalidState` | 同じ操作が実行待ち |
| 4.3〜5.2 | `Approve(3, RestartMedia)` → `Proceed`、t=4.8 `Approve(3, SetScene("Camera"))` → `Proceed{r2}`、`SRD{r2, OK}`、`BD{3, Completed}`、t=5.2 `SC{"Camera"}` | — | 持ち主 `none` |
| 5.5 | `StandbyOff` | `ErrInvalidState` | 持ち主が `manual` でない |
| 6.0 | `StreamStop` | nil、`StopStream#4[StopStream]`（`SetScene` は出ない） | CMD-STOP-3 |
| 6.1 | `StreamStop` | `ErrInvalidState` | 同じ操作が実行待ち |
| 6.2〜6.4 | `Approve(4, StopStream, None)` → `Proceed`、t=6.3 `SSC{false}`、t=6.4 `BD{4, Completed}` | — | 配信中でない |
| 7.0 | `OBSDisconnected`、4つの Command を順に | すべて `ErrNotConnected` | 未接続 |

別の列（持ち主が `auto` のときの StandbyOff）: 準備 PA("Camera")。`Command{StandbyOff}` → `ErrInvalidState`。

### SQ-18 StandbyOn の受け付け（SW）

| # | 準備と入力 | 期待 |
|---|---|---|
| a | PA("Camera")（待機シーン、持ち主 `auto`）。`Command{StandbyOn}` | `ErrInvalidState`（すでに待機シーン） |
| b | P("Camera", 配信中, OK)。t=1.0 `SC{"Standby"}`（人が手動で選んだ。持ち主 `none`）。t=1.1 `Command{StandbyOn}` | `ErrInvalidState`（すでに待機シーン） |
| c | P("Camera", 配信中, OK)。t=1.0 `LC{LOW}` → `AutoFail#1`（未完了）。t=1.1 `Command{StandbyOn}` | `ErrInvalidState`（切替の実行中） |

**d. AutoFail の期待だけが残っている間（タイムアウトして `BatchDone` を受けたあと）。** 準備 P("Camera", 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}`、`Approve(1, SetScene("Standby"), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Standby r1] |
| 6.0 | `SRD{r1, Timeout}`、`BD{1, Failed}` | — | 期限 = 9.3。`Pending` が空。`LastError` を記録 |
| 7.0 | `Command{StandbyOn}` | `ErrInvalidState`（期待が残る） | — |
| 8.0 | `LC{OK}` | — | — |
| 9.3 | `Command{StandbyOn}` | `ErrInvalidState`（`At == 期限`。期待は残る） | — |
| 9.4 | `Command{StandbyOn}` | nil、`StandbyOn#2`、戻り先=`"Camera"`（入口で期待が期限切れになった） | 期待が空。`SceneEpoch` は同じ |

**e. AutoRecover が未完了。** 準備 PA("Camera")。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{OK}`、`Approve(2, RestartMedia, None)` | `AutoRecover#2`、`Proceed` | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}` | — |
| 1.6 | `SC{"Camera"}`（応答より先に届いた） | — | 本体。持ち主 `none`。現在のシーン `"Camera"` |
| 1.7 | `Command{StandbyOn}` | `ErrInvalidState`（AutoRecover #2 が未完了） | — |
| 1.8 | `SRD{r2, OK}`、`BD{2, Completed}` | — | `Pending` が空 |
| 1.9 | `Command{StandbyOn}` | nil、`StandbyOn#3`、戻り先=`"Camera"` | — |

**f. StartScene が未完了、または期待が残る。** 設定 `CFG(start="Opening")`。準備 P("Camera", 配信中でない, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `SSC{true}` | `StartScene#1[SetScene("Opening")]` | — |
| 1.1 | `Command{StandbyOn}` | `ErrInvalidState`（StartScene が未完了） | — |
| 1.2 | `Approve(1, SetScene("Opening"), None)` | `Proceed{r1}` | — |
| 1.3 | `SRD{r1, OK}`、`BD{1, Completed}` | — | — |
| 1.4 | `Command{StandbyOn}` | `ErrInvalidState`（期待が残る） | — |
| 1.6 | `SC{"Opening"}` | — | 本体 |
| 1.7 | `Command{StandbyOn}` | nil、`StandbyOn#2`、戻り先=`"Opening"` | — |

**g. 受け付けたあと、承認の前に待機シーンへ切り替わっていた。** 準備 P("Camera", 配信中, OK)。t=1.0 `Command{StandbyOn}` → `StandbyOn#1`。t=1.1 `SC{"Standby"}`（期待なし。人が選んだ） → 手動、`SceneEpoch+1`、持ち主 `none`。t=1.2 `Approve(1, SetScene("Standby"), None)` → `Reject`（`Skip` ではない）。`BD{1, Rejected}` → `LastError` を記録。持ち主 `none` のまま。この列の却下は `SceneEpoch`（AP-COM-5）による。有効条件（AP-SON-2）での却下に届く列はない（V-13）。

### SQ-19 AutoRecover が届く間に回線がまた落ちた（SW）

準備 PA("Camera")。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{OK}`、`Approve(2, RestartMedia, None)` | `AutoRecover#2`、`Proceed` | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}`（最後の承認） | 期待 [Camera r2] |
| 1.6 | `LC{DOWN}` | —（待機シーン、持ち主 `auto`、不良） | — |
| 1.7 | `SRD{r2, OK}`、`BD{2, Completed}` | — | — |
| 1.9 | `SC{"Camera"}` | `AutoFail#3[SetScene("Standby")]`、戻り先=`"Camera"`（イベント起点。5秒の条件は掛からない） | 本体。持ち主 `none`、戻り先なし |
| 1.9 | `Approve(3, SetScene("Standby"), None)` | `Proceed{r3}` | — |
| 2.0 | `SRD{r3, OK}`、`BD{3, Completed}` | — | — |
| 2.3 | `SC{"Standby"}` | — | 本体。持ち主 `auto`、戻り先 `"Camera"` |

t=1.9 の `SC{"Camera"}` が t=1.7 の `BD{2, Completed}` より先に届いても、t=1.9 の出力は同じ（AutoRecover は抑止の Purpose ではなく、AutoFail とは Purpose が違う）。

### SQ-20 期待が複数あるとき（SW）

a と b は、準備 P("Camera", 配信中, OK)、t=1.0 `LC{LOW}` → `AutoFail#1`、`Approve` → `Proceed{r1}`、t=1.1 `SRD{r1, OK}`、`BD{1, Completed}`、t=1.2 `ScenesConfigured{CFG(Standby="Standby2")}` → `AutoFail#2[SetScene("Standby2")]`、t=1.2 `Approve(2, SetScene("Standby2"), None)` → `Proceed{r2}`、t=1.3 `SRD{r2, OK}`、`BD{2, Completed}` まで同じ。この時点で期待は [Standby r1（古い世代）, Standby2 r2]（2本目の AutoFail が出ることは V-17）。

**a. 後ろの期待に一致すると、前の期待も消える（前の期待は作用しない）。** t=1.6 `SC{"Standby2"}` → 本体、出力なし。期待が空。持ち主 `auto`、戻り先 `"Camera"`（r2 による）。`SceneEpoch` は同じ。

**b. 先頭の期待に一致すると、後ろは残る。** t=1.6 `SC{"Standby"}` → 本体、出力なし（`"Standby"` は Other）。期待 [Standby2 r2] が残る。持ち主 `none`（r1 は古い世代なので入れない）。t=1.9 `SC{"Standby2"}` → 本体。持ち主 `auto`、戻り先 `"Camera"`。

**c. 同じシーンへの期待が2つ（V-02）。** 準備 PM("Talk", OK)。11章の例（AutoFail の再送）は列では作れないので、StandbyOff の押し直しで作る。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOff}`、`Approve(2, SetScene("Talk"), None)` | `StandbyOffOther#2`、`Proceed{r2}` | 期待 [Talk r2] |
| 6.0 | `SRD{r2, Timeout}`、`BD{2, Failed}` | — | 期限 = 9.3。`LastError` を記録。持ち主 `manual` のまま |
| 6.5 | `Command{StandbyOff}`、`Approve(3, SetScene("Talk"), None)` | `StandbyOffOther#3`、`Proceed{r3}` | 期待 [Talk r2, Talk r3] |
| 6.6 | `SRD{r3, OK}`、`BD{3, Completed}` | — | r3 の期限 = 9.9 |
| 7.0 | `SC{"Talk"}`（1つだけ届く） | — | 本体。持ち主 `none`、戻り先なし（先頭の r2 による）。r3 は V-02。6章の文面: 残る。11章と提案: 消える |
| 7.1 | `Command{StandbyOn}` | V-02。6章の文面: `ErrInvalidState`（期待が残る）。提案: nil、`StandbyOn#4`、戻り先=`"Talk"` | V-02 |

### SQ-21 期待の寿命（SW）

準備はどれも P("Camera", 配信中, OK)。

**a. 長いトランジション。期限は承認の時点の長さで決まる。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 0.9 | `TransitionChanged{5000}` | — | `TransitionMs=5000` |
| 1.0 | `LC{LOW}`、`Approve(1, SetScene("Standby"), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Standby r1]（全体の長さ 5000 を写す） |
| 1.0 | `SRD{r1, OK}`、`BD{1, Completed}` | — | 期限 = 1.0 + 5 + 3 = 9.0 |
| 1.2 | `TransitionChanged{300}` | — | 期待の期限は 9.0 のまま |
| 2.0〜8.0 | `Tick` ×7 | — | 期待は残る |
| 9.0 | `Tick` | —（`At == 期限`） | 期待は残る |
| 10.0 | `Tick` | `AutoFail#2`（前回の送出から 9.0秒） | 期待が期限切れで消える。`SceneEpoch` は同じ。持ち主 `none` |

別の列: t=8.5 に `SC{"Standby"}` が届く → 本体（手動とみなさない）。持ち主 `auto`、戻り先 `"Camera"`。

**b. 応答を待つ間は期限切れにならない。** t=1.0 `LC{LOW}` → `AutoFail#1`、`Approve` → `Proceed{r1}`。`SceneRequestDone` も `BatchDone` も送らずに t=2.0〜60.0 の `Tick` を流す → 期待は残り、バッチは出ない。t=60.5 `SC{"Standby"}` → 本体。持ち主 `auto`、戻り先 `"Camera"`。

**c. 応答が5秒でタイムアウトしたあとに届いた本体の `SceneChanged`。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}`、`Approve(1, SetScene("Standby"), None)` | `AutoFail#1`、`Proceed{r1}` | — |
| 6.0 | `SRD{r1, Timeout}`、`BD{1, Failed}` | — | 期待は残る。期限 = 9.3。`LastError` を記録 |
| 7.0 | `Tick` | —（待機シーンへの期待が残る） | — |
| 8.0 | `SC{"Standby"}` | — | 本体（`SceneEpoch` は同じ）。持ち主 `auto`、戻り先 `"Camera"` |

**d. エラーの応答ではすぐ消える。** SQ-02c。続けて t=1.2 `SC{"Standby"}` が届いたら手動（持ち主 `none`）。

**e. 一致しない `SceneChanged` で全部消える。** t=1.0 `LC{LOW}` → `AutoFail#1`、`Approve` → `Proceed{r1}`。t=1.1 `SRD{r1, OK}`、`BD{1, Completed}`。t=1.2 `SC{"Talk"}` → 手動、`SceneEpoch+1`、期待が空、出力なし（Other）。

**f. 期限切れは手動として扱わない。** SQ-02d の続き。t=6.0 に出た `AutoFail#2` は、t=6.1 `Approve(2, SetScene("Standby"), None)` → `Proceed{r2}`（`SceneEpoch` が進んでいないので却下されない）。期限切れのあとに t=6.2 `SC{"Standby"}` が届いた場合は、r2 の期待に一致するので本体（持ち主 `auto`、戻り先 `"Camera"`）。`Approve(2)` の前に届いた場合は手動（SCM-13。持ち主 `none`、#2 は `Reject`）。

### SQ-22 上書きの状態と期限（SW）

準備 P("Camera", 配信中, OK)。ただし `OBSConnected` の `TransitionMs` を表の `G` にする。t=1.0 `LC{LOW}` → `AutoFail#1`、t=1.0 `Approve(1, SetScene("Standby"), override)` → `Proceed{r1}`、t=2.0 `SRD{r1, OK}`。期限は、その時刻の `Tick` で期待が残り、0.1秒あとの `Tick` で消えることで確かめる。

| # | `override` | `G` | 期限 | 式 |
|---|---|---|---|---|
| a | `None` | 300 | 5.3 | 2.0 + 0.3 + 3 |
| b | `None` | 5000 | 10.0 | 2.0 + 5 + 3 |
| c | `None` | nil | 12.0 | 2.0 + 10 |
| d | `Known{500}` | 300 | 12.0 | 2.0 + max(0.5 + 3, 10) |
| e | `Known{9000}` | 300 | 14.0 | 2.0 + max(9 + 3, 10) |
| f | `Known{500}` | nil | 12.0 | 同上（全体の長さは使わない） |
| g | `Unknown` | 300 | 12.0 | 2.0 + max(0.3 + 3, 10) |
| h | `Unknown` | 9000 | 14.0 | 2.0 + max(9 + 3, 10) |
| i | `Unknown` | nil | 12.0 | 2.0 + 10 |
| j | `None`、応答が `SRD{r1, Timeout}` | 300 | 5.3 | a と同じ（タイムアウトでも同じ式） |

### SQ-23 有効条件（SW）

**復帰のバッチの待ち時間中の変化。** a〜d は、準備 PA("Camera")、t=1.0 `LC{OK}` → `AutoRecover#2`、t=1.0 `Approve(2, RestartMedia, None)` → `Proceed` まで同じ。

| # | t=1.2 の入力 | t=1.5 `Approve(2, SetScene("Camera"), None)` | そのあとの状態 |
|---|---|---|---|
| a | `LC{LOW}` | `Reject` | `BD{2, Rejected}` → `LastError` は記録されない。持ち主 `auto`、戻り先 `"Camera"` を保つ。t=3.0 `LC{OK}` → `AutoRecover#3` |
| b | `SC{"Talk"}`（手動） | `Reject` | 持ち主 `none`、戻り先なし |
| c | `SSC{false}` | `Reject` | 持ち主 `none`、戻り先なし |
| d | `OBSDisconnected` | `Reject`（未接続） | 持ち主 `auto`、戻り先 `"Camera"` を保つ（下の d1、d2） |

d1: t=1.5 `BD{2, Rejected}`。t=1.6 `OBSConnected{true, "Standby", 300}` → `AutoRecover#3`（シーンも配信状態も同じなので持ち主を保つ。イベント起点）。

d2: `BatchDone` が再接続のあとになる。t=1.6 `OBSConnected{true, "Standby", 300}` → 出力なし（#2 が未完了）。t=1.7 `Approve(2, SetScene("Camera"), None)` → `Reject`（`ConnEpoch`）、`BD{2, Rejected}`。t=2.0〜5.0 `Tick` → 出力なし（t=1.0 の送出から5秒未満）。t=6.0 `Tick` → `AutoRecover#3`。

**e. StopStream は受信状態が何度変わっても却下されない。** 準備 P("Talk", 配信中, OK)。t=1.0 `Command{StreamStop}` → `StopStream#1`。t=1.1 `LC{LOW}`、t=1.2 `LC{OK}`、t=1.3 `LC{DOWN}` → 出力なし。t=1.4 `Approve(1, StopStream, None)` → `Proceed`。

**f. StandbyOn は受信状態が何度変わっても却下されない。** 準備 P("Camera", 配信中, OK)。t=1.0 `Command{StandbyOn}` → `StandbyOn#1`。t=1.1 `LC{LOW}`、t=1.2 `LC{OK}`、t=1.3 `LC{DOWN}` → 出力なし（抑止）。t=1.4 `Approve(1, SetScene("Standby"), None)` → `Proceed{r1}`。

### SQ-24 承認の Skip（SW）

**a. StartScene: すでに開始時のシーン（V-04）。** 設定 `CFG(start="Opening")`。準備 P("Opening", 配信中でない, OK)。t=1.0 `SSC{true}` → V-04（提案: `StartScene#1[SetScene("Opening")]`）。提案の場合、t=1.1 `Approve(1, SetScene("Opening"), None)` → `Skip`、期待は登録されない。`BD{1, Completed}` → `LastError` は記録されない。

**b. StandbyOffOther: 前の StandbyOff の切替が遅れて届いた。** 準備 PM("Talk", OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOff}`、`Approve(2, SetScene("Talk"), None)` | `StandbyOffOther#2`、`Proceed{r2}` | — |
| 6.0 | `SRD{r2, Timeout}`、`BD{2, Failed}` | — | 期限 = 9.3。`LastError` を記録 |
| 6.5 | `Command{StandbyOff}` | `StandbyOffOther#3`（承認はまだ） | — |
| 7.0 | `SC{"Talk"}` | — | 本体（r2）。持ち主 `none`、戻り先なし |
| 7.1 | `Approve(3, SetScene("Talk"), None)` | `Skip`（比較はシーン名。有効条件より先） | 期待は登録されない |
| 7.1 | `BD{3, Completed}` | — | 新しい `LastError` は記録されない |

**c. StandbyOffMonitored: `RestartMedia` の承認のあとに届いた。** 準備 PM("Camera", OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StandbyOff}`、`Approve(2, RestartMedia, None)` | `StandbyOffMonitored#2`、`Proceed` | — |
| 1.5 | `Approve(2, SetScene("Camera"), None)` | `Proceed{r2}` | — |
| 6.5 | `SRD{r2, Timeout}`、`BD{2, Failed}` | — | 期限 = 9.8。`LastError` を記録 |
| 7.0 | `Command{StandbyOff}`、`Approve(3, RestartMedia, None)` | `StandbyOffMonitored#3`、`Proceed` | — |
| 7.2 | `SC{"Camera"}` | — | 本体（r2）。持ち主 `none` |
| 7.5 | `Approve(3, SetScene("Camera"), None)` | `Skip` | — |
| 7.5 | `BD{3, Completed}` | — | 新しい `LastError` は記録されない |

**d. `RestartMedia` には `Skip` がない（V-16）。** c の t=7.0 の `Command{StandbyOff}` まで同じ（`Approve(3, RestartMedia)` はまだ）。t=7.1 `SC{"Camera"}` → 本体、持ち主 `none`。t=7.2 `Approve(3, RestartMedia, None)` → `Reject`。`BD{3, Rejected}` → `LastError` を記録（V-16）。

### SQ-25 重複の歯止め（SW）

準備 P("Camera", 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}` | `AutoFail#1` | — |
| 2.0 | `LC{DOWN}` | —（#1 が未完了） | — |
| 3.0〜10.0 | `Tick` ×8 | —（#1 が未完了。5秒たっても出ない） | — |
| 10.5 | `Approve(1, SetScene("Standby"), None)` | `Proceed{r1}` | 期待 [Standby r1] |
| 10.6 | `LC{LOW}`、`LC{DOWN}` | —（#1 が未完了で、待機シーンへの期待もある） | — |

出たバッチは AutoFail の1本だけ。

### SQ-26 配信の区切り（SW）

準備 P("Talk", 配信中, OK)。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `Command{StreamStop}` | `StopStream#1`（承認はまだ） | — |
| 1.1 | `SSC{false}`（OBS の画面で止めた） | — | `StreamEpoch+1` |
| 1.2 | `SSC{true}`（始め直した） | —（開始時の切替は無効。Other） | `StreamEpoch+1` |
| 1.3 | `Approve(1, StopStream, None)` | `Reject` | — |
| 1.3 | `BD{1, Rejected}` | — | `LastError` を記録（Command 由来） |

### SQ-27 RECONNECTING → RECONNECTED（SW）

7章の写像により、この列では switcher に Event が届かない（写像そのものは obs の変換関数のテストで確かめる。U-19）。switcher では、準備 PA("Camera") のあと t=1.0 `Command{StreamStop}` → `StopStream#2` を未完了のまま置き、`StreamStateChanged` を送らずに t=2.0〜5.0 の `Tick` だけを流して、持ち主 `auto`、戻り先 `"Camera"`、`Pending`（#2）、`StreamEpoch` がどれも変わらず、バッチが出ないことを確かめる。

### SQ-28 再接続（SW）

a〜c は、準備 PA("Camera")（LOW）、t=1.0 `OBSDisconnected`（`ConnEpoch+1`、期待が空、持ち主と戻り先は保つ）まで同じ。

| # | t=2.0 の入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| a | `OBSConnected{true, "Standby", 300}`（どちらも同じ） | — | 持ち主 `auto`、戻り先 `"Camera"` を保つ。`SceneEpoch`、`StreamEpoch` は同じ。t=3.0 `LC{OK}` → `AutoRecover#2` |
| b | `OBSConnected{true, "Camera2", 300}`（シーンが違う） | `AutoFail#2[SetScene("Standby")]`、戻り先=`"Camera2"` | 持ち主 `none`、戻り先なし。`SceneEpoch` は進まない |
| c | `OBSConnected{false, "Standby", 300}`（配信状態だけ違う） | — | 持ち主 `none`、戻り先なし。`StreamEpoch+1` |

**d. 切断前は停止で、再接続したら配信中だった。** 設定 `CFG(start="Opening")`。準備 P("Camera", 配信中でない, LOW)。t=1.0 `OBSDisconnected`。t=2.0 `OBSConnected{true, "Camera", 300}` → StartScene は出ない。`StreamEpoch+1`。通常の判定で `AutoFail#1[SetScene("Standby")]`、戻り先=`"Camera"`。

**e. 持ち主が `manual` のまま再接続。** 準備 PM("Talk", LOW)。t=1.0 `OBSDisconnected`。t=2.0 `OBSConnected{true, "Standby", 300}` → 出力なし、持ち主 `manual`、戻り先 `"Talk"` を保つ。t=3.0 `Command{StandbyOff}` → `StandbyOffOther#2[SetScene("Talk")]`。

### SQ-29 配信していない間の手動のシーン切替（SW）

準備 P("Talk", 配信中でない, LOW)。t=1.0 `SC{"Camera"}`、t=2.0 `SC{"Standby"}`、t=3.0 `SC{"Camera2"}`、t=4.0 `LC{OK}`、t=5.0 `LC{DOWN}`、t=10.0 `Tick` → どれも出力なし。持ち主 `none`、戻り先なしのまま。`SceneEpoch` は 3 進む。

### SQ-30 バッチ内の失敗（SW。続行と打ち切りそのものは core で確かめる。U-18）

switcher が確かめるのは、実行役が送る `BatchDone` の受け取りだけ。

**a. `RestartMedia` の失敗では続ける。** 準備 PA("Camera")。t=1.0 `LC{OK}` → `AutoRecover#2`、`Approve(2, RestartMedia, None)` → `Proceed`（実行役の RESTART は失敗する）。t=1.5 `Approve(2, SetScene("Camera"), None)` → `Proceed{r2}`。t=1.6 `SRD{r2, OK}`、`BD{2, Completed, Err: RESTART のエラー}` → `LastError` を記録（BD-C-2）。t=1.9 `SC{"Camera"}` → 本体、持ち主 `none`。

**b. それ以外の失敗では残りを捨てる。** SQ-03b（`SetScene` の `Error` → `BD{2, Failed}`。持ち主 `auto` のまま、5秒あけて再送）。

### SQ-31 本体の SetScene が、手動の切替のあとに届いた（V-08。必須ではない）

準備 P("Camera", 配信中, OK)。13章の「ほぼ同時の切替」の列。

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}`、`Approve(1, SetScene("Standby"), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Standby r1] |
| 1.1 | `SRD{r1, OK}`、`BD{1, Completed}` | — | — |
| 1.2 | `SC{"Camera2"}`（人の切替が、OBS で本体の要求より先に処理された） | `AutoFail#2`、戻り先=`"Camera2"` | 手動。期待が空 |
| 1.3 | `SC{"Standby"}`（#1 の切替が遅れて届いた。#2 は未承認） | — | V-08。文面と提案: 手動（`SceneEpoch+1`）。待機シーン、持ち主 `none` |
| 1.4 | `Approve(2, SetScene("Standby"), None)`、`BD{2, Rejected}` | `Reject`（`SceneEpoch`） | — |
| 2.0 | `LC{OK}` | —（持ち主 `none`。自動では戻らない） | V-08 |

別の列: t=1.2 に `Approve(2, …)` → `Proceed{r2}` が先に通っていれば、t=1.3 の `SC{"Standby"}` は r2 の期待に一致して本体になり、持ち主 `auto`、戻り先 `"Camera2"`。

### SQ-32 再送の間隔（SW）

準備はどれも P("Camera", 配信中, OK)。

**a. `Error` の応答のあと。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}` | `AutoFail#1` | `LastAutoSent[AutoFail]=1.0` |
| 1.0 | `Approve(1, SetScene("Standby"), None)` | `Proceed{r1}` | 期待 [Standby r1] |
| 1.1 | `SRD{r1, Error}`、`BD{1, Failed}` | — | 期待が空、`Pending` が空。`LastError` を記録 |
| 2.0〜5.0 | `Tick` ×4 | —（経過 1〜4秒） | — |
| 5.9 | `Tick` | —（経過 4.9秒） | — |
| 6.0 | `Tick` | `AutoFail#2`（経過 5.0秒ちょうど） | `LastAutoSent[AutoFail]=6.0` |
| 7.0 | `Tick` | —（#2 が未完了） | — |

**b. `Timeout` の応答のあと。**

| t | 入力 | 期待する出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | `LC{LOW}`、`Approve(1, SetScene("Standby"), None)` | `AutoFail#1`、`Proceed{r1}` | 期待 [Standby r1] |
| 6.0 | `SRD{r1, Timeout}`、`BD{1, Failed}` | — | 期待は残る。期限 = 9.3 |
| 7.0〜9.0 | `Tick` ×3 | —（5秒はたったが、待機シーンへの期待が残る） | 期待は残る |
| 10.0 | `Tick` | `AutoFail#2` | 期待が期限切れで消える。`SceneEpoch` は同じ |

**c. 実行役が詰まっている間。** t=1.0 に `LC{LOW}` → `AutoFail#1`。`Approve` も `BatchDone` も呼ばずに t=2.0〜30.0 の `Tick` を流す → バッチは出ない（#1 が未完了）。

**d. イベント起点には5秒の条件が掛からない。** a の t=1.1 まで同じ。t=2.0 `LC{DOWN}` → `AutoFail#2`（経過 1.0秒。LC-5）。

## 4. spec の不明点

spec（v2.1）が期待値を1つに決めていない箇所と、2つの規則がぶつかる箇所。表と Event の列では、ここの番号を引くだけにして、どれかを選んだ値を黙って書いていない。**提案** は裁定の案で、決まったら spec を直して変更履歴に足し、この文書の該当行から `V-xx` を外す。番号は v1.x の `U-xx`（spec 6章「細部の規則」が引いている）と重ならないよう `V-xx` にした。

### 結果が大きく変わるもの

**V-01 設定が変わったあとの期待は、持ち主と戻り先を「消す」方向にも作用しないのか**
- spec: 「期待の `ConfigEpoch` が今の値と違うとき（登録したあとにシーンの設定が変わった）は、本体の切替とみなすだけで、持ち主と戻り先には作用しない（消した持ち主と戻り先を、古い設定の期待で復活させないため）」（6章「見分け方」）と、「OBS に接続している間、持ち主が `none` 以外なら現在のシーンは待機シーンである」（6章「待機の持ち主と戻り先」）
- 不明な点: 理由として書かれているのは「入れる」方向（復活させない）だけ。文面どおり「消す」方向にも作用しないとすると、AutoRecover / StandbyOff の `SetScene(戻り先)` を送ったあとに、消す条件に当たらない設定の変更（別の監視対象の追加など。持ち主は保たれる）が入り、そのあと戻り先への `SceneChanged` が届いたとき、持ち主と戻り先が残ったまま現在のシーンが戻り先になる。2つ目の規則が崩れ、持ち主が `none` でないので、そのシーンで回線が悪くなっても AutoFail が出ない（手動の切替か配信の停止まで続く）。持ち主が `manual` の場合は、StandbyOn が出せず、StandbyOff は `RestartMedia` の承認で却下され続ける
- 決定（案どおり）: 古い世代の期待が作用しないのは「入れる」方向（AutoFail、StandbyOn）だけとする。「消す」方向（AutoRecover、StandbyOffMonitored、StandbyOffOther の期待どおりの `SceneChanged`）は、設定の世代によらず持ち主を `none` に、戻り先をなしにする
- 引いている行: SCE-14、SQ-15e

**V-02 同じシーンへの期待が複数あるとき、一致した期待より後ろの同名の期待を消すか。11章の例は作れない**
- spec: 「列の先頭から見て最初に一致する期待のシーン名と同じなら、本体の切替とみなし、その期待と、それより前にある期待をすべて消す」（6章）と、「同じ待機シーンへの期待が複数ある（AutoFail の再送など）とき、`SceneChanged` が1つだけ届いても、持ち主は最初に一致した期待の Purpose に従い、残りの期待は消える」（11章）
- 不明な点: (1) 同じシーン名の期待が2つあると、先頭から見て最初に一致するのは前のほうなので、6章の文面では後ろの期待が残る。11章は「残りの期待は消える」と書いている。残ると、その期待の期限まで、StandbyOn が `ErrInvalidState` になり（CMD-SON-7）、抑止が続き、人がそのシーンへ切り替え直しても本体の切替とみなされる。(2) 11章の例（AutoFail の再送で待機シーンへの期待が2つ）は、`Handle` と `Approve` を通る列では作れない。AutoFail は待機シーンへの期待が残る間は出ず（再送の歯止め）、StandbyOn は期待が1つでも残る間は受け付けないため。同じシーンへの期待が2つになるのは、`Timeout` のあとに StandbyOff を押し直した場合（戻り先への期待が2つ。CMD-SOFF-11）だけ
- 決定（案どおり）: (1) 一致した期待と同じシーン名の期待は、後ろにあってもすべて消す（11章に合わせる。OBS は現在のシーンへの `SetCurrentProgramScene` でシーン変更のイベントを出さない前提。この前提は spike ①② のついでに実機で確かめる）。(2) 11章の例を「StandbyOff の押し直しで、同じ戻り先への期待が2つ」に書き換える
- 引いている行: SCE-17、CMD-SOFF-11、SQ-20c

**V-03 前の配信の StartScene が未完了の間に、次の配信が始まったとき**
- spec: 「`StreamStateChanged{Streaming: true}` を受けたとき、StartScene のバッチを出す」（6章「配信開始時の切替」）と、「同じ Purpose の未完了のバッチがある間は、同じ Purpose の新しいバッチを出さない（イベント起点の判定にも再送にも掛かる）」（6章「目的と有効条件」）、「StartScene は再送しない」
- 不明な点: 実行役が詰まっていて前の配信の StartScene が `Pending` に残ったまま、配信が止まって始め直された場合、新しい StartScene を出すか。出さなければ、前のバッチは `StreamEpoch` で却下され、再送もしないので、この配信では開始時の切替が行われない
- 決定（案どおり）: 出さない（2つ目の規則を優先し、同じ Purpose のバッチを重ねない）。前のバッチの却下は `last_error` に記録される（BD-R-5）ので、利用者には見える。前のバッチが残る間は抑止が続き、`BatchDone` のあとの `Tick` から通常の判定に戻る
- 引いている行: SST-6

**V-04 配信が始まったとき、現在のシーンがすでに開始時のシーンだった**
- spec: 「`StreamStateChanged{Streaming: true}` を受けたとき、StartScene のバッチ（`[SetScene(開始時のシーン)]`）を出す」「`SetScene` の承認で、現在のシーンがすでに切替先のシーンなら `Skip`」「StartScene のバッチが未完了か、その期待が残っている間は、自動切替の判定をしない」（6章）
- 不明な点: バッチを出して承認で `Skip` にするのか、バッチを出さずにその場で通常の判定をするのか。前者だと、開始時のシーンが Monitored で不良のとき、AutoFail は StartScene の `BatchDone` のあとの `Tick` まで遅れる（最大で実行役の待ち + 1秒）
- 決定（案どおり）: 文面どおり出す（承認で `Skip`、`BatchDone{Completed}`、`last_error` には残らない）。規則を増やさない。遅れは1〜2秒で、開始時の切替を使う利用者には「開始時のシーンが出てから待機シーンへ」の順で見える
- 引いている行: SST-5、SQ-24a（出さない裁定なら AP-STARTSCENE-2 は到達しない行になる）

**V-05 StartScene の却下が「手動の切替による」かどうかの決め方**
- spec: 「バッチの失敗、却下、期待の期限切れは `last_error` に記録する。ただし、手動の切替で `SceneEpoch` が進んだための却下は記録しない」（6章「配信開始時の切替」、「実装前に決めたこと」の 6）と、承認の順序「共通の条件（接続、4つの Epoch）→ …」（U-01。4つの Epoch の中の順序は書かれていない）
- 不明な点: (1) 記録するかどうかを決めるのは `BatchDone{Rejected}` を受けた時点だが、その時点では却下の理由がわからない。`Approve` が理由を覚えておく必要がある。(2) 承認までに、手動の切替と、配信の停止や切断や設定の変更の両方が起きていた場合に、記録するか
- 決定（案どおり）: `Approve` が StartScene を却下するとき、バッチの `SceneEpoch` が今と違っていれば、ほかの条件が同時に崩れていても「手動の切替による却下」とし、`Pending` のそのバッチに印（`ManualRejected`）を付ける。`BatchDone{Rejected}` は印があれば記録しない（人が先にシーンを選んでいたなら、StartScene はどのみち実行されなかった）
- 引いている行: AP-COM-5、AP-STARTSCENE-3、BD-R-4、BD-R-6

**V-06 AutoFail が実行待ちの間に、人が別の Monitored のシーンへ切り替えたときの再送の時刻**
- spec: 「回線が悪い間に人が監視対象のシーンを選ぶと、すぐ待機シーンへ戻される」（6章「外部からの手動切替」）と、「同じ Purpose の未完了のバッチがある間は、同じ Purpose の新しいバッチを出さない」「`Tick` の再送なら、同じ Purpose の前回の送出から5秒以上たった」（6章）、「配信の停止で…各 Purpose の前回の送出時刻を消す」（消すのは配信の停止だけ）
- 不明な点: AutoFail が承認を待つ間に人が Monitored のシーンへ切り替えると、その `SceneChanged` の時点では未完了のバッチがあるので新しい AutoFail は出ない。前のバッチは `SceneEpoch` で却下され、そのあとは `Tick` の再送だけが出せるが、5秒の条件が前のバッチの送出から数えられる。回線が悪いまま、人が選んだ Monitored のシーンが最大で約6秒出る。1つ目の規則の「すぐ」と合わない。バッチが終わってから切り替えた通常の列（SQ-05a）では、イベント起点なのですぐ出る
- 決定（案どおり）: 手動の切替を受けたとき、各 Purpose の前回の送出時刻（`LastAutoSent`）も消す（配信の停止と同じ）。5秒の歯止めは「同じ状況で送り続けない」ためのもので、人がシーンを変えたら状況は新しい。これで、前のバッチの `BatchDone{Rejected}` のあとの最初の `Tick` で AutoFail が出る。文面どおり（消さない）とする場合は、SQ-05b の「文面」の値を使う
- 引いている行: SCM-8、SQ-05b

**V-07 消す条件に当たらない設定の変更でも、AutoFail / StandbyOn の期待は持ち主を入れないのか**
- spec: V-01 と同じ文。11章「期待の設定の世代」は、待機シーンの名前の変更、`auto_enabled` を false、戻り先を監視対象から外す、の3つを挙げている
- 不明な点: 文面は設定の変更の内容を問わない。AutoFail の `SetScene` を送ったあとに、関係のない変更（別の監視対象の追加、`StartScene` だけの変更）が入ると、届いた `SceneChanged` は本体の切替だが持ち主が入らず、待機シーンに持ち主 `none` で残る（回復しても自動では戻らない）。期待値そのものは文面で決まるが、11章が挙げた3つの場合の外まで同じでよいかを確かめたい。StandbyOn の期待なら、持ち主が `manual` にならず、StandbyOff が `ErrInvalidState` になる
- 決定（案どおり）: 文面どおり（内容を問わず入れない）。第1段階では `ScenesConfigured` は起動時の1回だけで、この列は起きない。第2段階で設定の編集を入れるときに、「入れようとしている持ち主と戻り先が、新しい設定でも消す条件に当たらなければ入れる」に緩めるかを決める
- 引いている行: SCE-13、SQ-15d

**V-08 本体が送った `SetScene` の `SceneChanged` が、手動の切替で期待が消えたあとに届いた**
- spec: 「手動の切替を受けたとき: …期待を全部消し」「どの期待とも一致しない `SceneChanged` は、すべて手動（外部）の切替とみなす」（6章）と、13章「残る窓は「承認から OBS が要求を受け取るまで」の間だけ。その間に人が切り替えた場合は、人の選択が上書きされることがある。その場合はもう一度切り替える」
- 不明な点: 期待値は文面で決まる（手動として扱う）。ただし結果は 13章の説明より重い。遅れて届いたのが待機シーンへの切替なら、待機シーンに持ち主 `none` で残り、回復しても自動では戻らない。人が選んだシーンのために出した AutoFail も `SceneEpoch` で却下される
- 決定（案どおり）: 文面どおり（手動として扱う）。13章の既知の制約に「この場合、待機シーンに残り、回復しても自動では戻らない。もう一度シーンを選ぶ」と書き足す
- 引いている行: SCM-16、SQ-31

### 設定と接続の境界

**V-09 現在のシーンと同じ名前の `SceneChanged`（期待なし）**
- spec: 「どの期待とも一致しない `SceneChanged` は、すべて手動（外部）の切替とみなす」（6章）と、「同じ値の Event（U-14）」（`StreamStateChanged`、`LinkChanged`、`ScenesConfigured` だけを挙げ、`SceneChanged` は挙げていない）
- 不明な点: 文面どおりなら手動として扱い、`SceneEpoch` を進め、持ち主と戻り先を消す（自動の待機中に届くと、回復しても戻らなくなる）。U-14 と同じに「何もしない」とする読み方もある
- 決定（案どおり）: 文面どおり手動として扱う（U-14 に足さない）。OBS は同じシーンへの変更でイベントを出さない前提で、V-02 の (1) と一緒に実機で確かめる。出すことがわかったら、この項目と V-02 を見直す
- 引いている行: SCM-14

**V-10 `ScenesConfigured` の「同じ内容」の範囲と、最初の1回の `ConfigEpoch`**
- spec: 「`ConfigEpoch` は、シーンの設定を変える `ScenesConfigured`（内容が今の設定と違うもの…）のたびに1つ進む」「今と同じ内容の `ScenesConfigured` では何もしない」「4つの Epoch は 0」「シーンの設定は初期値を持たず、最初の Event として `ScenesConfigured` を必ず受ける」（6章）
- 不明な点: (1) `Monitored` の順序だけが違うものは同じ内容か。(2) 最初の `ScenesConfigured` で `ConfigEpoch` が進むか（0 のままか 1 か）。(3) 最初の `ScenesConfigured` より前にほかの Event や `Approve` が来たとき
- 決定（案どおり）: (1) `Monitored` は集合として比べる（順序の違いは同じ内容）。`StartScene` は nil どうし、または指す文字列どうしで比べる。(2) 進める（最初の設定も「今の設定（なし）と違う」）。テストは絶対値ではなく、前後の差で確かめる。(3) 定めない（9章のとおり、中核ループを始める前に必ず渡す）。テストも書かない
- 引いている行: SCF-1、SCF-2

**V-11 OBS 未接続の間の `ScenesConfigured`**
- spec: 持ち主と戻り先を消す条件（6章「待機の持ち主と戻り先」）は接続の有無を書いていない。切断中は持ち主と戻り先を保ち、`OBSConnected` で「シーンも配信状態も同じなら保つ」
- 不明な点: 切断中（持ち主を持ったまま）に設定が変わったとき、消す条件をその場で確かめるか、`OBSConnected` まで延ばすか
- 決定（案どおり）: 接続中と同じ規則をその場で確かめる（待機シーンの名前が変わった、など）。`ConfigEpoch` も進める。未接続なので J は出ない
- 引いている行: SCF-21

**V-12 `ErrInvalidState` の理由の文の優先順**
- spec: StandbyOn の理由として「すでに待機シーン」「切替の実行中」「同じ操作が実行待ち」の3つ、ほかの Command は「同じ操作が実行待ち」だけを書いている（6章「手動操作」、U-04）。複数が同時に当たるときの順序と、ほかの理由（配信中でない、持ち主が `manual` でない）の文は書かれていない
- 不明な点: 例: 自動の待機中（待機シーン、持ち主 `auto`）で AutoRecover が未完了のときの StandbyOn は「すでに待機シーン」か「切替の実行中」か
- 決定（案どおり）: 確かめる順を、未接続 → 配信の条件 → シーンと持ち主の条件（すでに待機シーン、持ち主が `manual` でない）→ 同じ操作が実行待ち → 切替の実行中（未完了のバッチ、残っている期待）とし、最初に当たった理由を返す。表のテストは `cmdErr` の型だけを確かめ、理由の文は、理由が1つだけ当たる行（CMD-SON-3〜8）でだけ確かめる
- 引いている行: 2.12 の共通の前提、CMD-SON-3〜8

### 到達しない行

**V-13 有効条件の表にあるが、`Handle` と `Approve` を通る列では作れない組み合わせ**
- spec: 「目的と有効条件」の表の AutoFail（Monitored、持ち主が `none`、抑止に当たらない）、AutoRecover（待機シーン、抑止に当たらない）、StandbyOn（「承認の前に AutoFail が先に待機シーンへ届いた場合もここで却下し」）、U-01 の `Skip`、11章「Auto の承認も却下される」「StandbyOn…承認の前に待機シーンへ切り替わっていたら、`Skip` ではなく却下」
- 不明な点: 次の行は、4つの Epoch が同じままでは起こせない。
  - AP-AUTOFAIL-3（すでに待機シーンで `Skip`）と AP-AUTOFAIL-4 の「持ち主が `none` でない」: AutoFail が未完了の間は、待機シーンへの期待を登録できる Purpose がない（AutoFail の再送は歯止めで出ず、StandbyOn は受け付けない）
  - AP-AUTOREC-4（すでに戻り先で `Skip`）、AP-AUTOREC-5（`RestartMedia` で待機シーンでない）: 同じ理由で、戻り先への別の期待が存在しない
  - AP-AUTOFAIL-5、AP-AUTOREC-6（抑止による却下）: Auto が未完了の間は StandbyOn を受け付けず、StandbyOff は持ち主が `manual` でないので受け付けず、StartScene は配信の開始でしか出ない（`StreamEpoch` が進む）。SQ-10e の却下は `StreamEpoch` と重なり、どちらの規則で却下したかは区別できない
  - AP-SON-2、AP-SON-3（StandbyOn の有効条件での却下）: v2.1 で「期待が1つでも残っている間は StandbyOn を受け付けない」が入ったので、受け付けたあとに本体の切替で待機シーンへ届くことがない。届くのは手動の切替だけで、その却下は `SceneEpoch`（AP-COM-5）になる（SQ-18g。結果の `Reject` と `last_error` の記録は 11章のとおり）
  - AP-SOFFO-3（StandbyOffOther の有効条件での却下）: Epoch を進めずに持ち主が `manual` でなくなるのは、前の StandbyOff の期待で戻り先へ届いたときだけで、そのときは現在のシーンが切替先なので `Skip` が先に当たる（AP-SOFFO-2）。StandbyOffMonitored では、同じ状況の `RestartMedia` の承認が却下になる（AP-SOFFM-6。こちらは到達する）
- 決定（案どおり）: 規則は安全側の備えとして実装に残す。テストは、パッケージ内テストで `Pending` と状態を直接作って `Approve` を呼ぶ行として書く（列では書かない）。spec の 6章と 11章の該当の文に「通常の列では共通の条件で先に却下される」と書き足す。行ごと省く裁定でもよい
- 引いている行: AP-AUTOFAIL-3、AP-AUTOFAIL-4、AP-AUTOFAIL-5、AP-AUTOREC-4、AP-AUTOREC-5、AP-AUTOREC-6、AP-SON-2、AP-SON-3、AP-SOFFO-3、SQ-10e、SQ-18g

### `last_error` と細部

**V-14 StartScene の期待が、期限切れ以外の理由で消えたとき**
- spec: 「StartScene は再送しない。バッチの失敗、却下、期待の期限切れは `last_error` に記録する」（6章）
- 不明な点: 応答は返ったが `SceneChanged` が届く前に、配信の停止、OBS の切断、手動の切替で期待が消えた場合（バッチは `Completed` で終わっている）に、記録するか
- 決定（案どおり）: 記録しない。3つとも、開始時の切替が要らなくなったか、人の操作が優先された場合で、エラーではない
- 引いている行: OD-3、SCM-15、SSF-3

**V-15 `OBSConnected` で配信の区切りとみなしたときの再送待ち**
- spec: 「配信の停止…各 Purpose の前回の送出時刻（再送待ち）を消す」（`StreamStateChanged{false}` について。6章、U-16）と、「`OBSConnected` で…配信状態が違っていたら配信の区切りとみなす」
- 不明な点: `OBSConnected` で配信状態が切断前と違っていたとき、`LastAutoSent` を消すか
- 決定（案どおり）: 消さない（文面どおり）。再接続のあとの最初の判定はイベント起点で5秒の条件が掛からず、切断から再接続までに5秒以上かかるのが普通なので、結果は変わらない
- 引いている行: OC-7

**V-16 StandbyOff を押し直したあと、前の切替が届いてから `RestartMedia` の承認になったとき**
- spec: 「`RestartMedia` には `Skip` がなく、Purpose の有効条件で決める（現在のシーンが待機シーンでなければ却下）」（U-01）と、「`BatchDone{Rejected}`…が Command 由来のバッチ…なら、`last_error` に記録する」
- 不明な点: 期待値は文面で決まる（`Reject`、`last_error` に記録）。ただし、戻り先へはすでに届いていて目的は達している。同じ状況で `RestartMedia` の承認のあとに届いた場合は `SetScene` が `Skip` になり、記録されない（AP-SOFFM-5）。届く時刻が 0.1秒違うだけで `last_error` の有無が変わる
- 決定（案どおり）: 文面どおり（却下して記録する）。起きるのは StandbyOff がタイムアウトして押し直した場合だけで、直前の `Failed` がすでに `last_error` に出ている
- 引いている行: AP-SOFFM-6、SQ-24d

**V-17 再送の歯止めの「そのシーンへの期待」の比べ方と、待機シーンの名前を変えた直後の AutoFail**
- spec: 「同じ Purpose の未完了のバッチがなく、そのシーンへの期待も残っていない」（6章「再送の歯止め」）と、「`ScenesConfigured` を受けたら…期待はそのまま残す…そのあと通常の判定をする」
- 不明な点: 「そのシーン」を切替先のシーン名で比べると、AutoFail の `SetScene(旧待機シーン)` を送ったあとに待機シーンの名前が変わった場合、新しい待機シーンへの期待はないので、設定の変更の `Handle` で2本目の AutoFail（新しい待機シーン行き）が出る。OBS には続けて2つの切替が届き、届く順序で最後のシーンが変わる（SQ-15a では旧待機シーンに持ち主 `none` で残り、SQ-20 では新しい待機シーンに持ち主 `auto` で入る）。Purpose で比べる読み方（AutoFail の期待が残る間は AutoFail を出さない）なら2本目は出ない
- 決定（案どおり）: シーン名で比べる（文面どおり。Purpose を問わない）。2本目の AutoFail は出す。11章「期待の設定の世代」の期待値（持ち主 `none`、戻り先は空）は、どちらの読み方でも SQ-15a で成り立つ
- 引いている行: J の 5、SCF-19、SCF-20、SQ-15a、SQ-20

**V-18 StandbyOn / StandbyOff の期待が期限切れになったときの `last_error`**
- spec: 「戻すバッチが却下、失敗、期限切れになったときは消さない（`auto` なら条件が続く間 `Tick` で再送する。`manual` なら `last_error` に残り、もう一度 StandbyOff を受け付ける）」（6章「待機の持ち主と戻り先」）と、「`Rejected` は Command 由来と StartScene だけ記録する…StartScene の期待の期限切れも記録する」（U-05。期限切れの記録は StartScene だけを挙げている）
- 不明な点: StandbyOff の `SetScene` の応答が `OK` で、バッチが `Completed` で終わり、`SceneChanged` が届かないまま期待が期限切れになった場合。1つ目の文は `last_error` に残ると読めるが、U-05 の文面では記録するところがない（`BatchDone{Completed}` は記録せず、期限切れの記録は StartScene だけ）。StandbyOn の期待の期限切れは、どちらにも書かれていない。記録しなければ、API は 200 を返したのに何も起きず、利用者に見えない
- 決定（案どおり）: Command 由来の Purpose（StandbyOn、StandbyOffMonitored、StandbyOffOther）の期待の期限切れも `last_error` に記録する。AutoFail / AutoRecover の期待の期限切れは記録しない（`Tick` で再送する）
- 引いている行: 0章の E、TK-15、SQ-08e
