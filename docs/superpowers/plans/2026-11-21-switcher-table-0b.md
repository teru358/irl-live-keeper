# switcher「入口 × 状態」の差分表 — 区切り0b（M-11）

- 対象: カメラの spec `../specs/2026-11-21-multi-camera-design.md` v0.15 全体。
- 基準: `2026-11-21-switcher-table-0a.md`（以下「0a の表」）。
- シーンの期待、E/C/X/B は `2026-11-19-switcher-table.md` とコア spec 第6章を継承。
- 状態: レビュー反映済みの実装用期待値。H 番号は第7章の決定を表す。
- コード、既存の表、spec は変更しない。M-10 の型追加と M-11 実装の接続案を含む。
- 新しい行は `HB-`、列は `HB-SQ-`。タグ T は全行 `SW`。
- 置換行は既存の名前の末尾へ `/0b` を付ける（`MC-H-7/0b` など）。
- 0a 側の名前に `/0a` があれば、それも保持して `/0b` を付ける。
- 本文に置換を指定しない行は **0a の表のまま**。
- hide のない fixture の既存テストの意味は変えない。
- H の決定に依存するテストには番号を添え、反映範囲を追えるようにする。

## 0. 差分の読み方と表記

E は全 Handle/Approve の入口で先に行う。拒否する Command も例外でない。
シーンの E の期限境界は従来どおり `At > Deadline`。
項目の E は2秒の期限と、隠しかけの10秒の掃除を追加する（H-03）。
C は持ち主と戻り先だけを消し、項目の一覧・期待を消さない。
X はシーンの自動切替の抑止。Hide/Show は X を参照しない。
ただし Hide は StartScene が未完了か、その期待が残る間は出さない。
B は列に含まれる各バッチごとに行う。ID は列の順に採番する。
「変化なし」「—」は E 後。明記しない状態と Epoch は保つ。
「Jb」は更新後の整える手順→シーン判定→Show→Hide の判断を表す。
各行の出力は Jb の計算結果を具体的な列で記す。
Command は Jb を行わず、その Command のバッチだけを返す。

| 記号 | 意味 |
|---|---|
| S/A/B/D/T | 0a と同じ UUID。S は待機、A は m と s、B は m、D は s だけ、T はカメラなし |
| m/s | main/sub。既定 m=standby、s=hide。設定順 m,s |
| im/is | 各カメラの入力 UUID。名前とは別 |
| x/y | s の項目鍵。既定 x=(A,2)、y=(D,1)、ソース is |
| k | 項目鍵 `(ContainerUUID, ItemID)`。グループも同じ形 |
| Z | 接続先 `(host,port)` と Collection の組 |
| L(c) | 今の Z に属するカメラ c の一覧 |
| P/H/U | 隠しかけ / 隠した / 未決着 |
| Q(k,v,r) | 項目 k を v にする期待、ReqID=r |
| V(R) | R に「映る」カメラの集合（本体の一覧を表示と数える） |
| A(R) | R に「実際に表示」のカメラの集合（送信済み false も除く） |
| F(R)/R(R)/M(R) | 0a と同じ AutoFail / AutoRecover / StandbyOffMonitored |
| Hc(c) | HideCamera(c)[SetItemEnabled(k,false)…] |
| Sc(c) | ShowCamera(c)[必要なら RestartMedia, SetItemEnabled(k,true)…] |
| RA | 停止用 RestoreAll。現在 Z の一覧から構成 |
| MA | 手動の「すべて表示に戻す」。現在 Z の全カメラ項目から構成 |
| u(c) | 人が戻した印 |
| f(c) | 続けて失敗した回数。3以上なら使えない |
| B! | B に加えて、そのカメラの前回送出時刻を At に更新 |

各行の既定: 接続・配信中、自動有効、Studio Mode オフ、Stopping=false。
現在 A、m=s=OK、owner=none、一覧・期待・未完了なし、再送時刻なし。
S/A/B/D/T と im/is は地図にある。項目の Enabled は true。ただし H の項目は、明記がなければ地図 false。単独の P/H/U は x のみ、y は地図 false の一覧外項目とする。x/y と書く行は両方を追跡する。
表の出力は「バッチ」であり、送信は各 Action の承認後。
Hc/Sc が出ても、この時点では項目の Enabled を書き換えない。

## 1. 型と実行役・core との契約

### 1.1 入口

M-10 の型名をそのまま使う。以下のコードは署名の契約で、製品コードではない。
全入口は `At time.Time` を持つ（`Stopping{}` 等の略記にも At を含む）。

| 型 | 内容・責務 |
|---|---|
| ItemRequestDone | `{At, ReqID uint64, Result ItemResult}`。ItemOK/ItemTimeout/ItemError/ItemNotFound |
| StudioModeChanged | `{At, Enabled bool}`。現在の本来の接続からのみ |
| OBSConnected | 0a の項目に `StudioMode bool` を追加。最初の地図と原子的に入れる |
| HiddenRestored | `{At, Entries []HiddenEntry}`。設定の後、最初の接続より前に1回 |
| HiddenWriteFailed | `{At, BatchID int}`。最初の false の保存に失敗したとき |
| Stopping | `{At}`。世代ごとに高々1回の意味。重複は無作用 |
| Command | `Kind=CmdShowAllCameras` を追加。API は `/api/cameras/show-all` |
| ScenesConfigured | `StudioModeHide` を追加。standby/ignore/hide、既定 standby |

`HiddenEntry` の契約（H-01、H-02）:

| フィールド | 型・意味 |
|---|---|
| CameraID | string。設定から消えた id も保存できる |
| Host / Port | string / int。接続先の同一性、資格情報は含めない |
| Collection | string。UUID が同じ別コレクションを混同しない |
| ContainerUUID / ItemID | string / int。項目そのものの鍵 |
| SourceUUID | string。記録時の入力 UUID |
| State | HiddenState=P/H/U |
| SettledAt | *time.Time。応答の決着の時刻。未応答なら nil |
| WrittenAt | time.Time。ディスクへ書いた時刻 |

読み込む slice とその要素は Machine が複製する。破損ファイルの退避は core の責務。
接続先は Config に Endpoint を追加して Machine へ渡す。接続先変更は新世代。
M-10 の ItemResult と別の RequestResult を混用しない。

### 1.2 出力と承認

- `Handle(Event) ([]Batch, error)` を採用する（空は nil でもよい）。
- 非 Command の列順はシーン切替→ShowCamera→HideCamera。
- 同種のカメラ順は設定順、設定外は id 辞書順（H-01）。
- 項目順は ContainerUUID 辞書順→ItemID 昇順。項目鍵を重複させない。
- Batch に CameraID、固定した項目と SourceUUID、送出理由を追加する。
- Purpose に HideCamera/ShowCamera/RestoreAll を追加。
- 手動 MA も RestoreAll とし `FromCommand` と `RestoreScope=AllCameraItems` で識別する。
- 停止用は `RestoreScope=TrackedItems`。MA と RA を同じ未完了条件で混同しない。
- Action は M-10 の `SetItemEnabled{SceneUUID, ItemID, Enabled}`。
- SceneUUID は項目を所有するコンテナの UUID。program UUID とは限らない。
- 承認はバッチ全体で1回ではなく、各 Action の直前に1回ずつ行う。
- Skip はその Action のみ送らず、次へ進む。期待も ReqID も作らない。
- Reject は残りの Action を捨て、BatchDone{Rejected} を1回返す。
- Hc/Sc の SetItemEnabled と RestartMedia の要求失敗は後続を続け、最終的に Completed+Err。
- Hc/Sc に BatchDone{Failed} は来ない契約。第1段階の失敗で残りを捨てる実行とは目的別に分け、M-12 が担う（M-8）。
- RestartMedia の失敗でも後続の true を試す（H-04）。
- Completed は「全 Action を走査した」であって、全要求の成功ではない。

### 1.3 ReqID と保存の順（H-01 決定）

既存の `Approve(batchID, action, override, at)` を広げる。項目専用 API は作らない。
Machine が項目用 uint64 カウンタを持ち、Proceed に ReqID と MustPersist を載せる。
シーンの int の採番と別領域で、世代内では再利用しない。同じバッチの Action で対象を一意に引ける。

1. 実行役は各 Action の直前に core を介して Approve を呼ぶ。
2. Machine は Proceed のときだけ採番し、Q と切断をまたぐ ItemReceipts を登録する。
3. 最初の有効な false では全固定対象を P に予約し、予約前の H/U を巻き戻し台帳に保存する。
4. この承認の Proceed に MustPersist=true を付ける。core は一覧を同期で atomic write する。
5. 保存中は中核ループへの割り込みを許さない。上限1秒（超過も失敗）。成功後にだけ実行役へ Proceed を返す。
6. 失敗なら HiddenWriteFailed を Handle へ渡し、予約と Q を巻き戻して実行役には Reject を返す。
7. 実行役は Proceed.ReqID をそのまま ItemRequestDone に載せる。送信前の失敗は ItemError。
8. 送信後、応答前に切断された要求は必ず ItemTimeout を返す。切断通知より後の結果も台帳で解く。
9. 通常の OBS 応答は internal/obs の SetSceneItemEnabled と ClassifyItemResult を使う。切断時の未応答要求の Timeout 化は実行役の責務。

旧案の実行役採番・項目専用承認は、採番主体の二重化を避けるため撤回した（I-9）。

### 1.4 一覧の読み口と保存

Snapshot に HiddenEntries / RetainedEntries / HiddenRevision / StudioMode とカメラ別状態を載せる。
Snapshot と Revision は一覧の写しを読むためだけに使い、保存障壁の判定には使わない。
core は Revision が変わったときに永続内容の写しを読み、順序を固定して保存する。
送信前保存の必要性は Proceed.MustPersist だけが伝える。初回予約は1バッチ1回。
H への更新や削除も別の変更として保存する。通常更新の保存失敗は警告と再保存対象を残す。
停止完了は最終保存後。ファイル I/O は core が担い、Machine に持ち込まない。
写しは slice と要素を複製し、外から内部を変更できない。Revision だけで保存の種類を推測する旧案は撤回した。

## 2. 内部状態の差分

### 2.1 新しく保持する状態

