# switcher「入口 × 状態」の差分表 — 区切り0a

- 対象: カメラの spec `../specs/2026-11-21-multi-camera-design.md` v0.7、第1・3・4・6節の0a。
- 基準: `2026-11-19-switcher-table.md`（以下「第1段階の表」、1392行）。
- コアの spec `../specs/2026-11-18-core-design.md` 第6章の確定済み U/V 規則も引き継ぐ。
- 状態: 実装前のレビュー用。コードの変更や spec の改訂を承認する文書ではない。
- 対象実装: `internal/switcher/{types,machine,approve}.go` と既存の行・列のテスト。
- 地図と健康は M-5 の `internal/scenemap/scenemap.go` を利用する。
- 本文で変更しない行は「第1段階の表のまま」。同じ表を複製しない。
- 既存の行の置換は `LC-1/0a` のように元の名前へ `/0a` を付ける。
- 新しい行は `MC-`、列は `MC-SQ-`。タグ T はすべて `SW`。
- 複数行をまとめた置換は元の行名の範囲を併記し、テストでは各行を分ける。
- W-01〜W-06 は未決定。本表の該当箇所の期待値は、その案を採用した場合のもの。

## 0. 差分の読み方と表記

第1段階の表0章の E（入口で期限切れを処理）、C（持ち主と戻り先を消す）、
X（切替中の抑止）、B（バッチ登録）は引き継ぐ。シーンの文字列はすべて UUID に読み替える。
E は同値の Event、無視する Event、拒否する Command でも先に行う。
表の「変化なし」は E の処理後を指す。書いていない状態と Epoch は保つ。

| 記号 | この表での意味 |
|---|---|
| S | 待機シーン UUID。表示名 Standby |
| A / B | Camera / Camera2 の UUID。シーンの名前そのものではない |
| D | 手元シーン Desk の UUID |
| T / O | Talk / Opening の UUID。カメラなし |
| m / s / d | カメラ id main / sub / desk |
| im / is / id | 各カメラの入力 UUID。表示名は別に持つ |
| H(R) | 現在の地図・状態から求めた戻り先 R の健康 |
| H(now) | 現在のシーンの健康 |
| 不良 | 健康 LOW または DOWN。健康なしは不良に含めない |
| I(R) | R に映るカメラの再起動対象入力 UUID の列（4章） |
| W | 既存の RestartWait。列では0.5秒 |
| F(R) | `AutoFail[SetScene(S)]`、バッチの戻り先 R。B を伴う |
| R(R) | `AutoRecover[RestartMedia{Inputs:I(R), Wait:W}, SetScene(R)]`。B を伴う |
| M(R) | `StandbyOffMonitored[RestartMedia{Inputs:I(R), Wait:W}, SetScene(R)]`。B を伴う |
| — | バッチなし、または指定の欄の変化なし |

I(R) が空なら RestartMedia の Action 自体を省く。SetScene の前の待ちもない。
特記がなければ、接続・配信中、自動有効、持ち主 none、X なし、未完了・期待なし。
カメラは m、s の順で、ともに standby。hide に置き換えても0aの結果は同じ。
地図には S、A、B、D、T、O があり、A は m と s、B は m、D は d が映る。
待機 S はカメラが映っていても待機分類。列ごとの変更はその場に書く。

### J/0a（判断の差分）

第1段階の J の共通条件、X、同じ Purpose の未完了、切替先への期待、
Tick だけ5秒を待つ条件は第1段階の表のまま。比較対象のシーンは UUID。
候補の決め方だけを置き換える。

| 候補 | 条件 | 出力 |
|---|---|---|
| AutoFail | 現在が Monitored、H(now) が不良、持ち主 none | F(now) |
| AutoRecover | 現在が待機、持ち主 auto、戻り先が Monitored、H(戻り先)=OK | R(戻り先) |
| どちらでもない | 上記以外。健康なしも含む | — |

判断する入口は旧 LinkChanged を CamerasChanged に置換し、SceneMapChanged と
ItemEnabledChanged を加える。それ以外の判断の入口は第1段階の表のまま。
同値の CamerasChanged / SceneMapChanged は E のあと判断せずに終わる。
地図・項目が変わっても SceneEpoch、ConfigEpoch は進めない。
イベント起点の判断には5秒の制限を足さない。健康不変の変更も J/0a を行う。
待機 UUID 不明時の AutoFail は W-01 の案により出さない。
0b の「戻り先が待っている隠した項目」の条件は存在しない。
Handle の戻り値は今と同じく多くてもバッチ1本。バッチの列にはしない。

## 1. 型の変更

### 1.1 Event

すべて従来どおり At を持ち、Machine は時計も OBS も読まない。
以下の型名は設計上の名前であり、実装のフィールド名の確定ではない。

| 変更 | 0a の入口と内容 |
|---|---|
| LinkChanged を廃止 | `CamerasChanged{States map[string]scenemap.CameraState}`。差分でなく全カメラの組 |
| 追加 | `SceneMapChanged{Map scenemap.Map}`。接続世代を確認済みの完成した地図 |
| 追加 | `ItemEnabledChanged{SceneUUID string, ItemID int, Enabled bool}` |
| SceneChanged | シーン名 Scene を `SceneUUID string` に置換 |
| OBSConnected | Scene を `SceneUUID string` に置換、最初の `Map scenemap.Map` を追加 |
| OBSConnected の維持 | Streaming、TransitionMs、At は同じ |
| ScenesConfigured | Monitored を削除。`Cameras []CameraConfig` を追加 |
| CameraConfig | `ID string` と `OnBad scenemap.Motion`。設定の順を保持 |
| ScenesConfigured の維持 | Standby（名前）、StartScene（名前へのポインタ）、AutoEnabled、At |

ItemEnabledChanged の SceneUUID はグループ UUID のこともある。
0a では地図の Enabled を書き換え、分類・健康・戻り先を再評価するだけ。
項目の期待、隠した一覧、人が戻した印、OBS への表示変更は追加しない。
States の不足キー・未知キーの扱いは W-05。通常の core は設定済み全 id を渡す。
カメラの表示名・閾値はこの入口の比較対象に含めない（Judge と表示側の情報）。

### 1.2 Action と関連フィールド

