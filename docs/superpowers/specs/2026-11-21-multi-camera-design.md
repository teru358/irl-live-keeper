# irl-live-keeper 設計 — 複数のカメラ（第2段階 区切り0）

- 作成日: 2026-11-21
- 版: v0.17（変更は末尾の変更履歴を参照）
- 状態: 2026-11-21 の brainstorming でユーザーが節ごとに承認した内容を文書化したもの。v0.2〜v0.4 で設計レビュー1周目（Fable、codex sol）と閉鎖確認2回（codex terra）の指摘を、v0.5 で M-0 の spike の結果と設計レビュー2周目（Fable）の指摘を反映し、作りを減らした（コレクションの切替を切断として扱う、要求を UUID で送る、古い OBS は空の地図、守ることを先に書き細部は M-7 の表へ）
- 前提: コアの spec `docs/superpowers/specs/2026-11-18-core-design.md`（v2.15。以下「コアの spec」）と第2段階の spec `docs/superpowers/specs/2026-11-20-stage2-design.md`（v0.6。以下「第2段階の spec」）。この文書は両者の一部を置き換える（第10節の表）。食い違うときはこの文書を優先する

## 1. 目的と範囲

ユーザーの用途（2026-11-21 の言葉）: 「メインカメラとサブカメラをそれぞれのソースへ転送。ひとシーンにカメラソースを2本取り込む。手元カメラを適宜使用して、カメラシーンを切り替えて使う」。

- 送信の入力（カメラ）を1〜8本にする。カメラごとに MediaMTX の path、回線の判定、OBS のメディアソースを持つ
- カメラごとに、回線が悪いときの動きを択一で選ぶ: 待機シーンへ送る（`standby`）か、そのカメラのソースを OBS で隠す（`hide`）
- どのシーンにどのカメラが映るかは、本体が OBS から自動で調べる（`obs.monitored_scenes` と `obs.media_source` は廃止）
- RTMP と SRT の受付を個別に止められるようにする
- 第2段階の頭（区切り0）に置く。第2段階の残りの部品は、初めからカメラごとの形で作る（第9節）

**区切り0を2つに分ける**（2026-11-21 のユーザーの裁定。「隠す」は、OBS が固まるなどのめったにない障害の組み合わせで、レビューのたびに新しい穴が見つかったため）:

- **0a**: 複数のカメラ、受付の停止、シーンの地図、カメラごとの判定、シーンの健康、待機へ送る（第2節〜第4節、第6節のうち隠す・戻すを除く部分）。`on_bad: hide` は 0a では受け付けるが、`standby` と同じに扱う（設定の画面と status に「隠す動きは次の区切りで入ります」）
- **0b**: 隠す・戻す（第5節、第6節の HideCamera・ShowCamera・RestoreAll、第7節）。保証は現実的にする: 通常の動作では記録のない非表示を作らない。めったにない障害（OBS が5秒以上固まる、本体か OBS が落ちる、イベントの列があふれる）で非表示が残る場合は既知の制約にし、**「カメラのソースをすべて表示に戻す」**の操作で1回で戻せるようにする（第7節）。0a を実装して使えるようにしてから入る

作り方は、今の switcher を広げて、シーンの切替と「隠す」を1つの状態機械で判断する（2026-11-21 のユーザーの裁定。0b は 0a の switcher に目的を足す）。待機の持ち主と戻り先、期待の照合と期限、実行の直前の承認、目的ごとの有効条件、実行役とのチャネルの決まりは今のまま使う。

入れないもの: カメラごとの資格情報（資格情報は全カメラで共通）、カメラごとの「何もしない」（全体の `switch.auto_enabled: false` で足りる）、カメラごとの「使う / 使わない」の切替（下の運用で足りる。ユーザーの裁定）、悪いカメラの代わりに別のシーンへ送る形。

**使わない日のカメラの運用**（README に書く）: 待機へ送るかの判定は、いま映っているシーンだけで行う。ほかのシーンにある、その日使わないカメラが DOWN でも何も起きない。手元カメラを別のシーンに置く運用（ユーザーの例）では、手元のシーンへ切り替えると、手元カメラが届くまで待機シーンになり、届いて OK になったら（`recover_sec` のあと）手元のシーンへ自動で戻る。待ちを短くするには、そのカメラだけ `recover_sec` を上書きする。戻るときは、先にメインのシーンへ切り替えてから手元カメラを止める。OBS で手で非表示にした項目のカメラは、そのシーンでは判定しない。

**対応する OBS**（ユーザーの裁定。版は M-0 で決めた）: **OBS Studio 30.1.0 以降（obs-websocket 5.4.0 以降）**。UUID の項目、`InputSettingsChanged`、UUID で指定する要求がこの版で入った。つないだときに `GetVersion` の `obsWebSocketVersion` で確かめる（`rpcVersion` では見分けられない）。古ければ、地図を空（どのシーンにもカメラが映らない）として扱い、status の `warnings` に「OBS が古いため、自動の切替と隠す・戻すを止めています」と出す。空の地図では、すべてのシーンが管理外か待機シーンになるので、自動の切替と隠す・戻すは起きず、手動の操作と配信開始時の切替は今の規則（StandbyOff は StandbyOffOther）で動く。`hidden.json` の記録は照合せずに残す。README の「OBS Studio 28 以降」を書き直す。ユーザーの Windows は OBS 33.0.1、Linux の試験は OBS 33.0.0（obs-websocket 5.7.6）

**Studio Mode**（0b。M-0 で、OBS の既定〔「シーンを複製」がオン〕では、program のシーンの項目を隠しても配信の画面に効かず、次の遷移で効くとわかった。「シーンを複製」を切れば効く。本体はこの設定を OBS から読めない）: Studio Mode がオンの間（`GetStudioModeEnabled` と `StudioModeStateChanged`）の `on_bad: hide` のカメラの動きを、設定 `obs.studio_mode_hide` で選ぶ（2026-11-21 のユーザーの裁定。既定は `standby`）。Studio Mode がオフのときは、どの値でもふだんの「隠す」の動き

  | 値 | Studio Mode の間の動き |
  |---|---|
  | `standby`（既定） | hide のカメラを standby と同じに扱う（隠さず、悪ければ待機シーンへ送り、戻ったら戻り先へ戻す）。本体が隠している項目は「c を戻してよい」に入れて表示に戻す（複製された program には次の遷移まで効かない。既知の制約）。隠したつもりで乱れた映像を出すより安全なため |
  | `ignore` | hide のカメラの不調には何もしない（新しく隠さない、待機へも送らない。健康の計算でも悪いと数えない）。本体がすでに隠している項目は、Studio Mode をオンにしただけでは戻さず、そのまま残す。ただし、ほかの「戻す理由」（そのカメラが OK になった、配信の停止、`auto_enabled` の false、`on_bad` を `standby` に変えた）が来たら、Studio Mode の間でも表示に戻す（v0.15、2026-11-21 のユーザーの裁定。戻さないと、戻り先が待っている隠した項目が残り、待機シーンから戻れなくなるため。戻すのは本体が隠した項目だけ）。乱れた映像が配信に出ることがある |
  | `hide` | ふだんどおり隠す・戻す。OBS の「シーンを複製」を切っているときだけ効く（切っていなければ、隠したつもりで乱れた映像が出る。本体からは見分けられないので、使う人の責任。設定のコメントと README に書く） |

  Studio Mode がオンの間は、status の `warnings` に今の動きを出す（`standby`: 「Studio Mode のため、隠す動きの代わりに待機シーンへ送っています」、`ignore`: 「Studio Mode のため、隠す動きを止めています」、`hide`: 「Studio Mode でも隠しています（OBS の『シーンを複製』を切っていないと配信に効きません）」）

  **Studio Mode を切るときの一瞬の切替**（v0.17。M-13 の Windows の確認で見つけた F-9。0a から効く）: OBS は Studio Mode を切ると、program を一瞬 preview のシーンにしてから元へ戻す。`CurrentProgramSceneChanged <preview>` → `CurrentProgramSceneChanged <元の program>` → `StudioModeStateChanged false` の順に、10ms ほどの間に届く。preview が program と同じでも、同じシーンへの切替が2回届く（Windows の OBS 33.0.1 と Linux の 33.0.0 で確かめた）。本体がこれを人の切替と受け取ると、次のことが起きる（Linux で再現した）:
  - 自動の待機のとき: 持ち主と戻り先が消え、カメラが戻っても待機から戻らない。preview が悪い監視対象のシーンのときは、そのシーンが新しい戻り先になり、カメラが戻ると元のシーンでなく preview のシーンへ戻る。どちらになるかは、承認とイベントの順で変わる
  - 手動の待機のとき: 持ち主と戻り先が消え、待機の OFF でも戻らない

  そこで、obs の層（本来の接続のイベントを Event にして流す所）で次のように扱う。Machine と、0a・0b の表は変えない:
  - **取っておく**: Studio Mode がオンだと知っている間（接続の読み取りの `GetStudioModeEnabled` と、そのあとの `StudioModeStateChanged`）に `CurrentProgramSceneChanged` が届いたら、すぐには流さない。最初の1つが届いてから 250ms の間、取っておく。その間に届いた、ほかの流すイベントも、順を保つため後ろに並べて取っておく
  - **捨てる**: 取っておく間に `StudioModeStateChanged false` が届き、取っておいた `CurrentProgramSceneChanged` の最後のシーンが、取っておく前の program のシーンと同じなら、取っておいた `CurrentProgramSceneChanged` をすべて捨てる。ほかのイベントと `StudioModeChanged` は、届いた順に流す。最後のシーンが違えば、最後の1つだけを `SceneChanged` として流す（間の一瞬の切替は流さない）。捨てたときはログ `studio mode off bounce ignored`（`scenes`: 捨てたシーンの名前）を出す
  - **流す**: 250ms たっても `StudioModeStateChanged false` が届かなければ、取っておいたイベントを届いた順に、届いた時刻（`At`）のまま流す。Studio Mode の間の人の切替は、250ms 遅れて本体に届く。本体の切替の期待の期限は秒の単位なので、照合には響かない
  - **やめる**: 切断、読み直しの始まり、コレクションの切替では、取っておいたイベントを捨てる。今のシーンは、つなぎ直しの読み取りで読む
  - **先に流す**（sol のレビュー I-1）: 取っておく間に、エポックの goroutine に頼むイベント（トランジションの取り直し、地図の読み直しを起こすもの、`InputSettingsChanged`、program でない `SceneRemoved`）が届いたら、取っておいたものを窓の終わりと同じに流してから、そのイベントを扱う（取り直しの新しい値のあとに、取っておいた古い値が届かないため）。Studio Mode を切る瞬間には、program の切替2つと `StudioModeStateChanged` だけが届き、これらは混ざらない（Linux の実機で確かめた）ので、一瞬の切替は漏れない
  - **既知の制約**（I-2）: つなぎ直しの読み取りの最中に Studio Mode を切ると、シーンと Studio Mode の読み取りの境界をまたいで、一瞬の切替が人の切替として届くことがある。読み取りは短く、その最中に切ることはまれで、屋外配信では Studio Mode を勧めないため、直さない
  - Studio Mode がオフのふだんの使い方では、何も変わらない（取っておかない）

  **屋外配信では Studio Mode を勧めない**（2026-11-22 のユーザーの裁定）。屋外からは OBS の画面を操作できず、Studio Mode の間は隠す動きが効かないか（`standby`・`ignore`）、「シーンを複製」の設定しだいになる（`hide`）ため。README に書き、Studio Mode がオンの間の `warnings` の文の頭に「屋外配信では Studio Mode を勧めません。」を足す

