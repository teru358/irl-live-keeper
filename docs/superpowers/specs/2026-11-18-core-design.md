# irl-live-keeper 設計 — 全体構成と第1段階（コア）

- 作成日: 2026-11-18
- 版: v2.7（変更は末尾の変更履歴を参照）
- 状態: ユーザー承認済みの設計（セクション1〜5）を文書化したもの。v2.0 でシーンのモデルを 2026-11-19 のユーザーの裁定に合わせて書き直した（6章の末尾に「実装前に決めたこと」がある）
- 対象バージョン: MediaMTX v1.21.4（2026-11-15 リリース）、obs-websocket v5

## 1. 目的

屋外配信（IRL）で回線が不安定になったら、OBS を待機シーンへ自動で切り替え、回復したら元のシーンへ自動で復帰させる。リレーサーバー、OBS への取り込み、管理ページ、OBS 用オーバーレイを1つのシステムにまとめ、Windows を主に全 OS で「zip を展開して実行するだけ」で導入できる状態を最終形とする。

旧版 `rtmp-monitor`（Python + nginx-rtmp）はこのリポジトリと別に旧版として残す。コードは引き継がず、挙動だけを引き継いで明文化する。

### 位置づけ

- 普段のシーン操作（カメラの切替、トークのシーンへの切替など）は、IRL Link などの外部ツールで行う。本ツールは、IRL Link にない「回線不安定時の自動切替」を補う
- 本ツールが知るシーンは、待機シーン（1つ）と、監視対象のシーン（カメラ映像＝メディアソースを使うシーン。複数）だけ。監視対象のシーンで回線が悪くなったら、そのシーンを戻り先として覚えて待機シーンへ切り替え、回復したら戻り先へ戻す。それ以外のシーンには触らない
- 配信 PC に常駐する。管理ページを屋外から見ることは想定しない
- 本ツールの手動操作（配信の開始・停止、手動の待機 ON / OFF）は、ツールと配信のテスト用の位置づけである

### 前提

- 配置: OBS と同じマシン、または同じ LAN 内の別 PC。VPS は対象外
- 配信 PC は Windows を想定。Linux と macOS でも動くこと
- 入力: RTMP と SRT の両方（送信側は Larix / IRL Pro / Moblin / OBS など）

### 入れないもの（YAGNI）

Discord / Twitch 連携、手動の待機 ON / OFF 以外のシーンを切り替える操作（外部ツールで行う）、OBS シーンの自動作成、待機素材のアップロードと管理、複数の path（複数配信の同時受信）、SRT の RTT やロスを使った切替判定（表示だけに使う）。

## 2. 全体構成

```
[屋外: Larix / IRL Pro / Moblin / OBS]
        │ RTMP :1935/TCP、SRT :8890/UDP  ※ルーターで転送するのはこの2つだけ
        ▼
[MediaMTX]（子プロセス）── 読み出し ──► [OBS メディアソース]
    ▲ 設定生成・起動・監視   │ API（統計）
[irl-live-keeper]（Go の単一実行ファイル）
 ├ supervisor : mediamtx.yml を生成・起動し、落ちたら再起動する
 ├ ingest     : 1秒ごとに API を読み、受信バイト数の差分からビットレートを出す（SRT は RTT・ロスも）
 ├ switcher   : 状態機械（監視対象のシーンと待機シーンの切替を判定する）
 ├ obs        : obs-websocket v5 へ常に1本つなぐ（自動再接続）
 ├ web        : API / 管理ページ / オーバーレイ / SSE 配信
 └ config     : config.yaml が唯一の設定元
```

- 管理ページと API はパスワードで保護する。待ち受けの既定は `127.0.0.1`（OBS と同じ PC から操作する）。別 PC から操作するときは、利用者が config で LAN のアドレスを明示し、起動時に警告を出す。ルーターで転送しない
- 配布物: zip に `irl-live-keeper(.exe)`、`mediamtx(.exe)`、ライセンス表記（MediaMTX は MIT）を入れる。`config.yaml` は初回起動時に生成する
- 生成物（`config.yaml`、`mediamtx.yml`、`logs/`）は実行ファイルと同じディレクトリに置く
- Go の依存: obs-websocket は `github.com/andreykaipov/goobs`、HTTP は標準の `net/http`、設定は YAML（`gopkg.in/yaml.v3`）、ログは `log/slog`、Windows の Job Object は `golang.org/x/sys/windows`。管理ページは素の HTML と JS を `go:embed` で埋め込み、ビルド工程は持たない

### 段階

| 段階 | 内容 | spec |
|---|---|---|
| 1. コア | supervisor、ingest、switcher、obs、最小の JSON API、config.yaml。旧 rtmp-monitor を置き換えられる | **この文書** |
| 2. 管理ページ | 状態表示、グラフ、操作、設定編集（OBS のシーン一覧を `GetSceneList` で取得し、待機シーン、監視対象のシーン、開始時のシーンを選ばせる）、送信用 URL と OBS 用 URL の表示、認証画面 | 後で別に書く |
| 3. オーバーレイ | OBS のテキストソースを書き換えて、通信状況を配信に乗せる方向。表示する項目の候補: 受信ビットレート、回線の状態、プロトコル、SRT の RTT・ロス、接続時間、再接続の回数、待機中の経過時間、配信時間、送信ビットレート、ドロップしたフレーム。雛形の文字列で設定する案。ブラウザソースと排他にするかは第3段階で決める | 後で別に書く |
| 4. 配布 | 各 OS 向けクロスビルド、MediaMTX 同梱の zip、GitHub Actions でのリリース | 後で別に書く |

以下は第1段階（コア）の詳細設計である。

## 3. ディレクトリ構成

```
cmd/irl-live-keeper/main.go   … 起動と配線だけ
internal/config               … config.yaml の読み込み・検証・初回生成
internal/supervisor           … mediamtx.yml の生成、子プロセスの起動・再起動
internal/ingest               … Poller（API 読み取り）と Judge（受信状態の判定）
internal/switcher             … 純粋な状態機械（I/O なし）
internal/obs                  … goobs のラッパー（接続維持・イベント・操作）
internal/core                 … 中核ループ（各部品をつなぎ、状態を1か所で持つ）
internal/web                  … JSON API（第1段階）
internal/logging              … 標準出力 + 日付ごとのログファイル
internal/redact               … ログの行の中のパスワードを *** に置き換える（logging と supervisor が使う）
```

## 4. config.yaml

初回起動時に以下の既定値で生成する。`*` の値は生成時にランダムに作る。

```yaml
admin:
  listen: "127.0.0.1:8787"   # 管理 API。別 PC から操作するときだけ LAN のアドレスに変える（ルーターで転送しない）
  password: "*"              # Basic 認証のパスワード（ユーザー名は admin）

mediamtx:
  binary: ""                 # 空なら実行ファイルと同じディレクトリの mediamtx(.exe)
  rtmp_port: 1935
  srt_port: 8890
  rtsp_port: 8554            # OBS からの読み出し用（spike の結果で見直す）
  api_port: 9997             # 127.0.0.1 にだけ公開する

stream:
  name: live                 # MediaMTX の path 名
  publish_user: irl
  publish_pass: "*"
  read_user: obs             # OBS が読み出すときの資格情報（OBS 用 URL に埋め込む）
  read_pass: "*"
  publish_host: ""           # 送信用 URL の host。空なら起動時に LAN の IPv4 を自動で調べる（自動）。ポート転送やトンネル、VPN のアドレスが決まっていれば、そのホスト名か IP を書く（手動）

obs:
  host: 127.0.0.1
  port: 4455
  password: ""               # 空なら認証なしで接続する（OBS 側で認証を無効にしている場合）
  media_source: IRL          # MediaMTX を読むメディアソースの入力名
  standby_scene: Standby     # 待機シーン（1つ）。回線が悪い間に出す OBS のシーン名。既定値は Standby
  monitored_scenes:          # 監視対象のシーン（1つ以上）。カメラ映像（メディアソース）を使うシーン。既定値は Camera の1つ
    - Camera

on_stream_start:             # 配信開始時の切替（安全機能）
  enabled: false             # true にすると、配信が始まったときに scene へ切り替える
  scene: ""

switch:
  auto_enabled: true         # 自動切替。あえて乱れた映像を出したいときに false にする
  fail_kbps: 800             # これ未満が low_sec 続いたら LOW
  low_sec: 5
  recover_kbps: 1500         # これ以上が recover_sec 続いたら OK
  recover_sec: 10
  down_sec: 2                # 受信0または未接続がこれだけ続いたら DOWN

log:
  dir: logs
  keep_days: 14
```

検証（1つでも満たさなければ、項目名と理由を表示して起動を止める）:
- `recover_kbps > fail_kbps`
- 各秒数は 1 以上、`down_sec <= low_sec`
- ポートは 1〜65535 で、互いに重複しない（`admin.listen` のポートも含む）
- `stream.name`、`publish_user`、`publish_pass`、`read_user`、`read_pass` は空でなく、`[A-Za-z0-9_-]` の文字だけからなる（URL の区切り文字 `:`、`&`、`#` などで送信用 URL が壊れ、ログの置き換えも効かなくなるため）。生成する値もこの範囲で作る
- `publish_pass` と `read_pass` は8文字以上
- `publish_user` と `read_user` は `any` でない（MediaMTX は `any` を資格情報の照合なしで通すため）。また互いに異なる
- `obs.standby_scene` は空でなく、`obs.monitored_scenes` に含まれない
- `obs.monitored_scenes` は1つ以上で、どの名前も空でなく、互いに異なる
- `on_stream_start.enabled` が true のとき、`on_stream_start.scene` は空でなく、`obs.standby_scene` と異なる（待機シーンで配信が始まると、持ち主が `none` の待機になり自動で戻らないため）
- `admin.password` は空でない
- `stream.publish_host` は空か、ホスト名か IP アドレス（`net.ParseIP` で通る IP か、`[A-Za-z0-9.-]` の文字だけからなる 253 文字以内の名前）。スキーム、ポート、パス、`[ ]` は含めない
- `log.dir` は空でなく、`log.keep_days` は 0 以上

シーンの設定（`obs.standby_scene`、`obs.monitored_scenes`、`switch.auto_enabled`、`on_stream_start`）は、第2段階の管理ページで OBS のシーン一覧（`GetSceneList`）を取得して選べるようにする。そのため switcher は、これらを config から直接読まず、差し替えられる入力（Event `ScenesConfigured`。6章）として受け取る。第1段階では起動時に config から1回だけ渡す。初回生成の既定値は `standby_scene: Standby`、`monitored_scenes: [Camera]` とする。OBS にその名前のシーンがなければ、接続時に status の `warnings` に出る（6章末「実装前に決めたこと」の 4）。

`admin.listen` が loopback 以外（`0.0.0.0` や LAN のアドレス）のときは、起動するたびに警告をログに出す: 管理 API を LAN に公開していること、通信は平文で LAN 内では盗聴されうること、ルーターで転送しないこと。値を省略したときに LAN 公開と解釈することはない（省略時は既定の `127.0.0.1:8787`）。

## 5. 受信状態の判定（ingest）

### Poller

1秒ごとに次を読む（各リクエストのタイムアウトは 500ms）。

- `GET /v3/paths/get/{stream.name}`: `online`、`inboundBytes`、`source.type`（`rtmpConn` / `srtConn` など）、publisher の識別子 `source.id`（接続ごとに変わる UUID。spike ③ で確認。`onlineTime` は fallback として読まない）
- `source.type` が `srtConn` のときだけ `GET /v3/srt/conns/list` を読み、その path を publish している接続の `msRTT`、`mbpsReceiveRate`、`packetsReceivedLossRate` を取る（表示用）。この読み取りだけが失敗したときは `SRT` を nil にし、`Kbps` と `Err` には影響させない

非推奨のフィールドとエンドポイント（`ready`、`bytesReceived`、`/v3/srtconns/*`）は使わない。

出力する `Sample`:

```go
type Sample struct {
    At       time.Time // 取得時刻
    Interval time.Duration // 前回の Sample からの経過時間（この Sample が表す区間の長さ）
    Online   bool      // path に publisher がいるか
    Protocol string    // "rtmp" / "srt" / ""
    Kbps     float64   // 下記の「基準」からの inboundBytes の差分 × 8 / 1000 / 基準からの経過秒
    SRT      *SRTStats // SRT のときだけ
    Err      error     // path の API の失敗（このとき Kbps は 0 として扱う）
}
```

- `inboundBytes` の差分から自前でビットレートを出す。モバイル回線では TCP 接続が残ったままバイトだけ止まることがあるため、切断イベントではなく差分が 0 かどうかで途切れを判定する
- **基準:** Poller は「最後に path の API を読めたときの `inboundBytes`、時刻、publisher の識別子」を基準として持つ。`Kbps` の分母は基準からの経過時間で、`Interval`（前回の Sample から）とは別物である。API が失敗した回は基準を変えないので、失敗の次の回は2秒分のバイトを2秒で割る
- 次のときは、その回の `Kbps` を 0 とし、読めた値を新しい基準にする: 起動後の最初の読み取り、publisher の識別子が変わったとき（識別子がない状態からある状態への変化を含む。再接続や MediaMTX の再起動はこれか次の条件で必ず検出されるので、再起動の通知は使わない）、`inboundBytes` が基準より減ったとき
- `Online` が false の間は `Kbps` を 0 とする（publisher がいなくても、設定にある path は HTTP 200 で `online: false`、`source: null`、`inboundBytes: 0` を返す）
- 送信が TCP 接続を残したまま止まると、`online` は true のまま `inboundBytes` だけが増えなくなる。MediaMTX は約10秒（`readTimeout`）でその接続を閉じる。止まっていた送信が再開すると、たまっていたデータがまとめて届き、直後の `Kbps` が一時的に大きくなる（spike ③ で確認）
- path の API が失敗したときは `Err` を入れ、`Kbps` を 0、`Online` を false、`Protocol` を空にする。`Interval` は前回の Sample からの経過時間のまま
- 起動後の最初の Sample の `Interval` は 0 とする（累積に何も足さない）