| 型・箇所 | 変更 |
|---|---|
| SetScene | `SceneUUID string`。名前を送らない |
| RestartMedia | `Inputs []string, Wait time.Duration`。Inputs は入力 UUID |
| Batch.ReturnScene | 戻り先 UUID。空は戻り先なし |
| Expectation.Scene / ReturnScene | 切替先 UUID / 戻り先 UUID |
| PendingBatch.ReturnScene | 戻り先 UUID |
| Snapshot | シーン・戻り先の識別は UUID。名前の表示は地図から取得 |

SetScene の切替先はバッチ作成時の UUID に固定する。
送信済みの期待の UUID は地図更新や名前変更で書き換えない。
RestartMedia は対象列へ順に RESTART を送り、最後に1回だけ W 待つ。
各入力ごとに W 待つ形にはしない。対象の確定時点は W-03 を参照。

### 1.3 obs.Client の interface の要点

現行 `internal/obs/ws.go` の `Client` は引数の型を大きく変えず、識別の意味を変える。

| メソッド | 0a の契約 |
|---|---|
| Events | 新しい地図・項目 Event、UUID の接続・シーン Event を流す |
| SceneTransitionOverride(sceneUUID) | 切替先 UUID を `sceneUuid` に渡す |
| SetScene(sceneUUID) | `sceneUuid` で program を切り替える |
| RestartMedia(inputUUID) | `inputUuid` で1入力へ送る。列の実行と待ちは実行役 |
| StartStream / StopStream | 第1段階の表のまま |

Client に RestartMedia の列を渡す必要はない。Action に列を持たせる。
設定の MediaSource 1本を前提にしない。名前の存在確認は地図の名前解決へ移る。
最初の地図取得、古い OBS 判定、補助接続、世代の確認、表示値の重ね合わせは obs/core 側。
Machine にネットワーク要求や再読込タイマーを持ち込まない。
コレクション切替は obs が切断として扱い、旧接続の結果を Machine に渡さない。

## 2. 内部状態の差分（第1段階の表1章）

定数、Pending、期待の期限情報、通し番号、再送時刻は第1段階の表のまま。
Link と Monitored を削除し、次を持つ。

| 状態 | 値・意味 |
|---|---|
| MapKnown | false=地図なし（不明）。true=地図あり。空の地図も true |
| Map | scenemap.Map。Scenes / Groups / Inputs は UUID が鍵 |
| CameraStates | id → OK / LOW / DOWN。接続の有無によらず保持 |
| Cameras | id と on_bad の順序つき一覧。順は健康の同順位の代表選択に使う |
| Standby / StartScene | 設定された名前を保持。StartScene=nil は機能無効 |
| StandbyUUID / StartSceneUUID | 現在の地図の Scenes の Name から解決。引けなければ空 |
| Scene / PrevConnScene | 現在 / 切断直前の program UUID |
| ReturnScene | 戻り先 UUID。名前変更では変えない |
| Expectations | 切替先 UUID と戻り先 UUID。ConfigEpoch は既存どおり |

地図の所有権を明確にし、Event の map/slice を後から外で変更して状態を変えない。
Snapshot も内部の地図・状態の map を外へ直接公開しない。
分類・健康は保存した過去の判定を正とせず、必要なときに現時点の材料から計算する。

### U-15/0a 初期値

| 時点 | 期待する値 |
|---|---|
| New 直後 | MapKnown=false、地図なし、CameraStates と Cameras は空 |
| New 直後の維持 | 未接続、Streaming=false、PrevConnStreaming=false、Scene / PrevConnScene は空 |
| New 直後の維持 | 4 Epoch=0、持ち主 none、戻り先なし、期待・Pending・再送時刻は空 |
| 最初の ScenesConfigured 後 | Cameras の各 id を DOWN、設定の名前を保持、ConfigEpoch=1 |
| 地図なしの名前解決 | UUID は空。分類 Unknown、健康なし。設定名は失わない |
| 最初の OBSConnected 後 | MapKnown=true。地図・program・配信・遷移値をまとめて導入してから判断 |

最初の Event は引き続き ScenesConfigured。設定前の別 Event / Approve は定義しない。
切断で地図を「なし」にする。カメラ状態を DOWN に戻す規則は追加しない。
切断中にカメラの組が変われば保存するが、Action は出さない。
切断直前のシーン UUID は地図がなくても比較用に保つ。
設定のカメラ追加・削除を直接渡すテストでは、保持 id は状態維持、追加 id は DOWN、削除 id は除く（W-05）。
実運用の追加・削除・順変更・id変更は再起動反映。即時変更とは解釈しない。

## 3. 分類と健康の計算

| 場面 | 計算する材料と結果 |
|---|---|
| 接続して地図あり | `ClassifyWithMap(true, Map, sceneUUID, StandbyUUID)` |
| 切断中 | MapKnown=false。現在も戻り先も Unknown、健康なし |
| 空の地図で接続 | MapKnown=true。監視対象なし。名前解決できた待機だけ Standby、ほかは Other |
| 待機とカメラが同居 | 待機分類を優先。健康は返さない |
| Monitored | `CamerasIn(Map, R, nil, true)` と CameraStates から Health |
| Other / Unknown | 健康なし。OK とみなして自動で戻すこともしない |

0a では Health へ渡す motions を、設定上の hide を含めてすべて MotionStandby に正規化する。
studioMode は false、unusable は空でよい。Studio Mode の状態は0aに導入しない。
M-5 の Health に設定の hide をそのまま渡すと0bの健康になるので禁止する。
CamerasIn は id を辞書順で返すが、Health の cameraOrder には設定順を渡す。
非表示の経路は数えず、別の表示経路が1本でもあればそのカメラは残る。
グループ・入れ子の表示も含む。循環と重複の処理は M-5 の関数に任せる。

| 行 | 映るカメラと状態 | 分類 / 健康 / 悪い代表 |
|---|---|---|
| MC-H-1 | A: m=OK, s=OK | Monitored / OK / なし |
| MC-H-2 | A: m=OK, s=LOW | Monitored / LOW / s |
| MC-H-3 | A: m=LOW, s=DOWN | Monitored / DOWN / s |
| MC-H-4 | A: m=DOWN, s=DOWN | Monitored / DOWN / m（設定順） |
| MC-H-5 | A: m のみ表示 OK、s は人が非表示 DOWN | Monitored / OK / なし |
| MC-H-6 | A: 全カメラを人が非表示 | Other / 健康なし / なし |
| MC-H-7 | A: s は hide、m=OK, s=DOWN | Monitored / DOWN / s（0a） |
| MC-H-8 | S: m=DOWN が表示 | Standby / 健康なし / なし |