## 2. 設定

```yaml
cameras:                    # 1〜8台。上から順に表示する
  - id: main                # URL の path。英小文字・数字・- _ の 1〜32 文字（先頭は英小文字か数字）。重複不可
    name: メインカメラ       # 表示名（日本語可）。1〜32 文字
    on_bad: standby         # 回線が悪いとき: standby（待機シーンへ送る）/ hide（このカメラのソースを OBS で隠す）
  - id: sub
    name: サブカメラ
    on_bad: hide
    switch:                 # 省略可。書いた項目だけ、共通の switch の値を上書きする
      fail_kbps: 400
      recover_kbps: 800

mediamtx:
  rtmp_enabled: true        # RTMP の受付。使わないなら false にできる（RTMP と SRT のどちらかは true にする）
  srt_enabled: true         # SRT の受付。使わないなら false にできる。RTSP は OBS の読み出しに使うので止めない
```

- 初回に作る config.yaml のカメラは `main`（表示名「メインカメラ」、`on_bad: standby`）の1台。これで第1段階と同じ動きになる
- `rtmp_enabled` と `srt_enabled` の既定は両方 true（初回の起動がそのまま通り、どの配信アプリでもすぐ送れるため。2026-11-21 のユーザーの裁定）。止められることは、ひな形のコメントと、管理ページの送信の設定の画面（プロトコルごとのスイッチと「使わない方は止められます〔再起動して反映〕」の案内）で知らせる
- **なくなる設定**: `stream.name`（カメラの `id` に置き換え）、`obs.media_source`、`obs.monitored_scenes`（第3節で OBS から調べる）。まだ配布していないので古い形は読まない。残っていたら知らない項目として不正にし、理由に書き直し方を添える（`admin.listen` と同じ扱い）
- **検証**（コアの spec 4章に足す）:
  - `cameras` は1〜8台。`id` は `^[a-z0-9][a-z0-9_-]{0,31}$` で、重複しない。`all` と `all_others` は使えない（MediaMTX の path の設定で特別な名前のため）
  - `name` は1〜32文字（Unicode の文字数）で、制御文字を含まない。重複してよい
  - `on_bad` は `standby` か `hide`
  - `switch` の上書きは、共通の値と合わせたあとの5つの値で、共通の閾値と同じ規則を確かめる。理由の項目名は `cameras[<番号>].switch.<項目>`（番号は0から）。`switch` の下に知らない項目があれば不正
  - `rtmp_enabled` と `srt_enabled` の両方が false なら不正
  - `obs.studio_mode_hide` は `standby`、`ignore`、`hide` のどれか（既定 `standby`。0b で足す）
- **反映の区分**（第2段階の spec 7章の表を直す）: 表から `obs.monitored_scenes`、`obs.media_source`、`stream.name` を消す。すぐ反映に、カメラの `name`、`on_bad`、`switch` の上書きと `obs.studio_mode_hide`（0b）を足す（中核ループがカメラごとの Judge の閾値を差し替える。受信状態は保ち、積み上げた秒数は捨てる）。再起動して反映に、カメラの追加・削除・並べ替え・`id` の変更と、`rtmp_enabled`・`srt_enabled` を足す
- **config.yaml の書き換え**（第2段階の spec 7章）: `cameras` は対応づけのシーケンスで、文字で差し替える書き方に当たらない。`cameras` の区分は、変えたときに区分ごと Node で書き出す（区分の外のコメントと空行は残す。区分の中のコメントは残らないことを設定の画面で知らせる。ユーザーが承認した）
- **URL**: カメラごとに作る。資格情報は全カメラで共通
  - SRT: `srt://<host>:<srt_port>?streamid=publish:<id>:<user>:<pass>`（`srt_enabled` が false なら作らない）
  - RTMP: `rtmp://<host>:<rtmp_port>/<id>?user=<user>&pass=<pass>`（`rtmp_enabled` が false なら作らない）
  - OBS 用: `rtsp://<read_user>:<read_pass>@<obs_read_host>:<rtsp_port>/<id>`
  - 起動の表示（コアの spec 10章）はカメラごとに並べる
- **mediamtx.yml**（コアの spec 8章を置き換える部分）: path はカメラの `id` の数だけ書き、それ以外は受け付けない。`rtmp: no`、`srt: no` を `*_enabled` に合わせて書く。ポートの事前確認は、止めたプロトコルのポートを確かめない
- **ID を変えたとき**: 送信用 URL と OBS 用 URL が変わる。スマホのアプリは手で直す。OBS のメディアソースは、第2段階の spec 7章「OBS のメディアソースの URL の書き換え」をカメラごとに使える。保存のときに app が、今の地図から「変える前の `id` → そのカメラのメディアソース（入力の UUID）」を覚え、新しい世代の書き換えに渡す（区切り2の S2-31）。OBS につながっていなければ、新しい世代が最初の地図を作ったあとに、古い設定の `id`・`rtsp_port` で照合して探す

## 3. シーンの地図

本体は OBS から**シーンの地図**を作る: カメラごとのメディアソース、シーンごとの項目、どのシーンにどのカメラが映るか。型と、地図を作る・照合する・健康を計算する純粋な関数は、新しい葉のパッケージ `internal/scenemap` に置く（obs はすでに switcher を import しているので、obs に置くと循環する）。

### 識別

- シーン、グループ、入力は **UUID** で識別し、**OBS への要求も UUID で送る**（goobs v1.11.1 の `SetCurrentProgramScene`、`GetSceneSceneTransitionOverride`、`SetSceneItemEnabled`、`GetSceneItemList`、`GetGroupSceneItemList` は `sceneUuid` を、`TriggerMediaInputAction`、`GetInputSettings` は `inputUuid` を受ける）。名前は表示とログにだけ使う
- 項目は（シーンかグループの UUID、項目の ID）で識別する
- switcher の今のシーン、切断前のシーン、シーンの期待、戻り先、`SetScene` の切替先は UUID で持つ（コアの spec 6章「シーンの比較はシーン名の文字列の一致」「切替先はバッチを作った時点のシーン名で固定」を UUID に置き換える）。`SceneChanged` と `OBSConnected` は `SceneUUID` を持つ（`CurrentProgramSceneChanged` は `sceneUuid` を持つ）。設定の名前で書く待機シーンと開始時のシーンは、地図で名前から UUID を引く（同じ名前のシーンは OBS が作らせない）。引けなければ、今どおり status の `warnings` に出す
- UUID はシーンコレクションを複製すると元と同じになる（M-0 で確かめた）ので、地図、一覧、`hidden.json` の記録は「接続先〔`obs.host` と `obs.port`〕とシーンコレクションの名前と UUID の組」で識別する

### カメラのメディアソース

- `GetInputList`（`inputKind: ffmpeg_source`）で入力を集め、それぞれの `GetInputSettings` の `input`（URL）を読む。`is_local_file` が false と明示されているものだけを対象にする（`GetInputSettings` は既定値を省き、`ffmpeg_source` の `is_local_file` の既定は true。M-0 で確かめた）
- URL を解析し、スキームが `rtsp`、ポートが `mediamtx.rtsp_port`、path（先頭と末尾の `/` を除いたもの）がカメラの `id` と同じなら、その入力をそのカメラのソースとする。host と資格情報は見ない。URL はログに書かない
- 1台のカメラを複数のメディアソースで読んでいてよい。どのメディアソースにも当たらないカメラは status の `warnings` に出す。メディアソース以外の種類は調べない

