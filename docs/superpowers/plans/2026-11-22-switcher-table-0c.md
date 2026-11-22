# switcher「入口 × 状態」の差分表 — 区切り0c（M-0c）

- 正: [シーンごとの逃がし先 spec](../specs/2026-11-21-scene-fallback-design.md) v0.4 全節。
- 基準: [0b の表](2026-11-21-switcher-table-0b.md) 全体と [0a の表](2026-11-21-switcher-table-0a.md) 全体。
- 継承: [第1段階の表](2026-11-19-switcher-table.md) の内部状態・期待照合順。
- 補助: カメラ spec v0.17 第4〜7節、コア spec v2.16 第6章。食い違いは0cを優先。
- 対象は internal/switcher と internal/config の検証の設計。製品コード・spec は変更しない。
- Fable のレビュー後、sonnet が実装するための期待値。実装完了の報告ではない。
- 新規行は FB-、列は FB-SQ-、不変条件は FB-INV-。T はすべて SW（設定行だけ CFG）。
- 既存行の変更は元の行名に /0c を付ける。複数行の併記はテストでは個別に分ける。
- 本文にない行は「0b の表のまま」。0b が継承する0a・第1段階の行も同様。

## 0. 読み方・表記・空設定の例外

scene_fallbacks が空なら、逃がす列は従来どおり共通待機だけになる。
基本の出力は0bの表のまま。ただし以下は空設定でも新しい裁定を適用する。

| 置換対象 | 空設定でも変わる期待 |
|---|---|
| HB-SM-05/0c、HB-SQ-08/0c | [R(A),Hc(s)] を [Hc(s)] にする。非表示確認後に R(A) |
| 0b 3.3 の「承認では見ない」、復帰/OFF の AP 行 /0c | 生成時だけでなく承認時も入場条件と逆向きの待ちを検査 |
| W-04/0c、MC-SQ-05/0c の manual 別列 | 元 UUID 削除は全解除。SceneEpoch+1、期待・再送時刻を全消去、その後通常判断 |
| 待機 auto の StandbyOn 行 /0c | 期待中でも切替なしで manual 化。残存自動期待は照合用だけ |
| W-03/0c、MC-I-1〜5/0c の共有入力列 | 現 program に実表示される入力を Restart 対象から除外 |
| AP-SOFFM-3/0a/0c、MC-AP-3/0c、MC-SQ-08/0c | 承認で別候補・計画へ変われば Reject。通常の Replace{nil} で auto 化しない |
| シーン BatchDone /0c | 未完了解消後に再判断する。却下のまま次の Tick まで必ず待つ規則にはしない |
| 再送の規則 /0c | 同一要求だけ最短5秒、新候補は即時。候補別時刻を保持 |

E は全 Handle/Approve の最初。期限は At > Deadline、ちょうどなら残す。
項目 Q・P の期限、保存障壁、失敗回数、人の印は0bの表のまま。
C は owner=none、manual の戻り先なし、stack=[]。現在シーン・項目記録は変えない。
C だけではシーン期待を消さない。「手動共通解除」は C に SceneEpoch+1、
全シーン期待・全候補再送時刻の消去を加える。Pending は残し世代で却下する。
X と StartScene による Hide 抑止は0bの表のまま。待機 auto ON だけ明示的例外。
B は各バッチの採番・Pending 登録・4世代の固定。シーンの履歴は変えない。
非 Command の出力順はシーン→Show→Hide、カメラ順・項目順は0bの表のまま。