判断する Event の更新をすべて済ませてから1回だけ計算する。
承認ではその時点の地図・状態・設定で再計算し、送出時の健康を使い回さない。
健康独自の fail/recover の待ち時間はない。各カメラの Judge 済み状態を受け取る。
空の地図の「空」の意味（シーン名一覧を残すか）は W-01。地図なしとは区別する。

## 4. 再起動対象 I(R)

戻り先に映る standby 扱いのカメラのメディアソースを対象とする。
0a では hide も対象。入力の指定は名前でなく UUID。
対象の重複を除き、カメラの設定順、同カメラ内は入力 UUID の辞書順とする（W-03）。
「そのカメラの全ソース」か「表示経路のある入力だけ」かは W-03 の案による。
本表は前者。R へ映る id の Inputs を集め、非表示の別ソースも含める。

| 行 | 地図 | I(R) / Action |
|---|---|---|
| MC-I-1 | A に m と s。入力 im / is | [im, is]。RestartMedia を1つ、W 待ち1回 |
| MC-I-2 | m が im と im2 の2入力、R に m | [im, im2]。同じ入力を複数経路で読んでも1回 |
| MC-I-3 | A に m だけ。s の入力は別シーンにある | [im]。is を再起動しない |
| MC-I-4 | R にカメラなし | []。RestartMedia 自体なし、SetScene だけ |
| MC-I-5 | 対象が空の合成バッチ生成テスト | Wait=0.5秒でも待たない。空 Action を作らない |

通常の地図で Monitored かつ対象入力0は到達しないが、空リストの生成規則は直接確かめる。
再起動対象の地図変更による陳腐化は W-03。承認の Proceed 自体は Action を書き換えない。
実行中の複数入力の一部失敗は実行役で検証し、switcher は BatchDone.Err を記録する。

## 5. 入口ごとの差分表

各表は E 後の状態。F/R/M の出力は B を含む。明記のない cmdErr は nil。
J/0a は候補がなければ —、抑止や未完了があればバッチを増やさない。

### 5.1 CamerasChanged（旧2.2 LinkChanged 全体の置換）

各行とも、まず全 States を一括で置換する。途中のカメラ状態では判断しない。

| # | T | 前の状態 → 入力 | 出力 | 状態の変化 |
|---|---|---|---|---|
| LC-1/0a | SW | A、H(now)=OK → s=LOW | F(A) | States 更新。持ち主 none のまま |
| LC-2/0a〜LC-4/0a | SW | A、不良へ。AutoFail 未完了または S への期待あり | — | States のみ。Pending・期待を保つ |
| LC-5/0a | SW | A、LOW → DOWN、前バッチ失敗済み、未完了・期待なし | F(A) | 5秒未満でもイベント起点。再送時刻更新 |
| LC-6/0a | SW | A、不良 → OK | — | States 更新 |
| LC-7/0a | SW | 同上、AutoFail 承認待ち | — | Pending は残る。承認で Reject |
| LC-8/0a | SW | S、auto、戻り先 A、不良 → H(A)=OK | R(A) | auto / A を保つ |
| LC-9/0a | SW | 同上、AutoRecover 未完了または A への期待あり | — | States のみ |
| LC-10/0a | SW | S、auto、H(A) が LOW ↔ DOWN または OK → 不良 | — | States のみ |
| LC-11/0a〜LC-13/0a | SW | S の none/manual、または Other。状態変化 | — | States のみ |
| LC-14/0a〜LC-16/0a | SW | 非配信 / 未接続 / 自動無効。状態変化 | — | States のみ。未接続では地図なし |
| LC-17/0a、LC-18/0a、LC-20/0a | SW | X 中、H(now) が不良へ | — | States 更新。抑止解除後の Tick で判断 |
| LC-19/0a | SW | 全 id の状態が同じ組（map の列挙順だけ違う） | — | E 以外何もしない。J も行わない（U-14） |
| LC-21/0a | SW | StartStream / StopStream だけ未完了、A が不良へ | F(A) | 配信操作は X ではない |
| MC-CC-1 | SW | A: m=LOW,s=OK → m=OK,s=LOW | F(A) | 一括で最終健康 LOW。途中の全 OK で回復判断しない |
| MC-CC-2 | SW | S、auto/A、m=LOW,s=OK → m=OK,s=LOW | — | 最終 H(A)=LOW。途中で R(A) を出さない |
| MC-CC-3 | SW | A: m,s=OK、映らない d だけ DOWN へ | — | H(A)=OK 不変。d の状態は保存 |
| MC-CC-4 | SW | A が不良、前 F 失敗済み。d だけ変化 | F(A) | 健康不変でも非同値 Event は J。5秒制限なし |
| MC-CC-5 | SW | S、auto/A、m=DOWN,s=DOWN → m=OK,s=DOWN | — | 全回復でない。H(A)=DOWN |

### 5.2 SceneMapChanged（新設）

地図を差し替え、名前解決、分類、auto の戻り先の検査、J/0a の順。
既存項目の Enabled は本来の接続の値を core が重ねた Map を受け取る。
地図更新だけで現在の program UUID は変えない。人のシーン切替とは扱わない。