### シーンの項目と「映る」

- `GetSceneList` の各シーンの `GetSceneItemList` を読む。項目がシーン（入れ子）ならそのシーンを、グループ（`isGroup`）なら `GetGroupSceneItemList` を読む。**読むのはシーンとグループごとに1回**
- **カメラの項目**: ソースがカメラのメディアソースである項目
- **シーン S にカメラ c が映る**: S から c の項目までの道のうち**1本でも**、途中の項目（入れ子のシーンやグループの項目と、c の項目そのもの）がすべて「表示」であること（道ごとに評価する。循環する道はたどらない）。本体が隠した項目（第7節の一覧にあるもの）は表示として数える（隠したことでそのシーンのカメラから外れないため）。人が手で非表示にした項目は数えない
- **実際に表示されているカメラ**（第6節の「今のシーンを空にしない」に使う）: 上と同じだが、本体が隠した項目も非表示として数える
- **分類**（コアの spec 6章「シーンの設定と分類」を置き換える）: 待機シーンは今どおり `obs.standby_scene`（地図で UUID に引いたもの）。監視対象は、待機シーンでなく、カメラが1つ以上映るシーン。管理外は、カメラが映らないシーン。切断の間は「不明」

### 読む時期

- **最初の地図は本来の接続で読む**: OBS につながったら、今の名前の確かめと同じ位置で、`OBSConnected` を流す前に読み、`OBSConnected` に載せる。読めなければ接続の失敗として、今の再接続の規則で試し直す。**続けて3回失敗したら**、空の地図でつなぎ（古い OBS と同じ扱い）、`warnings` と `last_error` に理由を出し、補助の接続で読み直しを続ける（1つのシーンの読み取りが毎回失敗する場合に、手動の操作まで使えなくしないため）
- **読み直しは補助の接続で行う**（第2段階の spec 6章。自動切替の `SetScene` を待たせないため）。補助の接続は、本来の接続が OBS につながっている間は常に開いておく（第2段階の spec 6章「開く条件」を置き換える。統計の読み取りは S2-50 で「読む条件」として書く）
- 次のイベントを受けたら、0.3秒まとめてから全部を読み直す: `SceneCreated`、`SceneRemoved`、`SceneNameChanged`、`SceneItemCreated`、`SceneItemRemoved`、`InputCreated`、`InputRemoved`、`InputNameChanged`（グループの作成は `SceneCreated`〔`isGroup: true`〕、グループの名前の変更は `SceneNameChanged` で届く。M-0）。読み直しの間に次のイベント（下の `InputSettingsChanged` を含む）が来たら、終わったあとにもう一度読む。読み取りの途中で失敗したら、その回の結果は捨て、1秒あけて読み直す
- **グループの中への項目の追加は通知されない**（M-0）ので、つながっている間は**30秒ごと**にも読み直す
- `InputSettingsChanged` は、地図で `ffmpeg_source` とわかっている入力のものだけを扱い、全部を読み直さず、イベントの `inputSettings` でその入力の URL の照合だけをやり直す
- `SceneItemEnableStateChanged` は読み直さず、`ItemEnabledChanged{SceneUUID, ItemID, Enabled}` として流す（グループの中の項目は、グループの UUID で届く。M-0）
- 読み直しの結果は `SceneMapChanged{Map}` で流す。読み直しには接続の世代を付け、切断の前に始めたものの結果は捨てる
- obs は、イベントを読む goroutine では要求を送らない（今の `CurrentSceneTransitionChanged` の取り直しも別の goroutine にする）。goobs はイベントの列〔容量100〕があふれると古いイベントを捨てるので、読むのを止めないため

### 表示・非表示は本来の接続のイベントを正とする

補助の接続の読み取りと本来の接続のイベントには、共通の順序の保証がない。表示・非表示が変わると、OBS は本来の接続に必ず `SceneItemEnableStateChanged` を流し、同じ値の設定ではイベントを出さない（M-0。別の接続からの変化でも90回中取りこぼし0、同じ接続では応答が先でイベントが後〔20回中20回〕）。

- 読み直しの結果のうち、今の地図にすでにある項目の表示・非表示は使わず、今の地図の値を引き継ぐ。読み直しの結果の値を使うのは、今の地図にない新しい項目だけ
- core は、今の地図にない項目の `ItemEnabledChanged` を、項目ごとの最後の値としていつも覚えておき、新しい地図の新しい項目に重ねてから、`SceneMapChanged`（最初の地図は `OBSConnected`）を1回だけ switcher に渡す。覚えた値は、次の地図ができたときと切断のときに空にする
- `ItemEnabledChanged` は、届いた時点で1回だけ switcher に渡す（期待の照合、人の操作の判定、地図の書き換えはこのときだけ）
- **ずれを直す規則**: 本体の `SetItemEnabled(X, v)` の応答が `OK` で、期待が期限まで照合されなかったときは、X はすでに v だったとみなし、地図の値を v にして、期待どおりのイベントと同じ扱いにする（`OK` を返した時点で OBS の X は v なので安全。イベントの取りこぼしと古い読み取り値は、これで直る）。ただし、承認から期限までの間に X のイベントが1つでも届いていたら当てはめない（人の操作を上書きしないため）

### 接続の扱い

- **本来の接続の Timeout とずれ**: goobs は応答をバッファのないチャネル1本から1件ずつ読むので、要求が Timeout したあとに OBS が遅れて応答すると、goobs の中の応答の受け口（`handleOpcodes`）が、受け手のいない送信で止まる。以後はイベントも届かず、次の要求は送る段階で戻らず（goobs 自身の応答待ちの期限も効かない）、`Disconnect` も goobs の mutex で戻らない。ID の合わないエラーになるのは、別の要求がすでに応答を待っていたときだけ（M-6 で、偽の相手で再現した）。このため、本体の側で要求ごとに期限を持ち（期限で必ず戻る）、片づけの待ちに上限を置く。止まった goobs の goroutine は外から戻せず残る（既知の制約）。そこで、本来の接続も、要求の Timeout か ID の合わない応答を受けたら、Client を捨ててつなぎ直す（第2段階の spec 6章の補助の接続と同じ規則。obs のラッパーの1か所）。ID の合わない応答は `Error` ではなく `Timeout` と同じ「適用されたかわからない」として扱う。第1段階の `SetScene` の Timeout にも効く（コアの spec 7章、13章を直す）。まず M-6 で、偽の WebSocket の相手で、この止まり方が本当に起きることを確かめる
- **シーンコレクションの切替は、切断とつなぎ直しとして扱う**: `CurrentSceneCollectionChanging` を受けたら、obs のラッパーが要求の門を閉じ（以後の要求は送らずに失敗を返す）、`OBSDisconnected` を流す（切替の間に要求を送ると OBS が落ちうる。goobs の説明）。`CurrentSceneCollectionChanged` のあとに、つないだときと同じ読み取り（配信状態、今のシーン、トランジション、最初の地図）をして `OBSConnected` を流す。`Changed` が来ないまま、イベントが30秒途切れたときも、同じ読み取りをして `OBSConnected` を流す（保険。M-6 で足した）。切替の間（`Changing` から `Changed` まで）に届いたイベントは捨てる（読み取りの結果が新しい状態になるため）。`ConnEpoch` が進むので、実行待ちのバッチは今の規則で却下され、期待も消える。Command は切替の間 `ErrNotConnected` になる（status の `warnings` に「OBS のシーンコレクションを切り替え中」）
- GUI の「複製」では `CurrentSceneCollectionChanging` が出ないまま、全部のシーンの `SceneRemoved` が続く（M-0）。そこで、**今の program のシーンの `SceneRemoved`** を受けたときも、切替の始まりと同じに門を閉じて `OBSDisconnected` を流し、`CurrentSceneCollectionChanged` か、イベントが2秒途切れたときに、つないだときと同じ読み取りをして `OBSConnected` を流す
- 切断（コレクションの切替を含む）の前に、一覧の項目のうち今のコレクションのものは、そのまま一覧に残す（第7節。次の地図との照合で決める）

### メディアソースの見張り（0a。M-9 の Windows の確認で見つけた）

OBS のメディアソース（`ffmpeg_source`）は、OBS の起動の直後に1回だけ開こうとし、そのとき受付（MediaMTX の RTSP）がまだないと、試し直さずに止まる（黒いまま）。一度つながったあとの切断では、自分で試し直す。止まったソースは、`GetMediaInputStatus` が `OBS_MEDIA_STATE_PLAYING` を返すのに、再生の位置（`mediaCursor`）が 0 のまま進まない。RESTART（`TriggerMediaInputAction`）では戻らず、設定の入れ直し（`SetInputSettings`。プロパティの OK と同じ）で戻る（2026-11-21 に Windows の OBS 33.0.1 で確かめた）。

- **見張るとき**: カメラの受信が「届いていない」から「届いている」（Sample の `Online` が false → true）に変わったとき。と、OBS につながったとき（`OBSConnected`）に、そのとき届いているカメラについて
- **見張り方**: 5秒待ってから、補助の接続で、そのカメラのメディアソース（地図で見つけたもの）それぞれについて `GetMediaInputStatus` を1秒あけて2回読む。「`PLAYING` で、位置が進んでいる」なら何もしない
- **入れ直し**: そうでなければ（位置が進まない、`PLAYING` 以外のまま）、`GetInputSettings` で今の `input` を読み、同じ値を `SetInputSettings`（overlay）で入れ直す。ログに `media source refreshed`（`camera`、`source`〔ソースの名前〕）。10秒たっても同じなら、もう1回だけ入れ直す。それでも同じなら、status の `warnings` に「<ソースの名前> が映っていない可能性があります」を出し、そのカメラが次に「届いている」に変わるまで出したままにする
- 中核ループは止まらない（見張りは別の goroutine。core は止まらずに頼むだけ）。同じソースの見張りが重なったら、新しい方だけにする。OBS とつながっていない間と、補助の接続が使えない間は見張らない（次の `OBSConnected` で見張る）
- 映っているソースには触れない（位置が進んでいれば何もしない）ので、配信中でも見張ってよい