| 状態 | 初期値 | 内容 |
|---|---|---|
| Hidden | 空 | CameraID→Zと項目鍵→HiddenEntry |
| Retained | 空 | 別 Z または未照合の記録。全 Z を含む |
| ItemExpectations | 空 | 登録順の Q。項目、値、ReqID、BatchID、応答、期限、SawEvent |
| ItemReceipts | 空 | イベント先着後も応答を結びつける送信台帳（H-03） |
| UserShown | 全 false | カメラ別の「人が戻した」印 |
| LastItemSent | 空 | カメラ別の Hide/Show 共通の送出時刻（H-04） |
| ItemFailures | 全0 | カメラ別の連続失敗。3以上が unusable |
| StudioMode | false | OBSConnected で置換。切断中の値は判断に使わない |
| StudioModeHide | standby | 設定値。ScenesConfigured の同値比較に含める |
| StoppingReceived | false | 世代内で一度 true になったら戻さない |
| RestoreIssued | false | 停止用 RestoreAll を重ねない |
| Pending の拡張 | 空 | CameraID、固定対象、予約前の値、承認済み/送信扱い/結果の項目台帳 |
| HiddenRevision | 0 | 一覧・残した記録の永続化内容が変わるたび進む |

隠しかけの全件予約は送信済みとは異なる。
A(R) から除くのは予約しただけの未送信項目でなく、Proceed した false。
Hide/Show は同一カメラのどちらかが Pending にある間はどちらも新規に出さない。
他のカメラは独立だが、承認は常に先の false を含めて空になるか再計算する。
同じ項目の期待がある間は、その項目を次のバッチへ入れない。
送信台帳は BatchDone 後でも遅れた応答を解釈できる範囲で保つ（H-03）。

### 2.2 状態を消す・保つ入口

| 入口 | 一覧・残した記録 | 期待・未完了 | 印・回数・時刻 |
|---|---|---|---|
| OBSDisconnected | P/H/U を保つ | Q は消す。送信台帳は保つ。Pending は残し ConnEpoch で却下 | 保つ |
| 配信停止 | 全て保ち、表示へ向かう | 項目 Q は保つ。シーン期待だけ従来どおり消す | u は全消去。回数・時刻は保つ（M-3: 即時解除案は採らず、最大5秒の再送待ちを維持） |
| OBSConnected 同 Z | 地図で照合。P は即時削除しない | 旧 Pending は ConnEpoch 不一致 | StudioMode を導入、回数は保つ |
| OBSConnected 別 Z | 旧一覧を Retained、新 Z だけ照合 | 旧 Q は切断で消去済み | カメラの印・時刻は保持（H-05） |
| ScenesConfigured on_bad 変更 | 記録を消さず、standby へ変えた分を戻す | Q/Pending は保持。最新条件で承認 | 回数は受信状態変更でないので保持 |
| ScenesConfigured auto=false | 記録を保って戻す | 項目 Q は消さない | u は保持、Hide は auto 条件で停止 |
| studio_mode_hide 変更 | 新設定の有効動作を再計算 | Q は保持、以後の承認に新設定 | 印・回数は保持 |
| CamerasChanged c の状態変化 | 記録は保持 | Q/Pending は保持 | f(c)=0。OK になれば u(c)=false |
| Stopping | RA に備えて全て保つ | Auto/Hide/Show の後続を却下 | StoppingReceived=true |
| 世代終了 | 未復旧分をファイルへ残す | メモリは破棄 | 新世代は初期値 |

項目 Event は SceneEpoch を進めない。地図変更も進めない。
Hide/Show には SceneEpoch/ConfigEpoch と X を掛けない。
Hide/Show の共通条件は接続・ConnEpoch・StreamEpoch。停止 RA は接続・ConnEpoch だけ。
配信停止前の Show の後続も StreamEpoch で却下され、新バッチで再判断する。
ScenesConfigured で StudioModeHide のみ違う場合も ConfigEpoch+1。
カメラ削除時は u/f/送出時刻を捨て、一覧は復旧まで保持する（M-7）。

## 3. 計算規則の差分

### 3.1 健康と実際の表示を分ける

0a の nil hiddenByTool を、Z と SourceUUID に加え、その入力の今の CameraID が記録の CameraID と一致する述語に置換する。
この述語は、記録を現在のカメラの V(R)/A(R) に反映してよいかだけを判定し、復旧対象を選ぶ条件には使わない。
復旧対象の選択は現在 Z の記録の項目鍵を起点に、現在の地図に存在する項目を選び、その後で戻せる条件・Q・Pending・5秒制限を検査する。SourceUUID または CameraID が不一致でも、記録の CameraID に属する Sc の対象として現在の項目へ true を送る（旧ソースへの Restart はしない）。別 Z は触らず、項目が消えていれば記録を外す。
`Classify` は内部で nil を使うため、一覧を反映した V(R) から分類する必要がある。
一覧の H を無視して Other に落とすと auto の戻り先が消えるので禁止。

| 項目の状態 | V(R) の末端 | A(R) の末端 |
|---|---|---|
| 一覧なし、地図 true | 表示 | 表示 |
| 一覧なし、地図 false | 非表示 | 非表示 |
| H | 表示 | 非表示 |
| P、false が Proceed 済み | 表示 | 非表示（イベント未着でも） |
| P、まだ未承認の予約だけ | 表示 | 地図値 |
| U | 表示 | 表示（spec の明示例外） |

途中の親が人の false なら、一覧にある末端もその経路では映らない。
U を表示扱いにするのは末端だけで、親の false を乗り越えない。
異なるソースへ変わった項目は旧カメラの表示として捏造しない。
同じカメラへの別入力・別経路が1本でも残れば集合に1台として残る。

### 3.2 Studio Mode の有効動作

ScenesConfigured は新設定を先に適用し、その有効動作で健康と各目的の条件を計算する。on_bad=standby は Studio Mode の値によらず standby と数え、ignore は on_bad=hide だけに効く。

| Studio Mode / 設定 | hide カメラの健康 | Hide を出すか | 戻してよいか | I(R) |
|---|---|---|---|---|
| off / 全値 | 普段の hide | 条件成立なら出す | 普段の条件 | hide は含めない |
| on / standby | standby として不良を数える | 出さない | 不良でも true | standby として含める |
| on / ignore | 悪いと数えない | 出さない | Studio Mode だけを理由に戻さない | 含めない |
| on / hide | 普段の hide | 条件成立なら出す | 普段の条件 | 含めない |

ignore でも OK・配信停止・auto=false・on_bad=standby は戻す理由になる（H-07）。戻す対象は本体の一覧だけ。
ignore しか映らないシーンの健康は OK。無視を全 hide 不良判定へ再投入しない。
現在の scenemap.Health は Studio Mode を bool で standby 化するだけである。
0b では呼ぶ側で有効 Motion に変換する。standby は MotionStandby、ignore は集合から除かず不良を数えない MotionIgnore を追加して Health に渡す。Machine.classOf は ClassifyWithMap の直接使用をやめ、一覧を反映した V(R) で分類する（M-5）。

| # | 前提 | 分類・健康 / 出力 |
|---|---|---|
| MC-H-7/0b | m=OK、s=hide/DOWN | Monitored/OK、Hc(s) |
| HB-H-01 | s だけ映る、s=DOWN | Monitored/DOWN、F(D)、Hc なし |
| HB-H-02 | s だけ映る、s=OK、f(s)=3 | Monitored/DOWN、F(D) |
| HB-H-03 | m=standby/LOW、s=hide/OK | Monitored/LOW、F(A) |
| HB-H-04 | m=s=hide、両 DOWN | Monitored/DOWN、代表 m、F(A) |
| HB-H-05 | m=s=hide、m=OK、s=DOWN | Monitored/OK、Hc(s) |
| HB-H-06 | s=hide/DOWN、Studio on/ignore | Monitored/OK、Hide なし |

### 3.3 戻り先の待ちと「空にしない」

「戻り先が待っている隠した項目」は、次の全条件を満たすカメラがあること。
戻り先へ V(R) で映る / 状態 OK / f<3 / L に地図で false の項目がある。
そのカメラの一覧内の非表示項目は全シーンを対象に調べる読み方を採る（H-08）。
U でも地図 true なら待たない。地図 false で一覧に残るなら上記で待つ。
AutoRecover の生成だけを止める。承認の autoNeed と共通化して待ち条件を足さない。
OffMonitored は Command 時に auto=true なら待ちでも auto/R にして200。
auto=false なら健康・待ちで止めない。
Show が Pending でも非表示が解消すれば、次の判断で戻れる。

空にしない検査は、現在シーンと Proceed 済み SetScene の期待が残る切替先のうち監視対象の各 R で、候補 c の送信対象を false とした仮の A(R) が空かを見る。
他バッチですでに Proceed した false と、同じ Handle 内で先に候補にした Hc の全項目も仮の地図へ重ねる。
同じ入れ子の末端 k を通る経路は、全て同時に失われる。
経路数をカメラ台数として数えず、カメラ id の集合の大きさを見る。
生成時と各 false の承認時の両方で検査する。
F が生成・送信・OK だけでは現在シーンはまだ変わらないので Hide を許さない。
SceneChanged(S) を受けて初めて待機として隠せる。
待機切替が失敗しても、期限切れでも、現在シーンを勝手に S としない。

## 4. 入口ごとの表

全行 E 後。出力の Hc/Sc は B!、F/R/M/RA/MA は B を含む。MA は送出時刻を更新しない（I-6）。
Jb は同一カメラ未完了・項目期待・5秒の制限を常に含む。
再送条件を満たさない行は列の他の候補だけが出る。

### 4.1 CamerasChanged と Tick

| # | T | 前の状態 → 入力 | 出す列 | 状態の変化 |
|---|---|---|---|---|
| HB-CC-01 | SW | A、m OK、s OK→LOW | [Hc(s)] | H(A)=OK、f(s)=0 |
| HB-CC-02 | SW | Hc(s) 未完了中、s LOW→DOWN | [] | 状態更新、f=0、重ねない |
| HB-CC-03 | SW | L(s)=H、s DOWN→OK | [Sc(s)] | u=false、f=0、表示はまだ false |
| HB-CC-04 | SW | L(s)=H、Hide 未完了中に s OK | [] | Show は BatchDone 後、5秒経過で出る |
| HB-CC-05 | SW | A、m LOW、s DOWN へ一括変化 | [F(A),Hc(s)] | m が残るので空にならない。健康 LOW |
| HB-CC-06 | SW | D、s DOWN へ | [F(D)] | Hide は空にするため出ない |
| HB-CC-07 | SW | m=s=hide、両 DOWN へ | [F(A),Hc(m)] | m の候補を重ね、最後の s は隠さない（H-09） |
| HB-CC-08 | SW | 未接続で s の状態変化 | [] | 状態・回数のみ更新 |
| HB-CC-09 | SW | 配信停止中、L(s)=H、s LOW→DOWN | [Sc(s)] | Restart なし。停止中なので戻せる |
| HB-CC-10 | SW | 自動無効、L(s)=H、s DOWN→LOW | [Sc(s)] | 同上 |
| HB-CC-11 | SW | 同値の組、再送時刻を過ぎている | [] | E のみ。Jb なし |
| HB-TK-01 | SW | 前回 Hc/Sc 送出から4.999秒 | [] | 再送しない |
| HB-TK-02 | SW | s DOWN、x/y true、前回送出から5秒ちょうど、未完了・Q なし | [Hc(s)] | B!、時刻更新 |
| HB-TK-03 | SW | 5秒経過、同項目の Q あり | [] | Q がある対象へ送らない |
| HB-TK-04 | SW | Sc の失敗が3回、条件継続 | [Sc(s)] | unusable でも戻す試行は続ける（H-04） |
| HB-TK-05 | SW | s DOWN、P(x)、決着から10秒ちょうど、地図 true、Hc 未完了 | [] | P 保持。掃除は10秒超過だけ |
| HB-TK-06 | SW | s OK、U(x)、決着から10秒超、地図 true | [] | U 保持、表示項目なので Sc なし |