| # | T | 前の状態 → 地図の変更 | 出力 | 状態の変化 |
|---|---|---|---|---|
| MC-MAP-1 | SW | A は Other → m の表示経路追加、m=DOWN | F(A) | A が Monitored / DOWN |
| MC-MAP-2 | SW | A は Other → m の表示経路追加、m=OK | — | A が Monitored / OK |
| MC-MAP-3 | SW | A が Monitored → 最後のカメラがなくなる | — | Other、健康なし。SceneEpoch 不変 |
| MC-MAP-4 | SW | A: m=OK のみ → s=DOWN も映る | F(A) | H(now)=DOWN |
| MC-MAP-5 | SW | A: m=OK,s=DOWN → s が映らなくなる | — | H(now)=OK。承認待ち F は残して Reject へ |
| MC-MAP-6 | SW | S、auto/A、A の最後のカメラがなくなる | — | C。S に残る |
| MC-MAP-7 | SW | S、auto/A、A 自体が削除 | — | C。R の送信待ちは残すが承認で Reject |
| MC-MAP-8 | SW | S、manual/A、A の最後のカメラがなくなる | — | manual/A は保つ。次の Off は Other |
| MC-MAP-9 | SW | S、auto/A、不良の s が A から消え、m=OK は残る | R(A) | 戻り先は Monitored / OK。C しない |
| MC-MAP-10 | SW | A の名前だけ Camera → Main、UUID 同じ | J/0a | 戻り先・期待の UUID は保つ。ConfigEpoch 不変 |
| MC-MAP-11 | SW | S の設定名が見つからなくなる | J/0a（F は出さない。W-01） | StandbyUUID 空。持ち主の扱いは W-02 の案で C |
| MC-MAP-12 | SW | 待機名が再び解決可能、A は Monitored / DOWN | F(A) | StandbyUUID=S。新しい C の取り消しはしない |
| MC-MAP-13 | SW | 空の地図 → 通常地図、現在 A、m=DOWN | F(A) | MapKnown=true のまま。分類が監視対象へ |
| MC-MAP-14 | SW | 意味が同じ Map（map 列挙順だけ違う） | — | E 以外変化なし。J なし |
| MC-MAP-15 | SW | 切断中に古い読み直し結果 | — | core で捨てる。Machine に渡さない |
| MC-MAP-16 | SW | 読み直し失敗 | — | この Event 自体なし。直前の地図を維持 |
| MC-MAP-17 | SW | R の名前変更、UUID 同じ、S の auto/R | J/0a | auto/R 維持。名前変更を削除とみなさない |

同値比較は Collection、鍵と値、各 Item の全フィールドを対象とし、nil/空の辞書は同値とする案。
Items の順だけの変更は地図変更としてよいが、健康や対象列の順を変えない。
接続世代をまたいだ同じ Collection の文字列は同値判定の代わりにならない。

### 5.3 ItemEnabledChanged（新設）

既知のシーンまたはグループの該当 Item の Enabled だけを書き換える。
その後、影響する全シーンの経路を再評価し、auto の戻り先を検査して J/0a。

| # | T | 前の状態 → 入力 | 出力 | 状態の変化 |
|---|---|---|---|---|
| MC-ITEM-1 | SW | A: m=OK,s=DOWN、s の唯一の表示項目を false | — | A の健康 OK。地図だけ更新 |
| MC-ITEM-2 | SW | A: m=OK のみ、s=DOWN の項目を true | F(A) | A の健康 DOWN |
| MC-ITEM-3 | SW | A の最後のカメラ項目を false | — | A が Other、健康なし |
| MC-ITEM-4 | SW | S、auto/A、A の最後のカメラ経路を false | — | C。S に残る |
| MC-ITEM-5 | SW | S、auto/A、悪い s だけ false、m=OK が残る | R(A) | auto/A 維持。I(A)=[im] |
| MC-ITEM-6 | SW | S、manual/A、A の最後のカメラ経路を false | — | manual/A 維持。Off は Other |
| MC-ITEM-7 | SW | グループ内の s 項目を false、別の表示経路あり | J/0a | s は依然映る。健康を変えない |
| MC-ITEM-8 | SW | 入れ子の親項目を false、子のカメラが全部外れる | J/0a | 子項目自体の Enabled は変えない |
| MC-ITEM-9 | SW | 既知項目へ同じ Enabled | — | E 以外なし（W-06 の案） |
| MC-ITEM-10 | SW | 未知の項目・コンテナ | — | Machine の地図は変更なし。core が最後の値を記憶 |
| MC-ITEM-11 | SW | 切断中の旧接続の項目 Event | — | obs/core で破棄。地図を復活させない |

未知項目の最後の値は次の Map の新項目へ core が重ね、Map を1回渡したら記憶を消す。
同じ項目 Event を Map の到着後に再度流さない。切断でも core の記憶を消す。
0a では項目 Event を人の「シーン切替」として扱わず、SceneEpoch を進めない。

### 5.4 OBSConnected / OBSDisconnected

接続時は Map と program UUID をまとめて導入し、切断前との比較後に戻り先を検査する。

| # | T | 前提 → 入力 | 出力 | 状態の変化 |
|---|---|---|---|---|
| OC-1/0a、OC-2/0a | SW | 最初の接続、Map つき | J/0a | MapKnown=true。配信 false なら StreamEpoch 不変、true なら+1 |
| OC-3/0a | SW | 同じ UUID・配信で再接続、A が不良 | F(A) | 地図更新。SceneEpoch は進めない |
| OC-4/0a | SW | 同じ UUID・配信、S の auto/A、H(A)=OK | R(A) | auto/A 維持 |
| OC-5/0a | SW | 同じ UUID・配信、H(A) 不良 / manual / none | — | 持ち主を保つ。ただし auto の戻り先喪失は C |
| OC-6/0a | SW | 名前は同じでも program UUID が別 | J/0a | C。手動扱いせず SceneEpoch 不変（U-23） |
| MC-OC-1 | SW | program UUID 同じ、表示名だけ変化 | J/0a | 比較では同じ。名前だけを理由に C しない |
| MC-OC-2 | SW | 同じ S へ再接続、地図では A が消えていた | — | auto/A は C。manual/A は維持（W-04） |
| MC-OC-3 | SW | 空の地図つきで接続 | — | MapKnown=true、監視対象なし。auto の戻り先は C |
| OD-1/0a〜OD-3/0a | SW | 接続 → 切断 | — | MapKnown=false、地図と解決 UUID を消す。ConnEpoch+1、期待を全部消す |

切断時は PrevConnScene に現在 UUID、PrevConnStreaming に配信状態を保存。
Pending、CameraStates、現在 UUID、持ち主・戻り先、再送時刻は保つ。
配信状態が違う再接続の OC-7、OC-8 は、UUID と健康の読み替え以外は第1段階の表のまま。
OC-9〜OC-12 の未完了・抑止・遷移 nil・自動無効も第1段階の表のまま。
再接続で StreamEpoch が進んでも StartScene は出さず、LastAutoSent も消さない。
コレクション切替は必ず切断を通す。同じ UUID が複製先にあっても旧バッチを通さない。
コレクションをまたぐ持ち主の保持は W-04 の案で C とする。

### 5.5 SceneChanged（旧2.5・2.6の差分）