## 4. 回線の判定とシーンの健康

### カメラごとの判定

- Poller は1秒に1回、`GET /v3/paths/list` を1回読み、全カメラの Sample を**1組**（同じ `At`）にして出す。各 path の値の扱いはコアの spec 5章の1本の path と同じで、カメラごとに基準を持つ。`/v3/srt/conns/list` は、SRT のカメラがあるときだけ1回読み、path ごとに振り分ける。path の一覧の API が失敗したら、全カメラの Sample に `Err` を入れる
- Judge はカメラごとに1つ。閾値は共通の `switch` にカメラの上書きを合わせたもの。規則はコアの spec 5章のまま
- core は1組の Sample で全カメラの Judge を更新し、変わったカメラがあれば **`CamerasChanged{States}`**（全カメラの今の状態）を1回だけ switcher に渡す（1台ずつ渡すと、途中の組み合わせで誤って待機へ送るため）。Snapshot もそのあと1回作る
- 送っていないカメラは、今と同じく 0 kbps で、`down_sec` で DOWN になる

### シーンの健康

監視対象のシーン S の健康（OK / LOW / DOWN）を、そのときの地図とカメラの状態から計算する純粋な関数で決める。ここでの「hide のカメラ」は、Studio Mode がオフのとき、またはオンで `obs.studio_mode_hide` が `hide` のときの `on_bad: hide` のカメラ。Studio Mode がオンで `standby` なら standby として数え、`ignore` なら悪いと数えない（第1節）。

- S に映るカメラのうち、standby のカメラが1つでも OK でなければ、**悪い**
- S に映るカメラがすべて hide のカメラで、すべて「使えない」なら（全部隠れる）、**悪い**。「使えない」は、OK でないか、表示に戻すのに続けて3回失敗した（第6節）もの
- それ以外は **OK**
- 悪いときの LOW と DOWN は、悪いと判断したカメラのうちいちばん悪いもの（DOWN > LOW。3回の失敗で「使えない」ものは DOWN として数える）。ログとできごとの `kbps` は、そのカメラ（同じ状態が複数なら設定の順で先のもの）の値。回復（AutoRecover）のログとできごとの `camera` と `kbps` は、戻り先に映るカメラのうち、最後に OK になったもの（回復のきっかけ）の値（M-9 の Windows の確認で、kbps がいちばん低いカメラを書くと、回復していないカメラが戻ったように読めた。2026-11-21 のユーザーの裁定）
- 待機シーン、管理外のシーン、不明のときは健康がない。健康には独自の待ち時間を足さない

## 5. 本体が守ること（0b）

第6節の隠す・戻すの規則、第7節、M-11 の「入口 × 状態」の表は、次の4つを守るためにある。M-12 の結合のテストは、偽の OBS に対して入力の列を乱数で作り、この4つをいつも確かめる。**前提は通常の動作**（OBS が要求に5秒以内に応える、本体と OBS のどちらも落ちない、イベントの列があふれない）で、その外で守れない場合は既知の制約とし、第7節「すべて表示に戻す」で戻す（ユーザーの裁定）。

1. **記録のない非表示を作らない**: 本体が送った `false` で非表示になった項目は、一覧か `hidden.json` にある。前提の外（Timeout のあとに OBS が遅れて適用した、など）でも、Timeout の記憶（第7節）で拾える範囲は拾う
2. **配信中の今のシーンを空にしない**: 本体の操作で、配信中の今のシーン（監視対象）の、実際に表示されているカメラを0にしない（0にするときは、先に待機シーンへ送る）。数えるときは、承認して送ったがまだイベントが届いていない `false` の項目も非表示として数え、入れ子の同じ項目を通る道はすべて消えるものとして数える。人の操作で0になったときは、AutoFail が出るまでの間だけ映る（既知の制約）
3. **人の判断を優先する**: イベントで見えた人の操作を、本体は同じ不調の間は変え直さない（イベントを取りこぼした人の操作は見分けられない。既知の制約）
4. **戻す方へ向かう**: 動いている間は、配信の停止、`auto_enabled` の false、`on_bad` の変更、Studio Mode のあと、接続と要求が回復すれば戻すまで出し直す。世代の停止では RestoreAll を1回だけ試し、戻せなかったものは `hidden.json` に残して次の起動で戻す

## 6. switcher の変更

**0a と 0b の分け方**: 0a は、`CamerasChanged`、`SceneMapChanged`、`ItemEnabledChanged`（地図の書き換えだけ）、UUID の `SetScene`・`SceneChanged`・`OBSConnected`、`RestartMedia{Inputs}`、分類と健康、AutoFail・AutoRecover・StandbyOffMonitored の健康の条件、戻り先と地図。`Handle` は 0a では今どおり多くても1つのバッチを返す。0b で、`ItemRequestDone`、`StudioModeChanged`、`HiddenRestored`、`HiddenWriteFailed`、`Stopping`、`SetItemEnabled`、HideCamera・ShowCamera・RestoreAll、「戻り先が待っている隠した項目」、バッチの列、第7節を足す。

### 入口と出力

- `LinkChanged{State}` を **`CamerasChanged{States}`** に置き換える
- **`SceneMapChanged{Map}`**、**`ItemEnabledChanged{SceneUUID, ItemID, Enabled}`**、**`ItemRequestDone{ReqID, Result: OK | Timeout | Error | NotFound}`**、**`StudioModeChanged{Enabled}`**、**`HiddenRestored{Entries}`**（`hidden.json` の中身）、**`HiddenWriteFailed{BatchID}`**（第7節）、**`Stopping{}`** を足す。`OBSConnected` に最初の地図と Studio Mode を載せる。`SceneChanged`、`OBSConnected` のシーンは UUID
- `ScenesConfigured` から `Monitored` を外し、カメラの一覧（`id`、`on_bad`）を足す。`ConfigEpoch` を進める条件に、カメラの一覧（順序つき）を足す
- Action に **`SetItemEnabled{SceneUUID, ItemID, Enabled}`** を足す。`SetScene` は UUID。`RestartMedia` は **`RestartMedia{Inputs []UUID, Wait}`**（並べた入力に順に RESTART を送り、最後に1回だけ待つ。対象が空なら Action ごと省き、待たない）
- `Machine.Handle` は**バッチの列**を返す（コアの spec 6章 細部の規則の「多くても1つ」を置き換える）。列の順は、シーンの切替 → ShowCamera → HideCamera。Command を受け付けた `Handle` で Command のバッチだけを返す規則は残す
- 中核ループは、シーンの切替のバッチを、実行待ちの隠す・戻すのバッチより前に入れる（実行中のバッチは抜かない）

### 目的と有効条件

コアの spec 6章の表を次のように変える（書いていない目的は変えない。共通の条件は今のまま）。健康は承認のときにその時点の地図と状態で計算し直す。

| 目的 | バッチ | 有効条件（変えるところ） |
|---|---|---|
| AutoFail | `[SetScene(待機シーン)]` | 今のシーンが監視対象で、**健康が悪い** |
| AutoRecover | `[RestartMedia(戻り先に映る standby のカメラのソース), SetScene(戻り先)]` | **戻り先の健康が OK**。出すときだけ、加えて「戻り先が待っている隠した項目」がない |
| StandbyOffMonitored | `[RestartMedia(同上), SetScene(戻り先)]` | `auto_enabled` が true なら**戻り先の健康が OK**。悪いとき、または待っている隠した項目があるときは、今の健康が悪いときの `Replace`（持ち主を `auto` にし、200）と同じ（項目が戻れば AutoRecover が戻す）。`auto_enabled` が false なら待たずに出す |
| HideCamera(c) | `[SetItemEnabled(項目, false) …]`（c の項目のうち、地図で今表示のもの。全シーン） | 配信中、`auto_enabled`、c が hide のカメラ（Studio Mode の間は `obs.studio_mode_hide` が `hide` のときだけ）、c が OK でない、c に「人が戻した」印がない、`Stopping` を受けていない、StartScene のバッチが未完了でなく、その期待も残っていない、**今のシーンを空にしない**（下） |
| ShowCamera(c) | `[RestartMedia(c のソースのうち、表示の項目が1つもないもの), SetItemEnabled(項目, true) …]`（c の一覧の項目のうち、地図にあって非表示のもの） | 出すとき: c の一覧に地図にあって非表示の項目があり、「c を戻してよい」。承認は Action ごと（下） |
| RestoreAll | `[SetItemEnabled(項目, true) …]`（全カメラの一覧の項目のうち、地図にあるもの。`RestartMedia` なし） | `Stopping` を受けたとき1回だけ出す。共通の条件は接続と `ConnEpoch` だけ。カメラごとの規則を掛けない。「隠しかけ」と `false` の期待が残る項目は、地図で表示でも `Skip` しない |