### Judge

`Judge.Update(Sample) LinkState` は時刻を `Sample` から受け取る純粋な処理とし、内部で時計を読まない。

受信状態は `OK` / `LOW` / `DOWN` の3つ。起動直後は `DOWN`。

各 Sample は直前の Sample からの区間（`Interval`）を表す。「X 秒続く」は、条件を満たす Sample が連続している間の `Interval` の合計が X 秒以上になったこと、と定義する（1秒間隔なら `down_sec: 2` は2回連続、`low_sec: 5` は5回連続）。

| Sample の分類 | 条件 |
|---|---|
| zero | `!Online`、`Err != nil`、または `Kbps == 0` |
| below | `Kbps < fail_kbps`（zero を含む） |
| good | `Kbps >= recover_kbps`（zero は含まない） |

zero、below、good の各累積は、その条件を満たさない Sample が来た時点で 0 に戻る。

遷移: Sample ごとに、まず3つの累積を更新し、次の順に上から評価して、最初に当てはまったものだけを適用する。どれにも当てはまらなければ今の状態のまま。1つの Sample で状態は1回だけ変わる。
1. 状態が `LOW` / `DOWN` で、good が `recover_sec` 続いた → `OK`
2. 状態が `OK` / `LOW` で、zero が `down_sec` 続いた → `DOWN`（LOW と DOWN の区別は表示のため。切替の扱いは同じ）
3. 状態が `OK` で、below が `low_sec` 続いた → `LOW`
4. 状態が `DOWN` で、非 zero の Sample が来た → `LOW`（good の累積はそのまま続ける）

したがって `fail_kbps` 以上 `recover_kbps` 未満の Sample は、`OK` を `LOW` にはしないが、`OK` への復帰の累積を 0 に戻す。

### 実時刻の例（既定値。テストでこの値を確かめる）

`t` は Sample の `At`（開始からの秒）。特記がなければ `Interval` は 1 秒。

| # | 始めの状態 | Sample の列 | 期待 |
|---|---|---|---|
| 1 | OK | t=1、2 が zero | t=1 は OK のまま、t=2 で DOWN |
| 2 | OK | t=1〜5 が 600kbps | t=4 は OK のまま、t=5 で LOW |
| 3 | OK | t=1〜4 が 600kbps、t=5 が 1000kbps、t=6 以降 600kbps | t=5 で below の累積が 0 に戻り、t=10 で LOW |
| 4 | DOWN | t=1〜10 が 2000kbps | t=1 で LOW、t=9 は LOW のまま、t=10 で OK |
| 5 | LOW | t=1〜6 が 2000kbps、t=7 が 1200kbps、t=8 以降 2000kbps | t=7 で good の累積が 0 に戻り、t=17 で OK |
| 6 | OK | `Interval` 1.5 秒で t=1.5、3.0 が zero | t=1.5 は OK のまま（1.5 秒）、t=3.0 で DOWN（3.0 秒） |
| 7 | OK | t=1 だけ `Err`、t=2 以降 2000kbps | OK のまま変わらない |
| 8 | LOW | t=1、2 が zero | t=2 で DOWN |
| 9 | OK | t=1 が zero、t=2 が 600kbps、t=3〜5 が 600kbps | zero の累積は t=2 で 0 に戻り DOWN にならない。below は t=1 から続くので t=5 で LOW |
| 10 | OK（`down_sec: 2`、`low_sec: 2`） | t=1、2 が zero | t=2 で DOWN と LOW の両方が成り立つが、評価順で DOWN |
| 11 | DOWN（`recover_sec: 1`） | t=1 が 2000kbps | 「→ LOW」と「→ OK」の両方が成り立つが、評価順で t=1 に OK |

Poller の例（Judge ではなく Poller のテストで確かめる。1秒ごとに読む）:

| # | API の応答 | 期待する `Kbps` |
|---|---|---|
| P1 | t=0 で publisher がすでにいて `inboundBytes`=0（起動後の最初）、t=1 で同じ publisher のまま 87,500 | t=0 は 0（基準を取るだけ）、t=1 は 700 |
| P2 | t=0 で 0、t=1 で失敗、t=2 で 175,000 | t=1 は 0（`Err`）、t=2 は 700（2秒分を2秒で割る） |
| P3 | t=0 で 500,000、t=1 で publisher の識別子が変わり 600,000 | t=1 は 0（新しい基準）、t=2 からは新しい基準との差分 |
| P4 | t=0 で 500,000、t=1 で 20,000（識別子は同じ） | t=1 は 0（新しい基準） |

## 6. シーンの状態機械（switcher）

### シーンの設定と分類

switcher は、シーンの設定を Event `ScenesConfigured` で受け取り、差し替えられる入力として持つ（config を直接読まない）。第1段階では、config.yaml から作った `ScenesConfigured` を、中核ループを始める前に最初の Event として `Handle` に1回渡す。第2段階では、管理ページで設定を編集するたびに送る（送る前に 4章と同じ検証を通す。switcher は検証しない）。

OBS の現在のシーン名を、その時点の設定で3つに分類する。

| 分類 | 条件 |
|---|---|
| Monitored（監視対象） | シーン名が `Monitored` の列にある |
| Standby（待機シーン） | シーン名が `Standby` と同じ |
| Other（管理外） | 上のどちらでもない |

OBS に未接続などで現在のシーンがわからない間は、分類を「不明」とし、どの条件にも当てはまらないものとして扱う（status では `unknown`）。

### 入力（Event）と出力（Action）

```go
// Event はすべて At（時刻）を持つ
LinkChanged{State LinkState}
OBSConnected{Streaming bool, Scene string, TransitionMs *int} // TransitionMs: 全体のトランジションの長さ。固定長（カット、スティンガー）で取れなければ nil
OBSDisconnected{}
SceneChanged{Scene string}           // program のシーンの変更（CurrentProgramSceneChanged）だけ。Studio Mode の preview は扱わない
TransitionChanged{TransitionMs *int} // 全体のトランジションまたはその長さが変わった
StreamStateChanged{Streaming bool}   // 写像は 7章。出力の再接続では流れない
ScenesConfigured{Standby string, Monitored []string, AutoEnabled bool, StartScene *string} // シーンの設定。StartScene は配信開始時の切替が無効なら nil
Command{Kind: StreamStart | StreamStop | StandbyOn | StandbyOff}
SceneRequestDone{ReqID int, Result: OK | Timeout | Error} // 送った SetScene の応答（期待の期限の起点）
BatchDone{BatchID int, Result: Completed | Failed | Rejected, Err error} // バッチの終わり（成功、失敗、承認の却下のどれでも1回）
Tick{}        // 1秒ごと。再送と期待の期限の判定に使う

// Action
SetScene{Scene string}           // OBS のシーン名そのもの
RestartMedia{Wait time.Duration} // RESTART を送ったあと、次の Action までの待ち時間（定数 1秒。spike ② で決めた）
StartStream{}
StopStream{}
```

- `Machine.Handle(Event) (batch *Batch, cmdErr error)`。`Batch` は `BatchID`（通し番号）、`Purpose`（目的）、作った時点の `SceneEpoch`、`ConnEpoch`、`StreamEpoch`、`ConfigEpoch`、Action の列を持つ。待機シーンへ切り替えるバッチ（AutoFail、StandbyOn）は、さらに戻り先のシーン名（`ReturnScene`。作った時点の現在のシーン名）を持つ
- `Machine.Approve(batchID int, action Action, override Override, at time.Time) ApproveResult`。実行役が Action を送る直前に呼ぶ（9章）。`Override` はトランジションの上書きの状態で、`None`（上書きなし）、`Known{Ms}`（上書きあり・長さがわかる）、`Unknown`（上書きあり・長さ不明、または読み取りに失敗した）の3つを区別する。`SetScene` 以外では `None` を渡す。結果は次のどれか:
  - `Proceed{ReqID}`: 送る。`SetScene` なら期待を登録して ReqID を返す
  - `Skip`: 送らずに成功とみなす
  - `Reject`: バッチの残りを捨てる
  - `Replace{Batch}`: このバッチの残りを捨て、決め直した扱いに置き換える。`Batch` が nil でなければ、中核ループは代わりのバッチを列の先頭に入れる。`Batch` が nil なら、代わりに実行するものはない（switcher が `Approve` の中で自分の状態を変えて済ませた）。実行役は元のバッチの `BatchDone{Rejected}` を送るが、switcher は置き換えたバッチの却下を `last_error` に記録しない。v2.0 でこれを使うのは StandbyOffMonitored だけで、`Batch` は常に nil（下の「手動操作」を参照）
- `SceneEpoch` は手動とみなした `SceneChanged` のたびに、`ConnEpoch` は `OBSDisconnected` のたびに、`StreamEpoch` は配信状態が変わるたび（`StreamStateChanged` と、`OBSConnected` で配信状態が切断前と違っていたとき）に1つ進む。`ConfigEpoch`（設定の世代）は、シーンの設定を変える `ScenesConfigured`（内容が今の設定と違うもの。`AutoEnabled` と `StartScene` の変更を含む）のたびに1つ進む
- シーンの比較（期待との照合、`Skip` の判定、分類）は、すべてシーン名の文字列の一致で行う

`cmdErr` は `Command` を受け付けられないときだけ返し、型で2種類に分ける: `ErrNotConnected`（OBS に未接続）と `ErrInvalidState`（今の状態では受け付けられない。理由の文を持つ）。web はこの型から 503 と 409 を決め、自分では状態を判定しない。

### 待機の持ち主と戻り先

switcher は、待機シーンに「誰が入れたか」と「どこへ戻すか」を持つ。

| 状態 | 値 | 意味 |
|---|---|---|
| 待機の持ち主（`StandbyOwner`） | `none` | 本体は待機に入れていない（人が手動で待機シーンを選んだ場合を含む） |
| | `auto` | 自動切替で待機シーンに入れた。受信状態が回復したら戻り先へ戻す |
| | `manual` | 手動の待機（StandbyOn）で入れた。StandbyOff まで自動では戻さない |
| 戻り先（`ReturnScene`） | シーン名、またはなし | 待機シーンに入る直前のシーン。持ち主が `none` のときはなし |

- 持ち主と戻り先を入れるのは、待機シーンへの**期待どおりの `SceneChanged` が届いた時点**だけ（AutoFail なら `auto`、StandbyOn なら `manual`。戻り先はそのバッチが持つ `ReturnScene`）。バッチの却下、失敗、期待の期限切れでは何も入れない。このため、期待は `ReqID` のほかに、バッチの Purpose と `ReturnScene` を持つ
- 持ち主と戻り先を消す（`none`、なし）のは次のとき:
  - 戻り先への期待どおりの `SceneChanged` が届いたとき（AutoRecover、StandbyOffMonitored、StandbyOffOther）。戻すバッチが却下、失敗、期限切れになったときは消さない（`auto` なら条件が続く間 `Tick` で再送する。`manual` なら `last_error` に残り、もう一度 StandbyOff を受け付ける）
  - 手動（期待にない）の `SceneChanged` が届いたとき（「外部からの手動切替」）
  - `StreamStateChanged{Streaming: false}`（配信の停止）を受けたとき
  - `OBSConnected` で、シーンか配信状態が切断の直前と違っていたとき（どちらも同じなら保つ。websocket の瞬断だけで戻り先を忘れないため。配信状態が違っていたら配信の区切りとみなす）
  - `ScenesConfigured` で、待機シーンの名前が変わったとき、または持ち主が `auto` で戻り先が新しい設定の Monitored でなくなったとき（今いるシーンはそのまま。どちらでもなければ保つ）
  - 持ち主が `auto` の間に `auto_enabled` が false になったとき（待機シーンに残り、戻り先を忘れる）
- したがって、OBS に接続している間、持ち主が `none` 以外なら現在のシーンは待機シーンである

### 目的と有効条件（実行直前の承認）

決めた時点では正しかったが、実行するまでに状況が変わったバッチを実行しないため、各バッチは目的（Purpose）を持ち、`Approve` のたびにその目的の有効条件を**その時点の状態で**確かめる。共通の条件として、OBS に接続していて、`ConnEpoch` と `StreamEpoch` がバッチを作った時点と同じであること（前の配信のために作ったバッチが、停止と再開のあとの次の配信で実行されないようにする。例: 受け付けた StreamStop が実行待ちの間に、OBS の画面で配信を止めて始め直した）。シーンを切り替えるバッチは、さらに `SceneEpoch` が同じであること（作ったあとに人がシーンを切り替えていたら、人の操作を優先する）と、`ConfigEpoch` が同じであること（作ったあとにシーンの設定が変わっていたら、古い設定で決めた切替を実行しない）。