| # | T | 前提 → 入力 | 出力 | 状態の変化 |
|---|---|---|---|---|
| SCE-1/0a、SCE-2/0a | SW | S への AutoFail 期待に UUID 一致 | H(期待の戻り先)=OK なら R(戻り先)、不良なら — | auto と戻り先 UUID を入れる。地図で監視対象外なら C（W-02） |
| SCE-3/0a | SW | S への StandbyOn 期待に UUID 一致 | — | manual と期待の戻り先 UUID を入れる |
| SCE-4/0a〜SCE-8/0a | SW | 戻り先への期待に UUID 一致 | 届いたシーンが Monitored / 不良なら J/0a、ほかは — | C。未着 BatchDone による X は維持 |
| SCE-11/0a、SCE-12/0a | SW | StartScene の期待へ UUID 一致、届いたシーンが不良 | X がなければ F(到着 UUID)、X 中なら — | 戻り先は開始時のシーン UUID |
| MC-SC-1 | SW | 期待 A、A の名前変更後に UUID=A が到着 | J/0a | 本体の切替。一致した期待を処理、SceneEpoch 不変 |
| MC-SC-2 | SW | 期待 A、同じ名前だが UUID=B が到着 | J/0a | 手動。SceneEpoch+1、期待・再送時刻を消し C |
| SCM-2/0a〜SCM-5/0a | SW | 手動で A へ。H(A) 不良 | F(A) | SceneEpoch+1、期待・再送時刻を消し C のあと B |
| SCM-1/0a、SCM-10/0a | SW | 手動で A へ。H(A)=OK または自動無効 | — | 手動の共通変化のみ |
| SCM-14/0a | SW | 期待なし、現在と同じ UUID の Event | J/0a | 従来の V-09 どおり手動。単なる名前変更通知とは別 |

先頭から期待を照合し、一致した期待以前と同じ UUID の期待を消す規則は維持。
古い ConfigEpoch の期待が持ち主を入れない規則（V-01、V-07）も維持。
そのほかの SceneChanged の行は UUID・健康の読み替え以外は第1段階の表のまま。
名前変更だけの SceneMapChanged では SceneChanged の手動共通変化を起こさない。

### 5.6 ScenesConfigured（旧2.1と V-10 の差分）

新しい設定へ差し替え、id の状態を整え、名前解決と戻り先検査、J/0a の順。
内容が変わる場合のみ ConfigEpoch+1。Pending と期待は消さない。

| # | T | 前提 → 入力 | 出力 | 状態の変化 |
|---|---|---|---|---|
| SCF-1/0a | SW | 初回設定、地図なし | — | ConfigEpoch=1、各カメラ DOWN、名前保持 |
| SCF-2/0a | SW | id・on_bad・順・待機名・開始名・auto が全部同じ | — | E 以外何もしない |
| MC-CFG-1 | SW | カメラの順だけ m,s → s,m | J/0a | ConfigEpoch+1。同程度の悪い代表が s へ |
| MC-CFG-2 | SW | s の on_bad standby → hide | J/0a | ConfigEpoch+1。0aの健康・対象入力は同じ |
| MC-CFG-3 | SW | id 追加・削除を直接受ける | J/0a | ConfigEpoch+1。追加 DOWN、削除状態を除去（W-05） |
| SCF-6/0a、SCF-7/0a | SW | 待機名を変更、持ち主 auto/manual | J/0a | C。新名から UUID を引き直す |
| SCF-8/0a | SW | 待機名変更、新 S2 解決可能、現在 A 不良 | F(A) の切替先 S2 | ConfigEpoch+1、期待は旧 UUID のまま |
| MC-CFG-4 | SW | 待機名変更、新名を解決不能 | — | ConfigEpoch+1、解決 UUID 空。W-01 |
| SCF-9/0a、SCF-10/0a | SW | カメラ一覧変更で戻り先が監視対象外 | — | auto は C、manual は保持。地図との整合は W-05 |
| SCF-11/0a、SCF-12/0a | SW | auto=false へ変更 | — | auto のみ C、manual は保持 |
| MC-CFG-5 | SW | 開始時の名前だけ変更 | J/0a | ConfigEpoch+1。ここでは StartScene を出さない |
| SCF-21/0a | SW | 切断中に設定変更 | — | ConfigEpoch+1。待機名変更 / auto無効は直ちに C、監視対象判定は再接続まで保留 |

V-10 の「Monitored は集合として比較」を削除し、Cameras は順序つきで比較する。
StartScene は nil 同士または指す名前で比較。初回設定前の入口は引き続き定義しない。
同じ動きになる hide への変更も設定は異なるので Epoch を進める。
名前解決の結果だけが地図で変わっても ConfigEpoch は進めない。
旧 SCF-3〜5、SCF-9〜10 の「監視対象の追加・削除」は通常は地図更新の行へ移る。
未完了の承認却下、古い世代の期待の効き方は第1段階の表のまま。

### 5.7 Command と配信開始時の切替

受付の共通条件・拒否理由の順・重複操作は第1段階の表のまま。
StandbyOn と配信開始停止は健康で拒否しない。Command の入口では J を行わない。

| # | T | 前提 → 入力 | 出力 / cmdErr | 状態の変化 |
|---|---|---|---|---|
| CMD-SON-9/0a〜CMD-SON-12/0a | SW | 待機以外、On の受付条件を満たす | `StandbyOn[SetScene(S)]`、戻り先=現在 UUID / nil | B。健康不問 |
| CMD-SOFF-5/0a | SW | S、manual/A、H(A)=OK、自動有効 | M(A) / nil | manual 維持 |
| CMD-SOFF-6/0a | SW | 同上、H(A) 不良 | — / nil | auto/A に変更。回復待ち |
| CMD-SOFF-7/0a | SW | S、manual/T、T は Other | `StandbyOffOther[SetScene(T)]` / nil | B。再起動なし |
| CMD-SOFF-8/0a | SW | S、manual/A、自動無効、H(A) 不良 | M(A) / nil | B。auto に変えない |
| MC-CMD-1 | SW | S、manual/A、A が削除済み | `StandbyOffOther[SetScene(A)]` / nil（W-04） | B。失敗なら LastError、manual/A 維持 |
| SST-1/0a | SW | 配信 false→true、開始切替無効、A 不良 | F(A) | StreamEpoch+1 |
| SST-3/0a〜SST-5/0a | SW | 配信 false→true、開始 O が解決可能 | `StartScene[SetScene(O)]` | StreamEpoch+1、B。健康不問、Auto より先 |
| MC-START-1 | SW | 配信開始、開始名が解決不能 | —（W-01） | StreamEpoch+1、LastError 記録。次の Tick は通常判断 |
| SSF-1/0a〜SSF-3/0a | SW | 配信 true→false | — | C、期待・再送時刻を消す。地図とカメラ状態は保持 |