- **今のシーンを空にしない**: 今のシーンが監視対象で、c の項目を隠すと、今のシーンに実際に表示されているカメラ（第3節）が0になるなら、HideCamera(c) を出さず、承認でも却下する。**本体が送ったシーンの切替の期待が残る間（`SetScene` を承認してから、期待が当たるか消えるまで）は、その切替先（監視対象のとき）も同じように数え、どちらかが0になるなら隠さない**（v0.15。戻り先への切替がまだ届かないうちに隠すと、届いた先が空になるため。Fable のレビューの C-2）。今のシーンが変われば（AutoFail が届いた、人が切り替えた）出せる。待機シーンへの切替が失敗や期限切れになっても、今のシーンは変わらないので隠さない
- **「c を戻してよい」**: c が OK / 配信中でない / `auto_enabled` が false / c が hide のカメラでない（`on_bad` が standby、Studio Mode の間で `obs.studio_mode_hide` が `standby`、設定にないカメラ）/ 一覧の項目のソースがもう c のメディアソースでない（その項目だけ）、のどれか。c が OK でないときは `RestartMedia` を付けない
- **ShowCamera の承認**（Action ごと）: 共通の条件と `Stopping` を受けていないことを確かめたうえで、`RestartMedia` は c が OK なら `Proceed`、でなければ `Skip`。`SetItemEnabled(X, true)` は、X が c の一覧にあり、地図にあって非表示で、X について「c を戻してよい」なら `Proceed`、それ以外は `Skip`
- **戻り先が待っている隠した項目**: 戻り先に映る、OK のカメラで、一覧に地図にあって非表示の項目が残っているもの（3回の失敗で「使えない」カメラは待たない）。**加えて、戻り先に映るカメラがあるのに、実際に表示されているカメラ（第3節）が0のときも待つ**（v0.16。Studio Mode の `ignore` で、悪い hide のカメラの、本体が隠した項目だけが映る戻り先へ戻ると空になるため。M-11 の性質のテストで見つけた）。ある間は AutoRecover と StandbyOffMonitored を出さない（戻ってから空いた画面を映さないため）。承認では見ない
- HideCamera と ShowCamera は、`SceneEpoch`、`ConfigEpoch`、切替中の抑止を掛けない。同じカメラの HideCamera と ShowCamera は、どちらかが未完了の間はもう一方を出さない
- **`SetItemEnabled` の承認**: (1) 項目が地図にない（消えた）なら `Skip`（一覧から外す）。(2) HideCamera では、項目のソースが今の地図で c のメディアソースでなければ `Skip`（正常なほかのカメラを隠さないため）。(3) 地図の上ですでに目的の状態なら `Skip`（RestoreAll の例外は上）。(4) 目的の条件。`Proceed` のときは項目の期待を登録する
- **バッチ内の失敗**: `SetItemEnabled` が失敗しても、バッチを打ち切らずに次の項目へ進み、最後に `BatchDone{Completed}` に `Err` を付ける。`NotFound`（goobs のエラーの状態コード 600）なら、その項目を一覧から外す
- **再送**: HideCamera と ShowCamera は、条件が続く間、カメラごとに前回の送出から5秒以上たったときだけ出す（どの入口でも）。同じ項目への期待が残る間も出さない。続けて失敗した回数をカメラごとに数え、3回で `last_error` に記録し、そのカメラを「使えない」にする（第4節）。回数は、成功したときと、カメラの状態が変わったときに0に戻す
- **判断し直す入口**: コアの spec 6章の入口に、`CamerasChanged`、`SceneMapChanged`、`ItemEnabledChanged`、`ItemRequestDone`、`StudioModeChanged`、HideCamera と ShowCamera の `BatchDone` を足す

### 0a の細部（M-7 の表を作る中で決めたこと。表 `docs/superpowers/plans/2026-11-21-switcher-table-0a.md` の W-01〜W-06）

- **W-01 古い OBS**: obs-websocket 5.4.0 より古い OBS には UUID がないので、obs の層がシーンの名前を UUID の代わりに使い（`SceneUUID` に名前を入れる）、要求も名前で送る。地図はシーンの一覧（名前）だけを持ち、カメラの対応と項目は空。手動の操作と配信開始時の切替は第1段階どおり動く。待機シーンや開始時のシーンの名前が OBS にないときは、今どおり警告を出し、そのシーンへのバッチは出さない（StandbyOn は `ErrInvalidState`、StartScene は `last_error` に記録して再送しない）。空の地図（古い OBS と、最初の地図取得の3回の失敗）にも、シーンの一覧（UUID〔古い版では名前〕と名前）は入れる
- **W-02 地図の変更で古くなったバッチ**: 承認のときに、バッチの切替先が今の待機シーンや戻り先と一致するかを確かめ、違えば却下する。StandbyOffMonitored の戻り先が管理外になった、StandbyOffOther の戻り先が監視対象になった、も却下（押し直しで目的を選び直す）。待機シーンの UUID が変わった・消えたら、持ち主が auto でも manual でも消す（C）。待機シーンでなくなった期待が届いても、持ち主を入れない。「整える」手順を ScenesConfigured、SceneMapChanged、ItemEnabledChanged、OBSConnected、SceneChanged の更新後、J の前に順に行う: (1) 待機と開始時の名前を地図で引き直す。(2) 接続中で持ち主が none 以外なのに現在のシーンが待機 UUID でない（空を含む）なら C。(3) 接続中で持ち主が auto なのに戻り先が Monitored でなければ C。この手順により、待機シーンでなくなった期待、戻り先が監視対象でない AutoFail 期待では持ち主を入れない
- **W-03 RestartMedia の対象**: 戻り先に映る standby のカメラの、すべてのメディアソースを重複なしで集め、カメラの設定の順、同じカメラの中では入力の UUID の順に並べる。対象はバッチを作った時点で決め、承認で書き換えない
- **W-04 再接続のとき**: つないだときも、最初の地図を入れたあとに、持ち主が auto の戻り先を確かめ、監視対象でなければ消す（C）。接続先かシーンコレクションが変わった再接続は、UUID が同じでも、持ち主を auto でも manual でも消す。同じ接続先とコレクションでは、manual の戻り先がなくなったことでは消さない（StandbyOff が失敗したら `last_error` に残り、人が選び直す）。接続先（obs.host・obs.port）の変更は世代の作り直し（新しい Machine）とし、Machine はコレクションの名前だけを比較する
- **W-05 カメラの一覧の変更**（ふつうは再起動して反映だが、`ScenesConfigured` の型として）: 残る `id` の状態は保ち、足したカメラは DOWN、消したカメラは状態から除く。設定にない `id` は、地図の上でもカメラでないものとして数える。States の知らない鍵は無視し、足りない鍵は DOWN。設定にない id は、CameraID を空にした計算用の地図でカメラでないものとして扱う
- **W-06 同じ値の `ItemEnabledChanged`**: 地図の上ですでに同じ値なら、期限切れの処理のほかは何もしない

### 戻り先と地図

- 「持ち主が `auto` で、戻り先が監視対象でなくなったら、持ち主と戻り先を消す（待機シーンに残る）」を、`SceneMapChanged` と `ItemEnabledChanged` のたびに当てはめる（戻り先のシーンが消えた、カメラが映らなくなった。名前の変更は UUID で追うので消さない）
- 外部からの手動切替の表、StandbyOff の表、配信開始時の切替（コアの spec 6章）の「受信状態が良い / 悪い」は、「今のシーン（または戻り先）の健康が OK / 悪い」と読み替える。健康がないシーンは「Other」の行に当たる

## 7. 隠す・戻す

### 一覧

**隠した項目の一覧**（カメラごと）の項目は、接続先とシーンコレクションの名前、項目（シーンかグループの UUID と項目の ID）、そのときのソースの UUID、状態（「隠しかけ」か「隠した」）を持つ。

- **入れる（隠しかけ）**: HideCamera のバッチの最初の `SetItemEnabled` の承認で、バッチの全項目を「隠しかけ」として入れ、`Proceed` の前に core が `hidden.json` を1回書く（1バッチ1回）。書けなければ、core は `HiddenWriteFailed{BatchID}` を `Handle` に渡し（switcher はそのバッチの「隠しかけ」と期待を巻き戻す）、実行役には `Reject` を返す。警告と `last_error`「隠した項目を記録できないため、隠すのを止めました」（ユーザーが承認した）
- **隠した にする**: 期待どおりの `ItemEnabledChanged{Enabled: false}`、ずれを直す規則（第3節）、地図で非表示とわかったとき
- **外す**: 期待どおりの `true`（ずれを直す規則を含む）、項目か地図の上の項目そのものが消えた（同じコレクションの地図で）、`NotFound`、人が戻した（下）。外したら `hidden.json` を書き直す
- HideCamera の `BatchDone` で、送らなかった項目（`Skip`、却下）の「隠しかけ」を外す
- 「隠しかけ」は、期限切れ、要求の失敗、切断、配信の停止では外さない。地図で非表示なら「隠した」、表示なら、その要求の決着（`ItemRequestDone`）から10秒たったあとに外す（`Error` と `NotFound` なら決着の時点で外してよい）
- **Timeout の記憶**: 応答が `Timeout`（ID の合わない応答を含む）だった `SetItemEnabled(false)` の項目は、一覧に「未決着」として残し、`hidden.json` にも書く（**切断と再起動をまたいで**持つ。地図で表示とわかっても、10秒の規則で外さない）。その項目に `false` が届いたら（期待の有無によらず）本体の操作の遅れとみなし「隠した」にする。「未決着」は、その `false` が届いたとき、または「すべて表示に戻す」で消す。「未決着」の項目は健康と「実際に表示」の計算では表示として数える。のちに人がその項目を隠すと、本体のものとして扱われる（既知の制約。「すべて表示に戻す」で直る）
- **ほかのコレクションの地図**: 地図のコレクションか接続先が項目のものと違うときは、項目を捨てずに「残した記録」へ移す（`hidden.json` の残した記録と同じ扱い）