| Purpose | 出すバッチ | 有効条件（共通の条件に加えて） | 条件を満たさないとき |
|---|---|---|---|
| AutoFail | `[SetScene(待機シーン)]` | 配信中、`auto_enabled`、現在のシーンが Monitored、受信状態が LOW / DOWN、持ち主が `none`、切替中の抑止（下記）に当たらない | 却下 |
| AutoRecover | `[RestartMedia, SetScene(戻り先)]` | 配信中、`auto_enabled`、持ち主が `auto`、現在のシーンが待機シーン、受信状態が OK、切替中の抑止に当たらない | 却下 |
| StandbyOn | `[SetScene(待機シーン)]` | 配信中、持ち主が `none`、現在のシーンが待機シーンでない（通常の列では共通の条件で先に却下されるため起きない組み合わせがあり、テストは状態を直接作って `Approve` を呼ぶ。V-13） | 却下（`Skip` にはしない。`last_error` に記録する。承認の前に待機シーンへ切り替わっていた場合の却下で、通常の列ではそれは人の切替なので、`SceneEpoch`（共通の条件）による。StandbyOn のバッチが未完了の間と、その期待が残る間は AutoFail が出ず、AutoFail のバッチが未完了の間と、先に出た期待が残る間は StandbyOn を受け付けないため、AutoFail が先に待機シーンへ届く列はなく、この行の条件そのものでの却下は、状態を直接作ったテストで確かめる） |
| StandbyOffMonitored | `[RestartMedia, SetScene(戻り先)]` | 配信中、持ち主が `manual`、現在のシーンが待機シーン、`auto_enabled` が true なら受信状態が OK（false なら受信状態を見ない） | `auto_enabled` が true で受信状態だけが悪くなったなら、`Replace`（代わりのバッチなし）を返し、その場で持ち主を `auto` にする（シーンは変えない。回復したら AutoRecover で戻る）。それ以外は却下 |
| StandbyOffOther | `[SetScene(戻り先)]` | 配信中、持ち主が `manual`、現在のシーンが待機シーン | 却下 |
| StartScene | `[SetScene(開始時のシーン)]` | 配信中 | 却下（再送しない。`last_error` に記録する。手動の切替による却下は記録しない） |
| StartStream | `[StartStream]` | 配信中でない | 却下 |
| StopStream | `[StopStream]` | 配信中（`SceneEpoch` は見ない） | 却下 |

- バッチの `SetScene` の切替先は、バッチを作った時点のシーン名（待機シーン、戻り先、開始時のシーン）で固定する
- 受信状態は AutoFail、AutoRecover、StandbyOffMonitored（`auto_enabled` が true のとき）の条件にだけ入る。配信の開始・停止、手動の待機 ON、開始時の切替は、回線の状態が変わっても捨てない
- `SetScene` の承認で、現在のシーンがすでに切替先のシーンなら `Skip`（送らず、期待も登録しない）
- `BatchDone{Rejected}` または `{Failed}` が Command 由来のバッチ（StandbyOn、StandbyOff の2つ、StartStream、StopStream）か StartScene のバッチなら、`last_error` に記録する。API はすでに 200 を返しているので、ここで利用者に見えるようにする
- 実行待ちか実行中のバッチは、`BatchDone` が来るまで「未完了」として switcher が持つ。同じ Purpose の未完了のバッチがある間は、同じ Purpose の新しいバッチを出さない（イベント起点の判定にも再送にも掛かる）

### 自動切替

自動切替は、**OBS に接続していて、配信中で、`auto_enabled` が true のときだけ**行う。現在のシーンは OBS のイベントで追う。

- **AutoFail:** 現在のシーンが Monitored、受信状態が `LOW` か `DOWN`、持ち主が `none` → `[SetScene(待機シーン)]`。バッチは戻り先として、その時点の現在のシーン名を持つ。戻り先と持ち主（`auto`）が入るのは、待機シーンへの期待どおりの `SceneChanged` が届いた時点
- **AutoRecover:** 持ち主が `auto`、現在のシーンが待機シーン、受信状態が `OK` → `[RestartMedia, SetScene(戻り先)]`
  - `RestartMedia` は `TriggerMediaInputAction`（`OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART`）を `obs.media_source` に送る
  - RESTART と切替の順序、間に置く待ち時間（`RestartMedia.Wait`）は spike ②（12章）で決めた: RESTART を送り、1秒待ってから切り替える。Windows と Linux の OBS で、RESTART から再生開始（`MediaInputPlaybackStarted`）まで約 1.02秒で、切替のトランジションの時間も加わるので、戻った瞬間に映像が出た
- Other のシーンには触らない。持ち主が `none` の待機シーン（人が手動で選んだ）と、持ち主が `manual` の待機シーンからは、自動では戻さない
- `ScenesConfigured` を受けたら、新しい設定に差し替え、持ち主と戻り先を上の規則で確かめ（「待機の持ち主と戻り先」）、`ConfigEpoch` を進める。未完了のシーン切替のバッチは承認で却下される。期待はそのまま残す（そのあと届いた `SceneChanged` を手動と誤認しないため）。そのあと通常の判定をする
- 受信状態が `LinkChanged` で変わったときだけでなく、`SceneChanged`（本体の切替でも手動の切替でも）、`StreamStateChanged`、`OBSConnected`、`ScenesConfigured`、`Tick` のあとにも同じ判定をする
- **切替中の抑止:** StartScene、StandbyOn、StandbyOffMonitored、StandbyOffOther のどれかのバッチが未完了か、その期待が残っている間は、自動切替の判定をしない。Auto のバッチの承認も、この間は却下する。手動の待機を押した直後や配信開始時の切替の途中で回線が変わっても、その切替が自動切替で上書きされないようにするため。切替が届いたあと（または却下、失敗、期限切れのあと）は通常の判定に戻る
- **`auto_enabled` が false のとき:** Auto のバッチを出さない。実行待ちの Auto のバッチは承認で却下する。持ち主が `auto` なら `none` にし、戻り先を消す（待機シーンに残る）。あえて乱れた映像を出したいときのための設定で、配信の開始・停止と手動の待機（StandbyOn / StandbyOff）は使える。このときの StandbyOff は受信状態を見ずに戻り先へ戻す（「手動操作」。自動切替を切った利用者は、乱れた映像が出ることを承知している）

### 外部からの手動切替

普段のシーン操作は、OBS の画面や IRL Link などの外部ツールで行う。obs-websocket のシーン変更イベントには、誰が切り替えたかが含まれない。そこで次のように扱う。

- **期待シーン:** 期待は、`SetScene` を決めた時点ではなく、実行役が実際に送る直前の `Approve` が `Proceed` を返した時点で登録する。期待は列で持ち、送った `SetScene` ごとに1つ足す。各期待は `ReqID`、切替先のシーン名、バッチの Purpose と `ReturnScene`、登録した時点の `ConfigEpoch` を持ち、`SceneRequestDone` と1対1で対応する
- **期限:** 応答を待っている間は期限切れにしない。期限は `SceneRequestDone` を受けた時点から数え始め、「その時点 + 実際に使われるトランジションの長さ + 3秒」とする。OBS はトランジションが終わった時点で `CurrentProgramSceneChanged` を出すため、トランジションの長さを含める
  - 実際に使われる長さ: `Approve` に渡された `Override` が `None` なら全体の長さ、`Known{Ms}` なら上書きの長さ
  - 次のときは「長さ + 3秒」と「10秒」の長いほう（長さがなければ10秒）: `Override` が `Known`（固定長かどうかが応答からわからない）、`Override` が `Unknown`、`None` で全体の長さが取れない（固定長のトランジション）
  - `Result: Timeout` でも期限まで残す（タイムアウトは OBS が適用しなかったことを意味しない。要求を取り消す手段はない）。`Result: Error`（シーンが存在しない、など OBS がエラーを返した）なら、その期待をすぐ消す
- **本体の切替と外部の切替の見分け方:** `SceneChanged` が、列の先頭から見て最初に一致する期待のシーン名と同じなら、本体の切替とみなし、その期待と、それより前にある期待をすべて消す（後から送った要求が前の要求を上書きしたとみなす。OBS は進行中のトランジションを打ち切って次を始めることがある）。一致した期待の Purpose に応じて、持ち主と戻り先を入れるか消す（「待機の持ち主と戻り先」）。ただし、期待の `ConfigEpoch` が今の値と違うとき（登録したあとにシーンの設定が変わった）は、本体の切替とみなすだけで、持ち主と戻り先には作用しない（消した持ち主と戻り先を、古い設定の期待で復活させないため）。前にあって一緒に消した期待は、持ち主と戻り先に作用しない。どの期待とも一致しない `SceneChanged` は、すべて手動（外部）の切替とみなす。期限を過ぎた期待は消す。obs-websocket の応答とイベントのどちらが先に届いても、期待は送る前に登録してあるので判定は変わらない
- **手動の切替を受けたとき:** `SceneEpoch` を1つ進め、期待を全部消し、持ち主を `none` に、戻り先をなしにする。実行待ちのシーン切替のバッチは、`SceneEpoch` が進むので承認で却下される。そのうえで、新しいシーンについて通常の自動切替の判定をする。結果は切り替えた先の分類で決まる:

| 人が切り替えた先 | 受信状態 | 結果 |
|---|---|---|
| Monitored | OK | 何もしない |
| Monitored | LOW / DOWN | AutoFail が出る。**そのシーンが新しい戻り先になる**（待機シーンから入った場合も、トークのシーンなど Other から入った場合も同じ） |
| 待機シーン | どちらでも | 何もしない。持ち主は `none` で、回復しても自動では戻さない |
| Other | どちらでも | 何もしない |

- 回線が悪い間に人が監視対象のシーンを選ぶと、すぐ待機シーンへ戻される。これは意図した動きである（カメラ映像のシーンは、回線が悪い間は出さない。人が選んだシーンは戻り先として覚え、回復したらそこへ戻す）。あえて出したいときは `auto_enabled` を切る
- 配信していない間は、持ち主と戻り先を持たない。配信前に OBS の画面でシーンを確かめる操作では、何も起きない
- 待機の持ち主と戻り先を status に出す

### 再送の歯止め

自動切替の Action を出したあと、OBS のシーンが期待どおりに変わらない（失敗した、またはイベントが来ない）まま条件が続く場合は、`Tick` で再送する。自動切替のバッチを出すのは（イベント起点の判定でも `Tick` の再送でも）次をすべて満たすときだけ:
- 同じ Purpose の未完了のバッチがなく、そのシーンへの期待も残っていない
- `Tick` の再送なら、同じ Purpose の前回の送出から5秒以上たった
- 切替中の抑止に当たらない

### バッチ内の失敗

バッチは、Action が失敗した時点で残りを捨てる。例外は `RestartMedia` の失敗だけで、続く `SetScene` は実行する（映像の再始動に失敗しても、待機画面からは戻す）。

### 手動操作

本体の手動操作は、配信の開始・停止と、手動の待機 ON / OFF の4つだけで、ツールと配信のテスト用の位置づけである（普段のシーン操作は外部ツールで行う）。すべて OBS に接続している必要がある（未接続なら `ErrNotConnected`、web は 503）。

| Command | 受け付ける条件 | Action |
|---|---|---|
| StreamStart | 配信中でない | StartStream のバッチ。配信を始めるだけで、シーンは変えない |
| StreamStop | 配信中 | StopStream のバッチ。配信を止めるだけで、シーンは変えない |
| StandbyOn | 配信中、持ち主が `none`、現在のシーンが待機シーンでない、StartScene / AutoFail / AutoRecover のバッチが未完了でない | StandbyOn のバッチ（`[SetScene(待機シーン)]`）。バッチは戻り先として、その時点の現在のシーン名を持つ（分類を問わない） |
| StandbyOff | 配信中、持ち主が `manual` | 下記 |

条件を満たさない Command は `ErrInvalidState` を返し、web は 409 を返す。受け付ける条件を満たしていても、同じ操作のバッチが未完了なら同じく `ErrInvalidState`（理由: 同じ操作が実行待ち）を返す。

**手動の待機 ON:** 待機シーンへの期待どおりの `SceneChanged` が届いた時点で、持ち主を `manual` に、戻り先をバッチが持つシーン名にする。持ち主が `manual` の間は、自動切替をしない（回線が回復しても戻さない）。次のときは StandbyOn を受け付けず、`ErrInvalidState` を返す: 現在のシーンがすでに待機シーン（持ち主が `auto` の自動の待機中も、人が手動で待機シーンを選んだ持ち主 `none` のときも。理由: すでに待機シーン）、StartScene、AutoFail、AutoRecover のどれかのバッチが未完了か、期待が1つでも残っている（理由: 切替の実行中。タイムアウトして `BatchDone` を受けたあとも、期待が残る間は OBS が後から切り替えうるため）。受け付けたあと、承認の前に待機シーンへ切り替わっていた場合は、承認で却下して `last_error` に記録する。

**手動の待機 OFF の戻り方:** 受け付けた時点の戻り先の分類と受信状態で決める。`auto_enabled` が false のときは受信状態を見ず、OK の行と同じに扱う（戻り先が Monitored なら `[RestartMedia, SetScene(戻り先)]`、そうでなければ `[SetScene(戻り先)]`）。下の表は `auto_enabled` が true のとき。

| 戻り先 | 受信状態 | 動き |
|---|---|---|
| Monitored | OK | StandbyOffMonitored のバッチ（`[RestartMedia, SetScene(戻り先)]`） |
| Monitored | LOW / DOWN | シーンを変えず、持ち主を `auto` にする（バッチは出さない。`cmdErr` は nil で、web は 200 を返す）。回復したら AutoRecover で戻り先へ戻る |
| Monitored でない | どちらでも | StandbyOffOther のバッチ（`[SetScene(戻り先)]`） |

- `auto_enabled` が true のとき、StandbyOffMonitored のバッチが承認を待つ間（`RestartMedia.Wait` の待ちを含む）に受信状態が悪くなったら、`Approve` は `Replace`（代わりのバッチなし）を返し、その場で持ち主を `auto` にする。上の表の2行目と同じ結果になる。置き換えは `last_error` に記録しない
- 戻り先への期待どおりの `SceneChanged` が届いた時点で、持ち主を `none` に、戻り先をなしにする
- 回線が悪いままカメラ映像のシーンへ戻ってすぐ待機シーンへ飛ぶのを避けるため、戻り先が Monitored のときだけ受信状態を見る

### 配信開始時の切替（安全機能）