| 記号 | この文書の意味 |
|---|---|
| A / D / S | シーン1 / シーン2 / 共通待機の UUID。名前とは別 |
| B / C / E | 別の監視シーンの UUID（共通変化 B、解除 C、期限 E とは文脈で区別） |
| T | カメラなしの画像シーン UUID。画像ファイルの存在は検査しない |
| m / s | メイン standby / サブ hide。入力 im / is |
| x / y | s の項目 (A,2) / (D,1) |
| K=[A,D] | スタック。左が下、右が上。現在の退避先を含めない |
| auto/K、manual/A | owner と履歴、または manual の単独の戻り先 |
| H(U) / A(U) | 論理健康 / 実表示のカメラ集合。0bと同じ |
| OK記録(U) | OK のカメラの非表示の記録が残っている（使えないものを除く） |
| 表示0(U) | 実際の表示が0、すなわち A(U)=空 |
| 逆待(U) | 第3節定義の逆向きの待ちの対象がある |
| F(U;+L) | AutoFail[SetScene(U)]。到着で L を既存 K に追加 |
| R(U;K') | AutoRecover[RestartMedia(I(U)),SetScene(U)]。到着後 K' |
| OFF(U;K') | OFF 専用計画の切替。到着後 K'、空なら none、残れば auto |
| ON(U) | StandbyOn[SetScene(S)]。到着で manual/U、K=[] |
| Hc / Sc / MA / Q / P,H,U / u,f | 0bの表のまま。Hc/Sc の生成だけで地図は変わらない |
| Jc | 名前解決→分類・健康・実表示→削除・履歴整理→シーン判断→Show→Hide |

既定は接続・配信中、auto=true、Studio off、Stopping=false、Xなし、
未完了・期待・送出時刻なし。A は m+s、D は s だけ、S/T はカメラなし。
設定は A→D→S、m=s=OK、全項目 true。変更点は各行で明示する。
F は再起動なし。R/OFF は固定対象 I が空なら Restart と待ちを両方省く。
本表の W は1秒以上。旧表の0.5秒 fixture は製品の待ち時間の期待に使わない。
「—」「[]」「維持」は E 後の状態。省略した欄・世代は保つ。

## 1. 型・設定・status の差分

### 1.1 契約と所有権

以下のフィールド名は実装案であり、意味と写しの所有権をテストする。
既存の Handle/Approve、ReqID、保存印の API は変えない。

| 対象 | 追加・置換する形 | 検査する値 |
|---|---|---|
| config.OBS | SceneFallbacks []SceneFallback、YAML scene_fallbacks | 各要素は Scene/Fallback string、yaml scene/fallback |
| ScenesConfigured | SceneFallbacks []SceneFallback | 名前のまま。UUIDを config に保存しない |
| settings / Snapshot | 元の対応の写し、解決済み対応、Stack []string | slice/map は取り込み時・読み出し時に深く複製 |
| Batch / PendingBatch / Expectation | ArrivalPlan | 固定の追加列または切詰め位置、到着後 owner/manual 戻り先 |
| ArrivalPlan | Kind、AppendUUIDs、ReturnUUID、OwnerAfter、ManualReturnAfter、EffectsDisabled | ReturnUUID は切詰め対象自身を指す。添字の陳腐化だけに依存しない |
| OFF 専用計画 | ManualOrigin、Target、Direction、Skipped、ArrivalPlan | 通常 auto の K と別。先頭は manual 戻り先自身 |
| Expectation | 既存 ReqID・Purpose・ConfigEpoch・期限等に計画の写し | Proceed でだけ登録。Batch 側の後変更を参照しない |
| Snapshot | Stack（UUID列）と manual の単独の戻り先 | auto の ReturnScene は K[0] の派生値 |
| status.standby.stack | 下→上の現在名の配列 | auto 以外は []。現退避先は current scene |
| status.standby.return_scene | auto は最下段の現在名、manual は単独の現在名 | none はなし。改名は地図から即時反映 |

EffectsDisabled は待機 auto ON 特例などで残した期待の作用を無効にする印。
同じ ConfigEpoch でも無効化できる。照合は通常どおり消費する。
名前の未解決と UUID の削除は別の状態として保持する。
stack はプロセス内だけ。hidden.json に混ぜず、プロセス再起動で復元しない。

### 1.2 名前の解決

| # | T | 入力・前状態 | 計算結果・出力と期待 |
|---|---|---|---|
| FB-NM-01 | SW | 最初の設定、地図なし | 名前対応を保存、解決結果は不明。画像と推定せず切替なし |
| FB-NM-02 | SW | 地図更新、A名→D名が両方ある | UUID A→D を再作成。全行を引き直す |
| FB-NM-03 | SW | A名はある、D名がない、Sあり | この行の実行時だけ A→S。設定の文字列は保持、行番号・fallback 名を警告 |
| FB-NM-04 | SW | 元 A名がない | 適用元なし。scene 名と行番号を警告、別シーンに適用しない |
| FB-NM-05 | SW | 未解決 D名が後の Map に復活 | A→D に復元。ConfigEpoch は進めない |
| FB-NM-06 | SW | Sなし、A→D、Dは入れる | [F(D;+[A])]。S欠落を警告しても途中候補は使う |
| FB-NM-07 | SW | Sなし、A→D、Dも入れない | []。履歴不変、その地点で探索終了、警告 |
| FB-NM-08 | SW | Dを同UUIDで改名、設定は旧名 | 設定行は未解決扱い、履歴のDは現名で維持。名前追従しない |
| FB-NM-09 | SW | 同名Dが別UUIDで復活 | 対応は新UUID、旧履歴を名前で置換しない |
| FB-NM-10 | SW | 行なし、Sあり | 今のシーンの次はS。空設定も同じ計算経路 |

W-01 の「空の地図」はシーン一覧を保持する。nil の未取得地図とは区別する。
取得済みの一覧で存在するカメラなしシーンなら画像として扱える。
空の一覧・未取得を「カメラがない」という証拠にしない。

### 1.3 config の検証と保存時警告

行番号は人向けには1始まり、Field は既存流儀の obs.scene_fallbacks[0].scene 等。
検証のエラーは既存 ValidationError.Fields / ByField で返す。OBS接続は不要。

| # | T | 入力 | 期待 |
|---|---|---|---|
| FB-CFG-01 | CFG | 省略 / [] | 有効、空対応として同値。ひな形も読み直せる |
| FB-CFG-02 | CFG | A→D、A→T | 不正。重複した行と scene、先行行を示し、1行にまとめる修正案 |
| FB-CFG-03 | CFG | A→A | 不正。長さ1の輪 A→A と fallback の修正案 |
| FB-CFG-04 | CFG | A→D、D→B、B→A | 不正。該当行・項目、A→D→B→A の経路と輪を切る案 |
| FB-CFG-05 | CFG | scene が現在の standby_scene | 不正。scene 項目を指し、元を普通のシーンへ変える案 |
| FB-CFG-06 | CFG | A→現在の standby_scene | 有効。共通待機の明示を拒否しない |
| FB-CFG-07 | CFG | scene="" / fallback="" | 各空項目を不正。空白だけとは区別 |
| FB-CFG-08 | CFG | scene=" A "、fallback=" D " | 有効、空白を保存・解決時にも維持。trim しない |
| FB-CFG-09 | CFG | 33行以上の非循環対応 | 有効。任意の行数上限なし |
| FB-CFG-10 | CFG | on_stream_start.scene と元・先が同名 | 追加制約なし。他の検証を満たせば有効 |
| FB-CFG-11 | CFG | OBSに存在しない名前 | config として有効。存在検査は実行時の status 警告 |
| FB-CFG-12 | CFG | standby を S名→S2名、旧S名が新しい行の元/先に残る | 保存成功。warnings に各該当行・旧名と普通のシーンとして残る旨 |
| FB-CFG-13 | CFG | 同じ保存で新S2名が scene にある | ValidationError、その行の scene が箇所。警告だけで成功しない |
| FB-CFG-14 | CFG | standby名変更なし、旧名らしい文字列あり | 過去名警告なし。起動・status にも保存時警告なし |
| FB-CFG-15 | CFG | 保存後の起動 / status取得 | 過去名警告を再生成しない。未解決名警告は別途出す |
| FB-CFG-16 | CFG | 対応の並べ替えのみ | ScenesConfigured は同値、ConfigEpoch 不変、Jcなし |
| FB-CFG-17 | CFG | 対応の先を別名に変更 / 最初の設定 | ConfigEpoch+1。別名が同UUIDでも名前対応としては変化 |

保存時警告は旧設定と保存に成功した新設定を比較する経路で計算する。
現 config.Warnings() は起動用なので、過去名警告をそこに加えない。
保存 API の成功応答 warnings と保存画面に同じ内容を渡す契約とする。
画面/API自体は後続S2の範囲。ここでは検証・警告生成の入力と期待を固定する。

## 2. 内部状態と整理の差分

| 状態 | 初期値 | 更新する時点 |
|---|---|---|
| Stack | [] | 一致到着、明示解除・整理、切替不要ON/OFFだけ |
| ManualReturn | 空 | ON到着/特例、OFF到着/解除。autoとは別 |
| LastCandidateSent | 空の辞書 | バッチ生成時刻。却下・失敗だけでは削除しない |
| OFFPlan in Pending | なし | OFF生成時に専用探索結果を固定 |
| ArrivalPlan in Expectations | なし | Proceed 時に固定コピー |
| PrevConn の照合値 | 既存初期値 | 接続先・collection・現在UUID・配信状態の4条件 |

候補時刻のキーは (Purpose,ConnEpoch,StreamEpoch,SceneEpoch,ConfigEpoch,
TargetUUID,Direction,ArrivalPlan の意味的な内容)。Appendの順もキーに含める。
同じ UUID でも履歴計画が違えば別要求。候補 A→B→A と往復しても A の時刻は残す。
同じ目的の未完了・同じ行き先の未消費期待の抑止は時刻検査より先。
解除時に消すべき時刻と、単に失敗した時刻を混同しない。

| # | T | 入口・前状態 | 履歴・所有権の期待 |
|---|---|---|---|
| FB-ST-01 | SW | 新規 Machine | none、K=[]、戻り先なし。非配信で履歴なし |
| FB-ST-02 | SW | 切断、auto/[A,D] | 接続比較用に履歴保持。地図なしを削除扱いしない |
| FB-ST-03 | SW | 再接続4条件すべて同一 | 新地図で整理後、残る履歴とownerを保ちJc |
| FB-ST-04 | SW | 4条件のどれか不一致 | C。今のシーンから通常判断、過去履歴を再利用しない |
| FB-ST-05 | SW | 配信停止 | C、全シーン期待・候補時刻消去。項目記録/Q/Showは独立 |
| FB-ST-06 | SW | fallbacksのみ変更、auto/[A,D] | 到達可能性を見ず保持、以後は新対応。ConfigEpoch+1 |
| FB-ST-07 | SW | auto_enabled=false、auto | C、auto生成/承認なし。Show再試行は継続 |
| FB-ST-08 | SW | auto_enabled=false、manual/A | manual/A保持、K=[]。OFFは存在だけ要する |
| FB-ST-09 | SW | 人のSceneChanged、履歴内を選んだ場合も | 手動共通解除。そのシーンが悪ければ新規退避 |
| FB-ST-10 | SW | 下Aまたはmanual戻り先Aが削除 | 手動共通解除。全解除後にJc、抑止印を作らない |
| FB-ST-11 | SW | [A,D,B] の途中D削除 | [A,B]。SceneEpoch不変、残る履歴から判断 |
| FB-ST-12 | SW | Aが存在するが管理外化、auto/[A,D] | Aだけ外し[D]。下をDに。空になればnone |
| FB-ST-13 | SW | manual/A、Aは存在し管理外化 | manual/A維持。OFFではOtherへ直接戻れる |
| FB-ST-14 | SW | 待機名/UUIDの変更・消失 | auto/manualともC。旧期待は照合用だけ、作用を無効化 |
| FB-ST-15 | SW | 改名のみ、UUID不変 | 履歴維持、statusは新名。名前対応の再解決は別処理 |
| FB-ST-16 | SW | 整理時 K に現在UUIDが混入 | 現在要素を除く。空ならnone、同じ復帰の永久再送なし |

削除（存在しない）を管理外化（存在する）より先に判定する。
manual は現在待機・単独戻り先・空stack。auto は現在が途中/画像/待機でもよい。
auto の K は非空。none は空。非配信は常に空。
重複UUIDの整理は古い下側を残す安定重複排除とする（内部防御、C-02）。

## 3. 共通の計算規則

### 3.1 純粋な「入ってよい」関数

提案インターフェースは CanEnter(snapshot,candidateUUID,direction) →
{Allowed,Reason,Terminal}。snapshot は地図、健康入力、Hidden、u/f、
Studio値、有効動作、Proceed済みfalseを含む。状態変更・時計・OBS要求なし。
実装の classOf は存在しないUUIDにもOtherを返すので、存在検査を必ず先に置く。

| # | 地図で存在 | 分類 | 健康 | OK のカメラの非表示の記録が残っている | 実際の表示が0 | 方向・逆待 | 出力 |
|---|---|---|---|---|---|---|---|
| FB-EN-01 | 不明/なし | 任意 | 任意 | 任意 | 任意 | 任意 | false、Unknown/Missing |
| FB-EN-02 | あり | 監視 | LOW/DOWN | 任意 | 任意 | 任意 | false、BadHealth |
| FB-EN-03 | あり | 監視 | OK | true | false/true | 任意 | false、WaitingShow |
| FB-EN-04 | あり | 監視 | OK | false | true | 任意 | false、NoActualCamera |
| FB-EN-05 | あり | 監視 | OK | false | false | 逃がす、逆待あり | true、非終端 |
| FB-EN-06 | あり | 監視 | OK | false | false | 戻る、逆待あり | false、WaitingHide |
| FB-EN-07 | あり | 監視 | OK | false | false | 戻る、逆待なし | true、非終端 |
| FB-EN-08 | あり | 画像 | なし | 対象外 | 対象外 | 任意 | true、終端。設定行があっても先へ進まない |
| FB-EN-09 | あり | 待機 | 見ない | 対象外 | 対象外 | 任意 | true、終端 |

監視対象は本体の非表示記録も含むV。実表示はAを使う。仮想表示と混同しない。
OK記録は候補に映るOKカメラの本体記録全体を見る（0b H-08）、使えないものを除く。
「旧 waitingHidden の合成条件」をそのまま片方の列名で呼ばない。

| # | 戻る先の悪いカメラ | 有効動作 | 実表示 | その他 | 逆待の対象 |
|---|---|---|---|---|---|
| FB-RW-01 | LOW/DOWN | hide | あり | Hide有効 | 対象。false確認まで待つ |
| FB-RW-02 | LOW/DOWN | hide | あり | 今を空にするためだけHide不能 | 対象。現在が悪ければ先に逃がす |
| FB-RW-03 | LOW/DOWN | hide | あり | u=true | 対象外、人の表示判断を優先 |
| FB-RW-04 | LOW/DOWN | hide | あり | f>=3 | 対象外、使えない警告 |
| FB-RW-05 | LOW/DOWN | standby/ignore | あり | Studio読み替え後 | 対象外。ただしstandbyの健康不良で入れない場合あり |
| FB-RW-06 | LOW/DOWN | hide | なし | falseが実表示計算に反映済み | 対象外 |
| FB-RW-07 | OK | hide | あり | 任意 | 対象外。OK記録は別に検査 |

### 3.2 探索と計画

1. auto なら K を下から走査し、戻る方向で最初に入れる要素を選ぶ。
2. 下がOKでもShow待ち・表示0・逆待なら上も調べる。健康だけで固定しない。
3. 戻れる候補があれば R。選んだ要素自身と上を捨てる計画を固定する。
4. 戻れず、現在が不良の監視対象でowner=none/autoなら逃がす探索。
5. 列は現在自身を含めず、対応先から。指定なし/未解決行は共通待機へ。
6. 入れない監視候補は飛ばす。到着で現在と飛ばした監視対象を順に追加する計画。
7. 履歴内候補は戻る方向で検査。入れれば復帰、入れなければ次へ進む。
8. 履歴内候補を積み直さない。訪問済みUUIDで有限停止、到着先・共通待機を積まない。
9. 画像/待機は終端。現在が画像/待機なら自動退避せず履歴復帰だけ。
10. manual では通常auto探索なし。OFFは別の入口から別の計画を作る。

| # | 前状態・候補の可否 | 結果と到着後 |
|---|---|---|
| FB-EX-01 | A不良、D入れる | F(D;+[A])、auto/[A] |
| FB-EX-02 | A不良、D不良、S入れる | F(S;+[A,D])、auto/[A,D] |
| FB-EX-03 | D健康OKだがOK記録あり/表示0 | Dを飛ばしてF(S;+[A,D])。準備後にDへ戻れる |
| FB-EX-04 | auto/[A,D]、S、A不可、D可 | R(D;[A]) |
| FB-EX-05 | auto/[A,D]、S、A可、D可 | R(A;[])、none。最上段優先にはしない |
| FB-EX-06 | auto/[A,D]、B不良、A戻り待ち、D不可、B→T | F(T;+[B])。戻り待ちでも退避 |
| FB-EX-07 | auto/[A,B]、C不良、C→B→D→S、A/B不可、D可 | F(D;+[C])、到着で[A,B,C]。B遭遇で止めない |
| FB-EX-08 | 同上、Bが戻る方向で可 | R(B;[A])、Bを逃がす扱いにしない |
| FB-EX-09 | A不良、A→T、T→Dという設定 | F(T;+[A])。TからDへ進まず復帰待ち |

### 3.3 Studio Mode と判断順

有効動作と健康の算出式は0bの表のまま。設定 hide だけを読み替える。

| Studio値 | 有効hideカメラの扱い | 0cの差分 |
|---|---|---|
| off | hide | 逆待あり。今が悪ければ待たず逃がす |
| on/standby | standby | 不調を健康で数え個別列へ逃がす。逆待なし |
| on/ignore | ignore | 新規Hide/不調として数えることなし。OK記録・表示0検査は残る |
| on/hide | hide | 通常の逆待。複製ありの実機制約は0bのまま |

ignore で健康OKになっても表示0なら入れない。下が不可なら上を調べる。
onだけではShowしないが、OK/停止/auto=false等のShow理由は0bの表のまま。
承認と SetScene 直前も同じ snapshot 計算を使う。

## 4. 入口ごとの差分表

### 4.1 CamerasChanged

| # | T | 前状態 → 入力 | 出す列 | 状態変化・期待 |
|---|---|---|---|---|
| FB-CC-01 | SW | A、m悪化、s OK | [F(D;+[A])] | 到着前none/K=[]、BadCamera=m |
| FB-CC-02 | SW | auto/[A]、D、s悪化 | [F(S;+[D])] | 到着前[A]、到着で[A,D] |
| FB-CC-03 | SW | auto/[A,D]、S、sのみ回復、x/y H | [Sc(s)] | Show確認までRなし。K保持 |
| FB-CC-04 | SW | auto/[A,D]、S、mのみ回復、s不良表示 | [Hc(s)] | A健康OKでも逆待。false確認後R(A;[]) |
| FB-CC-05 | SW | auto/[A,D]、S、m/s同時回復、記録なし | [R(A;[])] | 下からAを選ぶ。Dを経由しない |
| FB-CC-06 | SW | auto/[A]、D不良、Aは逆待 | [F(S;+[D])] | 相互待ちを解消する退避を先送りしない |
| FB-CC-07 | SW | A、sのみ悪化、m OK | [Hc(s)] | A健康OK、シーン切替なし、K=[] |
| FB-CC-08 | SW | manual/A、m/s変化 | 項目列のみ | 自動シーン切替なし、manual保持 |
| FB-CC-09 | SW | 同値の全組 | [] | E以外なし。未知id/不足DOWNの正規化は0bのまま |

### 4.2 SceneChanged と SceneRequestDone

期待照合はE後、登録順に最初の一致を選び、一致とその前、および後ろの同じ行き先の期待をまとめて消す（V-02）。
一致した1件の計画だけを適用し、名前解決・整理・通常判断へ進む。

| # | T | 前状態 → 入力 | 出す列 | 状態変化・期待 |
|---|---|---|---|---|
| FB-SC-01 | SW | F(D;+[A])の期待→D到着 | [] | auto/[A]、ReturnScene=A、SceneEpoch不変 |
| FB-SC-02 | SW | [A,D]、R(D;[A])期待→D | [] | [A]、auto。D自身を残さない |
| FB-SC-03 | SW | [A,D]、R(A;[])期待→A | [] | C。到着でだけ解除 |
| FB-SC-04 | SW | ON(A)期待→S | [] | manual/A、K=[]。元auto履歴を破棄 |
| FB-SC-05 | SW | 応答前の一致到着 | 同じJc結果 | 計画1回適用。後のSRDは履歴へ無作用 |
| FB-SC-06 | SW | 期待が [Dへの追加、Sへの追加]、S到着 | 通常判断の列 | Sの計画だけ適用。Dの計画を先に積まない |
| FB-SC-07 | SW | 同じD宛の期待2件、D到着 | 通常判断の列 | 最初の一致だけの計画を適用、後のD期待も消す（V-02） |
| FB-SC-08 | SW | 消費済みDの重複イベント、他のD期待なし | 通常判断の列 | 不一致なので手動共通解除。重複を本体到着と推定しない |
| FB-SC-09 | SW | 古いConfigEpochの退避/ON期待→一致 | 通常判断の列 | 照合だけ、追加・所有権設定なし。新規履歴を復活しない |
| FB-SC-10 | SW | 古いConfigEpochの復帰/OFF期待→一致 | 通常判断の列 | C。古い部分切詰めを使わない（V-07） |
| FB-SC-11 | SW | ON特例でEffectsDisabledの自動期待→一致 | [] | 照合のみ、manualと戻り先を上書きしない、K=[] |
| FB-SC-12 | SW | 人がB不良へ切替、自動有効、B→S | [F(S;+[B])] | 手動共通解除後、到着前none、到着でauto/[B] |
| FB-SC-13 | SW | 人がS/T/健康なAへ切替 | [] | 手動共通解除、回復しても旧履歴へ戻らない |
| FB-SRD-01 | SW | SRD OK/Timeout | [] | 期限式は0bの表のまま。履歴は未確定 |
| FB-SRD-02 | SW | SRD Error | [] | 該当期待消去、履歴不変。同一要求の時刻保持 |
| FB-SRD-03 | SW | 到着で消費済みReqIDの結果 | [] | 無視、二重適用なし |

### 4.3 SceneMapChanged

全行で名前を引き直す。ConfigEpochは地図だけでは進めない。
同値地図はEのみ。ただし既存の初回照合・項目処理の例外は0bの表のまま。

| # | T | 前状態 → 入力 | 出す列 | 状態変化・期待 |
|---|---|---|---|---|
| FB-MP-01 | SW | A不良、D名復活、D入れる | [F(D;+[A])] | 新解決を使用、設定文字列は維持 |
| FB-MP-02 | SW | auto/[A,D]、S、A削除 | [] | 手動共通解除、SceneEpoch+1、期待/時刻全消去 |
| FB-MP-03 | SW | auto/[A]、B不良、B→D、A削除 | [F(D;+[B])] | 解除後同じ入口で新規判断。到着で[B] |
| MC-SQ-05/0c | SW | manual/A、S、A削除 | [] | manualも全解除。旧OffをReject、復活した同名を戻り先にしない |
| FB-MP-04 | SW | [A,D,B]、T、途中D削除、A/B不可 | [] | [A,B]、auto維持。全解除しない |
| FB-MP-05 | SW | [A,D]、S、A管理外化、D不良 | [] | [D]、auto。Aへ戻さない |
| FB-MP-06 | SW | [A]、S、A管理外化 | [] | none/K=[]。削除でないので手動共通解除なし |
| FB-MP-07 | SW | manual/A、S、A管理外化 | [] | manual/A、OFFでOtherとして戻す |
| FB-MP-08 | SW | 現D削除、OBSがTへ移動 | SceneChanged(T)で通常判断 | 外部切替として共通解除。Mapだけで架空の到着を作らない |
| FB-MP-09 | SW | D改名、UUID不変、K内D | 通常判断の列 | 履歴維持、名前対応は引き直し。SceneEpoch不変 |
| FB-MP-10 | SW | 共通待機S消失、auto/manual | 通常判断の列 | C、旧期待は照合用。途中候補が入れれば新規退避可 |
| FB-MP-11 | SW | 共有末端のfalseでR先表示0 | [] | R生成なし。承認待ちRもReject |

### 4.4 ItemEnabledChanged / ItemRequestDone

項目照合・保存・Qの消費規則を変えず、その結果でJcを行う。
Hide/Showは履歴の有無に依存せず続ける。

| # | T | 前状態 → 入力 | 出す列 | 状態変化・期待 |
|---|---|---|---|---|
| FB-IE-01 | SW | S、auto/[A,D]、m OK/s不良、最後の逆待x false確認 | [R(A;[])] | false実表示を反映、Kは到着まで保持 |
| FB-IE-02 | SW | S、auto/[A,D]、m不良/s OK、最後のOK記録true確認 | [R(D;[A])] | Show確認を経てから復帰 |
| FB-IE-03 | SW | 同上、true応答だけ、地図falseのまま | [] | OK記録/表示0が残る間はRなし |
| FB-IE-04 | SW | 応答前falseイベントで逆待解消、Hc未完了 | [R(A;[])] | 項目Pending自体はシーン復帰の抑止ではない |
| FB-IE-05 | SW | 不一致true、人が不調sを表示に戻す | [R(A;[])]（他条件成立時） | u(s)=trueで逆待対象外。新規Hideなし |
| FB-IR-01 | SW | ItemOK、0bの決着処理で待ちが最後に解消 | [R(A;[])] | 記録/実表示の更新を確認してから判断 |
| FB-IR-02 | SW | ItemError/NotFound、待ちは解消しない | [] | 履歴不変、失敗/警告処理は0bの表のまま |
| FB-IR-03 | SW | 切断後のItem結果 | [] | 台帳で0b処理、接続がないのでシーン生成なし |

### 4.5 StudioModeChanged / 接続 / 配信

| # | T | 前状態 → 入力 | 出す列 | 状態変化・期待 |
|---|---|---|---|---|
| HB-SM-05/0c | SW | S、auto/[A]、on/standby→off、s不良表示 | [Hc(s)] | A健康OKでも逆待。false確認後R(A;[]) |
| FB-SM-01 | SW | D、auto/[A]、s不良、off→on/standby | [F(S;+[D])] | 有効standbyの不調を数える |
| FB-SM-02 | SW | S、auto/[A,D]、on/ignore、A表示0、D入れる | [R(D;[A])] | ignoreによる健康OKだけでAを選ばない |
| FB-SM-03 | SW | S、auto/[A]、on/hide、s不良表示 | [Hc(s)] | false確認後R。offと同じ逆待 |
| FB-SM-04 | SW | Studio off時のOBSの一瞬のprogram変化 | [] | obs層で遮断しMachineへ流さない。K保持 |
| FB-OD-01 | SW | auto/[A,D]→OBSDisconnected | [] | ConnEpoch+1、シーン期待消去、履歴は比較用保持 |
| FB-OC-01 | SW | 同一4条件、Sで再接続、D可/A不可 | [R(D;[A])] | 新地図で整理してから判断。StartSceneなし |
| FB-OC-02 | SW | collection/接続先不一致、UUID同じ | 通常判断の列 | C。旧バッチはConnEpochでReject |
| FB-OC-03 | SW | 現在UUID/配信状態不一致 | 通常判断の列 | C。配信差ならStreamEpoch+1。再接続で開始時切替なし |
| FB-OC-04 | SW | 同一4条件、新地図で最下段削除 | 通常判断の列 | 手動共通解除を追加。SceneEpoch+1 |
| FB-SS-01 | SW | 配信false→true、開始時A設定、A不良 | [StartScene(A)] | StartScene未完了/期待中はAuto/Hideなし |
| FB-SS-02 | SW | StartScene(A)一致、BatchDone済み、A不良 | [F(D;+[A])] | 到着後通常判断、F到着で初めて[A] |
| FB-SS-03 | SW | StartScene到着がBatchDoneより先 | []（シーン） | PendingによるXを保持、BatchDone解消後に判断 |
| FB-SS-04 | SW | 配信停止、auto/[A,D] | Showの列のみ | C、シーン期待/時刻消去、現在そのまま |
| FB-SS-05 | SW | 非配信のCamerasChanged/Tick | Showの列のみ | stack空、自動シーン切替なし |

### 4.6 ScenesConfigured

| # | T | 前状態 → 入力 | 出す列 | 状態変化・期待 |
|---|---|---|---|---|
| FB-CF-01 | SW | auto/[A,B]、C不良、対応をC→B→Dへ変更、A/B不可 | [F(D;+[C])] | ConfigEpoch+1、既存[A,B]維持 |
| FB-CF-02 | SW | auto/[A,D]、対応からA/Dが到達不能になる | 通常判断の列 | 履歴を到達性で捨てない |
| FB-CF-03 | SW | 並べ替えだけ | [] | 同値、ConfigEpoch/履歴/再送時刻不変 |
| FB-CF-04 | SW | auto/[A,D]、auto=false | Sc等の列のみ | C。旧自動期待は照合だけ、新規autoなし |
| FB-CF-05 | SW | manual/A、auto=false | Sc等の列のみ | manual保持。古いOFFはConfigEpochでReject |
| FB-CF-06 | SW | standby名変更 | 通常判断の列 | C、履歴破棄。fallbacksだけの変更と分ける |
| FB-CF-07 | SW | 切断中の設定変更 | [] | 未取得地図で削除/管理外化としない。明示解除条件は適用 |
| FB-CF-08 | SW | カメラ削除でauto履歴の一部が管理外化 | 通常判断の列 | その要素だけ外す。W-05の状態組整合は0bの表のまま |

### 4.7 Command

Command は通常Jcを行わず、受付と固定計画の生成だけ。拒否でもEは先。
シーンバッチを返さないON/OFFは受付と承認を同一状態で完結させる。

| # | T | 前状態 → 入力 | 出す列 / cmdErr | 状態変化・期待 |
|---|---|---|---|---|
| FB-ON-01 | SW | D、auto/[A]→ON | [ON(A)] / nil | 到着までauto/[A]。到着でmanual/A、K=[] |
| FB-ON-02 | SW | T、auto/[A,D]→ON | [ON(A)] / nil | 最下段Aを保存。Tを戻り先にしない |
| FB-ON-03 | SW | T、none→ON | [ON(T)] / nil | 到着でmanual/T、K=[] |
| FB-ON-04 | SW | S、auto/[A,D]、期待あり→ON | [] / nil（200） | 同一状態で承認しmanual/A、K=[]、自動期待の作用を無効化 |
| FB-ON-05 | SW | D、auto/[A]、切替期待/未完了あり→ON | [] / ErrInvalidState | S上auto以外は従来の拒否を維持 |
| FB-ON-06 | SW | S、none または manual→ON | [] / ErrInvalidState | 変化なし |
| FB-ON-07 | SW | 未接続 / 非配信→ON | [] / ErrNotConnected または ErrInvalidState | 順は従来どおり。所有権変更なし |
| FB-ON-08 | SW | S名解決不能→ON | [] / ErrInvalidState | 警告、履歴不変 |
| FB-OFF-01 | SW | manual/A、A戻る方向で可→OFF | [OFF(A;[])] / nil | 到着でnone。ON前の途中履歴は復活しない |
| FB-OFF-02 | SW | manual/A、A不可、D逃がす方向で可→OFF | [OFF(D;[A])] / nil | 到着でauto/[A]。OFF専用計画 |
| FB-OFF-03 | SW | manual/A、A/D不可、Sが最初の可候補→OFF | [] / nil（200） | 切替なしでauto/[A,D]確定、manual戻り先を消す |
| FB-OFF-04 | SW | manual/A、A不可、途中T可→OFF | [OFF(T;[A])] / nil | Tは終端、到着でauto/[A] |
| FB-OFF-05 | SW | manual/T、T存在→OFF | StandbyOffOther[SetScene(T)] / nil | 再起動なし、到着でC |
| FB-OFF-06 | SW | manual/A、auto=false、A不良/表示0/待ちあり→OFF | [OFF(A;[])] / nil | 入場健康条件を無視、存在・世代は検査、到着でC |
| FB-OFF-07 | SW | manual/A、先の名前未解決、Sも引けない→OFF | [] / nil（案C-01/C-05） | 警告、manual/A維持、押し直せる。探索範囲を勝手にautoへ広げない |
| FB-OFF-08 | SW | manual/A、A名は引けず別名になったがUUIDは存在 | [OFF(A;[])]（可なら） / nil | 戻り先自身はUUID履歴。後続だけ名前対応を使用 |
| FB-OFF-09 | SW | manual/A、A UUID削除済み | [] / ErrInvalidState | 整理で全解除済み。存在しない元へ強制OFFしない |
| FB-MA-01 | SW | auto履歴/復帰待ち中→すべて表示に戻す | [MA] / nil | 0bの対象・保存規則、stackは受付で変えない |
| FB-MA-02 | SW | MAのtrue確認で最後の待ち解消 | JcによるR等 | 確認後だけ通常復帰。MAが履歴を直接消さない |

FB-OFF-07はspecの要求行として残すが、待機消失で先にCになる規則との到達可能性はC-05。
現在待機が候補となるOFFはSetScene(S)のSkipに流さず明示的所有権変更とする。

### 4.8 Tick・Approve・BatchDone

| # | T | 前状態 → 入力 | 出す列 | 状態変化・期待 |
|---|---|---|---|---|
| FB-TK-01 | SW | 同じF候補/計画、送出t=0、失敗済み、t=4.999 | [] | 時刻保持、履歴未確定 |
| FB-TK-02 | SW | 同上t=5 | 同じF | 5秒ちょうどで可、時刻更新 |
| FB-TK-03 | SW | t=1に別行き先/別計画が最善、未完了/期待なし | 新候補のF/R | 旧候補の時刻で待たない |
| FB-TK-04 | SW | 候補D→S→Dに戻る、Dの前回から5秒未満 | [] | 候補別時刻が残り迂回再送しない |
| FB-TK-05 | SW | 新候補でも同目的Pendingあり/同宛先期待あり | [] | 抑止維持。別候補なら並行送信してよいとはしない |
| FB-TK-06 | SW | 期待期限ちょうど / 超過 | 保持 / E後にJc | 期限切れ自体は履歴変更なし |
| FB-AP-01 | SW | F(D;+[A])、最新の探索結果も同じ | Proceed | SetSceneの期待に計画コピー、履歴不変 |
| FB-AP-02 | SW | D悪化で最新候補Sへ変化 | Reject | バッチを書き換えず、期待なし。BatchDone後再判断 |
| FB-AP-03 | SW | 行き先同じでも追加列/方向/切詰めが変わる | Reject | UUID一致だけで承認しない |
| FB-AP-04 | SW | R先健康OK、OK記録/表示0/逆待が発生 | Reject | Restart後のSetScene直前にも同じ検査 |
| FB-AP-05 | SW | Conn/Stream/Scene/ConfigEpochのいずれか古い | Reject | 既存共通条件を先に検査、期待を増やさない |
| FB-AP-06 | SW | Autoの承認、auto=false / 非配信 / X | Reject | 履歴不変、時刻は失敗だけで消さない |
| FB-AP-07 | SW | OFF計画が変化、現在Sが新最善 | Reject | 承認でauto化へReplaceしない。押し直しは専用探索 |
| FB-AP-08 | SW | ON、最下段/待機UUID/受付条件が変化 | Reject | manual化しない、LastErrorはBDで処理 |
| FB-AP-09 | SW | SetScene対象が現在UUID、明示ON/OFF特例以外 | Skip | 期待なし、到着とみなさず計画未適用。整理で現在混入を除く |
| FB-AP-10 | SW | Rの固定Restart入力が今programに実表示 | Skip（当該入力） | 新規入力を追加しない、残る固定対象とSetSceneへ進む |
| FB-AP-11 | SW | 固定Restart入力が今非表示、共通/目的条件成立 | Proceed | 設定順→UUID順、再起動後W>=1秒。待ち後再承認 |
| FB-AP-12 | SW | FまたはOFFの逃がす方向 | SetSceneのみ | Restartを付けない。戻り先自身へ戻るOFFはRと同じ除外 |
| FB-BD-01 | SW | Sceneバッチ Completed、到着未着 | 抑止とJc次第 | Pendingだけ外す、期待は残す、履歴未確定 |
| FB-BD-02 | SW | Auto Failed/Rejected | 抑止/5秒条件を満たす新候補だけ | 履歴不変、同一候補時刻保持、Auto失敗を手動エラーにしない |
| FB-BD-03 | SW | ON/OFF Failed/Rejected/期限切れ | 自動判断はowner等に従う | 未到着なら元の履歴/所有権保持、LastErrorで押し直し案内 |
| FB-BD-04 | SW | 項目BDでfが3に到達、逆待の対象が消える | JcのR等 | 使えない警告、項目記録は0bの表のまま保持 |
| FB-BD-05 | SW | StartScene未完了が解消、期待もなし、A不良 | [F(D;+[A])] | StartScene再送なし、通常退避だけ |

Approveは自分自身のPendingや再送時刻を理由に却下しない。
固定RestartMediaが複数入力を含む場合、入力ごとのSkipを実行役へ渡す方法はC-03。
生成時のIはW-03の対象から現programの実表示入力を引いたもの。
共有カメラ名だけで除外せず、入力UUID単位で入れ子の有効経路も数える。
ShowCameraの既存再起動にも配信中入力を再起動しない保証を適用する（C-03）。

## 5. 非同期の列のケース

各列は独立。tは秒。行に書く「承認」は各Action直前のApproveを指す。
SCはSceneChanged、SRDはSceneRequestDone、BDはBatchDone、IRはItemRequestDone。
「scene決着」はSetScene承認Proceed→SRD OK→BD Completed→SC一致の順。
「hide決着」は各false承認（保存障壁成功）→IR OK→false項目イベント→BD。
「show決着」は必要な固定Restartを承認・実行→W>=1秒→各true承認→IR OK→trueイベント→BD。
これらはテストヘルパーに展開し、応答・イベント・待ちを省略しない。
特に記した列では順序を逆にし、期待登録・一度だけの作用を確認する。

### FB-SQ-01 spec例1〜4：メイン悪化、サブ悪化、順に回復

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | A、全OK、K=[] | [] | none |
| 1 | m DOWN | [F(D;+[A])] | K=[]、再起動なし |
| 1.1 | scene決着(D) | [] | D、auto/[A] |
| 2 | s DOWN | [F(S;+[D])] | Dを空にするHideは出さない |
| 2.1 | scene決着(S) | [Hc(s)] | S、auto/[A,D]、ここで隠せる |
| 2.2 | hide決着(x,y) | [] | x/y H、false、復帰なし |
| 8 | s OK | [Sc(s)] | D健康OKだが表示0、Rなし |
| 8.1〜9.2 | show決着(x,y) | [R(D;[A])]（最後の待ち解消時） | Kは[A,D]のまま |
| 9.3 | R(D)のSetScene承認、scene決着 | [] | D、auto/[A]。isはhideなので復帰Rで再起動しない |
| 10 | m OK | [R(A;[])] | I=[im]、isは含めない |
| 10.1〜11.3 | Restart承認・W後SetScene承認、scene決着(A) | [] | A、none、K=[] |

### FB-SQ-02 spec例5：待機中に同時回復

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | S、auto/[A,D]、m/s DOWN、x/y H | [] | 開始状態 |
| 6 | m/s同時OK | [Sc(s)] | A/DともOKでもOK記録あり |
| 6.1〜7.2 | show決着(x,y) | [R(A;[])] | 下Aへ直接。DへのRなし |
| 7.3〜8.5 | R(A)のRestart/W/scene決着 | [] | A、none、K=[] |

### FB-SQ-03 spec例6：同時悪化で途中を飛ばす

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | A、全OK | [] | none/K=[] |
| 1 | m/s同時DOWN | [F(S;+[A,D]),Hc(s)] | D不良で飛ばす。Aにはmが残るのでxは隠せる |
| 1.1 | FのSetScene承認 | Proceed | 期待登録、K=[] |
| 1.2 | Hcのfalse承認/保存/IR/イベント/BD | [] | 保護対象Sは待機、空検査を維持 |
| 1.3 | FのSRD/BD/SC(S) | 必要な残りHcのみ | S、auto/[A,D]。実在しないD到着を作らない |

### FB-SQ-04 spec例7：メインだけ回復、逆向きの待ち

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | S、auto/[A,D]、m/s DOWN、x/y true、Hc送出時刻なし | [] | 前提として退避直後の短い露出 |
| 1 | m OK、s DOWN | [Hc(s)] | A健康OK、逆待あり、Rなし |
| 1.1 | false承認、保存、IR OK（イベント未着） | 0bの決着結果に従う | 実表示/待ちが残ればRなし |
| 1.2 | x/y false確認、最後の待ち解消 | [R(A;[])] | Kは到着まで[A,D] |
| 1.3〜2.5 | R(A)のRestart/W/scene決着 | [] | A、none。sは非表示のまま |

別列a: x/yにu(s)=trueならHcなし、逆待なしでR(A;[])。
別列b: f(s)>=3ならHcなし、警告。健康/OK記録/実表示の他条件が成立するときR。
別列c: 現Dが不良でxのHideが今を空にする共有経路なら、逆待から除外せずF(S)を先に行う。

### FB-SQ-05 サブだけ悪化（spec「隠すとの関係」）

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | A、m OK、s DOWN | [Hc(s)] | 健康OK、K=[]、シーン維持 |
| 1.1 | hide決着(x,y) | [] | x/y H。Aのmは表示 |
| 7 | s OK | [Sc(s)] | シーン切替なし |
| 7.1〜8.2 | show決着 | [] | A、none/K=[] |

### FB-SQ-06 履歴と新しい列の重なり

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | auto/[A,B]、現在C不良、A/B不可、D可 | [] | 前提 |
| 1 | 設定C→B→D→S | [F(D;+[C])] | ConfigEpoch+1、K=[A,B] |
| 1.1 | Approve、最新もA/B不可D可 | Proceed | 期待固定、Bを追加しない |
| 1.2 | SRD/BD | [] | K=[A,B] |
| 1.3 | SC(D)一致 | [] | auto/[A,B,C] |
| 7 | A不可、Bが戻る方向で可 | [R(B;[A])] | 下から最初の可候補B |
| 7.1〜8.3 | Rのscene決着 | [] | B、auto/[A] |

別列: t=1でBが可ならFでなくR(B;[A])。到着で[A]。
別列: 設定でCからA/Bへ到達不能にしても過去履歴はそのまま。

### FB-SQ-07 削除・期待・遅着（同じConfigEpoch）

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | B、auto/[A]、B不良、B→C、C可 | [F(C;+[B])] | ConfigEpoch=e |
| 0.1 | SetScene(C)承認Proceed | — | 期待登録、K=[A] |
| 0.2 | MapでA削除 | 同目的Pendingがある間[] | SceneEpoch+1、全期待/時刻消去、none/K=[]、ConfigEpoch=e |
| 0.3 | 別の未承認旧バッチをApprove | Reject | 古いSceneEpoch、期待なし |
| 0.4 | 旧FのBD | 新規判断のF（B不良が続く場合） | 新規計画の最下段はB、古いAなし |
| 0.5 | 送信済みCの遅着SC | Cが不良なら新規F、可なら[] | 期待なしなので手動共通解除、旧計画復活なし |

別列a: manual/Aも削除で全解除。Sなので通常auto退避なし。
別列b: auto=falseならB/C不良でも新規Fなし。
別列c: 途中要素だけ削除は残り保持、SceneEpoch不変。旧計画との差は承認でReject。

### FB-SQ-08 ON特例の受付・承認・遅着

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | 直接状態: S、auto/[A]、未消費自動期待あり | — | V-13方式、接続配信中 |
| 1 | ONの受付と同一状態で承認 | []、成功 | manual/A、K=[]、期待は作用無効化して残す |
| 1.1 | 残存自動期待と一致するSC(S) | [] | 照合消費のみ。manual/A維持、K復活なし |
| 2 | カメラ全OK | 項目Showだけ | OFFなしでは自動復帰なし |
| 3 | OFF | [OFF(A;[])] | 専用計画、到着までmanual/A |
| 3.1〜4.3 | scene決着(A) | [] | none/K=[] |

別列: DやTにいるautoは期待中ONを拒否。期待なしならON(A)を生成する。

### FB-SQ-09 最低滞在時間なしで2段切替

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | Aでm DOWN、s OK | [F(D;+[A])] | 新候補D |
| 1.1 | scene決着(D) | [] | auto/[A] |
| 2 | s DOWN | [F(S;+[D])] | 前のFから5秒未満でも新要求。滞在待ちなし |
| 2.1 | scene決着(S) | [Hc(s)] | auto/[A,D] |

### FB-SQ-10 画像終端・OFF専用範囲

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | A→T→D、A不良 | [F(T;+[A])] | Tに設定行があっても終端 |
| 0.1 | scene決着(T) | [] | auto/[A] |
| 1 | ON | [ON(A)] | 戻り先はTではなくA |
| 1.1 | scene決着(S) | [] | manual/A、K=[] |
| 2 | OFF、A不可/T可 | [OFF(T;[A])] | Tで止める、Dまで探索しない |
| 2.1 | scene決着(T) | [] | auto/[A]、ON前の履歴を合成しない |
| 3 | A回復、準備完了 | [R(A;[])] | Tからも復帰 |
| 3.1〜4.3 | scene決着(A) | [] | none/K=[] |

### FB-SQ-11 Studio ignore・表示0・上位候補

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | S、auto/[A,D]、Aは本体記録で監視対象だが実表示0、Dは実表示あり | — | Aのカメラはhide、f>=3でOK記録対象外という独立fixture |
| 1 | Studio on/ignore | []（Dも戻れない場合） | A健康OKでも表示0で不可、noneへ早期解除しない |
| 2 | Dの準備完了（Item/Map） | [R(D;[A])] | 下A不可なので上Dへ |
| 2.1 | scene決着(D) | [] | auto/[A] |
| 8 | Aのtrue表示確認・OK記録なし | [R(A;[])] | 準備後に下へ戻る |

別列: 履歴が[A]だけでA表示0ならSに残る。ignoreは空入場の許可ではない。

### FB-SQ-12 再接続・auto=false・開始時切替

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | D、auto/[A]、m不良、s OK | [] | 開始状態 |
| 1 | OBSDisconnected | [] | K保持、ConnEpoch+1、期待消去 |
| 2 | 同じ4条件でOBSConnected | [] | 新地図で[A]保持、StartSceneなし |
| 3 | ScenesConfigured auto=false | 必要なSc | C、Dに残る |
| 4 | m OK | [] | 旧Aへ戻らない |
| 5 | auto=true | [] | D健康OK、過去履歴を復活しない |
| 6 | 配信停止 | 必要なSc | シーン期待/時刻消去、K=[] |
| 7 | 配信開始、on_stream_start=A | [StartScene(A)] | A不良でもX中Auto/Hideなし |
| 7.1 | SC(A)を先に受ける | [] | まだStartScene Pending |
| 7.2 | SRD/BD、A不良 | [F(D;+[A])] | StartScene再送なし |

別列: 再接続時の4条件を1つずつ変え、すべてCを検査。collection差はUUID同一でも解除。

### FB-SQ-13 承認前後・応答順・期限・再送

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | A不良、D可→F(D)生成 | [F(D;+[A])] | K=[] |
| 0.1 | D悪化、F(D)承認 | Reject | 期待なし、K=[] |
| 0.2 | BD Rejected | [F(S;+[A,D])] | 新候補は即時 |
| 0.3 | F(S)承認Proceed | — | 期待固定 |
| 0.4 | D回復、SC(S)がSRDより先 | 通常判断（Pending抑止を除く） | 送出計画を一度だけ適用、承認後変化で期待を捨てない |
| 0.5 | SRD/BD | R(D)等の通常判断 | 二重追加なし |

別列a: Error/Reject/期限切れで到着なしならKは変更しない。
別列b: Timeout応答は期待を期限まで保持。Deadlineちょうどで残り、超過で消える。
別列c: 期限後の遅着は手動切替。識別不能のV-08として記録する。
別列d: 同候補失敗はt=5まで再送なし、新候補へ変われば即時、戻った候補の時刻も保持。

### FB-SQ-14 古い設定世代・共有入力・Restart待ち

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | auto/[A,D]、S、R(D;[A])生成 | [R(D;[A])] | 固定Iから現program実表示入力を除外済み |
| 0.1 | Restart承認・実行 | — | 入力順固定、W開始 |
| 0.5 | 待ち中にDが表示0/逆待へ | [] | 待ち終了後SetScene承認でReject、K維持 |
| 1.2 | BD Rejected | 最新の別候補ならR等 | 同一候補の5秒制限は維持 |

別列a: SetScene承認後、設定変更→古い復帰のSC一致なら部分復帰せずC。
別列b: 古いF/ON期待なら追加/所有権設定なし、履歴がなければnoneで到着先に残る。
別列c: Aと現在Bが同入力imを入れ子経路で共有。R(A)生成時im除外、再起動なしでSetScene。
別列d: 固定I=[im,id]のうちimが承認時に実表示へ変化。imだけSkip、idは許可時だけ実行（C-03）。

## 6. M-12 結合テストの判定

### 6.1 独立oracleと判定点

0b HB-INV-01〜04 の判定方法は0bの表のまま。次の差分を加える。
偽OBSは program・項目グラフ・入力UUID・要求の実適用・永続ファイルを別に持つ。
Machineの探索関数/CanEnter/Healthをoracleに流用しない。
seed、名前対応、地図、全Event、Approve結果、適用、応答、SC、保存を出力する。
通常列は応答5秒以内・クラッシュなし・イベント欠落なし。遅延・順序逆転・重複は含む。
欠落・タイムアウト・再起動など保証外の列は別にし、記録回収の到達性を検査する。

| カメラspec第5節 / 既存不変条件 | 追加する判定 |
|---|---|
| 本体が隠した項目を記憶 / HB-INV-01 | 退避が2段でもfalse実送信前に同Z/SourceUUIDの記録が保存済み。ON/OFFで消さない |
| 配信中に今を空にしない / HB-INV-02 | 現programに加えProceed済み全個別退避期待先を保護。共有経路と未着falseを重ねる |
| 人の判断を優先 / HB-INV-03 | 手動SC・最下段削除でSceneEpoch+1、旧期待/時刻を全消去。新規退避は許す |
| 戻す方へ向かう / HB-INV-04 | 項目Showを履歴と独立に再試行。復帰待ちで今の不良退避を止めない |

監視対象への入場は生成・各承認・SetScene直前でCanEnter相当を独立計算する。
退避方向には逆待を課さず、復帰方向には課す。OFFは先頭/後続で方向を変える。
管理外を画像と呼んでも空でない保証はしない。
未着の人の切替、承認後の変化、使えない/人の印の例外は保証範囲と分けて記録する。

### 6.2 スタックの不変条件

| # | 判定点 | 独立に検査する期待 |
|---|---|---|
| FB-INV-01 | 各Handle/Approve後、Snapshot/status読出し後 | KのUUIDに重複なし。現在UUIDとSを含まない |
| FB-INV-02 | 同上 | owner=manual/noneならK=[]。非配信でもK=[] |
| FB-INV-03 | 接続中の整理後 | autoならK非空、全要素は存在し監視対象。return_scene=K[0] |
| FB-INV-04 | バッチ生成・Proceed・SRD・BD・期限後 | それだけで追加/切詰めしない。例外は列挙済みの解除/整理/切替不要ON/OFF |
| FB-INV-05 | 一致SC後 | 消費したうち一致1件だけの計画。古いConfig/無効作用では復活しない |
| FB-INV-06 | 新旧設定比較後 | 対応の到達可能性だけでは履歴を失わない。改名はUUID履歴維持 |
| FB-INV-07 | manual ON/OFF前後 | manual戻り先は単独。OFFがON前の中間履歴を復元しない |
| FB-INV-08 | 送出ログ | 同一キーの送出差>=5秒、別キーに前候補の5秒待ちを転用しない |
| FB-INV-09 | Snapshotのslice/mapを呼出側で変更後 | 内部履歴・計画・解決対応に変化なし |

切断中は存在/分類を検査せず、接続比較用履歴として検査する。
未送信の計画も現在UUIDを到着後履歴に入れないことを別に検査する。
途中削除・管理外化で変わった計画は承認再計算と到着後整理の両方で照合する。

### 6.3 安定区間の到達性

非循環の名前対応・固定地図・固定Judge状態・正常接続で末尾に安定区間を置く。
全カメラOK、全Show成功なら下から最も元へ戻り、最後はnone/K=[]に到達する。
全候補が準備できた後は同じ固定状態の下で逃がす/戻るを往復し続けない。
有限の監視シーン数nに対し、各到着で退避列を進むか復帰Kを短くすることを観測する。
項目到達上限18秒+BはHB-INV-04のまま。シーン往復全体に同じ数値を流用しない。
シーン側は各候補の5秒歯止め、各要求の応答上限5秒、W、期待の長さ、Tick間隔を
生成した有限経路ごとに足した上限で検査する。固定の「全体18秒」を設けない。
乱数列には設定並べ替え/改名/欠落復活/削除/Studio三値/ON特例/OFF専用範囲を混ぜる。
全解除直後の新規退避を禁止するoracleにしない。人の意図の優先と別の性質である。

## 7. spec の不明点と案（レビューで扱うもの）

第3節・第7節のユーザー裁定を再び未決にしない。ここでは残る境界だけを挙げる。
表中で案に依存する行はこの番号を参照する。specはこの作業では変更しない。

| 番号 | 決める人 | 不明点 | 案・表への影響 |
|---|---|---|---|
| C-01 | ユーザー | OFFで後続名がすべて引けず切替できないとき、警告とmanual維持は明記されるがHTTP成功/409は未指定 | 200で警告を表示しmanual維持、押し直せる。FB-OFF-07の案。S2の画面で「切替なし」を分かるようにする |
| C-02 | 設計者 | 防御的整理で重複UUIDが既に入った不正状態の残し方 | 下側を残す安定重複排除。通常経路は生成時に重複防止し、不正状態fixtureだけで検査 |
| C-03 | 設計者 | RestartMediaは複数Inputsを1Actionに持つが、現Approve結果はAction単位のSkipしか表せない | 実行役が固定Inputsの各入力直前に同じ世代/目的と実表示を再承認できる契約を足す。新対象は追加しない。全体Skipで非表示入力まで落とす案は対象固定と個別Skipの意図を損なうため推奨しない |
| C-05 | 設計者 | OFFで全候補名が引けない行と、待機名/UUID消失時にmanualを解除する規則の両立 | 整理による解除を優先。通常の取得済み地図ではSが残れば必ず終端候補となるため、全候補不能は到達不能なら直接状態テストに限定する。FB-OFF-07を通常経路の保証としない |
| C-04 | ユーザー（S2） | 個別退避中の表示文言と「待機の回数・合計・最長」に数える範囲 | spec第5節どおりS2へ留保。statusにはstack/currentをそのまま渡し、M-0cで集計定義を追加しない |

C-03はShowCameraも含め「配信中の入力を再起動しない」を満たす接続契約の整理。
実装時にswitcherだけで満たせない場合、実行役への引継ぎを明記してレビューする。
C-01/C-04は利用者が見る動作なので、設計者の都合だけで確定しない。
現時点のテスト案は表の案を基準にするが、レビューの裁定後に該当行だけ更新する。

## 8. 実装への受け渡しと検証範囲

- rows_fb_*_test.goの行名をFB-*、seqの列名をFB-SQ-*に対応させる案とする。
- 既存rows_events/commands/approve、rows_hb_review/conn、prop_hbを例外索引で照合する。
- helpersの既定fixtureは0bのまま保ち、0c用対応と複数履歴fixtureを明示的に追加する。
- 空設定の退行は冒頭の例外を除いて0bの表のまま。意味変更を機械的移植に埋めない。
- scenemapのHealthと表示経路を再利用するが、入場・方向・履歴探索はswitcherの責務。
- config検証は保存前、switcherは検証済みのScenesConfiguredを受ける契約のまま。
- 保存API/画面/status配線、READMEの改名・手動非表示・プロセス再起動の説明はS2へ渡す。
- scene switch退避reason=fallback、元と先を記録。BadCamera/kbpsは退避元から、同率は設定順。
- 回復camera/kbpsは戻り先で最後にOKになったカメラ。画像/手動に架空のカメラを作らない。
- Fableレビュー→sonnet実装、0b M-13 Windows確認後に0cを入れる順序を維持する。
- 文書確認は行IDの重複、入口/必須列の網羅、specとの差分、変更ファイル限定を検査する。
- この文書の作成ではGoのテストや実機試験を実施済みとは主張しない。