### 項目の期待

本体が `Proceed` した `SetItemEnabled` ごとに（項目、目的の状態、`ReqID`）を持つ。期限は `ItemRequestDone{OK}` から2秒。`Timeout` なら応答の時刻から2秒、`Error` と `NotFound` ならすぐ消す。**配信の停止では消さず、切断でだけ消す**（シーンの期待と違い、持ち主と戻り先に作用しないため）。届いた `ItemEnabledChanged` が期待に当たれば本体の操作、当たらなければ人の操作とみなす。照合の順と応答より先に届いたイベントの扱いは、シーンの期待に合わせて M-11 の表で決める。応答より先にイベントが届いても、期待は応答から2秒まで残す（人の `false`→`true` のあとに本体の `false` が遅れて適用されるのを、本体の操作として拾うため）。**送った要求の応答は、切断しても必ず `ItemRequestDone` で返る**: 実行役は、応答が来ないうちに接続が切れたら `Timeout` を返す（v0.15。Fable のレビューの I-1、I-2）

### 人が途中で触ったとき

- 一覧にある項目に、期待にない `true` が届いたら、その項目を一覧から外す。そのとき「c を戻してよい」が成り立たない（配信中、`auto_enabled`、hide のカメラ、c が OK でない）なら、カメラに「人が戻した」印を付け、印のある間は HideCamera を出さない。印は、そのカメラが OK になったときと、配信の停止で消す
- 「人が戻した」印は、健康の計算には効かない（全部隠れるシーンで人が表示に戻しても、そのシーンは悪いので待機へ送る。ユーザーが承認した）
- 一覧にない項目の変化は、地図を書き換えるだけ（Timeout の記憶に当たるものを除く）

### すべて表示に戻す（0b）

めったにない障害で非表示が残った場合の逃げ道（ユーザーの裁定）。

- 管理ページの状態の画面と、小さな窓に「カメラのソースをすべて表示に戻す」を置く。確認の文に「手で隠したカメラのソースも表示に戻ります」と添える
- 押したら、今のコレクションの地図で、**すべてのカメラの項目**（一覧にないものを含む）のうち非表示のものと、**一覧（隠しかけ・隠した・未決着）にある項目と、`false` の期待が残る項目**（地図ではまだ表示でも）に `SetItemEnabled(true)` を送る（目的 RestoreAll と同じ形。`Stopping` のときとの違いは、一覧にない項目も含むこと）。押す前に作られた HideCamera のバッチの残りは、承認で却下する。一覧の項目、「未決着」、Timeout の記憶、今のコレクションの `hidden.json` の記録は、**項目ごとに `true` が決着したとき**（期待どおりの `true` が届いた、地図で表示とわかった）に消す。`true` が失敗した項目の記録は残し、`last_error` に「表示に戻せなかった項目があります。もう一度押してください」（v0.15。受け付けた時点で消すと、送ったあとの `false` が記録のない非表示になるため。Fable のレビューの C-1）
- そのあとは通常の規則に戻る（配信中で hide のカメラが悪ければ、また隠す）
- API: `POST /api/cameras/show-all`（独自ヘッダーが要る。OBS につながっていなければ 503）。ログ `camera shown`（`reason: manual_all`）

### 世代を止めるとき

本体の終了と中での再起動では、第2段階の spec 7章の止める順の 3 の前に、app が core に `Stopping` を渡し、最大2秒待つ。

- switcher は `Stopping` を受けたら、Auto、HideCamera、ShowCamera を出さず、実行待ちと実行中のそれらのバッチの残りの Action を承認で却下する（すでに送った要求はそのまま終わる）。Command は `ErrInvalidState`（理由「終了中」）
- RestoreAll のバッチを1本出す。中核ループはそれを、実行中のバッチの次（列の先頭）に入れる。同じ本来の接続で後から送る `true` は、先に送った `false` のあとに適用される
- **待ちの終わり**: RestoreAll の `BatchDone`（すべての要求に応答があった。イベントは待たない）か、2秒。`BatchDone` のときは、`true` が `OK` だった項目を一覧から外し、`hidden.json` を書いてから、core が app に知らせる。2秒で終わらなければそのまま止める（残りは `hidden.json` に残り、次の起動で戻す）
- OBS につながっていなければ、RestoreAll は出さず、すぐ終わる
- 配信中に中で再起動すると、新しい世代はカメラの初期値が DOWN なので、OK のカメラも `recover_sec` の間いったん隠すか待機へ送る（第2段階 7章の確認の文に1行足す）

### `hidden.json`

- 置き場所（`--dir`）の `state/hidden.json` に、一覧と残した記録（カメラの `id`、接続先、シーンコレクションの名前、シーンかグループの UUID、項目の ID、ソースの UUID、状態、書いた時刻）を書く。一時ファイルに書いてから名前を替える
- 世代を作るとき（作る順の 6）に読み、`HiddenRestored` として switcher に渡す。空の地図（古い OBS と、最初の地図の3回の失敗）では照合せず、記録を触らずに残す（次に地図ができたときに照合する。空の地図のあとに、補助の接続の読み直しで最初の空でない `SceneMapChanged` が届いたときも、そこで照合する。v0.15）。switcher は、まだ照合していない記録を、**最初の地図ができたとき**（つないだときと、コレクションの切替のあと。どちらも `OBSConnected`）に照合する: 接続先とコレクションの名前が今のものと同じで、項目が地図にあり、ソースの UUID が同じで、まだ非表示のものは「隠した」として一覧に戻す（そのあとは第6節の規則で、隠したままか、ShowCamera で戻る。設定にないカメラの記録は「c を戻してよい」で戻る）。項目がないもの、ソースが違うもの、表示になっているものは捨てる。接続先かコレクションの名前が違うものは、触らずに残す（件数を status の `warnings` に出す）。空の地図（古い OBS）では照合せずに残す
- 読めないときは、警告を出して空として扱い、壊れたファイルは `hidden.json.broken` に名前を替えて残す

### 記録

- ログ: `camera hidden`（`camera`、`items`、`reason`〔`low` / `down`〕）、`camera shown`（`camera`、`items`、`reason`〔`ok` / `stream_stop` / `auto_off` / `config` / `studio_mode` / `shutdown` / `restore` / `manual_all`〕。`restore` は、`hidden.json` から照合した記録のうち、設定にないカメラの項目を戻したとき。設定にあるカメラの記録は、起動し直したあとも、ふだんの理由〔OK なら `ok`〕で戻す。悪い hide のカメラの項目は、今いないシーンのものも含めて隠す〔M-13 の Linux で確かめた〕）、`camera changed`（`camera`、`from`、`to`、`kbps`。今の `link changed` を置き換える）、`hidden item dropped`
- できごと（第2段階の spec 8章の一覧に足す）: `camera_hidden`、`camera_shown`（値はログと同じ）

## 8. OBS の動きの確かめ

### M-0 で確かめたこと（2026-11-21。結果は scratchpad の `m0/m0-results.md`）

1. 最低の版: OBS 30.1.0 / obs-websocket 5.4.0（protocol.md の 5.3.0 と 5.4.0 の差分、UUID 対応のコミット）
2. グループの中の項目の `SceneItemEnableStateChanged` は、グループ名とグループの UUID で届く。`SetSceneItemEnabled` はグループの名前でも UUID でも効く。同じ値ではイベントが出ない。別の接続からの変化も必ず届く。同じ接続では応答が先
3. websocket ではグループを作れず、名前も変えられない。GUI で作ると `SceneCreated`（`isGroup: true`）と `SceneItemCreated`、名前の変更は `SceneNameChanged`。**グループの中への項目の追加は通知されない**
4. `SetCurrentSceneCollection` では `Changing` → `Changed` → 応答。GUI の「複製」では `Changing` がなく、全部のシーンの `SceneRemoved`・`InputRemoved` → 作成 → `CurrentProgramSceneChanged` → `SceneCollectionListChanged` → `Changed`
5. `GetInputSettings` は既定値を省く。`ffmpeg_source` の `is_local_file` の既定は true
6. Studio Mode（既定の「シーンを複製」がオン）では、program のシーンの項目を隠しても配信の画面に効かない（次の遷移で効く）。設定をオフにすると効く
7. 入れ子のシーン N の項目を隠すと、N を入れたシーンでも消える（イベントは N の分の1件）
8. コレクションを複製すると、シーンと入力の UUID は元と同じ

### M-6 の初めと M-9 で確かめること

- 本来の接続の Timeout と ID のずれ（第3節「接続の扱い」）が、偽の WebSocket の相手で起きること
- 切れた接続の未処理の要求を、OBS があとで適用するか（適用しないなら、Timeout の記憶は「隠しかけを切断をまたいで保ち、最初の地図で決める」に減らせる）
- 配信中にコレクションを切り替えられるか。切替の途中と終わりに `CurrentProgramSceneChanged`、`CurrentSceneTransitionChanged` が出るか
- 今のシーンの名前を変えたときに `CurrentProgramSceneChanged` が出るか（UUID で持つので、出なくても困らないことを確かめる）
- `OK` の応答なのに表示の変化のイベントが出ない場合があるか
- Studio Mode を Windows の実機で（M-9）