`on_stream_start.enabled` が true のとき（既定は false）、配信が始まったら指定のシーンへ切り替える。前回の配信のシーンのまま始まるのを防ぐための機能で、誰が配信を始めたか（本体の StreamStart、OBS の画面、外部ツール）を問わない。

- `StreamStateChanged{Streaming: true}` を受けたとき、StartScene のバッチ（`[SetScene(開始時のシーン)]`）を出す。受信状態は見ない（開始時のシーンを優先する）
- StartScene のバッチが未完了か、その期待が残っている間は、自動切替の判定をしない（「切替中の抑止」）
- 開始時のシーンに切り替わったあとは、通常の規則に従う。開始時のシーンが Monitored で受信状態が不良なら、続けて AutoFail が出て、開始時のシーンが戻り先になる
- 切替の前に人がシーンを切り替えたら、`SceneEpoch` が進むので StartScene は承認で却下される（人の操作を優先する）
- StartScene は再送しない。バッチの失敗、却下、期待の期限切れは `last_error` に記録する。ただし、手動の切替で `SceneEpoch` が進んだための却下は記録しない（人が自分でシーンを選んだ結果で、エラーではない）
- 出すのは `StreamStateChanged{Streaming: true}` のときだけ。`OBSConnected` で、切断中に配信が始まっていたとわかった場合は出さない（開始の瞬間はすでに過ぎていて、配信の途中で予期しない切替になるため）
- 開始時のシーンに待機シーンは指定できない（4章の検証）
- **既知の制約:** 配信の開始は `STARTED` で検知する（7章の写像）。そのため、配信の最初のわずかな間は切替前のシーンが配信に出る。`STARTING` の時点で切り替えられるかは spike（12章）で確かめる

### OBS 未接続の間

`OBSDisconnected` から `OBSConnected` までは Action を出さず、`Approve` もすべて却下する。`OBSDisconnected` で期待をすべて消す。`ConnEpoch` が進むので、切断前のバッチは再接続後に実行されない。`OBSConnected` で配信状態と現在のシーンを取り直してから判定を再開する。シーンか配信状態が切断の直前と違っていたら、持ち主を `none` に、戻り先をなしにする（配信状態が違っていたら配信の区切りとみなす）。どちらも同じなら保つ。配信状態が停止から配信中に変わっていても、配信開始時の切替は出さない。

### 配信の停止

`StreamStateChanged{Streaming: false}` を受けたら、持ち主を `none` に、戻り先をなしにし、期待と、各 Purpose の前回の送出時刻（再送待ち）を消す。`StreamEpoch` が進むので、実行待ちのバッチは承認で却下される。

### 細部の規則

v1.x で「入口 × 状態」の表を作る中で決めた規則（U-01〜U-23。番号は v1.x の表の文書の不明点の番号）を、新しいシーンのモデルで確かめ直した。今も成り立つものを残し、言い直したものは言い直した。Intro、Pause、手動保持、配信開始の待ちについての規則（U-02、U-03、U-06、U-08、U-13、U-17、U-20）は、対象がなくなったので削除した。

- **承認の順序（U-01）:** 共通の条件（接続、4つの Epoch）→ `SetScene` で現在のシーンがすでに切替先のシーンなら `Skip` → Purpose の有効条件、の順に確かめる。ただし StandbyOn は `Skip` にせず、有効条件（現在のシーンが待機シーンでない）で却下する。`Skip` は目的を達しているので `last_error` に残さない。`RestartMedia` には `Skip` がなく、Purpose の有効条件で決める（現在のシーンが待機シーンでなければ却下）
- **実行中の操作の重複（U-04）:** 受け付ける条件を満たす Command でも、同じ操作のバッチが未完了なら `ErrInvalidState`（理由: 同じ操作が実行待ち）を返し、状態を変えない。同じ操作として扱う Purpose は、StreamStart なら StartStream、StreamStop なら StopStream、StandbyOn なら StandbyOn、StandbyOff なら StandbyOffMonitored / StandbyOffOther。StandbyOn は、このほかに StartScene、AutoFail、AutoRecover が未完了の間も受け付けない（理由: 切替の実行中）
- **`last_error`（U-05、U-18）:** `BatchDone{Failed}` は Auto と StartScene でも記録する。`Rejected` は Command 由来と StartScene だけ記録する（`Replace` で置き換えたバッチは記録しない）。StartScene の期待の期限切れも記録する。`RestartMedia` だけが失敗して続く `SetScene` が成功したバッチは `Result: Completed` で `Err` に RESTART のエラーを入れ、`Err` があれば記録する。switcher は `last_error` を消さず、新しい記録で上書きする
- **`Tick`（U-07）:** `Tick` のたびに、イベント起点と同じ自動切替の判定を行い、再送の5秒の条件だけを足す。5秒の起点は、`Handle` がそのバッチを返した Event の `At`。その Purpose を一度も出していなければ、5秒の条件は満たしたものとする
- **切替中の自動判定（U-12）:** 「自動切替」の「切替中の抑止」のとおり。v1.x の「Live / Fail 以外の役割への期待が残っている間」を、Purpose（StartScene、StandbyOn、StandbyOffMonitored、StandbyOffOther）で言い直した
- **期限（U-09、U-10、U-11）:**
  - `Override` が `Unknown` のときは、全体の長さがあれば「全体の長さ + 3秒」と「10秒」の長いほう、なければ10秒
  - 期限に使う全体の長さと `Override` は、`Approve` の時点の値を期待に写す
  - `At > 期限` で期限切れとする（ちょうどは残る）。期限切れの期待は、`Tick` に限らず、すべての `Handle` と `Approve` の入口でその時刻を使って消す
  - 期限切れそのものは手動の切替として扱わない（`SceneEpoch` を進めず、持ち主と戻り先も変えない）
- **同じ値の Event（U-14）:** 今と同じ値の `StreamStateChanged` と `LinkChanged`、今と同じ内容の `ScenesConfigured` では何もしない（Epoch も進めない）
- **初期値（U-15）:** 受信状態は DOWN、切断前の配信状態は false、切断前のシーンは空、4つの Epoch は 0、持ち主は `none`、戻り先はなし。シーンの設定は初期値を持たず、最初の Event として `ScenesConfigured` を必ず受ける
- **配信停止で消す再送待ち（U-16）:** 各 Purpose の前回の送出時刻を消す
- **Command と自動判定:** Command を受け付けた `Handle` では、Command のバッチだけを返す。自動切替の判定は次の Event か `Tick` で行う（1回の `Handle` が返すバッチは多くても1つ）
- **`ReqID`（U-21）:** `SetScene` 以外の `Proceed` の `ReqID` は 0
- **未知の番号（U-22）:** 未完了の一覧にない `BatchID` の `Approve` は `Reject`、`BatchDone` は無視。期待にない `ReqID` の `SceneRequestDone` は無視（期待が先に `SceneChanged` で消える通常の列で起きる）
- **再接続でシーンか配信状態が変わっていたとき（U-23）:** 手動の切替としては扱わない（`SceneEpoch` を進めない）。持ち主を `none` に、戻り先をなしにするだけで、そのあと通常の判定をする（配信開始時の切替は出さない）
- **テストの置き場所（U-18、U-19）:** バッチ内の失敗の続行と打ち切りは実行役の規則なので core（T9）で確かめる。配信状態の写像は obs（T7）の変換関数で確かめる

### 検討事項（後で決める）

回線が悪い間に人が監視対象のシーンへ切り替えると、本体はすぐ待機シーンへ戻す。屋外の配信者は OBS の画面を見ていないので、切り替えたのに待機シーンになっていることに気づけない場合がある。配信者に知らせる方法の候補を挙げる。どれを入れるかは後で決める。

| 案 | 内容 | 備考 |
|---|---|---|
| A | 待機シーンの名前で IRL Link に見せる（待機シーンに、回線不良とわかる名前を付ける。IRL Link は現在のシーン名を表示する） | 設定だけで済み、本体の変更は要らない。**最初に採る。** README で案内する |
| B | OBS のテキストソースを書き換える | 視聴者にも見える。第3段階のオーバーレイと同じ仕組みを使える |
| C | 時間の上限つきの抑止など（人が切り替えた直後の一定時間は待機シーンへ戻さない、など） | A で足りなければ検討する |
| D | オーバーレイで知らせる | 第3段階で検討する |

### 実装前に決めたこと（v2.0 の書き直しで残った7点）

1. **配信中の `ScenesConfigured`:** 新しい設定に差し替えたうえで、次を確かめる。待機シーンの名前が変わった、または戻り先が新しい設定で使えなくなった（持ち主が `auto` で戻り先が Monitored でなくなった）ときは、持ち主を `none`、戻り先を空にする（今いるシーンはそのまま）。どちらでもなければ持ち主と戻り先を保つ。どの場合も、未完了のシーン切替のバッチは承認で却下する（設定の世代 `ConfigEpoch` を進め、シーンを切り替えるバッチの共通の条件に加える）。期待はそのまま残す（届いた `SceneChanged` を手動と誤認しないため）。そのあと通常の判定を行う
2. **StandbyOn の受け付け:** 現在のシーンがすでに待機シーンのとき（持ち主が `none` でも）は `ErrInvalidState`（理由: すでに待機シーン）。StartScene、AutoFail、AutoRecover のどれかが未完了の間も `ErrInvalidState`（理由: 切替の実行中）。StandbyOn の有効条件に「持ち主が `none`、現在のシーンが待機シーンでない」を入れ、承認の前に待機シーンへ切り替わっていたら、`Skip` にせず却下して `last_error` に記録する（通常の列では人の切替なので `SceneEpoch` による。AutoFail が先に届く列は作れない。6章 V-13）
3. **`OBSConnected` で配信状態が違っていたとき:** 配信の区切りとみなし、持ち主を `none`、戻り先を空にする。切断中に配信が始まっていた場合でも、配信開始時の切替は出さない（開始の瞬間はすでに過ぎていて、配信の途中で予期しない切替になるため）
4. **初回生成の既定値:** `obs.standby_scene: Standby`、`obs.monitored_scenes: [Camera]`。OBS に存在しなければ、接続時に status の `warnings` に出る。利用者は第2段階の管理ページで、OBS のシーン一覧から選び直す
5. **`auto_enabled` が false のときの StandbyOff:** 受信状態を見ない。戻り先が Monitored なら `[RestartMedia, SetScene(戻り先)]`、そうでなければ `[SetScene(戻り先)]`。自動切替を切った利用者は、乱れた映像が出ることを承知している
6. **StartScene の再送:** しない。失敗、却下、期限切れは `last_error` に記録する。ただし、手動の切替で `SceneEpoch` が進んだための却下は記録しない（人が自分でシーンを選んだ結果で、エラーではない）
7. **開始時のシーンと待機シーンが同じ設定:** 許さない。4章の検証に「`on_stream_start.scene` は `obs.standby_scene` と異なる」を加える

### 細部の規則（v2.1 の表を作る中で決めたこと）

表 `docs/superpowers/plans/2026-11-19-switcher-table.md` の 4章（V-01〜V-18）で挙がった不明点を、次のとおり決める（すべて表の案どおり）。

- **設定の世代と持ち主（V-01、V-07）:** 古い `ConfigEpoch` の期待が作用しないのは、持ち主と戻り先を「入れる」方向（AutoFail、StandbyOn の期待）だけ。「消す」方向（AutoRecover、StandbyOffMonitored、StandbyOffOther の期待）は、設定の世代によらず持ち主を `none` に、戻り先を空にする。入れる方向は、設定の変更の内容を問わず作用しない（第2段階で設定の編集を入れるときに見直す）
- **同じシーンへの期待（V-02、V-09）:** `SceneChanged` が期待と一致したら、その期待より前の期待に加え、同じシーン名の期待もすべて消す。期待のない、現在のシーンと同じ名前の `SceneChanged` は手動の切替として扱う。どちらも「OBS は現在のシーンへの切替要求でイベントを出さない」前提で、spike ①② のついでに実機で確かめる
- **配信開始時の切替（V-03、V-04、V-05、V-14）:** 前の配信の StartScene が未完了の間に次の配信が始まっても、新しい StartScene は出さない。配信開始の時点ですでに開始時のシーンにいても StartScene を出し、承認で `Skip` にする。StartScene を承認で却下するとき、バッチの `SceneEpoch` が今と違えば（ほかの条件が同時に崩れていても）「手動の切替による却下」として印を付け、その `BatchDone{Rejected}` は記録しない。StartScene の期待が、期限切れ以外（配信の停止、切断、手動の切替）で消えたときも記録しない
- **手動の切替と再送の時刻（V-06）:** 手動の切替を受けたら、各 Purpose の前回の送出時刻も消す（配信の停止と同じ）。人がシーンを変えたら状況は新しいので、AutoFail が5秒待たずに出る
- **遅れて届いた本体の切替（V-08）:** 手動の切替で期待が消えたあとに届いた本体の切替は、手動として扱う（13章の既知の制約に追記する）
- **設定（V-10、V-11）:** `ScenesConfigured` の「同じ内容」は、`Monitored` を集合として比べる（順序の違いは同じ内容）。最初の `ScenesConfigured` でも `ConfigEpoch` は進む。OBS 未接続の間の `ScenesConfigured` も、接続中と同じ規則でその場で持ち主と戻り先を確かめる
- **`ErrInvalidState` の理由（V-12）:** 確かめる順は、未接続 → 配信の条件 → シーンと持ち主の条件（すでに待機シーン、持ち主が `manual` でない）→ 同じ操作が実行待ち → 切替の実行中。最初に当たった理由を返す
- **通常の列では起きない条件（V-13）:** 有効条件の表のうち、共通の条件で先に却下されるため通常の列では起きない組み合わせも、安全側の備えとして実装に残す。テストは、状態を直接作って `Approve` を呼ぶ形で書く
- **再接続と再送待ち（V-15）:** `OBSConnected` で配信の区切りとみなしたときは、前回の送出時刻を消さない
- **StandbyOff の押し直し（V-16）:** 前の切替が届いたあとに `RestartMedia` の承認になったときは、却下して記録する
- **待機シーンの名前の変更と AutoFail（V-17）:** 再送の歯止めの「そのシーンへの期待」は、切替先のシーン名で比べる（Purpose は問わない）
- **手動の待機の期限切れ（V-18）:** Command 由来の Purpose（StandbyOn、StandbyOffMonitored、StandbyOffOther）の期待の期限切れも `last_error` に記録する。AutoFail と AutoRecover の期待の期限切れは記録しない（`Tick` で再送する）