StartStream / StopStream の Command と承認は第1段階の表のまま。
配信停止後もカメラ状態の Event と地図更新を受け、次の配信で最新の値を使う。
StartScene 到着後の健康読み替えは5.5。開始済みへの再接続では出さない。

### 5.8 Approve（旧2.13の差分）

E → 接続・Epoch → SetScene の UUID による Skip → Purpose の条件、の順は維持。
StandbyOn の Skip 例外、RestartMedia に Skip がないことも維持。
共通条件を満たし、先行 Skip がない場合の差分を以下に示す。

| # | T | 前の状態・承認対象 | 結果 | 状態の変化 |
|---|---|---|---|---|
| AP-AUTOFAIL-1/0a | SW | A が Monitored / 不良、none、自動有効、Xなし、SetScene(S) | Proceed | UUID=S の期待、戻り先 A を登録 |
| AP-AUTOFAIL-2/0a | SW | 送出後に H(A)=OK | Reject | 期待を増やさない |
| AP-AUTOFAIL-4/0a | SW | 送出後に A が Other（最後の項目が非表示など） | Reject | 地図変更で Epoch 不変でも拒否 |
| AP-AUTOREC-1/0a、AP-AUTOREC-2/0a | SW | S、auto/A、H(A)=OK、RestartMedia または SetScene(A) | Proceed | Restart は ReqID=0、SetScene は期待登録 |
| AP-AUTOREC-3/0a | SW | 戻り先 A が不良へ | Reject | auto/A 維持。回復後は再送可能 |
| MC-AP-1 | SW | 戻り先 A が監視対象外へ、auto は C 済み | Reject | 期待を増やさない |
| AP-SOFFM-1/0a、AP-SOFFM-2/0a | SW | S、manual/A、H(A)=OK、自動有効 | Proceed | SetScene なら期待登録 |
| AP-SOFFM-3/0a | SW | S、manual/A、H(A) 不良、自動有効 | Replace{nil} | auto/A。Pending に Replaced。期待なし |
| AP-SOFFM-4/0a | SW | S、manual/A、自動無効、H(A) 不良 | Proceed | manual 維持。健康で Replace しない |
| MC-AP-2 | SW | 送出後の Map で manual の戻り先が Other | Reject（W-02） | manual/戻り先を保つ。押し直せば Other |
| MC-AP-3 | SW | Restart の後に項目追加、H(A) が不良へ | AutoRecover は Reject、OffMonitored は Replace{nil} | 健康を再計算。SetScene を送らない |
| MC-AP-4 | SW | 地図上のシーン名だけ変更、UUID 不変 | 健康条件を満たせば Proceed | 期待は元の UUID。名前で Reject しない |
| MC-AP-5 | SW | 共通条件成立、SetScene の対象 UUID が現在 UUID | Skip（On を除く） | 期待なし。健康より先に判定 |
| MC-AP-6 | SW | 対象入力が送出後に消えたが戻り先は健康 OK | Proceed（W-03） | Action の Inputs は固定。実行失敗は Err に残す |

AutoFail / AutoRecover の配信・auto・持ち主・X 条件は削除しない。
承認に再送5秒や同 Purpose の未完了なしを掛けない（自分自身が Pending にあるため）。
待機 UUID が変わった古いバッチと送信済み期待の扱いは W-02。
StandbyOffOther が承認前に Monitored になった場合も W-02 の案で Reject。
StandbyOn / StartScene / StartStream / StopStream の目的条件は健康を参照しない。
共通条件による Reject と LastError の扱い、BD、SRD、期限の式は第1段階の表のまま。

## 6. 列のケース（第1段階の表3章の差分・追加）

各列の t は秒。`完了(X)` は各 Action を直前承認し、SetScene の Proceed で期待を登録、
SRD{OK} と BatchDone{Completed} を受けた後、対象 UUID の SceneChanged を受ける操作。
Restart があれば W=0.5秒を空けて SetScene を承認する。承認省略の意味ではない。
完了時点まで他の Event が割り込まない列では、各承認は Proceed を期待する。

### MC-SQ-01 片方だけ悪化、両方悪化、順に回復

準備: A、m=OK,s=OK。バッチ番号は列内の通し番号。