HB-CC-07 は設定順 m,s で候補を重ねる。m を隠す候補の後、s は最後の1台なので残す。
一般の mixed standby/hide は「悪い hide がある」だけで待機へ送らない。
LC の全 standby 行・同値・正規化は 0a の表のまま。

### 4.2 ItemEnabledChanged

処理順: 接続確認→イベント受信印→Q 照合→Timeout 記憶→人の判定→地図更新→整える→Jb。
Q は同じ Z、鍵、Enabled で先頭から照合する。
照合範囲は同じ項目内だけ。Q は一致でも消さず SawEvent を立て、応答から2秒を超えるまで保つ（Error/NotFound は即時消去）。
別項目の期待を消すと一括 hide の後続イベントを人と誤認するため。
応答前の一致でも一覧を変更し、後着の結果は送信台帳で解釈する。

| # | T | 前の状態 → イベント | 出す列 | 状態の変化 |
|---|---|---|---|---|
| HB-IE-01 | SW | s DOWN、Hc(s) 未完了、Proceed 済み P(x)、Q(x,false,r) → x false | [] | Q 照合、P→H、地図 false |
| HB-IE-02 | SW | s DOWN、自動無効、Sc(s) 未完了、H(x)、Q(x,true,r) → x true | [] | Q 照合、x を一覧から外す、地図 true |
| HB-IE-03 | SW | s DOWN、Hc(s) 未完了、H、Q なし、s DOWN → x true | [] | x を外し u(s)=true、同じ不調中の Hide なし |
| HB-IE-04 | SW | s OK、Sc(s) 未完了、H(x)、Q なし → x true | [] | x を外し u は付けない |
| HB-IE-05 | SW | s DOWN、Hc(s) 未完了、H、Q なし、配信停止中 → x true | [] | x を外し u は付けない |
| HB-IE-06 | SW | s DOWN、Hc(s) 未完了、一覧なし → 既知 x false | [] | 地図だけ。人の非表示を一覧へ加えない |
| HB-IE-07 | SW | s DOWN、Hc(s) 未完了、一覧なし → 既知 x true | [] | 地図だけ。未完了 Hc により重ねない |
| HB-IE-08 | SW | s DOWN、Hc(s) 未完了、U、Q なし → x false | [] | 遅れた本体の false として U→H |
| HB-IE-09 | SW | s DOWN、U(x)、Q なし、x false→true | [] | U 保持、u(s)=true |
| HB-IE-10 | SW | s DOWN、Hc 未完了、送信済み P(x)、応答前 x false | [] | P→H、Q 維持、SawEvent=true。後着 OK から2秒保つ |
| HB-IE-11 | SW | s DOWN、自動無効、Sc(s) 未完了、true 応答前に x true | [] | 一覧を外す、後着 OK で再追加しない |
| HB-IE-12 | SW | s DOWN、Q(x,false)、送信済み P(x) → x true | [] | u=true、一覧と Q を保持。Q 終了時に地図 true なら一覧を外す |
| MC-ITEM-9/0b | SW | 同値 x、Q/一覧/記憶の処理なし | [] | E のみ（W-06 の通常ケース） |
| HB-IE-13 | SW | s DOWN、Hc(s) 未完了、同値 x false、U あり | [] | 同値判定より先に U→H（H-02） |
| HB-IE-14 | SW | s DOWN、自動無効、Sc(s) 未完了、同値 x true、Q(x,true) あり | [] | 先に照合して一覧を外す。W-06 の例外 |
| MC-ITEM-10/0b | SW | 未知鍵、Q/一覧なし | [] | core が最新値を覚える。再通知なし |
| HB-IE-15 | SW | s DOWN、Hc(s) 未完了、未知鍵に追跡中の Q がある | [] | 照合は可能。地図に項目を捏造せず次の Map を待つ |
| HB-IE-16 | SW | s DOWN、Hc(s) 未完了、人が親グループを false | [] | 子の一覧は保持。全経路を再計算 |

人の SceneChanged はシーンの期待を消すが項目の期待は消さない。
false Q が残る間の人の true は u を付けても一覧を外さない。Q 終了時に地図 true なら外す。
承認から適用の間に人が同じ false を送ると本体の期待に一致し、回復で戻す。識別不能の既知制約（M-9）。
u のある同じ不調中は、少なくとも新しい false を出さないことを検査する。

### 4.3 ItemRequestDone と項目 E

未知 ReqID は E 以外無作用（U-22 の項目版）。重複結果で回数を二重に増やさない。
要求結果を受けた時点の健康と一覧から判断し直す。

| # | T | 前の状態 → 結果 | 出す列 | 期待・一覧・回数 |
|---|---|---|---|---|
| HB-IR-01 | SW | s DOWN、Hc(s) 未完了、P、Q(false,r) → ItemOK | [] | 決着時刻=At、期限=At+2秒、P 保持、地図未変更 |
| HB-IR-02 | SW | s DOWN、Hc(s) 未完了、P、Q(false,r) → ItemTimeout | [] | P→U、期限=At+2秒、永続化。失敗はバッチ単位集約 |
| HB-IR-03 | SW | s DOWN、Hc(s) 未完了、P、Q(false,r)、地図 true → ItemError | [] | Q 消去、送信済み P を外す。決着時刻記録 |
| HB-IR-04 | SW | s DOWN、Hc(s) 未完了、一覧あり → ItemNotFound | [] | Q 消去、該当項目を一覧から外す |
| HB-IR-05 | SW | s DOWN、自動無効、Sc(s) 未完了、H、Q(true,r) → ItemOK | [] | 期限=At+2秒、イベントか補正まで H 保持 |
| HB-IR-06 | SW | s DOWN、自動無効、Sc(s) 未完了、H、Q(true,r) → ItemTimeout | [] | 期限=At+2秒、H 保持。false の U を新設しない |
| HB-IR-07 | SW | s DOWN、自動無効、Sc(s) 未完了、H、Q(true,r) → ItemError | [] | Q 消去、H 保持。再送対象 |
| HB-IR-08 | SW | s DOWN、先着 false で H(x)、Q 残存 → ItemOK | [] | H 維持、期限 At+2秒、送信台帳に成功 |
| HB-IR-09 | SW | s DOWN、Hc(s) 未完了、先着 false で H → ItemTimeout | [] | H 維持。すでに観測した false を U に戻さない |
| HB-IR-10 | SW | s DOWN、自動無効、Sc(s) 未完了、先着 true で削除済み → ItemError | [] | 一覧再追加なし、失敗は台帳に残す |
| HB-IR-11 | SW | 未知 r または処理済みの結果 | [] | E のみ |
| HB-EX-01 | SW | OK の Q、SawEvent=false、At=期限 | [] | まだ Q を保つ |
| HB-EX-02 | SW | s DOWN、Hc 未完了、ItemOK の Q(x,false)、SawEvent=false、At>期限 → Tick | [] | 地図 false、期待一致相当、P→H、Q 消去 |
| HB-EX-02-C | SW | s DOWN、Hc 未完了、ItemOK の Q(x,false)、SawEvent=false、At>期限 StoppingReceived=true → CmdShowAllCameras | [] / ErrInvalidState{終了中} | 地図 false、期待一致相当、P→H、Q 消去 |
| HB-EX-02-A | SW | s DOWN、Hc 未完了、ItemOK の Q(x,false)、SawEvent=false、At>期限 → 未知バッチ Approve | Reject | 地図 false、期待一致相当、P→H、Q 消去 |
| HB-EX-03 | SW | s DOWN、通常 Sc 未完了、ItemOK の Q(x,true)、SawEvent=false、At>期限 → Tick | [] | 地図 true、期待一致相当で一覧削除 |
| HB-EX-03-C | SW | s DOWN、通常 Sc 未完了、ItemOK の Q(x,true)、SawEvent=false、At>期限 StoppingReceived=true → CmdShowAllCameras | [] / ErrInvalidState{終了中} | 地図 true、期待一致相当で一覧削除 |
| HB-EX-03-A | SW | s DOWN、通常 Sc 未完了、ItemOK の Q(x,true)、SawEvent=false、At>期限 → 未知バッチ Approve | Reject | 地図 true、期待一致相当で一覧削除 |
| HB-EX-08 | SW | s DOWN、MA Completed、x=H、true ItemOK、SawEvent=false、At>期限 → Tick | [] | MA の Q だけ消去、地図 false と記録を保持。読み直しの表示確認待ち（HB-SQ-10C） |
| HB-EX-08-C | SW | HB-EX-08 と同じ状態、At>期限、StoppingReceived=true → CmdShowAllCameras | [] / ErrInvalidState{終了中} | MA の Q だけ消去、地図と記録を保持 |
| HB-EX-08-A | SW | HB-EX-08 と同じ状態、At>期限 → 未知バッチ Approve | Reject | MA の Q だけ消去、地図と記録を保持 |
| HB-EX-04 | SW | s DOWN、Hc 未完了、OK の Q(x,false)、H(x)、地図 false、SawEvent=true、期限超過 → Tick | [] | Q だけ消し、地図と人の判断を上書きしない（x=false、H 維持） |
| HB-EX-04-C | SW | s DOWN、Hc 未完了、OK の Q(x,false)、H(x)、地図 false、SawEvent=true、期限超過 StoppingReceived=true → CmdShowAllCameras | [] / ErrInvalidState{終了中} | Q だけ消し、地図と人の判断を上書きしない（x=false、H 維持） |
| HB-EX-04-A | SW | s DOWN、Hc 未完了、OK の Q(x,false)、H(x)、地図 false、SawEvent=true、期限超過 → 未知バッチ Approve | Reject | Q だけ消し、地図と人の判断を上書きしない（x=false、H 維持） |
| HB-EX-05 | SW | s DOWN、Hc 未完了、Timeout の Q、U(x)、期限超過 → Tick | [] | Q 消去のみ、U を保つ |
| HB-EX-05-C | SW | s DOWN、Hc 未完了、Timeout の Q、U(x)、期限超過 StoppingReceived=true → CmdShowAllCameras | [] / ErrInvalidState{終了中} | Q 消去のみ、U を保つ |
| HB-EX-05-A | SW | s DOWN、Hc 未完了、Timeout の Q、U(x)、期限超過 → 未知バッチ Approve | Reject | Q 消去のみ、U を保つ |
| HB-EX-06 | SW | s DOWN、Hc 未完了、P、地図 true、決着後10秒超過（10秒ちょうどは保持） → Tick | [] | P 削除。U は対象外 |
| HB-EX-06-C | SW | s DOWN、Hc 未完了、P、地図 true、決着後10秒超過（10秒ちょうどは保持） StoppingReceived=true → CmdShowAllCameras | [] / ErrInvalidState{終了中} | P 削除。U は対象外 |
| HB-EX-06-A | SW | s DOWN、Hc 未完了、P、地図 true、決着後10秒超過（10秒ちょうどは保持） → 未知バッチ Approve | Reject | P 削除。U は対象外 |
| HB-EX-07 | SW | s DOWN、Hc 未完了、P(x)、まだ応答なし | [] | 時間だけで P/Q を消さない |