## 7. OBS 接続（obs）

```go
type Client interface {
    Events() <-chan switcher.Event          // OBSConnected / OBSDisconnected / SceneChanged / TransitionChanged / StreamStateChanged
    SceneTransitionOverride(scene string) (ms *int, has bool, err error) // GetSceneSceneTransitionOverride
    SetScene(name string) error             // SetCurrentProgramScene。タイムアウトは ErrTimeout で区別して返す
    RestartMedia(input string) error
    StartStream() error
    StopStream() error
}
```

- **配信状態の写像:** obs-websocket の `StreamStateChanged` は、`outputState` で次のように扱う。`STARTED` で `Streaming: true`、`STOPPED` で `Streaming: false` を流す。`STARTING`、`STOPPING`、`RECONNECTING`、`RECONNECTED` では何も流さない（OBS から配信サイトへの出力の再接続を、配信の停止として扱わないため。`outputActive` は再接続中に false になる）。`OBSConnected` の `Streaming` は `GetStreamStatus` の `outputActive || outputReconnecting` とする
- **トランジションの上書き:** 実行役は `SetScene` の `Approve` の直前に、切替先シーンの `SceneTransitionOverride` を読み、`Approve` に渡す（期待の期限に使う。6章）。読み取りに失敗したら `Override` を `Unknown`（上書きあり・長さ不明）にする。上書きの変更を知らせるイベントはないので、毎回読む

- `Run(ctx)` で接続を保つ。切れたら待ち時間を 1→2→4…秒と延ばし、上限5秒で再接続を続ける
- 接続したら、配信状態、program の現在のシーン、現在のトランジションとその長さを取得して `OBSConnected` を流す。あわせてシーン一覧と入力一覧を取得し、config のシーン名と `media_source` が存在しなければ警告を status に出す
- シーンの取得・設定・監視は program だけを対象にする（`GetCurrentProgramScene`、`SetCurrentProgramScene`、`CurrentProgramSceneChanged`）。Studio Mode の preview は扱わない
- トランジションの変更（`CurrentSceneTransitionChanged`、`CurrentSceneTransitionDurationChanged`）を `TransitionChanged` として流す
- goobs の要求は context を受け取らず、応答待ちの既定は10秒。ラッパーは `WithResponseTimeoutDuration` で応答待ちを5秒にする。タイムアウトは失敗として返す
- **接続の失敗で残さない:** goobs v1.11.1 の `New` は、認証の失敗やタイムアウトで失敗したとき、開いた接続を閉じずにエラーを返す（Client も返さないので `Disconnect` を呼べない）。そのままでは再接続のたびに接続と goroutine が残る。ラッパーは `WithDialer` で自前の dialer を渡し、開いた接続を手元に持つ。`New` が失敗したら、その接続を自分で閉じる。`ctx` が終わったら、接続中（dial と認証の待ち）でも接続を閉じ、`Run` を早く抜ける（認証の待ちは goobs の中にあるので、最長で応答待ちの5秒は残る）
- バッチ内の失敗の扱いは 6章「バッチ内の失敗」のとおり（実行役が行う）
- 中核ループは `Client` インターフェースだけに依存し、テストでは偽物に差し替える

## 8. MediaMTX の管理（supervisor）

### 生成する mediamtx.yml

起動のたびに config から生成して上書きする（先頭に「自動生成。編集しないこと」と書く）。v1.21.4 で動くことを確かめた見本は `internal/supervisor/testdata/reference-v1.21.4.yml`（spike ③）。要点:

- 既定値に頼らず、v1.21.4 にあるすべてのプロトコルと機能のスイッチを明示して書く。有効にするのは RTMP（`:rtmp_port`、`rtmpEncryption: "no"`）、SRT（`:srt_port`）、RTSP（`:rtsp_port`、`rtspTransports: [tcp]`、`rtspEncryption: "no"`）、API（`127.0.0.1:api_port`）だけ。HLS、WebRTC、MoQ、playback、metrics、pprof は `false` にする。RTSP の UDP とマルチキャスト（既定では 8000/8001 UDP などを開く）と MoQ（既定で有効、8892/8893）を止めるのは、事前確認していないポートで起動に失敗するのと、意図しない待ち受けを開くのを防ぐため
- path は `stream.name` の1本だけ。それ以外の path は受け付けない
- `authInternalUsers`（既定の匿名ユーザーは置き換えて残さない）:
  - publish: `stream.publish_user` / `stream.publish_pass`（RTMP と SRT で共通）、IP 制限なし
  - read: `stream.read_user` / `stream.read_pass`、IP 制限なし。パスワードなしの読み出しは作らない。トンネルやポートプロキシ、送信元を書き換える転送を使うと、インターネットからの接続も 127.0.0.1 や LAN のアドレスに見えるため、IP だけで許可すると生映像を誰でも読めてしまう
  - api: 本体の起動のたびにランダムに作るユーザー名とパスワード（MediaMTX の再起動では変えない。変えると、作成時に資格情報を受け取った Poller が認証に失敗し続けるため）、`127.0.0.1` と `::1` からだけ許可。Poller はこの資格情報で API を呼ぶ
- MediaMTX の API の応答は、ログにそのまま書かない。`/v3/rtmp/conns/list` は RTMP の publisher のクエリ（`user=…&pass=…`）を平文で返す（spike ③ で確認）

送信用 URL（status と、起動時の標準出力に表示する。10章のログの規則を参照）。`<host>` は `stream.publish_host` が空でなければその値（手動）、空なら起動時に1回調べた LAN の IPv4（自動。ネットワークインターフェースのうち、有効で loopback でないものの、プライベートアドレス〔`net.IP.IsPrivate`〕の IPv4 を、インターフェースの順で最初の1つ。外部への通信はしない）。自動で見つからなければ `127.0.0.1` にして警告をログに出す。自動のときは、起動時の表示に「屋外から送るときは、host をポート転送やトンネルのアドレスに置き換える（`stream.publish_host` で固定できる）」と添える:
- SRT: `srt://<host>:<srt_port>?streamid=publish:<name>:<user>:<pass>`
- RTMP: `rtmp://<host>:<rtmp_port>/<name>?user=<user>&pass=<pass>`

OBS メディアソース用 URL は RTSP（`rtsp://<read_user>:<read_pass>@127.0.0.1:<rtsp_port>/<name>`。OBS は同じ PC で動くので host は `127.0.0.1`）とする（spike ① で決めた。Windows の OBS で遅延は約1秒）。OBS のメディアソースは、「ローカルファイル」を外し、FFmpeg のオプションに `rtsp_transport=tcp`、「再接続までの遅延」は2秒、「アクティブになったときに再生を再開」は解除を推奨する（12章の spike ②）。どれを選んでも read の資格情報を埋め込む（候補: `rtsp://<read_user>:<read_pass>@<host>:<rtsp_port>/<name>`、`rtmp://<host>:<rtmp_port>/<name>?user=<read_user>&pass=<read_pass>`、`srt://<host>:<srt_port>?streamid=read:<name>:<read_user>:<read_pass>`）。この URL も status と起動時の標準出力にだけ表示する。

### 起動と再起動

- 起動前に、`rtmp_port`（TCP）、`srt_port`（UDP）、`rtsp_port`（TCP）、`api_port`（TCP）を、MediaMTX と同じ形（全インターフェース、api だけ 127.0.0.1）で listen できるか確かめ、すぐ閉じる。全インターフェースのものは、`:port`（Windows では IPv6 と IPv4 の両方を受ける形で開ける）に加えて、IPv4 の `0.0.0.0:port` でも確かめる（Windows では、ほかのプロセスが IPv4 の `0.0.0.0:port` を使っていても `:port` は開けてしまい、使用中に気づけないため。Windows の実機で確認）。これは補助の診断で、ポートの予約ではない（閉じてから MediaMTX が bind するまでに、別のプロセスが取ることはありうる）。使用中なら、どのポートかと「前回の mediamtx が残っていないか確認すること」を示して起動を止める
- MediaMTX が起動直後に落ちたときは、そのログに出た bind などのエラー行を `last_error` に出す。事前確認を通ったあとの起動失敗も、これで理由が見える
- MediaMTX の標準出力と標準エラーは、`component=mediamtx` を付けて本体のログに流す。その際、`publish_pass`、`read_pass`、api のパスワードの文字列を `***` に置き換える（置き換えの規則は10章「ログ」の細部と同じで、`internal/redact` を使う。MediaMTX のログに RTMP のクエリや SRT の streamid が出る場合に備える。4章で使える文字を制限しているので、URL エンコードで形が変わることはない）
- 落ちたら再起動する。待ち時間は 1→2→4…秒と延ばし、上限30秒。60秒安定して動けば待ち時間を1秒に戻す
- 落ちている間、Poller の API 呼び出しは失敗するので受信状態は `DOWN` になる。status では `mediamtx: "down"` として区別する
- 本体の終了時（Ctrl+C、SIGTERM）は MediaMTX を止めてから終了する
- 本体が強制終了されても子プロセスを残さない:
  - **Windows:** 本体の起動時に Job Object（`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`、breakaway は許可しない）を作り、本体自身をその Job に入れる。子は生成された時点で Job を継承するので、`Start` から Job への割り当てまでの間に本体が落ちて子が残る、という窓がない。Job のハンドルは子に継承させず、プロセスの終了まで閉じない（本体自身が Job に入っているので、閉じると本体も終了する）。本体がすでに入れ子にできない Job の中にいて（UI 制限付きの Job など）割り当てに失敗したときは、警告をログに出し、子を `Start` した直後に Job へ割り当てる方式に切り替えて起動を続ける（この場合は割り当てまでの短い窓が残る）。本体が起動する子はすべてこの Job に入る（第2段階以降で子プロセスを足すときは、本体と一緒に終了してよいものに限る）
  - **Linux:** `SysProcAttr.Pdeathsig = SIGKILL`。Go の Pdeathsig は、子を fork した OS スレッドが終わった時点で発火する（プロセスの終了より前のことがある。go.dev/issue/27505）。そのため、子の起動から `Wait` が返るまでを、`runtime.LockOSThread()` した専用の goroutine 1本で行い、子が生きている間はその goroutine を終わらせない
  - **macOS:** 相当する仕組みがないため、正常終了時の停止だけとする（既知の制約）

## 9. 中核ループ（core）と API（web）

### 中核ループ

goroutine 1本が switcher の状態を持ち、次を `select` で受ける: ingest の `Sample`、obs の `Events()`、web からの Command、1秒ごとの `Tick`。

- `Sample` は Judge に通し、受信状態が変わったら `LinkChanged` を switcher に渡す
- シーンの設定は、ループを始める前に config から `ScenesConfigured` を作って `Machine.Handle` に1回渡す（チャネルは通さない）。第2段階で管理ページから送る経路は、第2段階の spec で決める
- switcher が返したバッチは obs で実行する。中核ループを止めないよう、実行は専用の実行役 goroutine 1本に渡す。実行役はバッチ内の Action を返された順に1つずつ実行し、バッチを1つずつ処理するので、バッチ同士は重ならない（`[RestartMedia, SetScene(戻り先)]` の順序が保証される）
- **実行直前の承認:** 実行役は各 Action を送る直前に、承認要求を中核ループに送って応答を待つ。中核ループは `Machine.Approve` を呼び、`Proceed` / `Skip` / `Reject` / `Replace` を返す。`Reject` と `Replace` なら、実行役はそのバッチの残りを捨てる。`Replace` の代わりのバッチは、中核ループが自分の列の先頭に入れる（実行役には渡さない。実行役は次のバッチとして受け取る。代わりのバッチが nil なら何も入れない）。実行役は、1つのバッチの `BatchDone` を送り終えるまで次のバッチを受け取らない。1つのバッチの中では、最後の `SceneRequestDone` を送ってから `BatchDone` を送る。`SetScene` の期待は、この承認の時点で登録される（6章）。`RestartMedia.Wait` の待ちは RESTART の送出後に実行役が行い、続く `SetScene` の承認はその待ちのあとに取る
- `SetScene` を送ったら、応答（成功、タイムアウト、エラー）を `SceneRequestDone{ReqID, Result}` として中核ループに戻す。バッチが終わったら（最後まで実行した、失敗で残りを捨てた、承認で却下された）、`BatchDone{BatchID, Result, Err}` を1回だけ戻す

**中核ループと実行役のチャネルの決まり**（デッドロックを防ぐため）:
- 中核ループは、どのチャネルへの送信でも止まらない。実行待ちのバッチは中核ループが列で持ち、`select` の送信 case として先頭の1つだけを実行役に差し出す（列が空のときはその case を `nil` チャネルにして無効にする）。実行役が承認待ちや応答待ちで詰まっていても、中核ループは他の入力を受け続ける
- 中核ループの `select` が受けるのは、`Sample`、obs の `Events()`、web の Command、`Tick`、実行役からの承認要求、`SceneRequestDone`、`BatchDone`、終了の `ctx.Done()`
- 承認要求と Command は、容量1の応答用チャネルを持つ。中核ループはそこへ1回だけ送り、止まらない
- 実行役から中核ループへの送信と、応答の受信と、`RestartMedia.Wait` の待ちは、必ず `ctx.Done()` と一緒に `select` する。終了時はどちらの側も待ちを解いて抜ける
- Command は応答用のチャネルを持ち、switcher の `cmdErr` を web に返す（Action の実行結果は待たない）
- 状態が変わるたびに `Snapshot` を作り、`atomic.Pointer` で公開する。web はこれを読むだけ