| t | 入力 | 出力 | 期待する状態 |
|---|---|---|---|
| 1.0 | States{m:OK,s:LOW} | F(A)#1 | H(A)=LOW。owner none |
| 1.1〜1.4 | 完了(#1) | — | S、auto/A |
| 2.0 | States{m:DOWN,s:DOWN} | — | H(A)=DOWN、悪い代表 m |
| 3.0 | States{m:OK,s:DOWN} | — | H(A)=DOWN、悪い代表 s。S のまま |
| 4.0 | States{m:OK,s:OK} | R(A)#2、Inputs=[im,is] | auto/A 維持 |
| 4.1〜4.9 | 完了(#2) | — | A、C。両入力を順に再起動、待ちは1回 |

別列: t=1 で m だけ DOWN でも F(A)。s だけ DOWN でも同じ。
別列: t=1 で両方 DOWN を1 Event で渡しても F は1本。
別列: s を hide 設定にして全手順を繰り返す。結果・対象入力とも同じ。

### MC-SQ-02 同じ健康でも原子的に組を扱う

準備: S、auto/A、m=LOW,s=OK。未完了・期待なし。

| t | 入力 | 出力 | 期待する状態 |
|---|---|---|---|
| 1 | States{m:OK,s:LOW} | — | H(A)=LOW。途中の全 OK による R はない |
| 2 | 同じ組を別の map 列挙順で渡す | — | E 以外なし。再送時刻不変 |
| 3 | States{m:OK,s:OK} | R(A) | 全回復したときだけ戻す |

### MC-SQ-03 手元カメラを使わない日から使う

準備: B に m だけ映る。D に d だけ。m=OK,d=DOWN、配信中。

| t | 入力 | 出力 | 期待する状態 |
|---|---|---|---|
| 1 | Tick（B のまま） | — | d の DOWN は B に影響しない |
| 2 | 人の SceneChanged(D) | F(D)#1 | 手動共通変化。戻り先候補は D |
| 2.1〜2.4 | 完了(#1) | — | S、auto/D |
| 3 | d の Judge がまだ DOWN、Tick | — | 手元の回復待ち |
| 4 | recover_sec を満たした States{m:OK,d:OK} | R(D)#2、Inputs=[id] | m の入力は再起動しない |
| 4.1〜4.9 | 完了(#2) | — | D、C |
| 5 | 人の SceneChanged(B) | — | m は OK。SceneEpoch+1 |
| 6 | 手元送信停止、States{m:OK,d:DOWN} | — | B のまま。待機へ送らない |

### MC-SQ-04 人が OBS の項目を非表示にする

準備: A、m=OK,s=OK。s の項目は一意。

| t | 入力 | 出力 | 期待する状態 |
|---|---|---|---|
| 1 | ItemEnabledChanged(A,s項目,false) | — | A に m のみ。SceneEpoch 不変 |
| 2 | States{m:OK,s:DOWN} | — | H(A)=OK |
| 3 | ItemEnabledChanged(A,s項目,true) | F(A)#1 | H(A)=DOWN |
| 3.1〜3.4 | 完了(#1) | — | S、auto/A |
| 4 | ItemEnabledChanged(A,s項目,false) | R(A)#2、Inputs=[im] | m が残るので戻れる |
| 4.1〜4.9 | 完了(#2) | — | A、C。非表示の s はそのまま |

別列: S、auto/A のとき最後の m も非表示にすれば C、戻らない。
別列: 同じ操作をグループの親項目に行い、すべての経路への影響を確認する。

### MC-SQ-05 戻り先が消える

準備: S、auto/A、m=LOW。A への未完了なし。

| t | 入力 | 出力 | 期待する状態 |
|---|---|---|---|
| 1 | SceneMapChanged（A を削除） | — | C、S に残る。ConfigEpoch 不変 |
| 2 | States（全 OK） | — | 戻り先なしなので R を出さない |
| 3 | A を同じ名前・別 UUID=A2 で作成した Map | — | 戻り先を勝手に復元しない |
| 4 | 人の SceneChanged(A2)、健康 OK | — | A2 を現在 UUID とする |

別列: t=1 の前に R(A) 承認待ちなら、削除後の Approve は Reject。
別列: manual/A の場合は保持し、Off の実行失敗を記録する（W-04）。

### MC-SQ-06 コレクション切替と再接続（W-04 の案）

準備: collection X、S、auto/A。R(A)#1 が承認待ち。m=OK。

| t | 入力 | 出力 | 期待する状態 |
|---|---|---|---|
| 1 | OBSDisconnected | — | ConnEpoch+1、地図なし、期待なし。Pending は残る |
| 1.1 | X の遅れた Map・項目通知 | — | obs/core で捨てる |
| 2 | OBSConnected、collection Y、同じ UUID の S | — | 新地図導入。collection が違うので C |
| 2.1 | #1 の RestartMedia を Approve | Reject | ConnEpoch 不一致。旧接続の入力へ送らない |
| 2.2 | BatchDone{#1,Rejected} | — | Pending から除く。Auto の却下は記録しない |
| 3 | 人が Y の A を選択、健康 OK | — | 新接続のシーンとして判断 |

同じ collection への再接続なら UUID・配信が同じ場合 owner を保ち、新地図で健康を判断。
U-23 により再接続そのものでは SceneEpoch を進めず、StartScene も出さない。

### MC-SQ-07 古い OBS / 最初の地図取得に3回失敗

準備: 設定済み、m=DOWN、開始シーン O を設定。
空の地図でもシーン名一覧を保持する W-01 の案を使う。

| t | 入力 | 出力 | 期待する状態 |
|---|---|---|---|
| 0 | OBSConnected{Streaming:false, SceneUUID:A, 空の地図} | — | MapKnown=true、A は Other。警告は obs/core 側 |
| 1 | Command{StreamStart}、承認 | StartStream、Proceed | 健康で開始を妨げない |
| 2 | StreamStateChanged{true} | StartScene[SetScene(O)] | 空の地図でも開始切替は有効 |
| 2.1〜2.4 | StartScene 完了 | — | O は Other、AutoFail なし |
| 3 | Command{StandbyOn}、完了 | StandbyOn[SetScene(S)] | S、manual/O |
| 4 | Command{StandbyOff}、完了 | StandbyOffOther[SetScene(O)] | 再起動なし。O、C |
| 5 | States の変更、Tick | — | カメラが映らないので Auto なし |

最初の地図取得は、1〜2回の失敗では OBSConnected を流さない。
3回目の失敗でのみ空の地図で接続し、補助接続で回復後の Map を流す。
古い OBS の空の地図は自動を止める。UUID 要求自体の対応可否は W-01 に残す。

### MC-SQ-08 承認までに地図が変わる

準備: S、manual/A、m=OK,s=OK、自動有効。

| t | 入力 | 出力 | 期待する状態 |
|---|---|---|---|
| 1 | Command{StandbyOff} | M(A)#1 | Inputs=[im,is]、manual/A |
| 1.1 | RestartMedia の Approve | Proceed、ReqID=0 | 期待なし |
| 1.2 | Map 変更で DOWN の d が A に映る | — | X 中。ConfigEpoch 不変、H(A)=DOWN |
| 1.6 | SetScene(A) の Approve | Replace{nil} | auto/A、Replaced、期待なし |
| 1.7 | BatchDone{#1,Rejected} | — | LastError を記録しない |
| 2 | d が A から外れる Map | R(A)#2 | H(A)=OK。イベント起点で回復 |

別列: #1 を AutoRecover にすれば t=1.6 は Reject、auto/A を保つ。
別列: A 自体が Other へ変わる manual の列は Reject と LastError（W-02）。

## 7. 第1段階のテストの置き換え方

コードはこの文書では変更しない。行名と意味を維持する移植の案を示す。
カメラ1台 m、on_bad=standby、A と B の双方に同じ入力 im が映る地図を既定にする。
S、T、O も Scenes に登録し、カメラ項目を持たせない。名前と UUID は異なる文字列にする。

| ファイル・補助 | 置換・追加案 |
|---|---|
| helpers_test.go の cfg / cfgOf | Monitored を Cameras=[{main,standby}] に置換 |
| conn | 最初の Map を載せ、scene 名の代わりに UUID を渡す |
| lc(t, LOW) | `CamerasChanged{At:at(t), States:{main:LOW}}`。全状態の組を返す |
| setup_test.go の link(LOW) / linkTo | CameraStates[main]=LOW を設定する補助へ |
| baseState | 地図あり・m=OK・A を現在 UUID にする |
| fresh / disconnected | MapKnown=false、解決 UUID 空を期待する |
| sc / set / waiting / addExp | 引数と保存値を UUID にする |
| actsFor / wantB | 復帰バッチは Inputs=[im]。Other は再起動なし |
| cloneState / diffState | 地図、Items、Cameras、States を深く複製・比較 |
| P / PA / PM | 設定→カメラ組→Map つき接続。以後の時刻と承認順は維持 |
| cfgChange / withMonitored | 設定変更と地図変更の補助へ分離。両方を同時に流さない |

`testMap(visibleSceneUUIDs...)` は定数 UUID の入力 im と main の対応を作り、
各シーンに Enabled=true の入力項目を1つ置く。名前解決用に全シーン名も登録する。
地図の追加・削除とカメラ状態の変更は独立した補助にし、期待値の計算には実装の Health を使わない。
複数カメラの地図は別の fixture とし、1台移植の既定値を増やさない。

| 既存テスト | 維持と置換の範囲 |
|---|---|
| rows_events_test.go | LC は全組へ。OC は Map、SC は UUID。分類変更の SCF は MAP 行へ移す |
| rows_commands_test.go | 1台の健康=従来の Link。出力・cmdErr・持ち主は従来と同じ |
| rows_approve_test.go | 1台の健康で旧結果を再現。RestartMedia に Inputs を加える |
| seq_test.go | SQ の時刻、期限、再送、Epoch、LastError は維持。分類変更の列だけ入口を Map にする |
| setup_test.go | 直接状態を作る V-13 の行も、新しい地図・UUID の前提を満たす |

旧 withMonitored による「分類追加」まで ConfigEpoch+1 を期待するテストは、そのまま移植できない。
分類の期待結果は再現し、ConfigEpoch の変化は MAP 行なら0、設定変更なら+1として区別する。
旧 V-10 の順序不問テストは廃止し、Cameras の順変更で+1になる MC-CFG-1 に置換する。
U-15 の Link=DOWN は設定後の CameraStates[main]=DOWN に置換する。
それ以外の E、期限、Pending、期待、抑止、再送、Command 結果は第1段階の表のまま。
将来の実装検証は switcher と scenemap のテスト、続いて obs/core の連携テストで行う。
この文書だけの変更で、既存コードのテスト結果を0aの実装完了の証拠にはしない。

## 8. spec の不明点（未決定の案）

### W-01 名前解決不能と「空の地図」の手動操作

spec は警告を要求し、古い OBS でも手動操作と開始切替を維持するが、
Scenes まで空だと名前から UUID を引けない。UUID 非対応の古い OBS への送信も未定義。
案: 「空」はカメラ対応と項目を空にし、取得できたシーン UUID・名前一覧は保持する。
それでも引けない待機への AutoFail は出さず警告。On は ErrInvalidState、状態不変。
開始名が引けなければバッチを出さず LastError に記録し、StartScene の再送はしない。
完全な UUID 非対応版については手動の名前要求を obs 側で許すか、対応対象外とするかを別途裁定する。
参照: MC-MAP-11〜12、MC-CFG-4、MC-START-1、MC-SQ-07。

### W-02 地図変更で古くなったバッチ・期待と待機の持ち主

地図の変更は ConfigEpoch を進めないため、旧対象への送信や古い期待の所有権設定が残る。
案: 承認で固定切替先が現在の待機・戻り先に一致するか確認し、不一致なら Reject。
OffMonitored の戻り先が Other へ変わったら Reject、OffOther が Monitored へ変わっても Reject。
押し直し時に目的を選び直す。地図変更では既存バッチを別 Purpose に置換しない。
待機 UUID が変更・消失したら auto/manual とも C。
送信済み期待は照合用に残すが、待機先が現在の待機でなければ持ち主を入れない。
AutoFail の期待の戻り先が監視対象外なら、到着時も auto を入れない。
参照: MC-MAP-11、SCE-1/0a、MC-AP-2。Skip の先行規則は変更しない。

### W-03 RestartMedia の対象範囲・順・確定時点

「戻り先に映る standby のカメラのソース」は、カメラの全入力か、表示中入力だけか曖昧。
案: 映るカメラの全入力を重複なしで集め、設定順→入力 UUID 辞書順に固定する。
バッチ作成時に対象を確定し、承認で書き換えない。陳腐化した入力の失敗は既存の Err で扱う。
健康は Action ごとに最新値で再計算する。入力の個別再承認は追加しない。
参照: MC-I-1〜5、MC-AP-6。表示される入力だけに絞る裁定なら対象列の行を更新する。

### W-04 再接続の地図検査、コレクション変更、消えた manual の戻り先

同じ UUID・配信なら保持する旧規則と、新地図の戻り先検査の順序を明文化する必要がある。
案: 接続時も地図導入後に auto の戻り先を検査し、監視対象外なら C。
接続先または collection が変わった再接続は、UUID が同じでも auto/manual とも C。
同じ接続先・collection では manual の戻り先削除を理由に C しない。
OffOther の送信が NotFound なら LastError を残し、manual と UUID を保って手動選択を待つ。
接続先識別は core/obs で保持し、Machine に必要な境界情報を渡す方法は実装時に決める。
参照: MC-OC-2〜3、MC-CMD-1、MC-SQ-05〜06。

### W-05 カメラ一覧変更と地図・状態の整合

通常は再起動反映だが、型として ScenesConfigured の一覧変更をどう扱うかは未記載。
案: 残る id の状態は維持、追加は DOWN、削除は状態から除く。未知 States キーは無視、不足は DOWN。
一覧変更で古い Map の CameraID が残る場合は、設定外 id を対応なしとして扱う派生地図で計算する。
地図を再取得して追加 id の入力照合が済むまでは、そのカメラを勝手に映ると推定しない。
切断中は分類不明なので「監視対象外」を理由に C せず、再接続で検査する。
参照: U-15/0a、MC-CFG-3、SCF-9/0a、SCF-21/0a。

### W-06 同値の ItemEnabledChanged

U-14 は CamerasChanged と SceneMapChanged を挙げるが、項目の同値通知は明記しない。
案: 既知項目が同じ値なら E 以外何もしない。未知項目は地図を変えず core の記憶だけ更新する。
理由: OBS は同値設定で通知しない前提で、重複通知を再送のきっかけにしないため。
参照: MC-ITEM-9〜10。これは同じ UUID の SceneChanged を手動扱いする V-09 とは分ける。