期限切れを起こす拒否 Command があっても、Command の出力へ自動バッチを混ぜない。
HB-EX-03 とその C/A は通常 Sc の補正。MA は HB-EX-08 とその C/A に従い、OK の無イベント補正で表示確認を代用しない。
ItemError の際に地図 false が確認済みなら H を外さない（H-03）。

### 4.3a 切断後の要求結果

以下はすべて未接続、送信台帳に r があり、Q は切断で消去済み。

| # | T | 前の状態 → 入力 | 出す列 | 状態の変化 |
|---|---|---|---|---|
| HB-IR-12 | SW | s DOWN、P(x)、地図 true → ItemTimeout(r) | [] | P→U、決着 At。Q は戻さない |
| HB-IR-13 | SW | s DOWN、P(x)、地図 true → ItemError(r) | [] | P 削除、台帳に結果、Q は戻さない |
| HB-IR-14 | SW | s DOWN、P(x) → ItemOK(r) | [] | 決着 At、P 保持、Q は戻さない。再接続地図 true でも10秒超まで保持 |
| HB-IR-15 | SW | s DOWN、P(x) → ItemNotFound(r) | [] | 該当記録を削除、Q は戻さない |

### 4.4 SceneMapChanged

表示値の重ね方は 0a の表のまま。本来の接続のイベントを上書きしない。
構造照合は地図の同値判定だけで省略せず、未照合記録がある場合を考慮する（H-02）。

| # | T | 前の状態 → 地図 | 出す列 | 状態の変化 |
|---|---|---|---|---|
| HB-MP-01 | SW | s DOWN、Hc(s) 未完了、同 Z、一覧の x が消えた | [] | x を一覧から外す、対応 Q は整理 |
| HB-MP-02 | SW | s DOWN、H(x)、is の URL 変更で CameraID s→m（SourceUUID is は同じ） | [Sc(s) の x] | 健康への照合述語は不一致なので旧 s として数えない。復旧は記録の鍵 x を選び、旧 id s の Sc から現在の x に true を送る（Restart なし）。記録は表示確認まで保持。新 m を隠さない |
| HB-MP-03 | SW | s DOWN、Proceed 済み P(x)、Hc 未完了、地図 false | [] | P→H、未応答でも記録保持 |
| HB-MP-04 | SW | s DOWN、Hc(s) 未完了、P の x が地図 true、10秒未満 | [] | P 保持 |
| HB-MP-05 | SW | s DOWN、Hc(s) 未完了、U の x が地図 true | [] | U 保持、10秒掃除しない |
| HB-MP-06 | SW | s DOWN、Hc(s) 未完了、U の x が地図 false | [] | H-02 決定どおり H に決着。補助地図の既存値で上書きしない |
| HB-MP-07 | SW | m=s OK、別 Z の地図、全項目 true | [] | 旧記録を Retained へ。新 Z と UUID が同じでも触らない |
| HB-MP-08 | SW | s DOWN、空地図→有効地図、未照合 H(x) 記録あり、x/y false | [] | 初回照合をここでも行う（H-02） |
| HB-MP-09 | SW | s DOWN、全 s 項目が H、m true、ソースと親経路は同じ | [] | V(R) で Monitored を維持、auto の戻り先を消さない |

ソースが変わった項目を「消えた」と同じに即時削除しない。
Sc の true は現在の項目を戻す。旧 SourceUUID または CameraID との不一致は戻す理由であり、健康への照合不成立によって復旧候補から除外しない。

### 4.4a SceneChanged の 0b 差分

| # | T | 前の状態 → 入力 | 出す列 | 状態の変化 |
|---|---|---|---|---|
| HB-SC-01 | SW | D、s DOWN、F の期待あり → 期待どおり SceneChanged(S) | [Hc(s)] | auto/D 維持、シーン期待一致。項目 Q/一覧を保持 |
| HB-SC-02 | SW | S、s DOWN、owner=none → 人の SceneChanged(A) | [Hc(s)] | C、SceneEpoch+1。m が残るので隠せる |
| HB-SC-03 | SW | A、s DOWN、Hc(s) 未完了 → 人の SceneChanged(D) | [F(D)] | C、SceneEpoch+1、項目 Q と一覧は保持 |
| HB-SC-04 | SW | HB-SC-03 後、Hc の y false 承認 | Reject | D を空にするため送らない |
| HB-SC-05 | SW | A、m=s OK、H(x)、Q(x,true)、Sc 未完了 → 人の SceneChanged(T) | [] | C、SceneEpoch+1、項目 Q/H/台帳は変わらない |

### 4.5 接続、配信、設定、Studio Mode

HB-CF を含む ScenesConfigured は新設定適用→有効動作→健康→各目的の条件の順で計算し、on_bad=standby は Studio on/ignore でも standby と数える（3.2）。

| # | T | 前の状態 → 入口 | 出す列 | 状態の変化 |
|---|---|---|---|---|
| OD-1/0a/0b | SW | 接続→OBSDisconnected | [] | ConnEpoch+1、全 Q 消去、送信台帳・P/H/U・Pending 保持 |
| HB-OC-01 | SW | s DOWN、同 Z 再接続、P(x)、地図 true、決着後1秒、旧 Hc 送出から1秒、y=false | [] | 最初の Map で P を保持、StudioMode 導入。5秒制限で再送なし |
| HB-OC-02 | SW | m=s OK、別 collection Y へ再接続、全項目 true | [] | C、X の一覧は残した記録、Y 分だけ照合 |
| HB-OC-03 | SW | 空地図で再接続 | [] | 記録を照合せず残す。カメラ表示操作なし |
| HB-OC-04 | SW | 配信停止へ変わった再接続、s DOWN、一覧 H(x/y)、地図 false、送出時刻なし | [Sc(s)] | C、u 消去、項目を戻す。StartScene なし |
| SSF-1/0a/0b | SW | true→false、s DOWN、一覧 H(x/y)、地図 false、送出時刻なし | [Sc(s)] | C、シーン期待・自動再送記録消去、項目 Q は保持 |
| HB-ST-01 | SW | false→true、開始シーン O あり、s OK/H(x/y)、地図 false | [StartScene(O),Sc(s)] | StartScene 未完了か期待残存中の Hc なし。s OK なので Sc を出す |
| HB-ST-02 | SW | m=s OK、false→true、開始切替なし | [] | 最新の地図・状態で判断、u は前の停止で消去済み |
| SCF-11/0a/0b | SW | auto true→false、s DOWN、一覧 H(x/y)、地図 false、送出時刻なし | [Sc(s)] | auto owner は C、manual は維持。項目 Q は残す |
| MC-CFG-2/0b | SW | A m OK/s DOWN、前の F は失敗済み、s standby→hide | [Hc(s)] | ConfigEpoch+1、健康 OK、F を新規に出さない |
| HB-CF-01 | SW | s DOWN、s hide→standby、L(s)=H(x/y) | [F(A),Sc(s)] | 健康 DOWN、V(A) に s を数える |
| HB-CF-02 | SW | s を設定から削除、L(s)=H | [Sc(s)] | 設定外でも一覧 id を走査して戻す。u/f/送出時刻を破棄 |
| HB-CF-03 | SW | s DOWN/H(x/y)、on_bad=hide、Studio on、StudioModeHide ignore→standby | [F(A),Sc(s)] | ConfigEpoch+1、新設定で s を standby と数え健康 DOWN。Sc は Restart なし、Q/Pending 保持 |
| HB-SM-01 | SW | off→on、設定 standby、s DOWN/H | [F(A),Sc(s)] | s を standby として健康 DOWN、Hide 停止 |
| HB-SM-02 | SW | off→on、設定 ignore、s DOWN/H | [] | H 保持、健康で s を悪いと数えない |
| HB-SM-03 | SW | off→on、設定 hide、s DOWN/H(x/y) | [] | 通常 hide 継続、対象は地図 false のため新規 Hide なし |
| HB-SM-04 | SW | on/ignore→off、s OK/H(x/y)、前回 Sc は失敗済み、送出から5秒超 | [Sc(s)] | 再試行、通常戻す条件 |
| HB-SM-05 | SW | on/standby→off、現在 S、auto/A、s DOWN、地図 true | [R(A),Hc(s)] | A 健康 OK、待機中に隠せる。後の R 承認も通常条件で検査 |
| HB-SM-06 | SW | 同値 StudioModeChanged | [] | E のみ（決定） |

Sc/Hc の再送5秒と未完了条件はどの入口でも省略しない。
同値設定の判定に StudioModeHide を含める以外、同値 ScenesConfigured は 0a の表のまま。

### 4.5a 照合と予約の補足