### API（第1段階）

すべて Basic 認証（ユーザー名 `admin`、パスワード `admin.password`）。POST は、さらに独自ヘッダー `X-IRL-Live-Keeper: 1` を必須とし、ない場合は 403 を返す。ブラウザが Basic 認証を覚えたあと、別のサイトのページからフォームで POST されて配信が止まる（CSRF）のを防ぐため。フォームの POST は独自ヘッダーを付けられない。

- `GET /api/status` → 200

```json
{
  "link":    {"state": "OK", "kbps": 4200.5, "online": true, "protocol": "srt",
              "srt": {"rtt_ms": 85.2, "loss_rate": 0.01, "recv_mbps": 4.2}},
  "obs":     {"connected": true, "streaming": true, "scene": "Camera", "scene_class": "monitored",
              "standby": {"owner": "none", "return_scene": null},
              "auto_enabled": true,
              "warnings": []},
  "mediamtx": "running",
  "urls":    {"publish_srt": "srt://…", "publish_rtmp": "rtmp://…", "obs_read": "rtsp://obs:…@…"},
  "last_error": null
}
```

`last_error` は、なければ null、あれば `{"at": "<RFC3339 の時刻>", "source": "switcher" | "mediamtx", "message": "<理由>"}`。switcher と supervisor の最新の1件のうち、新しいほうを出す（supervisor の記録には時刻がないので、core が変化を見た時刻を付ける）。OBS に一度も接続していない間の `scene_class` は `unknown`。

  - `scene_class`: 現在のシーンの分類。`monitored` / `standby` / `other` / `unknown`（OBS に未接続などで現在のシーンがわからない）
  - `standby.owner`: 待機の持ち主。`none` / `auto` / `manual`。`standby.return_scene`: 戻り先のシーン名。なければ `null`
  - `auto_enabled`: 自動切替が有効か

- `POST /api/stream/start`、`/api/stream/stop`、`/api/standby/on`、`/api/standby/off`
  - 200: 受け付けた（Action の完了は待たない）
  - 401: 認証失敗
  - 403: 独自ヘッダーがない
  - 409: `ErrInvalidState`（理由を本文に入れる）
  - 503: `ErrNotConnected`（OBS に未接続）。中核ループが止まっている、または5秒以内に Command を受け取らなかったときも 503

**API の細部（v2.4 で決めたこと）:**
- 確かめる順序は、パス（未知なら 404）→ 認証 → メソッド → 独自ヘッダー。認証に失敗したら、ヘッダーの有無によらず 401 とし、`WWW-Authenticate: Basic realm="irl-live-keeper"` を付ける。CSRF で問題になるのは、認証は通るがヘッダーのない要求なので、それが 403 になる。401 と 403 では Command を渡さない
- `GET /api/status` も認証が要る（`urls` に資格情報を含む URL があるため）。401 の本文に状態は出さない
- 独自ヘッダーは値がちょうど `1` のときだけ通す。ない、空、ほかの値は 403
- メソッドが違うときは 405 とし、`Allow` を付ける（status は GET、操作は POST）
- パスワードの比較は、両方を SHA-256 にしてから `subtle.ConstantTimeCompare` で行う（長さの違いで比較の時間が変わらないように）
- 本文はすべて JSON（`Content-Type: application/json; charset=utf-8`）。status 以外は、200 が `{"ok": true}`、エラーが `{"error": "<理由>"}`
- 想定外のエラーは 500
- web は要求、`Authorization`、status の中身をログに書かない（10章）。待ち受けのアドレスと `http.Server` の設定は main で行う

## 10. エラー処理とログ

方針: **わからないときは待機側に倒す。ただし誤って何度も切り替えないよう歯止めをかける。**

| 事象 | 対応 |
|---|---|
| MediaMTX が落ちた | 8章のとおり再起動。受信状態は DOWN になり、配信中で監視対象のシーンなら待機シーンへ |
| 起動時にポートが使用中 | どのポートかと、前回の mediamtx の残りを疑うよう示して起動を止める |
| MediaMTX が起動直後に落ち続ける | ログの bind などのエラー行を `last_error` に出し、8章の待ち時間で再起動を続ける |
| API の失敗・タイムアウト | その回は受信0。DOWN は `down_sec` 続いたときだけなので、1回では切り替わらない。差分の基準は変えない |
| 再接続（publisher が変わった、`inboundBytes` が減った） | その回は0として扱い、差分の基準を取り直す |
| 実行待ちのバッチが古くなった | 実行直前の承認で、目的の有効条件をその時点の状態で確かめ、満たさなければ却下して残りを捨てる。Command 由来なら `last_error` に記録する |
| OBS から配信サイトへの出力が再接続した | 配信の停止として扱わない（7章の写像） |
| OBS と切断した | 自動切替を止め、手動操作は 503。再接続後に状態を取り直して再開 |
| OBS の操作が失敗した | ログと `last_error` に記録。自動切替は、条件が続く間、最短5秒あけて再送 |
| シーン名やメディアソース名が OBS にない | 接続時に照合し、status の `warnings` に出す |
| config.yaml がない | 既定値で生成し（`*` はランダム）、送信用 URL と管理 API のパスワードを表示して起動を続ける。`obs.password` が空のままでは、OBS 側で認証が有効なら接続に失敗し、status に認証失敗として出る |
| config.yaml が不正 | 項目名と理由を表示して起動を止める |

### 起動と終了の手順（main）

1. 実行ファイルと同じディレクトリの `config.yaml` を読む。なければ生成する。不正なら項目名と理由を標準エラーに出して終了する（終了コード 1）。`log.dir` と `mediamtx.binary` が相対パスなら、実行ファイルのディレクトリを基準にする
2. ログを開く（下の「ログ」）。`config.Warnings()` の警告をログに出す
3. Windows では `supervisor.InitJob()` を1回呼ぶ（失敗は警告にして続ける。8章）
4. ポートの事前確認（8章）。使用中なら理由を出して終了する（終了コード 1）
5. 起動時の表示を標準出力に1回だけ書く（slog を通さない）: 送信用 URL（SRT、RTMP）、OBS 用 URL、管理 API のアドレス、config.yaml を生成したときは管理 API のパスワード
6. supervisor、Poller（1秒ごと）、obs、core、管理 API の HTTP サーバーを動かす。HTTP サーバーには `ReadHeaderTimeout` などの時間の上限を付ける
7. Ctrl+C（SIGINT）か SIGTERM で終了を始める。HTTP サーバーを止め、ctx を取り消し、MediaMTX が止まるのを待ってから終了する（終了コード 0）。管理 API の待ち受けに失敗したときも、MediaMTX を止めてから終了する（終了コード 1）

### ログ

- `log/slog` のテキスト形式で、標準出力と `<log.dir>/irl-live-keeper-YYYY-MM-DD.log` の両方に書く。日付が変わったら新しいファイルに切り替える
- 起動時に `keep_days` を過ぎたログファイルを消す。依存は増やさない
- 自動切替のたびに理由を残す。例: `msg="scene switch" purpose=auto_fail from="Camera" to="Standby" reason=low kbps=620 for=5s`
- パスワードと、パスワードを含む URL（送信用、OBS 用）はログファイルに書かない。起動時に標準出力にだけ1回表示する（config.yaml を生成したときは、生成した管理 API のパスワードもあわせて表示する）
- 細部（v2.4 で決めたこと）: 日付はローカル時刻で決め、書き込みのたびに日付を見て、変わっていたら新しいファイルを開く。消すのは、名前が `irl-live-keeper-YYYY-MM-DD.log` の形で、その日付が「今日 − `keep_days` 日」より前のファイルだけ（`keep_days: 14` で今日が 11-20 なら 11-05 以前を消し、11-06 は残す）。ほかのファイルには触らない。ログの行の中のパスワード（`admin.password`、`publish_pass`、`read_pass`、`obs.password` のうち空でないもの）は、標準出力にもファイルにも `***` に置き換えて書く（パスワードを含む URL も、パスワードの部分が `***` になる）。置き換えは、各パスワードの出現位置（slog が引用してエスケープした形も含む）を集め、重なる区間や隣り合う区間を1つに併合してから、区間ごとに `***` にする（2つのパスワードが途中で重なっても、どちらの文字も残さない）。起動時の表示は slog を通さず、標準出力に直接書く

## 11. テスト方針

### 単体テスト（`go test -race ./...`）