## 9. 第2段階のほかの部分への影響

- **status**（コアの spec 9章、第2段階の spec 6章）: `link` を `cameras`（カメラごとの `id`、`name`、`on_bad`、`state`、`kbps`、`online`、`protocol`、`srt`、`sources`〔メディアソースの名前〕、`hidden`〔隠している項目の数〕、`user_shown`〔人が戻した印〕、`unusable`〔3回の失敗〕）に置き換える。`obs` に、今のシーンに映るカメラの `id` の一覧（`scene_cameras`）、今のシーンの健康（`scene_health`。なければ null）、`studio_mode` を足す。`scene_class` は地図で決める。`urls` はカメラごと
- **管理ページ**（第2段階の spec 5章、7章「入力の画面」）: 状態の画面の回線の行をカメラごとに。送信の設定の画面をカメラごとの URL と QR に。設定の画面に「カメラ」の区分（追加、削除、並べ替え、`id`、表示名、`on_bad`、閾値の上書き）。監視対象のシーンを選ぶ欄とメディアソースを選ぶ欄はなくす（`GET /api/obs/inputs` は、URL の書き換えの対象の確認に使う）
- **小さな窓**（第2段階の spec 4章）: 回線の行をカメラごとに1行。カメラの数に合わせて窓の高さを変える
- **API**（第2段階の spec 6章）: `GET /api/status` は上の形。`event: sample` は1組
- **グラフ**（第2段階の spec 8章）: カメラごとの線。閾値の横線と色の帯は選んだカメラのもの。待機の印に加えて、隠した・戻した印。直近10分はカメラごとに600件
- **配信の記録**（第2段階の spec 8章）: `sample` のできごとは1組で1秒に1件（S2-4）。`auto_fail`・`auto_recover` の `kbps` は第4節のカメラの値、`camera` を足す。`settings_applied` と meta の設定は、カメラの一覧（`id`、`name`、`on_bad`、合わせたあとの閾値）と待機シーンなど（`monitored` はなくす）。購読の形の「設定を返す関数」もこの形。まとめはカメラごとの平均と最低の kbps、LOW と DOWN の時間と割合、隠した回数と合計の時間。待機の回数・合計・最長は配信全体。CSV はカメラごとの列
- **ソース**（第2段階の spec 9章）: 使える値に、カメラごとの値（`camera.<id>.kbps`、`camera.<id>.state`、`camera.<id>.name`、`camera.<id>.hidden` など）と、今のシーンの健康を足す。`link.*` は、設定で先頭のカメラの値の別名として残す（並べ替えで指す先が変わることを画面に書く）。`obs.scene_class` は地図で。回線の状況のテストは、カメラごとに状態を選べるようにし、「自動の待機中」の戻り先は、地図の監視対象のシーンの最初のもの（なければ「—」）
- **メディアソースの URL の書き換え**（第2段階の spec 7章）: カメラごとに行う（`update_obs_input` はカメラごとの印）
- **README**: 複数のカメラの使い方、`on_bad` の選び方、使わない日のカメラの運用、全部隠れるシーンで人が表示に戻しても待機へ送ること、人が「全部隠れる」シーンへ切り替えると待機へ送るまで隠れた画面が映ること、Studio Mode の間の動きと `obs.studio_mode_hide` の選び方（既定は待機へ送る。`hide` は OBS の「シーンを複製」を切っているときだけ。屋外配信では Studio Mode を勧めない〔v0.17〕）、コレクションの切替の間は操作を断ること。2台以上を同じ上り回線から送ると帯域を分け合うこと、メディアソースの「ハードウェアデコードを使用」。対応する OBS の版
- **既知の制約**（コアの spec 13章に足す）: 配信の開始と同時に今のシーンの健康が悪いと、本体は待機シーンへの切替を送る。それが OBS に届く前（約0.3秒）に人が別のシーンへ切り替えると、本体の切替があとで効いて、人の切替が残らない（第1段階からの性質。M-9 の Linux の確認で見えた）。`hidden.json` は、落ちたあとに人が隠し直した項目と区別できない。補助の接続は常に2本目として開く。HideCamera は全シーンの項目を1つのバッチで送るので、項目が多いと実行中の間だけ AutoRecover が待つ

## 10. 置き換える箇所

| 文書と箇所 | 置き換えの中身 |
|---|---|
| コアの spec 1章（前提の OBS の版）、README | 第1節「対応する OBS」 |
| コアの spec 3章（ディレクトリ構成）、第2段階の spec 3章（`--dir` の中身） | `internal/scenemap` と、置き場所の `state/hidden.json` を足す |
| コアの spec 4章 | 第2節 |
| コアの spec 5章 | 第4節「カメラごとの判定」 |
| コアの spec 6章 分類、入口と出力、シーンの比較（名前 → UUID）、切替先の固定（名前 → UUID）、目的と有効条件、自動切替（判断し直す入口、戻り先を消すとき）、再送の歯止め、バッチ内の失敗 | 第3節、第6節、第7節 |
| コアの spec 6章 外部からの手動切替の表、StandbyOff の表、配信開始時の切替 | 第6節「戻り先と地図」の読み替え |
| コアの spec 6章 未接続の間、配信の停止 | 項目の期待は切断でだけ消す。一覧の「隠しかけ」は消さない。「人が戻した」印は配信の停止で消す |
| コアの spec 6章 細部の規則 U-01（`RestartMedia` の承認の括弧書きは AutoRecover と StandbyOffMonitored のこと）、U-14（同じ値の `CamerasChanged`、同じ内容の `SceneMapChanged`）、U-15（初期値: カメラごとに DOWN、地図は不明、一覧は空）、U-22（期待にない `ReqID` の `ItemRequestDone` は無視）、Command と自動判定（バッチの列）、V-10（カメラの一覧は順序つきで比べる）、v2.0 の初回の既定値 | 第6節、第7節 |
| コアの spec 7章（obs） | `Client` の引数を UUID に。接続のときの名前の確かめを第3節（最初の地図、カメラのメディアソースの警告、待機シーンと開始時のシーンの UUID）に。`RestartMedia` は入力の列。`SetSceneItemEnabled`、`GetVersion`、Studio Mode、地図の読み取り、第3節のイベント、イベントを読む goroutine で要求を送らないこと、本来の接続の Timeout とずれでのつなぎ直し、コレクションの切替を切断として扱うこと |
| コアの spec 8章 | 第2節 |
| コアの spec 9章 | 第9節の status。中核ループは Sample の1組で `CamerasChanged` を1回、バッチの列をシーンの切替を先にして列へ、地図にない項目の変化を覚えて重ねる、`hidden.json` を書いて `HiddenWriteFailed` を返す |
| コアの spec 10章 | `link changed` を `camera changed` に。`camera hidden`・`camera shown`・`hidden item dropped` を足す。`scene switch` の `kbps` は第4節のカメラの値で、`camera` を足す。エラーの表の「メディアソース名が OBS にない」は第3節の警告に。起動の表示はカメラごとの URL |
| コアの spec 11章、13章 | 第11節のテスト。第9節の既知の制約。goobs の制約に、Timeout とずれ |
| 第2段階の spec 6章「補助の接続」の開く条件、API の表 | 第3節（常に開く）、第9節 |
| 第2段階の spec 7章 反映の区分、すぐ反映の届け方、入力の画面、config.yaml の書き換え、メディアソースの URL の書き換え、作る順と止める順、再起動の確認の文 | 第2節、第7節、第9節。作る順の 6 で `hidden.json` を読む。止める順の 3 の前に `Stopping` |
| 第2段階の spec 8章、9章、4章、5章、13章 | 第9節 |

## 11. テスト

- **config**: `cameras` の検証の表、`*_enabled` の両方 false、古い3つの項目のエラーの文、生成した値が検証を通ること
- **URL と mediamtx.yml**: カメラごとの URL、止めたプロトコルの URL と設定とポートの事前確認、path の数
- **シーンの地図と健康**（`scenemap` の純粋な関数）: URL の照合の表（スキーム、ポート、path の前後の `/`、host を見ない、資格情報つき、`is_local_file` が false・true・項目がない）。たどり方の表（入れ子、グループ、途中の非表示、本体が隠した項目を「映る」では表示・「実際に表示」では非表示として数える、同じカメラを2つのソースで、ひし形、循環、読むのはシーンごとに1回）。分類。健康の規則の表（Studio Mode、3回の失敗を含む）
- **switcher**: 0a（M-7）と 0b（M-11）のそれぞれで「入口 × 状態」の表を作り、Fable か codex sol でレビューしてから実装する。0a では、今の列のテストを、カメラ1台・`on_bad: standby`・地図でそのカメラが監視対象のシーンに映る形に置き換えて、同じ結果になることを確かめる。0b では、第5節の4つを守ることを、表の行ごとに確かめる
- **obs**: 本来の接続の Timeout とずれでつなぎ直すこと（偽の WebSocket の相手で）、コレクションの切替（`Changing` からと、今の program のシーンの `SceneRemoved` から）を切断として流すこと、イベントを読む goroutine で要求を送らないこと、地図の読み取りの順とまとめ方と30秒、`InputSettingsChanged`、古い版で空の地図、最初の地図の3回の失敗
- **core**: Sample の1組で `CamerasChanged` が1回、地図にない項目の変化を重ねて1回渡すこと、接続の世代の違う結果を捨てること、シーンの切替のバッチを先に入れること、`hidden.json` をバッチごとに1回書くことと `HiddenWriteFailed`、`Stopping` と2秒
- **結合**（0b の M-12。偽の OBS で、実行役を含めて）: 入力の列を乱数で作り、第5節の4つをいつも確かめる。「すべて表示に戻す」も。列には、切替の失敗、長いトランジション、項目のイベントの欠け・重なり・遅れ、Timeout のあとに届く応答、一部の項目の失敗、配信の停止と再開、古い地図の結果、コレクションの切替、Studio Mode、止めるときの RestoreAll を含める
- **ingest**: `/v3/paths/list` の1回の読み取りから1組の Sample、カメラごとの基準、SRT の振り分け、API の失敗で全カメラに `Err`
- **実機の確認**（0a の M-9 と 0b の M-13。ユーザーと一緒に。隠す・戻すに関わるものは M-13）:
  - Linux（画面なしの OBS）: ffmpeg で2〜3本の偽のカメラを送り、帯域を絞る中継で1本ずつ乱す。待機へ送る・戻る、隠す・戻す、全部隠れて待機へ（待機シーンへ届いてから隠れること）、戻るときに隠していたカメラが先に表示に戻ること、入れ子のシーンとグループ、人が途中で表示を変える、強制終了のあとの `hidden.json` からの戻し、シーンコレクションの切替（websocket と GUI の複製）、OBS を先に閉じたとき、受付の停止、使わない日のカメラの運用
  - 第1段階の確認のやり直し（カメラ1台）: コアの spec 11章の手動の結合テストと、Windows の手順書の W と T の項目。残りの T-12 と T-15 もここで
  - Windows: スマホ2台（できれば別の回線）で同じ流れ。OBS のデコードの負荷と、同じ回線から2台を送ったときの乱れ方。Studio Mode で隠さず待機へ送ること