| # | T | 前の状態 → 入力 | 出す列 | 状態の変化 |
|---|---|---|---|---|
| HB-OC-05 | SW | 同 Z、H(x)、s OK、他一覧なし → 再接続地図 x true | [] | H を削除、u は付けない。イベントで人の操作を観測していない |
| HB-OC-06 | SW | Stopping 済み未接続、s DOWN、H(x) → OBSConnected 同 Z、x false | [] | H を照合、StudioMode 導入。RA/Auto/Hc/Sc を出さない |
| HB-MP-10 | SW | s DOWN、Hc[x,y] 未完了、y は未送信予約 P → 地図 y false | [] | y は P のまま、H にしない |
| HB-IE-17 | SW | s DOWN、Hc[x,y] 未完了、y 未送信 P → 人の y false | [] | 地図だけ false、P 維持。承認 Skip 後 BatchDone で予約前へ戻す |
| HB-HR-07 | SW | 接続済み、s OK、一覧なし → 初回 HiddenRestored | [] / nil | 順違反として無視、記録を再投入しない |

### 4.6 HiddenRestored、HiddenWriteFailed、Stopping、Command

| # | T | 前の状態 → 入口 | 出す列 / cmdErr | 状態の変化 |
|---|---|---|---|---|
| HB-HR-01 | SW | 設定後・未接続→HiddenRestored | [] / nil | Entries を Retained に複製。まだ照合しない |
| HB-HR-02 | SW | s DOWN、初回 Map、同 Z/同ソース/false の H(x) 記録、y=false | [] / nil | H として一覧に戻す |
| HB-HR-03 | SW | s OK、初回 Map、同 Z の H(x) 記録、x は地図 true、y=false | [] / nil | 記録削除、永続化 |
| HB-HR-04 | SW | s DOWN、初回 Map、別 Z の H(x) 記録、現 Z の x/y=false | [] / nil | Retained 維持、件数を警告へ |
| HB-HR-05 | SW | s OK、初回 Map、U(x) 記録、x true、y=false | [] / nil | H-02 の例外で true でも U 保持 |
| HB-HR-06 | SW | 2回目の HiddenRestored | [] / nil | H-01 決定どおり無視し、一覧を再投入しない |
| HB-HW-01 | SW | 初回予約保存失敗→HiddenWriteFailed(#h) | [] / nil | #h が作った P/Q を予約前へ巻き戻す |
| HB-HW-02 | SW | 同上 | [] / nil | LastError「隠した項目を記録できないため、隠すのを止めました」、警告 |
| HB-HW-03 | SW | 不明/処理済み BatchID | [] / nil | 他バッチの一覧・期待を消さない |
| HB-SP-01 | SW | 接続中、一覧 H(x)→Stopping | [RA] / nil | StoppingReceived=true、RestoreIssued=true |
| HB-SP-02 | SW | 未接続→Stopping | [] / nil | 停止済み印、core はすぐ app へ完了通知 |
| HB-SP-03 | SW | 2回目の Stopping | [] / nil | RA を再発行しない |
| HB-SP-04 | SW | 停止中→任意の Command | [] / ErrInvalidState{終了中} | E 以外変化なし、未接続でもこの理由を優先する |
| HB-SP-05 | SW | 停止中→回線/地図/項目/応答/Tick | [] / nil | 照合・保存は続けるが Auto/Hc/Sc は生成しない |
| HB-MA-01 | SW | 接続中、s DOWN、x=H、y は一覧外 false → CmdShowAllCameras | [MA] / nil | 非表示カメラ項目 ∪ 現 Z の全一覧 P/H/U ∪ false Q の鍵を固定。記録保持、受付以前の Hc を無効化 |
| HB-MA-02 | SW | 未接続→同 Command | [] / ErrNotConnected | 503。コレクション切替中も同じ |
| HB-MA-03 | SW | MA 未完了→同 Command | [] / ErrInvalidState{同じ操作が実行待ち} | 409、重ねない |
| HB-MA-04 | SW | 有効地図、対象0、現 Z 記録なし → 同 Command | [] / nil | 200、他 Z 記録は保持 |
| HB-MA-05 | SW | s DOWN、x=H、別 Z 記録あり → MA 受付 | [MA] / nil | 現 Z と別 Z の記録を保持。項目ごとの true 決着で現 Z の対象だけ消す |
| CMD-SOFF-5/0a/0b | SW | S manual/A、健康 OK、待っている項目あり、auto=true | [] / nil | auto/A に変える。Show は次の入口で判断 |
| HB-OF-01 | SW | 同上、auto=false | [M(A)] / nil | 待たず、I(A) は standby 扱いだけ |
| HB-MA-06 | SW | 空地図、現 Z に P/H/U 記録 → CmdShowAllCameras | [] / nil | 200、全記録を触らず保持、カメラ表示操作なし |
| HB-SP-06 | SW | 接続中、有効地図、一覧も false Q も空 → Stopping | [] / nil | 停止済み印、RA なし、core が保存後即完了通知 |

MA は受け付けた同じ Handle で Hc/Sc/Auto を混ぜない。
その後は通常規則なので、同じ不調の hide が再発することは仕様どおり。MA は B で送出時刻を更新しないが、同項目の Q・同カメラの Pending が消え、Hide/Show 共通の前回送出から5秒以上のときだけ再生成できる。期待一致のイベント後も Q は応答から2秒まで保持する（H-03）。
停止 RA の対象が空ならバッチを返さず、core は保存後ただちに完了通知する（H-06）。
既存 Command の配信条件・重複・X の拒否順は、停止判定を先頭へ足す以外 0a の表のまま。

### 4.7 Approve

E→Pending の存在→目的別の共通条件→項目の(1)〜(4)→Proceed の順。
Show の Restart は項目(1)〜(3)を行わず、共通条件の後にカメラ状態だけを見る。

| # | T | 前の状態・対象 | 結果 | 状態の変化 |
|---|---|---|---|---|
| HB-AP-01 | SW | Hc の最初の有効な false | Proceed{ReqID,MustPersist=true} | 全固定項目を P 予約、Q と台帳登録、core が1秒以内に保存してから返す |
| HB-AP-02 | SW | Hc の2個目以降 | Proceed{ReqID,MustPersist=false} | 全件予約の再書込み不要。個別 Q と台帳登録 |
| HB-AP-03 | SW | (1) 項目が地図にない | Skip | 一覧から外す、Q なし |
| HB-AP-04 | SW | (2) Hc、ソースが今は別カメラ | Skip | 新カメラを隠さない。既存記録の復旧判断は残す |
| HB-AP-05 | SW | (3) Hc、すでに false | Skip | Q なし。予約しただけなら BatchDone で P を外す |
| HB-AP-06 | SW | Sc、地図 true、H(x)、false Q なし | Skip | H を削除。U の場合は U を保持する別行 HB-AP-27 |
| HB-AP-07 | SW | Hc、c OK / uあり / 自動無効 / 非配信 / on_bad=standby / Studio on かつ設定 standby・ignore | Reject | c が有効な hide でない場合も残りを送らない |
| HB-AP-08 | SW | Hc、StartScene 未完了または期待残存 | Reject | false を送らない |
| HB-AP-09 | SW | Hc、現在または期待の残る監視対象切替先の仮 A(R) が空 | Reject | 生成と同じ検査。F の OK だけでは待機到着でない |
| HB-AP-10 | SW | Hc、SceneEpoch/ConfigEpoch だけ変化 | Proceed | 最新条件を満たすなら Q 登録 |
| HB-AP-11 | SW | Hc/Sc、StreamEpoch/ConnEpoch 不一致 | Reject | Q 追加なし |
| HB-AP-12 | SW | Sc Restart、c OK、復旧対象の SourceUUID/CameraID が一致 | Proceed | 固定入力を実行。Q なし |
| HB-AP-13 | SW | Sc Restart、c 不良または対象の SourceUUID/CameraID 不一致 | Skip | 次の true へ進む |
| HB-AP-14 | SW | Sc true、一覧あり/地図 false/その項目を戻せる | Proceed | Q(true) 登録 |
| HB-AP-15 | SW | Sc true、一覧から外れた/戻す条件が消えた | Skip | 他項目は続ける |
| HB-AP-16 | SW | Sc true、SourceUUID または現在の CameraID が記録と不一致 | Proceed | 当該項目は戻せる理由あり |
| HB-AP-17 | SW | RA/MA true、地図 true、P/U または false Q あり | Proceed | Skip 例外、Q(true) 登録。false Q は応答期限まで保持 |
| HB-AP-18 | SW | RA true、地図 true、H だけで false Q なし | Skip | 送らず成功扱い |
| HB-AP-19 | SW | RA、StreamEpoch だけ違う | Proceed | 接続・ConnEpoch 一致だけを使う |
| HB-AP-20 | SW | 停止中の Auto/Hc/Sc 後続 | Reject | 送信済み要求だけは結果を処理 |
| HB-AP-21 | SW | RA、未接続/ConnEpoch 不一致 | Reject | 残る記録を保存して停止へ |
| HB-AP-22 | SW | MA の一覧外 true | Proceed | カメラ項目の対象であることを再確認（H-06） |
| HB-AP-23 | SW | AutoRecover、生成後に待つ項目ができた | Proceed | 健康ほか従来条件成立時。待つ条件は承認に足さない |
| HB-AP-24 | SW | OffMonitored、生成後に待つ項目ができた | Proceed | 同上。健康悪化なら従来どおり Replace |
| HB-AP-25 | SW | MA 受付より前に生成した Hc の残り | Reject | 新しい false は送らず、未送信予約は BatchDone で巻き戻す |
| HB-AP-26 | SW | 停止中、受付済み Command 由来または StartScene の承認、従来条件成立 | Proceed | 停止による追加却下はしない。従来の接続・Epoch・目的条件で承認 |
| HB-AP-27 | SW | 通常 Sc、地図 true、U(x)、false Q なし | Skip | U 記憶は保持。明示 MA の表示確認・停止 RA の true OK で消す |
| HB-AP-28 | SW | RA/MA、地図 true、U(x) | Proceed | true を送信。MA は期待どおりの true イベントまたは読み直し地図の表示確認で U 削除、RA は true OK で削除 |

最初の Action が Skip なら保存を要する false はまだない。
最初の Proceed 候補のときに全対象を予約する（H-01）。
「全対象」にすでに H/U があるときは上書きせず、予約前の値を巻き戻し台帳に持つ。
Sc の Restart 対象は、c の入力のうち地図で表示の項目が1つもない入力。
入力 UUID を重複排除して辞書順、Wait は末尾に1回。c 不良なら生成時にも Restart を省く。
親経路で見えないだけで項目 true の入力は「表示の項目あり」として除く（H-08）。

### 4.8 Hide/Show/Restore の BatchDone

Hc/Sc は要求失敗でも後続を走査し Completed+Err。Failed は発生しない（M-8）。

| # | T | 前の状態 → 完了 | 出す列 | 状態の変化 |
|---|---|---|---|---|
| HB-BD-01 | SW | s DOWN、Hc、一部 Skip/Reject、送信済みあり、送出から5秒未満 | [] | 未送信 P を予約前へ巻き戻す。元 H/U は保持。Pending 削除 |
| HB-BD-02 | SW | s DOWN、Hc Completed、Err=nil、実要求成功、送出から5秒未満 | [] | f=0、unusable=false |
| HB-BD-03 | SW | s DOWN、Hc Completed+Err、f=0、送出から5秒未満 | [] | f=1、LastError はまだ更新しない。3到達時のみ記録 |
| HB-BD-04 | SW | s DOWN、Hc Completed+Err、送出から5秒未満、f が2→3 | [] | unusable=true、last_error、健康を再計算 |
| HB-BD-05 | SW | s DOWN、Hc Rejected、実要求なし、送出から5秒未満 | [] | 失敗回数を増やさない。未送信 P を掃除 |
| HB-BD-06 | SW | s DOWN、Hc 全 Skip Completed、f=1、送出から5秒未満 | [] | f=1 を保持、Pending 削除 |
| HB-BD-07 | SW | s OK、Sc の true OK、イベント未着、送出から5秒未満 | [] | 通常 Sc は一覧をまだ外さず Q のイベント/2秒補正待ち |
| HB-BD-08 | SW | RA 完了、true OK/Timeout/Error が混在 | [] | OK の項目のみ外す。失敗分保持、保存後に停止完了通知 |
| HB-BD-09 | SW | RA Rejected/Failed | [] | 未復旧分保持、core は完了通知して停止可能 |
| HB-BD-10 | SW | 不明/重複 BatchDone | [] | E のみ、回数・保存を二重更新しない |
| HB-BD-11 | SW | MA Completed+Err、x true OK（イベント未着）、y true Error | [] | x/y 記録保持。x の Q は応答から2秒まで、以後も読み直し地図の表示確認まで記録を保持。LastError「表示に戻せなかった項目があります。もう一度押してください」 |

MA の true は ItemOK だけでは記録を消さない。期待どおりの true イベントまたは読み直し地図での表示確認で、その項目の P/H/U・Timeout の記憶・永続記録を消す。MA の OK 後にイベントが来ない場合は、応答から2秒を超えた E で Q だけを消し、通常 Sc の無イベント補正による記録削除はしない。記録は次の読み直しで表示と確認するまで残す。停止 RA の OK 削除とは分ける。 MA による表示確認待ちは Q の消去後も送信台帳で保持し、次の読み直しが true なら U も削除する（一般照合の U 保持とは別）。未確認の P は10秒掃除でも消さない。失敗した要求だけで確認待ちを成功扱いにせず、false の地図なら保持する。

ItemRequestDone ごとに f を増やさない。複数項目が失敗しても1バッチで+1。
同じバッチ内の成功1件で、他項目の失敗を打ち消さない。
最後の送出から5秒未満なら BatchDone 起点の Jb でも同カメラを出さない。
シーンの BatchDone がもともと J を行わない規則は 0a の表のまま。
ただし StartScene は未完了か期待が残る間 Hide を止め、両方が消えた後の判断入口で再開する。

## 5. 列のケース

時刻 t は秒。各列は独立。送信は常に個別の Approve を挟む。
`送信(k,v,r)` は Proceed→初回 false の保存成功→実送信を指す。
`完了` の略記でも ItemRequestDone と BatchDone とイベントの順を下記に示す。
待ち W=0.5秒。RA の2秒は core の停止待ちで、Machine の Q の期限と別。

### HB-SQ-01 メイン standby、サブ hide（要求 a）

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | s LOW | [Hc(s)#1] | A の健康 OK、まだ地図 true |
| 1.1 | x/y false 承認、保存、送信 | — | 全件 P、個別 Q、m は表示を維持 |
| 1.2 | 両 ItemOK、BatchDone Completed | [] | 決着時刻、Q の期限3.2 |
| 1.3 | x/y false | [] | H、s は V(A) に残るが A(A) は m のみ |
| 7 | s OK | [Sc(s)#2] | f=0、u=false、Restart(is,W) あり |
| 7.1〜7.6 | Restart→true の個別承認・送信 | — | Q(true)、H は維持 |
| 7.7 | ItemOK、BatchDone、x/y true | [] | 一覧なし、A のまま、健康 OK |

### HB-SQ-02 サブだけのシーン（要求 b）

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | D、s DOWN | [F(D)#1] | Hide なし、s は表示 |
| 1.1 | SetScene(S) 承認、SceneRequestDone OK | — | D のまま、Hide を許さない |
| 1.2 | BatchDone Completed | — | シーンイベント待ち |
| 1.3 | SceneChanged(S) | [Hc(s)#2] | auto/D、待機到着後なので隠せる |
| 1.4〜1.6 | false 承認保存→ItemOK→false→BatchDone | [] | L(s)=H |
| 8 | s OK | [Sc(s)#3] | D に待つ項目あり、R(D) はまだ出ない |
| 8.1〜8.8 | Restart、true 承認→OK→true | [R(D)#4] | 非表示が解消したので戻れる |
| 9 | R の承認・完了・SceneChanged(D) | [] | D、C。hide 入力を R で再 Restart しない |

別列: t=1.1 の SetScene が Error、または期待期限切れなら D に残り Hide なし。

### HB-SQ-03 隠している途中で人が戻す（要求 c）

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | s DOWN、Hc 発行 | [Hc(s)#1] | x/y は固定 |
| 1.1 | x false Proceed、保存→OK→x false | — | x=H、y=P |
| 1.2 | 人が x true、true Q なし、false Q 残存 | [] | x の記録を保持し u(s)=true |
| 1.3 | y false の承認 | Reject | u があるので残りを送らない |
| 1.4 | BatchDone Rejected | [] | 未送信 y の P を外す、f は増やさない。x は false Q 終了時に地図 true なら外す |
| 7 | Tick、まだ s DOWN | [] | 同じ不調中、隠し直さない |
| 8 | s OK | [] | u 消去 |
| 9 | s DOWN | [Hc(s)] | 新しい不調として隠せる |

### HB-SQ-04 Timeout と同じ Z の再接続（要求 d）

開始 A、m OK、s DOWN。y は地図 false の一覧外項目、x のみ表示。

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | Hc(s)#1、x false 承認保存・送信 | — | P(x)、Q(false,r) |
| 6 | ItemTimeout(r)、BatchDone Completed+Err | [] | U(x)、期限8、f=1、送信台帳に決着 |
| 6.1 | OBSDisconnected | [] | Q 消去、U・台帳保持 |
| 7 | 同 Z の OBSConnected、x true、s DOWN | [Hc(s)#2] | U を保ち表示と数える。5秒経過なので再送対象 x |
| 7.1 | x false 承認保存→ItemOK→false→BatchDone | [] | U→H、f=0、Q は9.1まで保持 |
| 18 | Tick | [] | H 保持、対象はすでに false |
| 21 | s OK | [Sc(s)#3] | true に向かう |

### HB-SQ-04B 切断が結果より先

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | A、s DOWN、x false を Proceed・保存・送信 | — | P、送信台帳あり |
| 2 | OBSDisconnected | [] | Q 消去、台帳と P 保持 |
| 2.1 | 実行役から ItemTimeout | [] | 台帳で P→U、Q は再生成しない |
| 2.2 | 旧 BatchDone Completed+Err | [] | f=1、旧 Pending 削除 |
| 3 | s OK（未接続） | [] | f=0 |
| 7 | 同 Z 再接続、x true | [] | U 保持。s OK のため Hc なし、Sc も非表示対象なし |
| 20 | 遅れた x false | [Sc(s)] | U→H、応答待ちの期待なしでも拾う |

再起動を挟む同列では t=7 の前に HiddenRestored(U x) を渡し、同じ期待値を検査する。

### HB-SQ-05 配信停止（要求 e）

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | s DOWN、Hc→false OK、まだイベント未着 | — | P、Q(false) |
| 2 | StreamStateChanged(false) | [] | 項目 Q 保持、C、u 消去。Hide 後続は StreamEpoch で Reject |
| 2.1 | 遅れた false | [] | H、停止によって人扱いにしない |
| 2.2 | Hide BatchDone | [] | 未完了解消、5秒制限のためまだ Show なし |
| 6 | Tick | [Sc(s)] | c 不良なので Restart なし |
| 6.1〜6.3 | true 承認→OK→イベント→BatchDone | [] | 一覧なし、配信 false |

### HB-SQ-06 本体の停止と2秒（要求 f）

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | Hc 実行中、x false 送信済み、y 未送信 | — | x/y=P、x の false Q あり |
| 1.1 | Stopping | [RA#2] | 列の先頭、実行中バッチの次へ |
| 1.2 | y false の承認、旧 BatchDone | Reject、[] | y は未送信 P から除く |
| 1.3 | RA x true 承認 | Proceed | 地図 true でも P/false Q があるので送る |
| 1.4 | true ItemOK、RA BatchDone Completed | [] | x を外し保存、イベントを待たず app へ通知 |
| 1.5 | 停止完了 | — | 保存済み、プロセスが終了できる |

別列: RA の要求が戻らなければ t=3.1 で core が待ちを打ち切る。
未復旧記録を消さず、新世代の HiddenRestored に渡す。RA の自動再送はない。

### HB-SQ-07 hidden.json からの起動（要求 g）

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 0 | ScenesConfigured→HiddenRestored(H x、別 Z の H z) | [] | 未照合の Retained |
| 1 | OBSConnected、非配信、同 Z、x false/同ソース | [Sc(s)] | x=H、z は残した記録。Restart なし |
| 1.1 | true 承認→OK→true→BatchDone | [] | x 削除、z 保持 |
| 2 | Snapshot と保存 | — | 別 Z の件数を警告 |

別列: 配信中・s DOWN なら隠したまま。設定にないカメラの x は非配信でなくても戻す。
空地図なら t=1 は []。あとで有効地図が来たとき照合（H-02）。

### HB-SQ-08 Studio Mode の3値（要求 h）

各列の開始 A、m OK、s DOWN、x/y=H、最後の Hide から5秒以上。
standby 列の F は Item ではなく SceneRequestDone Error→BatchDone で終了し A に残す。
Sc は t=1.1 に true の OK・イベント・BatchDone まで成功させる。

| t | 入力・設定 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | on、standby | [F(A),Sc(s)] | standby 扱い、s を戻す |
| 1.1 | standby 列の上記完了 | [] | A、一覧なし、x/y true |
| 8 | standby 列 s OK | [] | 戻す項目なし、u=false、f=0 |
| 9 | standby 列 off | [] | A、一覧なし |
| 1 | on、ignore（独立列） | [] | s は健康で悪いと数えず H 保持 |
| 8 | ignore 列 s OK | [Sc(s)] | ignore 中も OK は戻す理由 |
| 8.1 | true OK・イベント・BatchDone | [] | 一覧なし |
| 9 | ignore 列 off | [] | A、追加操作なし |
| 1 | on、hide（独立列） | [] | H 保持 |
| 8 | hide 列 s OK | [Sc(s)] | 通常の OK 回復 |
| 8.1 | true OK・イベント・BatchDone | [] | 一覧なし |
| 9 | hide 列 off | [] | A、追加操作なし |

### HB-SQ-08B ignore 中の他の戻す理由

次の各行は独立。開始 Studio on/ignore、A、s DOWN、x/y H、送出時刻なし。

| # | T | 入力 | 出す列 | 状態の変化 |
|---|---|---|---|---|
| HB-SM-07 | SW | StreamStateChanged(false) | [Sc(s)] | H は true の決着まで保持、Restart なし |
| HB-SM-08 | SW | ScenesConfigured auto=false | [Sc(s)] | H 保持、Restart なし |
| HB-SM-09 | SW | ScenesConfigured s on_bad=standby | [F(A),Sc(s)] | 新設定 on_bad=standby の s を数え健康 DOWN、ConfigEpoch+1。ignore の対象外。H と Q/Pending 保持、Restart なし |

### HB-SQ-09 コレクションを切り替えて戻る（要求 i）

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | X、x=H→OBSDisconnected | [] | H 保持、Q 消去 |
| 2 | Y の OBSConnected、同じ UUID の x、m=s OK、全項目 true | [] | X の x は Retained。Y の x を勝手に戻さない |
| 3 | Y→OBSDisconnected | [] | X 記録保持 |
| 4 | X の OBSConnected、x false、s OK | [Sc(s)] | 同一ソース照合成功、H として復元 |
| 5 | true の完了 | [] | X の記録削除、Y の記録はそのまま |

### HB-SQ-09B 本体切替の期待中の空検査

各行は独立、m OK、s LOW、一覧なし。D は s だけの監視対象。

| # | T | 前の状態 → 入力 | 出す列 | 状態の変化 |
|---|---|---|---|---|
| HB-GD-01 | SW | S auto/D、SetScene(D) Proceed・OK、期待残存 → CamerasChanged s OK→LOW | [] | D を空にする Hc を生成しない |
| HB-GD-02 | SW | T、StartScene(D) Completed、期待残存 → Tick | [] | 完了後も期待が残るので Hc なし |
| HB-GD-03 | SW | S auto/D、Hc(s) 発行済み、その後 SetScene(D) Proceed、期待残存 → y false 承認 | Reject | 期待先 D を空にするので却下 |



### HB-SQ-10 すべて表示に戻す（要求 j）

開始 A、m OK、s DOWN、前回 Hc 送出時刻なし、x=H、y は人の false、別 Z に記録。

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 2 | CmdShowAllCameras | [MA#1] | x/y 対象、現 Z の記録も保持。送出時刻は更新しない |
| 2.1 | x/y true 承認・送信、両 ItemOK、MA BatchDone | [] | x 記録保持、y は一覧外でも送る。Q(x/y,true) の期限4.1、別 Z 保持。Hide/Show 送出時刻なし |
| 2.2 | x true イベント | [] | x 記録削除、Q(x,true) は SawEvent=true で保持。同項目 Q があるので再 Hide なし |
| 2.3 | y true イベント | [] | y true、Q(y,true) も保持。再 Hide なし |
| 2.4 | Tick | [] | Q(x/y,true) が残るため false バッチなし、地図 x/y true |
| 4.1 | Tick | [] | 期限ちょうどは Q 保持 |
| 4.2 | Tick | [Hc(s)#2] | Q(x/y,true) 消去。前回送出なし、m が残るので x/y 両方を対象に生成、LastItemSent(s)=4.2 |
| 4.3 | #2 x/y false 承認→ItemOK→false イベント→BatchDone | [] | x/y=H、地図 false、false Q の期限6.3 |
| 8 | Tick | [] | false Q は期限超過で消去、x/y は既に H/false。前回送出から3.8秒で、新しい Hide なし |

### HB-SQ-10A MA が実行中 Hide の残りを止める

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 1 | s DOWN、Hc#1[x,y]、x Proceed・保存・送信、y 未承認 | — | x/y P、x false Q |
| 2 | MA Command | [MA#2] | 地図 true の x/y も全一覧として対象、記録保持 |
| 2.1 | #1 y 承認 | Reject | MA より前の Hc の残りは送らない |
| 2.2 | #1 BatchDone Rejected | [] | y は未送信 P を巻き戻し、x P と台帳保持 |
| 2.3 | x false OK・イベント | [] | x H。5秒未満なので Hc 再生成なし |
| 2.4 | #2 x/y true 承認→OK→イベント→BatchDone | [] | 項目ごとに記録・U を削除、x/y true |

### HB-SQ-10B MA とイベント未着・失敗

開始 A、s DOWN、Hc#1 は t=1 送出、x false Proceed・保存・ItemOK 済み、地図 true、y は対象外。

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 2 | MA Command | [MA#2] | false Q と一覧の x が対象、記録保持 |
| 2.1 | 遅れた x false イベント | [] | x H、Q の一致でも期限まで保持 |
| 2.2 | #1 BatchDone、#2 x true Proceed→ItemError→BatchDone | [] | x H 保持、last_error に再操作案内 |
| 2.3 | Snapshot・ファイル検査 | — | x の記録は一覧と hidden.json の両方に残る |

### HB-SQ-10C MA の OK 後もイベントが来ない

開始 A、s DOWN、x=H、地図 false、y は地図に存在せず対象外。最後の Hide/Show から5秒以上。各 MA true は同じ表示確認規則を使う。

| t | 入力・処理 | 出す列 | 期待する状態 |
|---|---|---|---|
| 2 | CmdShowAllCameras | [MA#1] | x を対象に固定、記録保持 |
| 2.1 | x true 承認→ItemOK→BatchDone（イベント未着） | [] | x=H、地図 false、Q の期限4.1。OK だけでは削除しない |
| 4.1 | Tick | [] | Q と H 保持 |
| 4.2 | Tick | [] | Q だけ消去、MA の true 補正で地図や H を変えない。次の読み直し待ち |
| 5 | SceneMapChanged、読み直しで x true を確認 | [Hc(s)#2] | 現 Z の x 記録を削除後、通常判断で x を再 Hide 候補にする。Q/未完了なし、5秒条件成立、m は表示を維持。人のイベントではないため u は付けない |

別列: t=5 の読み直しが x false なら []、H とファイルの記録を保持する。MA の true が失敗した場合は HB-SQ-10B のとおり保持する。

## 6. 第5節の4つを M-12 で確かめる案

期待値の計算に Machine の Health/CamerasIn をそのまま使わない。
偽 OBS は実際の program、全項目、適用済み要求列、永続ファイルを別に持つ。
乱数の seed と入力・承認・適用・応答・イベント・保存の全列を失敗時に出力する。
通常動作の列と、保証外の障害を入れた列を分ける。
前者には5秒以内の応答、クラッシュなし、イベント欠落なしの制約を掛ける。
イベント遅延・重複・応答前イベント、補助地図の遅れは通常列にも入れる。

### HB-INV-01 記録のない非表示を作らない

- 判定点: false の保存障壁、実送信、偽 OBS の適用、イベント処理、ファイル書換え。
- 偽 OBS が「どの接続のどの要求が値を変えたか」を事実として記録し、本体が true→false にした鍵を人の非表示と別に保持する。すでに false への false は含めない。
- 各鍵が適用の瞬間に同 Z/SourceUUID の hidden.json のファイル内容にあることを判定。Machine の分類を oracle に使わない。
- true が決着するまで記録がファイルから消えたら失敗。
- 未送信予約の削除は、本体 false が一度も送られていないことと照合。
- 送信前保存失敗の列では、偽 OBS に false が1件も届かないことも判定。
- Timeout 保証外列では U の保存と遅着 false の回収を別の到達性として確認。
- MA に例外は設けない。受付・部分失敗を含め同じ保存条件を検査する（C-1）。

### HB-INV-02 配信中の今のシーンを空にしない

- 判定点: 各 false の Proceed 直前、偽 OBS での各 false の適用直後、本体 SetScene の適用直後。
- 承認時には既承認 false の集合を全て重ねた項目グラフを独立にたどる。
- 入れ子の共有末端、ひし形、同カメラ複数入力、循環を生成する。
- 承認までに本体に渡したイベントで決まる program と本体が Proceed した SetScene の先を独立に数え、監視対象で配信中なら空にしない。本体に未着の人の切替とのすれ違いは除外する。
- F の OK 応答だけでは待機到着とみなさず、program の実適用も別に記録。
- 人が空にした列は本体の違反から除外し、AutoFail の到達性を確認。
- Studio Mode hide/複製ありは program 複製の既知制約として別列にする。
- U の表示扱いによる仮想表示と実 OBS の差は保証外列として記録する。

### HB-INV-03 人の判断を優先する

- 判定点: 期待不一致の true を処理した直後と、その後の各 false の承認。
- oracle にカメラごとの「同じ不調の区間」を持つ。
- 偽 OBS が人の操作として出した true イベントを本体に渡した事実から観測を立て、oracle が独立計算した戻せる条件がなければ区間に印を立てる。
- 人が隠した項目に本体が true を送らない（明示 MA を除く）。未送信 P を H にしないこともここで検査。
- 区間内で本体の新しい false が Proceed しないことを判定する。
- 区間を閉じるのは OK 到達と配信停止。単なる Tick/地図変更では閉じない。
- 観測より前に送信済みの false の遅着は取消不能として別に数える。
- OK 無イベント補正が、その承認以降に1件でも項目イベントを見た場合に走らないこと。

### HB-INV-04 戻す方へ向かう

- 安全性だけでなく、入力列の末尾へ安定した回復区間を加える到達性検査にする。
- 接続正常、要求成功、地図確定、Tick 継続、対象ソース維持を回復区間の前提にする。
- 非配信/auto=false/on_bad=standby 等の戻せる条件を固定し、全対象の true 到達を確認。
- 回復条件に OK を加え、Studio on/ignore のままの回復と S auto/A から A への復帰も検査する。
- 上限は 5秒×3 + 2秒 + 全対象のバッチ処理時間 B + Tick 間隔1秒 = 18秒+B。B は回復区間に許す全要求数 N に応答上限5秒と各 Wait の合計を足した 5N秒+ΣWait とする。
- f>=3 でも試行が永久停止しないことを確認（H-04）。
- 別 collection の項目は現 collection で true にせず、戻った後に同じ検査を行う。
- Stopping は1回の RA と2秒まで。未復旧ならファイルに残ることを代わりに確認。
- 次世代へファイルを渡し、接続回復・非配信の安定区間で復旧を確認。

## 7. H-01〜H-09 の決定と撤回理由

指揮者の決定と spec v0.15 を反映した。ここに未決の採否はない。

### H-01 型・ReqID・保存障壁

決定: 既存 Approve を広げ、Machine が uint64 ReqID を振り Proceed に返す。MustPersist はこの承認前の同期保存を示す。上限1秒、割り込みなし、失敗は HiddenWriteFailed。Snapshot/Revision は一覧の写しの読み口だけ。

反映: 1.3〜1.4、HB-AP-01/02、HB-HW-01〜03、HB-HR-01/06。

撤回・選択の理由: 旧案の項目専用 API・実行役採番は二重の所有者になるため撤回。Revision の差だけでは保存失敗時の Reject と警告を区別できないため、障壁は Proceed の印にした。

### H-02 U と初回照合

決定: U は同 Z/同ソースなら地図 true でも保持し、false なら H。削除・別ソースは一般照合に従う。空地図の後の最初の空でない SceneMapChanged でも照合する。通常 Sc の true は U 記憶を消さず、明示 MA の true 決着と停止 RA の true OK で対象を消す。

反映: HB-MP-05/06/08、HB-HR-02〜05、HB-OC-05、HB-AP-17/27、HB-MA-06、HB-BD-08。

撤回・選択の理由: 一般照合で U の地図 true を捨てる読みは、切断・再起動をまたぐ記憶を失うため採らない。停止 RA の OK 削除は停止契約を優先する。

### H-03 項目照合と境界

決定: 照合は同じ項目内。先着一致で SawEvent を立てても Q は応答から2秒まで保持。Error/NotFound は即消去。境界は2秒も10秒も >。台帳は切断で消さず、結果処理と期待・バッチで不要になった後に捨てる。

反映: HB-IE-10/12、HB-IR-08/12〜14、HB-EX-01〜07、HB-SQ-04B。

撤回・選択の理由: 応答前一致で Q を削除する旧案は遅着 false を記録できないため撤回。地図 false の記録は Error でも消さない。

### H-04 失敗単位と再送

決定: Hide/Show 共通時刻。バッチ単位で失敗+1、実要求が全成功なら0。全 Skip と実要求なし Reject は維持。3到達時だけ last_error。3以降も戻す試行を続け、Hide の失敗も unusable に反映。Restart 失敗でも true は続ける。

反映: HB-BD-02〜06、HB-TK-04、1.2。

撤回・選択の理由: 失敗を項目ごとに数えると項目数で歯止めが変わるため採らない。全 Skip は実成功でないため0にしない。

### H-05 collection 境界

決定: u/f/送出時刻はカメラ単位で保持。OK と配信停止で u を消す。設定から削除したカメラの u/f/送出時刻は捨てる。

反映: 2.2、HB-CF-02、HB-SQ-09。

撤回・選択の理由: 再接続で回数を0にすると失敗の歯止めが無効になる。別 collection でも同じ不調中は人の判断を尊重する。

### H-06 MA と停止 RA

決定: MA は非表示全カメラ項目・全一覧 P/H/U・false Q を対象にし、受付以前の Hc の残りを Reject。記録は各 true 決着で消し、失敗は残して last_error。RA 対象0ならバッチなしで core が保存後即完了。

反映: HB-MA-01〜06、HB-AP-17/25/26、HB-BD-08/11、HB-SQ-10/A/B。

撤回・選択の理由: 受付時の削除と再生成防止印は記録なし非表示を作るため撤回。INV-01 の MA 例外も撤回。空 RA を実行役へ流す必要はない。通常保存失敗は警告と再保存対象を残す。

### H-07 Studio ignore

決定: ignore は on_bad=hide にだけ効く。設定変更は新設定適用後に健康と目的条件を計算する。ignore 対象は新しく隠さず、待機へ送らず、健康で悪いと数えない。Studio on だけでは戻さず、OK・非配信・auto=false・on_bad=standby で本体の一覧だけ戻す。

反映: 3.2、HB-SM-02/04/07〜09、HB-SQ-08/B、HB-INV-04。

撤回・選択の理由: ignore が他の戻す理由も無効にする読みは待機から復帰不能になるため撤回。人が隠した項目には true を送らない。

### H-08 待ちと Restart

決定: 待ちは戻り先へ映るカメラの一覧全体の非表示を調べる。Show の Restart は項目 Enabled を使い、親経路だけの不可視では再起動しない。

反映: 3.3、HB-AP-12/13、HB-SQ-02。

撤回・選択の理由: 戻り先の経路だけ待つ代案は全シーンの Hide との対応を失うため採らない。

### H-09 全部 hide・不良

決定: 特別な抑止は置かず、同じ Handle 内の先行 Hc 候補を重ねて通常の空検査を使う。設定順 m,s で [F(A),Hc(m)]、s が残る。

反映: 3.3、HB-CC-07、HB-SC-01。

撤回・選択の理由: F を生成する列だけ Hide を止める旧案は、期待待ちや X により F が出ない列で結果が変わるため撤回。

## 8. 実装への受け渡しと文書の検証

- 0a の表のままの行を先に走らせ、hide のない fixture の退行を検査する。
- HB の各行を rows の名前に、HB-SQ を seq の名前にそのまま使用する。
- テストの状態差分には Hidden/Retained/Q/送信台帳/u/f/送出時刻を追加する。
- Snapshot と保存データの所有権を検査し、外から slice を変えても状態が変わらないこと。
- M-10 の既存型を使い、承認の ReqID と保存印を M-12 の実行役に渡す。
- 本文は実装完了や M-12 の成功を主張しない。製品コードとテストは変更していない。
- レビュー指摘と反映先は第9章で追跡する。

## 9. レビュー対応の索引

| 指摘 | 反映先と対応 |
|---|---|
| C-1 / H-06 | HB-MA-01/04/05/06、HB-AP-17/25/28、HB-BD-11、HB-SQ-10/A/B、HB-INV-01。受付削除を撤回、項目ごとの表示確認で削除。OK のみ・無イベント期限超過では保持（HB-SQ-10C） |
| 閉鎖確認 新1 | HB-BD-11、HB-AP-27/28、HB-SQ-10/C、HB-EX-08 と C/A、4.3・4.8。MA は true OK だけで記録を消さず、無イベント期限後も次の読み直しの表示確認まで保持 |
| 閉鎖確認 新2 | 2.1、4.6、HB-SQ-10。t=2.2〜4.1 の再 Hide を抑止、t=4.2 に Q 消去後 x/y を再生成 |
| 閉鎖確認 新3 | 3.2、4.5、HB-SM-09、HB-CF-03、H-07。新設定適用後に健康 DOWN を計算し [F(A),Sc(s)]、Restart なし |
| C-2 | 3.3、HB-AP-08/09、HB-ST-01、HB-SC-01〜04、HB-GD-01〜03、HB-INV-02。期待が残る切替先も保護 |
| I-1 | HB-IE-10/12、HB-IR-08、HB-EX-02〜05。応答から2秒まで Q 保持 |
| I-2 | 1.3、OD-1/0a/0b、HB-IR-12〜15、HB-SQ-04B。切断後も台帳で結果処理 |
| I-3 | HB-IE-17、HB-MP-03/10、HB-AP-05、HB-BD-01。Proceed 済みだけ H、未送信は元 H/U を含め巻き戻し |
| I-4 | HB-AP-07。on_bad と Studio の有効 hide 条件 |
| I-5 | HB-SM-02/04/07〜09、HB-SQ-08/B、H-07。ignore 中も他理由で復旧 |
| I-6 | HB-SQ-04/B、HB-SQ-10/C、4.6。Q と Pending の解消後、共通5秒条件を満たして再 Hide。t=4.2 に両項目、t=8 は出力なし |
| I-7 | HB-IE/IR/MP/HR/BD 各行、HB-TK-02/05/06、HB-CF-01/03、HB-SM-03/05/09、HB-SQ-10/C、HB-EX-02〜06 の C/A 行。設定適用後の健康と Q の境界を含め状態・出力を具体化 |
| I-8 | HB-SC-01〜05、HB-OC-05/06、HB-MA-06、HB-AP-26/28、HB-SP-01〜06。停止中 OBSConnected は本文の HB-OC-06、空対象 Stopping は実在する HB-SP-06。SceneChanged、初回照合、空地図 MA、停止中承認も本文の行名へ対応 |
| I-9 / H-01 | 1.2〜1.4、HB-AP-01、HB-HW-01〜03。Approve 拡張・Machine 採番・Proceed 保存印・1秒同期保存 |
| I-10 | HB-INV-01〜04。独立 oracle、ファイル、SetScene 適用、人の未着操作の除外、ignore、数値上限 |
| M-1 | 3.1、4.4、HB-MP-02、HB-AP-04/16。健康への照合述語（Z・SourceUUID・CameraID）と復旧対象の選択（現 Z の記録の項目鍵）を分離。URL の CameraID 変更でも現在項目へ true |
| M-2 | HB-TK-05、HB-EX-06。10秒ちょうどは保持 |
| M-3 | 2.2、HB-SQ-05。提案は不採用、共通5秒再送を維持。通常の到達性に違反せず、既存の制限を統一するため |
| M-4 | HB-BD-03/04、H-04。last_error は3到達時だけ |
| M-5 | 3.2。有効 Motion と MotionIgnore、V(R) で classOf |
| M-6 | HB-HR-06/07。再投入と接続後初回を無視 |
| M-7 | 2.2、HB-CF-02。設定削除時 u/f/時刻を破棄 |
| M-8 | 1.2、4.8。Hc/Sc は Failed を返さず失敗を集約して Completed |
| M-9 | 4.2、HB-INV-03。人の同値 false を識別できない既知制約 |
| H-02〜05 / H-08 / H-09 | 第7章に各決定・理由・反映行を記載 |

新しく残した不明点はない。M-3 は理由を記して既存規則を維持した。
この改訂は文書のみ。M-11/M-12 の実装・テストの成功を示すものではない。