- **switcher:** 整合は状態機械の中心（`Handle`）だけで守り、入口ごとに確認を足さない。テストは「入口 × 状態」の表1つにまとめる。表は `docs/superpowers/plans/2026-11-19-switcher-table.md` に作ってある。入口は各 Event、各 Command、`Approve`（Purpose ごと）。状態の軸は、現在のシーンの分類（Monitored / 待機シーン / Other）、受信状態（OK / LOW / DOWN）、配信中か、OBS 接続、`auto_enabled`、待機の持ち主（none / auto / manual）、戻り先の分類（Monitored / それ以外）、期待の有無と Purpose、未完了のバッチの有無と Purpose、配信開始時の切替の有効・無効、設定の世代（バッチを作ったあとに `ConfigEpoch` が進んだか）。全軸の掛け算にはしない: ありえない組み合わせ（OBS 未接続で期待あり、持ち主が none 以外で現在のシーンが待機シーンでない、配信中でないのに持ち主がある、など）は除き、入口ごとに結果に関係する軸だけを展開する（例: Command は配信中 × 接続 × 持ち主 × 戻り先の分類 × 受信状態 × 未完了のバッチ、`LinkChanged` は分類 × 配信中 × 接続 × `auto_enabled` × 持ち主 × 未完了のバッチ、`Approve` は 6章「目的と有効条件」の表の各行の条件の成否。ただし、通常の列では共通の条件で先に却下されるため起きない組み合わせがあり、その行のテストは状態を直接作って `Approve` を呼ぶ。6章 V-13）。表の各行で、出るバッチ、`cmdErr` の型、`Approve` の結果、持ち主と戻り先を確かめる。時間に依存する状態（期待の期限、再送の5秒）は表に入れず、下の Event の列のテストで確かめる。時刻は Event の `At` で注入する。6章末「実装前に決めたこと」の7点も表とテストに入れる。表とは別に、Event の列で確かめる必須のケース:
  - AutoFail は、配信中、`auto_enabled`、現在のシーンが Monitored、持ち主が none のときだけ出る。Other、待機シーン、配信停止中、OBS 未接続では何も出さない
  - 戻り先の確定: AutoFail のバッチを出しただけでは持ち主も戻り先も入らず、待機シーンへの期待どおりの `SceneChanged` が届いた時点で持ち主が auto、戻り先が切替前のシーン名になる。バッチの却下、失敗、期待の期限切れでは何も入らない
  - AutoRecover で `RestartMedia` が `SetScene(戻り先)` より先に出る。戻り先への期待どおりの `SceneChanged` で持ち主と戻り先が消える。戻すバッチが失敗したら持ち主は auto のままで、5秒あけて再送される
  - 手動で Monitored へ: 自動の待機中に、受信状態が不良のまま人が別の Monitored のシーンへ切り替えると、持ち主と戻り先が消えたうえで AutoFail が出て、そのシーンが新しい戻り先になる。Other のシーン（トークなど）から不良のまま Monitored へ入った場合も同じ。受信状態が OK なら何も出ない
  - 手動で Other / 待機シーンへ: 自動の待機中に人が Other へ切り替えると、持ち主と戻り先が消え、回復しても何も出ない。人が手動で待機シーンを選んだ場合は持ち主が none のままで、回復しても戻さない
  - StandbyOn: 期待どおりの `SceneChanged` で持ち主が manual、戻り先が切替前のシーン（Monitored でも Other でも）になる。持ち主が manual の間は、受信状態が変わっても何も出ない
  - StandbyOff: 戻り先が Monitored で OK なら `[RestartMedia, SetScene(戻り先)]`。戻り先が Monitored で不良ならバッチを出さず持ち主が auto になり、回復したら AutoRecover が出る。戻り先が Monitored でなければ、受信状態によらず `[SetScene(戻り先)]`
  - 置き換え: StandbyOffMonitored の待ち時間中に回線が悪化すると、`Approve` が `Replace`（代わりのバッチなし）を返し、持ち主が auto になり、`last_error` は記録されない
  - 配信開始時の切替: 有効なら `StreamStateChanged{true}` で、受信状態が不良でも StartScene が出る（AutoFail より先）。StartScene が未完了か期待が残っている間は、受信状態が変わっても Auto のバッチが出ず、Auto の承認も却下される。開始時のシーンが Monitored で不良なら、届いたあとに AutoFail が出る。無効なら StartScene は出ず、通常の判定だけをする。切替の前に人がシーンを切り替えたら、StartScene は却下される
  - 切替中の抑止: StandbyOn を受け付けた直後に回線が悪化しても、AutoFail が出ない
  - `auto_enabled` を false にする `ScenesConfigured`: 以後 Auto のバッチが出ない。実行待ちの Auto のバッチは承認で却下される。持ち主が auto なら none になり戻り先が消える（シーンはそのまま）。StandbyOn は受け付ける。true に戻すと判定が再開する
  - `auto_enabled` が false のときの StandbyOff（6章末「実装前に決めたこと」の 5）: 受信状態が不良でも、戻り先が Monitored なら `[RestartMedia, SetScene(戻り先)]`、そうでなければ `[SetScene(戻り先)]` が出る。持ち主は auto にならず、承認の前に受信状態が変わっても `Replace` にも却下にもならない
  - 配信の停止: `StreamStateChanged{false}` で持ち主、戻り先、期待、再送待ちが消える
  - 配信中の `ScenesConfigured` による設定の差し替え: 監視対象の列を差し替えると、以後の分類と判定が新しい設定で行われる（例: 持ち主が none で、未完了のバッチも期待もないときに、Other だった現在のシーンを監視対象に足すと、不良なら AutoFail が出る。監視対象から外すと、不良でも出なくなる）。
  - 配信中の `ScenesConfigured` と持ち主（6章末「実装前に決めたこと」の 1）: 待機シーンの名前が変わると、持ち主（auto でも manual でも）と戻り先が消え、今いるシーンはそのまま。持ち主が auto で戻り先が監視対象から外れると消える。持ち主が manual で戻り先が監視対象から外れても保たれ、StandbyOff は `[SetScene(戻り先)]` になる。どちらでもない変更（別の監視対象の追加など）では保たれる。どの場合も `ConfigEpoch` が進み、未完了のシーン切替のバッチ（AutoFail、AutoRecover、StandbyOn、StandbyOff、StartScene）は承認で却下され、StopStream と StartStream は却下されない。期待は残り、そのあと届いた期待どおりの `SceneChanged` は手動とみなされない。今と同じ内容の `ScenesConfigured` では `ConfigEpoch` が進まない
  - StreamStart / StreamStop: StartStream と StopStream のバッチだけが出て、`SetScene` は出ない
  - 再送が5秒以上あく
  - 各 Command の 409 になる条件（同じ操作の未完了のバッチを含む。持ち主が manual でないときの StandbyOff）
  - StandbyOn の受け付け（6章末「実装前に決めたこと」の 2）: 現在のシーンが待機シーンなら、持ち主が auto でも none（人が手動で選んだ）でも `ErrInvalidState`。StartScene、AutoFail、AutoRecover のどれかが未完了の間も `ErrInvalidState`。受け付けたあと、承認の前に待機シーンへ切り替わっていたら、`Skip` ではなく却下になり、`last_error` に記録される
  - 期待の設定の世代: AutoFail を送ったあと、`SceneChanged` が届く前に `ScenesConfigured` で待機シーンの名前を変えると、届いた旧待機シーンの `SceneChanged` は本体の切替とみなされるが、持ち主は `none`、戻り先は空のまま。`auto_enabled` を false にした場合と、戻り先を監視対象から外した場合も同じ
  - AutoFail の期待だけが残っている間（要求がタイムアウトして `BatchDone` を受けたあと）の StandbyOn は `ErrInvalidState`
  - AutoRecover の最後の承認のあと、`SceneChanged` が届く前に回線が悪くなった場合: 戻り先へ届いた時点で持ち主と戻り先が消え、通常の判定で AutoFail が出て、そのシーンが新しい戻り先になる
  - 同じシーンへの期待が複数ある（StandbyOff の押し直しで、同じ戻り先への期待が2つある。1回目の要求がタイムアウトしたあとに押し直すと作れる）とき、期待どおりの `SceneChanged` が1つだけ届くと、持ち主と戻り先は最初に一致した期待（古いほう）に従って `none`、なしになり、その期待より前の期待と、同じシーン名の期待（後ろのもの）はすべて消える（6章「細部の規則」V-02）。そのあとの StandbyOn は、期待が残っていないので受け付けられる
  - 配信を止めて始め直したあとに、前の配信の遅れた `SceneChanged` が届いた場合: 期待は配信の停止で消えているので手動の切替とみなされ、持ち主は `none`。そのシーンが監視対象で回線が悪ければ AutoFail が出る
  - StartScene の再送（6章末「実装前に決めたこと」の 6）: StartScene のバッチが失敗、却下、期限切れになっても、`Tick` を何回送っても StartScene は再び出ない。3つとも `last_error` に記録される。ただし、人が先にシーンを切り替えたための却下は記録されない
  - 期待: 承認の時点で登録され、応答を待つ間は期限切れにならない。期限は応答の時点 + 実際に使われるトランジションの長さ（上書きを含む）+ 3秒、上書きありか長さ不明なら10秒以上。タイムアウトの応答でも期限まで残り、エラーの応答ではすぐ消える。応答が5秒でタイムアウトしたあとに届いた本体の `SceneChanged` を手動とみなさない。一致した期待より前の期待も消える（前の期待は持ち主と戻り先に作用しない）。一致しない `SceneChanged` で全部消える。期限切れは手動として扱わない
  - 有効条件: 復帰のバッチ `[RestartMedia, SetScene(戻り先)]` の待ち時間中に、受信状態の悪化、手動のシーン切替、配信の停止、OBS の切断のどれかが起きたら、`SetScene(戻り先)` の `Approve` が却下される。一方、StopStream と StandbyOn のバッチは、承認の前に受信状態が何度変わっても却下されない
  - 承認の `Skip`: 現在のシーンがすでに切替先のシーンなら送らない（比較はシーン名）
  - 重複の歯止め: Monitored のシーンで LOW → DOWN と続いても、AutoFail のバッチは1本だけ出る
  - 上書きの状態: `Override` が `None` / `Known` / `Unknown` のそれぞれで期限が 6章の式どおりになる
  - 配信の区切り: StopStream のバッチが実行待ちの間に配信が止まって始め直されたら（`StreamEpoch` が進む）、その StopStream は却下される
  - 配信状態: `RECONNECTING` → `RECONNECTED` の列では、持ち主、戻り先、未完了のバッチがどれも変わらない
  - 再接続: `OBSConnected` でシーンも配信状態も切断の直前と同じなら持ち主と戻り先が保たれ、シーンが違っていたら消える（`SceneEpoch` は進まない）。シーンが同じで配信状態だけが違っていた場合も消える（6章末「実装前に決めたこと」の 3）。切断前は停止で再接続したら配信中だった場合、配信開始時の切替が有効でも StartScene は出ない
  - 配信していない間の手動のシーン切替では、何も出ず、持ち主も戻り先も入らない
  - バッチ内の失敗: `RestartMedia` の失敗では続け、それ以外の失敗では残りを捨てる
- **ingest.Judge:** 時刻は `Sample.At` と `Interval` で注入する。5章「実時刻の例」の表の全行を、そのままテストにする。あわせて、閾値ちょうどの値（`fail_kbps`、`recover_kbps` と等しい kbps）を確かめる
- **ingest.Poller:** `httptest` のサーバーで、spike ③ で録った MediaMTX v1.21.4 の JSON を返す。5章の Poller の例（P1〜P4）の全行、タイムアウト、SRT の統計の取り出しと、その読み取りだけの失敗。JSON の形を手書きで再現しない。録った実物と違う形への fallback（別のフィールド名も読む、など）も足さない。偽の形でテストが通り続ける穴になるため
- **config:** 既定値の生成、読み込み、4章の検証すべて
- **supervisor:** mediamtx.yml の生成をゴールデンファイルと比べる（全プロトコルのスイッチが明示されていること、RTSP が TCP だけであること、MoQ が無効であること、read に資格情報が要ることを含む）。再起動の待ち時間は、偽の子プロセス（Go のヘルパープロセスの手法）で確かめる。ポートの事前確認
- **core:** obs.Client の偽物を使い、Sample → 切替 → 承認 → Action の実行 → `SceneRequestDone` と `BatchDone` までを通しで確かめる。偽物の応答を遅らせ、承認の却下でバッチが捨てられること、実行役が応答待ちで詰まっている間も中核ループが Sample と Command を受け続ける（デッドロックしない）ことを確かめる。偽物は、途中のシーン変更のイベントを省略する列（打ち切られたトランジション）も流せるようにする
- **web:** `httptest` で認証、独自ヘッダーがない POST の 403、`cmdErr` の型と 503 / 409 の対応、status の JSON の形

goobs のラッパーは実際の OBS を相手に手動で確かめる。自動テストにはしない。

### 手動の結合テスト（チェックリスト）

1. ffmpeg で RTMP と SRT のそれぞれへ送り、status の kbps と protocol を確かめる
2. 監視対象のシーンで送信を止めて、約2秒で待機シーンに切り替わる。status の `standby` が `auto` と切替前のシーン名になる
3. 低いビットレート（例: 500kbps）で送って、約5秒で待機シーンに切り替わる
4. 送信を戻して、約10秒で戻り先のシーンに戻り、映像が出る
5. MediaMTX を kill して、再起動される（配信中で監視対象のシーンなら、待機シーンを経て戻り先に戻る）
6. OBS を再起動して、再接続し、判定が再開する
7. 手動の待機 ON / OFF。回線が悪いときの OFF（待機シーンのまま持ち主が auto になり、回復したら戻る）と、監視対象でないシーンから ON にしたときの OFF（受信状態によらず戻る）
8. IRL Link（または OBS の画面）から、回線が悪いときにカメラのシーン（監視対象）へ切り替えると、待機シーンへ戻される。別のカメラのシーンを選んだ場合、回復すると IRL Link で最後に選んだカメラのシーンへ戻る
9. IRL Link から監視対象でないシーン（トークなど）へ切り替えると、回線の状態が変わっても何も起きない。自動の待機中に切り替えた場合は、回復しても戻されない。回線が悪いまま、そこからカメラのシーンへ切り替えると待機シーンへ切り替わる
10. 回線が良いときに IRL Link から待機シーンを選ぶと、そのまま残る（自動では戻らない）
11. 長いトランジション（例: 5秒のフェード、スティンガー）と、シーンごとのトランジションの上書きで自動切替しても、本体の切替が手動の切替として扱われないこと（待機の持ち主が auto になり、回復で戻る）
12. 配信中に OBS から配信サイトへの出力を切って再接続させても（上り回線を一時的に切る）、待機の持ち主と戻り先が保たれること
13. `on_stream_start` を有効にして、別のシーンのまま配信を始めると、開始時のシーンへ切り替わる（OBS の画面から始めた場合と、API から始めた場合）。回線が悪いときも開始時のシーンへ切り替わり、それが監視対象なら続けて待機シーンへ切り替わる。無効なら StartScene は出ないが、通常の自動切替は行う（監視対象のシーンのまま回線が悪い状態で始めると、待機シーンへ切り替わる）。切替に失敗したとき（開始時のシーンを OBS から消しておく）は、再送されず status の `last_error` に出る
14. `switch.auto_enabled` を false にすると、回線が悪くても切り替わらない。手動の待機 ON / OFF は使える。回線が悪いままカメラのシーンから ON にして OFF にすると、待機シーンに残らず、すぐカメラのシーンへ戻る（受信状態を見ない）
15. 本体を強制終了して、MediaMTX が残らない（Windows と Linux）
16. OBS 用 URL から read の資格情報を外すと読み出せないこと

## 12. 最初の spike（実装の前に行う。コードは捨てる前提）

① と ② と ④ には Windows の OBS 実機が必要で、ユーザーに操作してもらいながら進める。

1. **OBS からの読み出し:** rtsp / rtmp / srt のうち、遅延が小さく再接続が確実な URL と、メディアソースの設定（`reconnect_delay_sec`、バッファリング、ネットワークバッファ）を決める。決まった URL を 8章の OBS 用 URL とする
2. **Fail → Live の復帰:** 待機シーン中はメディアソースが非表示になる。`restart_on_activate`（既定 ON）だと、非表示中に再接続しても表示されるまで再生を始めない。この挙動のもとで、切替の前に `RESTART` を送ると映像がすぐ出るか、RESTART と切替の順序と待ち時間をどうするかを決める。結果は 6章の定数に反映する
   - **結果（2026-11-20、Windows と Linux の OBS）:** 送信が止まると、メディアソースは RESTART を受けるまで再生を戻さない（RESTART が要る）。RESTART から再生開始まで約 1.02秒で、待ち時間は1秒にした。`restart_on_activate` が有効だと、本体が RESTART のあとに切り替えた瞬間に再生がもう一度やり直され、「再接続までの遅延」の間（約2秒）黒い画面が出た（Windows で再現性あり）。そのため `restart_on_activate` は解除を推奨する。解除しても、送信がない間は OBS が裏で「再接続までの遅延」ごとに読み直すので、人が IRL Link などでカメラのシーンへ切り替えたときも映像はすぐ出た。カメラの接続が確立してから画面に出るまでの時間は「再接続までの遅延」に依存するので、2秒を推奨する（1秒にすると、送信がない間に MediaMTX のログが毎秒2行増える）
3. **（済）テスト用の JSON:** 実際の API の応答（`/v3/paths/get/{name}`、`/v3/rtmp/conns/list`、`/v3/srt/conns/list`）を、RTMP と SRT それぞれ送信中・未送信の状態で録り、`internal/ingest/testdata/` に置く。TCP 接続が残ったままバイトが止まる状況を再現できれば、それも録る。あわせて、MediaMTX のログに publish の資格情報が出るかを確かめる（8章の置き換えが効いているかの確認）
4. **配信開始時の切替（①② とあわせて行う）:** 配信開始時の切替（6章）を、obs-websocket の `StreamStateChanged` の `STARTING` の時点で送れるか、送ったシーンで配信が始まるか（`STARTED` で送る場合に、切替前のシーンが配信に出る長さも測る）を確かめる。結果で 6章と 7章の写像を見直す

## 13. 既知の制約