## 変更履歴

| 日付 | 版 | 変更 | 理由 | commit |
|---|---|---|---|---|
| 2026-11-21 | v0.1 | 初版 | 2026-11-21 の brainstorming（ユーザーが節ごとに承認） | f704f2c |
| 2026-11-21 | v0.2 | UUID で識別、最初の地図を本来の接続で、読み直しの間の変化を重ねる、`InputSettingsChanged` の絞り、コレクションの切替、1組の Sample、「隠しかけ」と一覧の照合、ShowCamera と待ちを状態で、HideCamera が待機を待つ、承認の順、`ItemRequestDone`、失敗の続行と回数、`Stopping`、`hidden.json` の照合、`id` の変更、`scenemap`、M-0 | 設計レビュー1周目（Fable: C1/I13/M9、codex sol: C0/I13/M2）と 2026-11-21 のユーザーの裁定（OBS の版を上げる、使わないカメラは運用で） | 411cf41 |
| 2026-11-21 | v0.3 | 表示・非表示はイベントを正に、コレクションの切替の間の却下、全部隠れるにしない、ShowCamera の承認を Action ごとに、RestoreAll、「隠しかけ」の10秒、`hidden.json` を書けなければ隠さない、接続先 | 閉鎖確認（codex terra: C0/I6） | f23628a |
| 2026-11-21 | v0.4 | 地図にない項目の変化をいつも覚えて重ねる、RestoreAll の `Skip` と待ちの終わり、Timeout の記憶、隠した項目のあるシーンへの切替、残した記録の照合、地図がないときの3つの場合、実機の確認の範囲 | 閉鎖確認 2回目（codex terra: C0/I5）と指揮者の読み直し | 8245fce |
| 2026-11-21 | v0.5 | M-0 の結果を入れた（最低の版 OBS 30.1.0、グループの中の追加が通知されないので30秒ごとの読み直し、GUI の複製は program のシーンの削除で切替とみなす、Studio Mode では hide を standby として扱う〔指揮者の提案〕）。本来の接続も Timeout とずれでつなぎ直す（第1段階にも効く）。要求とシーンの識別を UUID に。コレクションの切替を切断として扱い、`MapUnknown` と特別な却下をなくした。古い OBS は空の地図。最初の地図の3回の失敗で空の地図。ずれを直す規則（`OK` の応答でイベントが来なければその値）。イベントを読む goroutine で要求を送らない。Timeout の記憶を切断をまたいで持ち、最初の `false` か `true` の `OK` で消す。ソースがカメラのものでなくなった項目は戻す。今のシーンを空にしないを「実際に表示」で。列の順は切替 → Show → Hide。3回の失敗で「使えない」。StandbyOff で待つときは持ち主を `auto` に。読み直しの間の `InputSettingsChanged` でもう一度読む。`hidden.json` はバッチごとに1回書き、`HiddenWriteFailed`。一覧の項目にコレクションを持たせ、違う地図では残した記録へ。期待は切断でだけ消し、人が戻した印は配信の停止でも消す。StartScene の間は隠さない。第5節に守ることの4つを置き、細部は M-7 の表へ | M-0 の spike と、設計レビュー2周目（Fable: C1/I12/M8） | d939791 |
| 2026-11-21 | v0.6 | 区切り0を 0a（複数のカメラと待機へ送る）と 0b（隠す・戻す）に分けた。0b の守ることを、通常の動作を前提にして現実的にし（今のシーンを空にしないの数え方に、送ったがまだ届かない `false` と入れ子の道を足した）、障害で残る非表示は既知の制約と「カメラのソースをすべて表示に戻す」で受ける。Timeout だった隠す要求は「未決着」として一覧と `hidden.json` に残し、表示でも外さない。空の地図では `hidden.json` を照合せずに残す。Studio Mode の既知の制約 | 閉鎖確認（codex terra: C1/I5/M1）と、2026-11-21 のユーザーの裁定（分けて、隠すは現実的に） | 75dd11c |
| 2026-11-21 | v0.7 | `rtmp_enabled`・`srt_enabled` の既定を両方 true と決め、止められることの知らせ方を書いた | 2026-11-21 のユーザーの裁定 | 8273a51 |
| 2026-11-21 | v0.8 | 第6節に「0a の細部」（W-01〜W-06。古い OBS は名前を UUID の代わりに使う、地図の変更で古くなったバッチ、RestartMedia の対象、再接続のとき、カメラの一覧の変更、同じ値の表示の変化）を足した | M-7 の表の下書き（codex sol）の不明点を指揮者が決めた | 742c0c3 |
| 2026-11-21 | v0.9 | W-01 のシーン一覧保持、W-02 の整える手順と期待の所有権、W-04 の接続先変更による世代更新、W-05 の States 正規化と計算用地図を明記 | M-7 表の Fable レビューと指揮者の決定 | 31de82e |
| 2026-11-21 | v0.10 | 第3節「接続の扱い」の goobs の止まり方を、M-6 で再現した形（遅れた応答で受け口が止まり、イベントと次の要求と `Disconnect` が止まる）に直し、本体の側の期限と片づけの上限を書いた。コレクションの切替で `Changed` が来ないときの30秒の保険と、切替の間のイベントを捨てることを足した | M-6 の実装 | a3fe192 |
| 2026-11-21 | v0.11 | 既知の制約に、配信開始の直後の約0.3秒の人の切替を足した | M-9 の Linux の確認 | 87fca65 |
| 2026-11-21 | v0.12 | Studio Mode の間の hide のカメラの動きを、設定 `obs.studio_mode_hide`（`standby` 既定 / `ignore` / `hide`）で選ぶ形にした（第1節、第2節の検証と反映の区分、第4節の健康、第6節、第9節の README） | 2026-11-21 のユーザーの裁定 | 818dedb |
| 2026-11-21 | v0.13 | 第3節に「メディアソースの見張り」を足した（OBS を先に起動すると止まるメディアソースを、カメラが届き始めたときに見つけて設定を入れ直す） | M-9 の Windows の確認（ユーザーと一緒に原因を絞り込んだ） | 2196cfd |
| 2026-11-21 | v0.14 | 第4節: 回復のログとできごとの代表のカメラを、戻り先に映るカメラのうち最後に OK になったものにした（前は kbps がいちばん低いもの） | M-9 の Windows の確認（F-6）、ユーザーの裁定 | 24fbfa6 |
| 2026-11-21 | v0.15 | 第1節: Studio Mode の `ignore` でも、ほかの戻す理由では隠した項目を戻す（H-07）。第6節: 空にしない検査に、本体の切替の期待が残る切替先を入れる、StartScene の期待が残る間は隠さない（C-2）。第7節: すべて表示に戻すの対象と記録を消す時点（C-1）、期待を応答から2秒残す・切断でも応答を Timeout で返す（I-1、I-2）、空の地図のあとの最初の地図での照合（H-02） | M-11 の表の Fable のレビュー、H-07 は sol の助言とユーザーの裁定（案 A） | 1e90ae7 |
| 2026-11-21 | v0.16 | 第6節「戻り先が待っている隠した項目」に、戻り先の実際に表示されているカメラが0のときも待つ、を足した | M-11 の性質のテスト（INV-02）で見つけた列 | 0c1c747 |
| 2026-11-22 | v0.17 | 第1節の Studio Mode に「Studio Mode を切るときの一瞬の切替」を足した（obs の層で、Studio Mode の間の program の切替を 250ms 取っておき、`StudioModeStateChanged false` が続いて元のシーンに戻っていれば捨てる。Machine と 0a・0b の表は変えない）。屋外配信では Studio Mode を勧めない（README と第1節） | M-13 の Windows の確認（F-9）と Linux の再現、2026-11-22 のユーザーの裁定（非推奨にし、直しも入れる）。先に流す・既知の制約は、実装の sol のレビュー（I-1、I-2） | この版の commit |