- macOS では、本体が強制終了されると MediaMTX が残ることがある
- 本体と外部ツールがほぼ同時にシーンを切り替えると、最後に届いた切替が残る。本体は SetScene を送る直前に承認を取る（9章）ので、人の操作が先に届いていればバッチは失効する。残る窓は「承認から OBS が要求を受け取るまで」の間だけ。その間に人が切り替えた場合は、人の選択が上書きされることがある。その場合はもう一度切り替える。手動の切替で期待が消えたあとに届いた本体の切替は手動として扱うので、それが待機シーンへの切替なら、待機シーンに持ち主 `none` で残り、回復しても自動では戻らない。その場合はもう一度シーンを選ぶ
- 外部ツール（IRL Link など）にもビットレートによる自動切替の機能があり、それを有効にすると二重に切り替わる。自動切替はどちらか一方だけで使う
- 管理 API を LAN に公開した場合、通信は平文の HTTP と Basic 認証なので、LAN 内で盗聴されるとパスワードと操作を奪われうる。操作はシーンの切替と配信の開始・停止に限られるため、第1段階では TLS を入れず、利用者の判断で LAN 公開を選ぶ形にする
- 本体が要求したのと同じシーンを、人がほぼ同時に選んだ場合は、本体の切替と区別できない（obs-websocket のイベントに発信者が含まれないため）。その場合は本体の切替として扱う（例: 自動切替と同時に人が待機シーンを選ぶと、待機の持ち主は `auto` になり、回復したら戻る）
- 回線が悪い間に人が監視対象のシーンへ切り替えると、すぐ待機シーンへ戻される。屋外の配信者がそれに気づく手段は、当面は待機シーンの名前だけ（6章「検討事項」の案 A）
- 配信開始時の切替は `STARTED` で検知するので、配信の最初のわずかな間は切替前のシーンが配信に出る（6章、12章の spike ④）
- SRT の streamid と RTMP のクエリに含めた資格情報は暗号化されない（SRT のパスフレーズは第1段階では使わない）
- goobs の制約（v1.11.1。v1.0.0 以降のすべての版で同じ作り）: WebSocket の接続はできたが認証が終わらずに `New` が失敗したとき（OBS が固まって Hello や認証の応答を返さず、応答待ちの5秒を過ぎた場合）、ラッパーが接続を閉じても（7章）、goobs の中の goroutine が残る。goobs の受信の goroutine は、閉じるフレーム以外の読み取りのエラー（接続を閉じたときのエラーを含む）では内部のチャネルを閉じずに抜けるので、opcode を処理する goroutine が閉じられないチャネルを待ったまま残る。応答待ちを過ぎたあとに Identified が届いた場合と、Identify の送信に失敗した場合は、認証の結果を送る goroutine が、受け手のいないチャネルへの送信で止まったまま残る。認証が終わる前に、通常の読み取りのエラーで接続が切れた場合も、同じく opcode を処理する goroutine が残る。再接続は最短5秒ごとなので、その状態が続くと1時間に数百本（数 MB）増えうる。OBS が起動していない（接続が拒否される）ときと、パスワードが違う（OBS が認証の失敗で接続を閉じる）ときは起きない。goobs を使い続け、上流に報告した（https://github.com/andreykaipov/goobs/issues/277、2026-11-20）。直るまでの制約とする（2026-11-20 のユーザーの裁定。Go の obs-websocket v5 のライブラリは事実上 goobs だけで、活発に保守されている）
- MediaMTX の受信サーバーの設定（ポートなど）や path の認証設定を変えると、配信が切れる。第2段階の設定編集では「配信中に変えると切れる」設定として扱う

## 変更履歴

| 日付 | 版 | 変更 | 理由 | commit |
|---|---|---|---|---|
| 2026-11-18 | v1.0 | 初版。設計セクション1〜5と、外部ツールからの手動切替を尊重する手動保持（6章） | brainstorming での承認。手動保持は IRL Link との競合についてのユーザーの指摘 | 764e12a |
| 2026-11-18 | v1.1 | 管理 API の待ち受けの既定を `127.0.0.1` にし、LAN 公開時の警告を追加（2章、4章、13章）。Judge に実時刻の例の表を追加（5章）。switcher のテストを「入口 × 状態」の表にし、テスト用 JSON に fallback を足さないことを明記（11章）。変更履歴の表を追加 | agentic-fx での裁定と教訓（既定は安全側、整合は状態機械の中心で守る、時刻の契約は実時刻の例で固定する、フィクスチャは実物から作る）を、ユーザーが選んで取り込んだ | 39c8ea4 |
| 2026-11-18 | v1.2 | 読み出しにも認証を必須にした（4章、8章）。実行待ちのバッチを世代番号で失効させ、実行直前に承認を取る（6章、9章）。期待シーンを送信時に登録し、期限をトランジション時間から決め、列で持つ（6章）。配信開始は Intro になってから始める。バッチは失敗した時点で残りを捨てる（6章、7章）。手動保持を再接続で解除しない条件、配信停止で消すこと、Pause の直前の役割の寿命を定義（6章）。Judge の評価順、Kbps の分母、新しい publisher の検出を定義（5章）。MediaMTX の全プロトコルのスイッチを明示し、RTSP を TCP だけにした（8章）。Pdeathsig のスレッド固定、Job Object を本体自身に掛ける方式に変更、ポートの事前確認を補助の診断にした（8章）。名前とパスワードの使える文字を制限（4章）。`cmdErr` の型で 503 と 409 を分け、POST に独自ヘッダーを必須にした（9章）。テスト表の状態の軸を追加（11章）。13章の2項目が変更履歴の後ろにずれていたのを戻した | 設計レビュー1周目（Fable: Critical 1 / Important 8 / Minor 8、codex sol: Critical 1 / Important 10 / Minor 2）。読み出しの認証はユーザーの裁定 | 1a0378c |
| 2026-11-19 | v1.3 | 世代番号を廃止し、バッチの目的（Purpose）ごとの有効条件を承認の時点で確かめる方式に変えた。配信停止や Pause は受信状態の変化で捨てない。GoLive / PauseOff は回線の悪化で Fail へ決め直す（6章）。`Approve` を `Machine.Approve` に分け、`Proceed` / `Skip` / `Reject` を返す。`SceneRequestDone` と `BatchDone` を追加し、同じ Purpose の未完了のバッチがある間は重ねて出さない（6章、9章）。期待の期限を応答の時点から数え、タイムアウトでは消さず、Transition Override の長さを使う。一致した期待より前の期待も消す（6章、7章）。中核ループと実行役のチャネルの決まりを追加（9章）。配信状態の写像を定義し、出力の再接続を停止として扱わない（7章）。手動保持は配信中だけ入る（6章）。配信開始の待ちを消す条件を列挙（6章）。Poller の残りの未定義を決めた（5章）。Windows の Job の割当失敗時とハンドルの扱い（8章）。テスト表の展開の仕方（11章） | 設計レビュー2周目（Fable: Critical 0 / Important 5 / Minor 6、codex sol: Critical 0 / Important 7 / Minor 1） | 6566a38 |
| 2026-11-19 | v1.4 | 配信開始の待ちに「Intro 確認済み」の段階を足し、確認後の StartIntro のタイムアウトで開始を取り消さない。`Approve` に `Replace{Batch}` を足して GoLive / PauseOffLive の決め直しを受け渡す。トランジションの上書きを `Override`（`None` / `Known` / `Unknown`）で渡す。`StreamEpoch` を足し、前の配信のバッチを次の配信で実行しない。実行役が次のバッチを受け取る順序と、`SceneRequestDone` と `BatchDone` の順序を明記（6章、7章、9章、11章） | 設計レビュー3周目（codex sol、新しい方式に絞った周: Critical 0 / Important 4 / Minor 0）。Critical 0 が続いたので、この版で設計レビューを終える | 26ed572 |
| 2026-11-19 | v1.5 | 6章に「細部の規則」を追加（承認の順序、配信開始の待ちの細部、実行中の操作の重複、`last_error`、`Tick` の判定、切替中の自動判定の停止、期限の境界、初期値など23項目） | switcher の「入口 × 状態」の表を作る中で見つかった不明点（U-01〜U-23）。技術で決まるものとして、表の案どおりに決めた | 9f3fbb8 |
| 2026-11-19 | v1.6 | publisher の識別子を `source.id` に決めた。path の応答、送信が止まったときと再開したときの挙動を 5章に記録。8章に見本の設定ファイルの場所と、API の応答をログに書かない注意を追加。spike ③ を済みにした | spike ③（MediaMTX v1.21.4 を実際に動かして応答を録った。設定の項目名はすべて正しかった） | 1510eea |
| 2026-11-19 | v1.7 | api の資格情報は本体の起動のたびに作り、MediaMTX の再起動では変えないと明記（8章） | T9 の準備中に、supervisor（再起動のたびに作り直す）と Poller（作成時に1回だけ受け取る）の食い違いを見つけた | 5c448f9 |
| 2026-11-19 | v2.0 | シーンのモデルを作り直した。固定の4役割（Intro / Live / Fail / Pause）、`obs.scenes`、GoLive、PauseOn / PauseOff、配信開始の待ち（Intro の確認）、手動保持を廃止した。代わりに、待機シーン（1つ）と監視対象のシーン（複数）を設定し、待機の持ち主（none / auto / manual）と戻り先のシーンを持つ。シーンの設定は Event `ScenesConfigured` で差し替えられる入力にし、Action はシーン名を持つ。手動操作は配信の開始・停止と手動の待機 ON / OFF だけにした。`switch.auto_enabled` と、配信開始時の切替（`on_stream_start`）を追加。手動で監視対象のシーンへ入ったときは、手動保持ではなく通常の判定をする（回線が悪ければ待機シーンへ切り替え、そのシーンを戻り先にする）。`Replace` は代わりのバッチなしの形を足した。status と API を合わせて変更。位置づけ、通知の検討事項、書き直しで残った7点の決定（6章末「実装前に決めたこと」。設定の世代 `ConfigEpoch`、StandbyOn を受け付けない条件、再接続で配信状態が違っていたときの扱い、初回生成の既定値、`auto_enabled` が false のときの StandbyOff、StartScene を再送しないこと、開始時のシーンに待機シーンを指定できないこと）、第3段階のオーバーレイの方向、spike ④ を追加（1章、2章、4章、6章、9章、10章、11章、12章、13章）。v1.x の「入口 × 状態」の表は作り直す | 2026-11-19 のユーザーの裁定。旧版の4役割はツール内蔵の手動切替のためのもので、普段のシーン操作を IRL Link などの外部ツールで行う今は要らない | 4c93cd8 |
| 2026-11-19 | v2.1 | 期待に登録時の `ConfigEpoch` を持たせ、設定が変わったあとの期待は持ち主と戻り先に作用しないとした。期待が残る間は StandbyOn を受け付けない。開始時の切替が無効でも通常の自動切替は行うと結合テストを直した。テストのケースを5つ足した（6章、11章） | spec v2.0 の設計レビュー（codex sol: Critical 0 / Important 3 / Minor 0） | 5d87123 |
| 2026-11-19 | v2.2 | 6章に「細部の規則（v2.1 の表を作る中で決めたこと）」を追加（V-01〜V-18）。13章に遅れて届いた本体の切替の制約を追記 | v2.1 の switcher の「入口 × 状態」の表を作る中で見つかった不明点。技術で決まるものとして、表の案どおりに決めた | 33f4d5a |
| 2026-11-19 | v2.3 | status の `last_error` の形と、接続前の `scene_class` を定義（9章） | T9 の検収で、`last_error` の形が決まっていないことがわかった | 8a5a31f |
| 2026-11-20 | v2.4 | API の細部を決めた: 確かめる順序（404 → 401 → 405 → 403）、`WWW-Authenticate`、独自ヘッダーの値、本文の形、中核ループが受け取らないときの 503、想定外の 500（9章）。ログの日付の切替、消すファイルの範囲、パスワードの置き換え（10章）。`log.dir` と `log.keep_days` の検証（4章）。送信用 URL の host を自動と手動で切り替える `stream.publish_host`（4章、8章）。OBS 用 URL は spike ① まで仮に RTSP（8章）。main の起動と終了の手順（10章）。`internal/redact` を足し、MediaMTX のログの置き換えも10章と同じ規則にした（3章、8章）。OBS の接続に失敗したときに接続を自分で閉じる（7章）。goobs の中に残る goroutine を既知の制約にした（13章）。パスワードの比較を固定長にする（9章） | T10・T11 の準備で、決まっていない点が見つかった。publish_host と OBS 用 URL は 2026-11-20 のユーザーの裁定。7章・9章・13章は T10〜T11 のレビュー（codex sol: Critical 0 / Important 1 / Minor 1） | この版の commit |
| 2026-11-20 | v2.5 | 6章の StandbyOn の有効条件と「実装前に決めたこと」の 2 から、AutoFail が先に待機シーンへ届いて却下される例を除き、通常の列での却下は `SceneEpoch` によると直した。11章の「同じシーンへの期待が複数ある」の例を、6章 V-02 の決定に合わせて「StandbyOff の押し直しで、同じ戻り先への期待が2つ」に書き換えた。古い「表を作り直す」の文を消し、表の場所だけを書いた。6章の StandbyOn の有効条件の行と11章に、V-13（通常の列では起きない組み合わせは、状態を直接作って `Approve` を呼ぶ）を添えた | 文書の残り（T9 の報告から） | この版の commit |
| 2026-11-20 | v2.6 | ポートの事前確認で、全インターフェースのポートは IPv4 の `0.0.0.0:port` でも確かめる（8章） | Windows の実機で、ほかのプロセスが IPv4 で使っているポートを見落とした | この版の commit |
| 2026-11-20 | v2.7 | spike ①② の結果を反映した。OBS 用 URL は RTSP に決定（8章）。RESTART のあとの待ち時間は1秒（6章）。メディアソースは `restart_on_activate` の解除と「再接続までの遅延」2秒を推奨（8章、12章） | Windows の実機の spike ①②（ユーザーと一緒に行った） | この版の commit |
