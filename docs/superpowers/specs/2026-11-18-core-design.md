# irl-live-keeper 設計 — 全体構成と第1段階（コア）

- 作成日: 2026-11-18
- 版: v1.2（変更は末尾の変更履歴を参照）
- 状態: ユーザー承認済みの設計（セクション1〜5）を文書化したもの
- 対象バージョン: MediaMTX v1.21.4（2026-11-15 リリース）、obs-websocket v5

## 1. 目的

屋外配信（IRL）で回線が不安定になったら、OBS を待機画面のシーンへ自動で切り替え、回復したら自動で復帰させる。リレーサーバー、OBS への取り込み、管理ページ、OBS 用オーバーレイを1つのシステムにまとめ、Windows を主に全 OS で「zip を展開して実行するだけ」で導入できる状態を最終形とする。

旧版 `rtmp-monitor`（Python + nginx-rtmp）はこのリポジトリと別に旧版として残す。コードは引き継がず、挙動だけを引き継いで明文化する。

### 前提

- 配置: OBS と同じマシン、または同じ LAN 内の別 PC。VPS は対象外
- 配信 PC は Windows を想定。Linux と macOS でも動くこと
- 入力: RTMP と SRT の両方（送信側は Larix / IRL Pro / Moblin / OBS など）

### 入れないもの（YAGNI）

Discord / Twitch 連携、OBS シーンの自動作成、待機素材のアップロードと管理、複数の path（複数配信の同時受信）、SRT の RTT やロスを使った切替判定（表示だけに使う）。

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
 ├ switcher   : 状態機械（Live と待機のどちらにするかを判定する）
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
| 2. 管理ページ | 状態表示、グラフ、操作、設定編集、送信用 URL と OBS 用 URL の表示、認証画面 | 後で別に書く |
| 3. オーバーレイ | 通信状況とビットレートを OBS のブラウザソースで表示 | 後で別に書く |
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

obs:
  host: 127.0.0.1
  port: 4455
  password: ""               # 空なら認証なしで接続する（OBS 側で認証を無効にしている場合）
  media_source: IRL          # MediaMTX を読むメディアソースの入力名
  scenes:                    # 役割 → 実際の OBS シーン名
    intro: Intro
    live: Live
    fail: Fail
    pause: Pause

switch:
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
- `obs.scenes` の4つは空でなく、互いに異なる
- `admin.password` は空でない

`admin.listen` が loopback 以外（`0.0.0.0` や LAN のアドレス）のときは、起動するたびに警告をログに出す: 管理 API を LAN に公開していること、通信は平文で LAN 内では盗聴されうること、ルーターで転送しないこと。値を省略したときに LAN 公開と解釈することはない（省略時は既定の `127.0.0.1:8787`）。

## 5. 受信状態の判定（ingest）

### Poller

1秒ごとに次を読む（各リクエストのタイムアウトは 500ms）。

- `GET /v3/paths/get/{stream.name}`: `online`、`available`、`inboundBytes`、`source.type`（`rtmpConn` / `srtConn` など）、publisher の識別子（`source.id` か `onlineTime`。どちらを使うかは spike ③ で録った実物を見て1つに決め、もう一方を fallback として読まない）
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
- 次のときは、その回の `Kbps` を 0 とし、読めた値を新しい基準にする: 起動後の最初の読み取り、MediaMTX の再起動後の最初の読み取り、publisher の識別子が変わったとき（再接続）、`inboundBytes` が基準より減ったとき
- `Online` が false の間は `Kbps` を 0 とする
- path の API が失敗したときは `Err` を入れ、`Kbps` を 0 とする。`Interval` は前回の Sample からの経過時間のまま

### Judge

`Judge.Update(Sample) LinkState` は時刻を `Sample` から受け取る純粋な処理とし、内部で時計を読まない。

受信状態は `OK` / `LOW` / `DOWN` の3つ。起動直後は `DOWN`。

各 Sample は直前の Sample からの区間（`Interval`）を表す。「X 秒続く」は、条件を満たす Sample が連続している間の `Interval` の合計が X 秒以上になったこと、と定義する（1秒間隔なら `down_sec: 2` は2回連続、`low_sec: 5` は5回連続）。

| Sample の分類 | 条件 |
|---|---|
| zero | `!Online`、`Err != nil`、または `Kbps == 0` |
| below | `Kbps < fail_kbps`（zero を含む） |
| good | `Kbps >= recover_kbps` |

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
| P1 | t=0 で `inboundBytes`=0（起動後の最初）、t=1 で 87,500 | t=0 は 0（基準を取るだけ）、t=1 は 700 |
| P2 | t=0 で 0、t=1 で失敗、t=2 で 175,000 | t=1 は 0（`Err`）、t=2 は 700（2秒分を2秒で割る） |
| P3 | t=0 で 500,000、t=1 で publisher の識別子が変わり 600,000 | t=1 は 0（新しい基準）、t=2 からは新しい基準との差分 |
| P4 | t=0 で 500,000、t=1 で 20,000（識別子は同じ） | t=1 は 0（新しい基準） |

## 6. シーンの状態機械（switcher）

### 役割

OBS の現在のシーン名を、config の割り当てで役割に変換する: `Intro` / `Live` / `Fail` / `Pause` / `Other`（管理外）。

### 入力（Event）と出力（Action）

```go
// Event はすべて At（時刻）を持つ
LinkChanged{State LinkState}
OBSConnected{Streaming bool, Scene string, TransitionMs *int} // TransitionMs: 現在のトランジションの長さ。固定長（スティンガーなど）で取れなければ nil
OBSDisconnected{}
SceneChanged{Scene string}          // program のシーンの変更（CurrentProgramSceneChanged）だけ。Studio Mode の preview は扱わない
TransitionChanged{TransitionMs *int} // トランジションまたはその長さが変わった
StreamStateChanged{Streaming bool}
Command{Kind: StreamStart | StreamStop | GoLive | PauseOn | PauseOff}
Approve{Gen int, Action Action}      // 実行役が Action を送る直前の承認要求（9章）。応答は承認か却下
ActionFailed{Gen int, Action Action, Err error}
Tick{}        // 1秒ごと。再送と期待の期限の判定に使う

// Action
SetScene{Role}
RestartMedia{Wait time.Duration} // RESTART を送ったあと、次の Action までの待ち時間（spike ② で決める定数）
StartStream{}
StopStream{}
```

`Machine.Handle(Event) (batch Batch, cmdErr error)`。`Batch` は `Gen`（世代番号）と Action の列を持つ。

`cmdErr` は `Command` を受け付けられないときだけ返し、型で2種類に分ける: `ErrNotConnected`（OBS に未接続）と `ErrInvalidState`（今の状態では受け付けられない。理由の文を持つ）。web はこの型から 503 と 409 を決め、自分では状態を判定しない。

### 世代番号

switcher は世代番号 `Gen` を持ち、次のときに1つ進める: `LinkChanged`、手動とみなした `SceneChanged`、`StreamStateChanged`、`OBSConnected`、`OBSDisconnected`、手動保持に入ったときと解けたとき、Command を受け付けたとき。期待どおりの（本体の）`SceneChanged` と `Tick` では進めない。

新しいバッチは、その時点の `Gen` を持つ。`Approve` で `Gen` が今の値と違えば却下し、実行役はそのバッチの残りを捨てる。これで「決めた時点では正しかったが、実行するまでに状況が変わった」バッチが実行されない（例: 復帰のバッチの RESTART の待ち時間中に回線が再び切れた、人が Pause に切り替えた、OBS が切断した）。

### 自動切替

自動切替は、**OBS に接続していて、配信中で、現在の役割が Live か Fail のときだけ**行う。Intro、Pause、Other には触らない。現在のシーンは OBS のイベントで追うので、OBS 側での手動切替を尊重する。

- Live のとき、受信状態が `LOW` か `DOWN` → `[SetScene(fail)]`
- Fail のとき、受信状態が `OK` → `[RestartMedia, SetScene(live)]`
  - `RestartMedia` は `TriggerMediaInputAction`（`OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART`）を `obs.media_source` に送る
  - RESTART と切替の順序、間に置く待ち時間（`RestartMedia.Wait`）は spike ②（12章）で決め、定数として実装する
- 受信状態が `LinkChanged` で変わったときだけでなく、`SceneChanged`（本体の切替によるもの）、`StreamStateChanged`、`OBSConnected` のあとにも同じ判定をする。ただし手動保持中は判定しない（次節）

### 外部からの手動切替（手動保持）

OBS のシーンは、本体のほかに OBS の画面や IRL Link などの外部ツールからも切り替えられる。obs-websocket のシーン変更イベントには、誰が切り替えたかが含まれない。そこで次のように扱う。

- **期待シーン:** 期待は、`SetScene` を決めた時点ではなく、実行役が実際に送る直前の `Approve` を承認した時点で登録する。期待は列で持ち、送った `SetScene` ごとに1つ足す。現在のシーンがすでに目的のシーンなら、イベントが来ないことがあるので期待を登録しない
- **期限:** 承認した時刻 + トランジションの長さ + 3秒。OBS はトランジションが終わった時点で `CurrentProgramSceneChanged` を出すため、トランジションの長さを含める。長さが取れない（固定長のトランジション）ときは、承認した時刻 + 10秒とする
- **本体の切替と外部の切替の見分け方:** `SceneChanged` が、列の先頭から見て最初に一致する期待のシーンと同じなら、本体の切替とみなしてその期待を消す。どの期待とも一致しない `SceneChanged` は、すべて手動（外部）の切替とみなし、期待を全部消す。期限を過ぎた期待と、`ActionFailed` になった `SetScene` の期待は消す。obs-websocket の応答とイベントのどちらが先に届いても、期待は送る前に登録してあるので判定は変わらない
- **手動保持:** 手動で役割が Live か Fail に切り替えられたら、手動保持に入る。保持中は、そのシーンを自動切替で打ち消さない。待っていた再送もすべて取り消す（世代番号が進むので、実行待ちのバッチも失効する）
- **保持の解除:** 受信状態が OK と不良（LOW / DOWN）の間で変わったら解除する。LOW と DOWN の間の変化では解除しない。解除したら、その時点の受信状態で通常どおり判定する。例: 回線が悪いときに手動で Live にした場合は、回線がいったん OK に戻り、再び悪くなった時点で Fail に切り替える
- 手動保持は、次のときにも解除する:
  - 手動で役割が Live / Fail 以外（Intro、Pause、Other）に切り替えられたとき。この場合は自動切替の対象外なので、もともと何もしない
  - 本体の Command（GoLive、PauseOff など）を受け付けたとき
  - `OBSConnected` で、シーンか配信状態が切断の直前と違っていたとき。どちらも同じなら保持を続ける（websocket の瞬断だけで人の選択を打ち消さないため）
  - `StreamStateChanged{Streaming: false}`（配信の停止）を受けたとき。あわせて期待と再送待ちも消す
- Intro、Pause、Other への手動切替は、これまでどおり自動切替の対象外とする。Pause への手動切替では直前の役割を覚える（下の「手動操作」を参照）
- 手動保持中かどうかと、いつから保持しているかを status に出す

### 再送の歯止め

自動切替の Action を出したあと、OBS のシーンが期待どおりに変わらない（失敗した、またはイベントが来ない）まま条件が続く場合は、`Tick` で再送する。再送するのは次をすべて満たすときだけ:
- 同じ切替の前回の送出から5秒以上たった
- 同じ目的のバッチが実行待ちでも実行中でもなく、そのシーンへの期待も残っていない
- 手動保持中でない

### バッチ内の失敗

バッチは、Action が失敗した時点で残りを捨てる。例外は `RestartMedia` の失敗だけで、続く `SetScene` は実行する（映像の再始動に失敗しても、待機画面からは戻す）。

### 手動操作

すべて OBS に接続している必要がある（未接続なら `ErrNotConnected`、web は 503）。

| Command | 受け付ける条件 | Action |
|---|---|---|
| StreamStart | 配信中でない、配信開始の待ちがない | 役割がすでに Intro なら `[StartStream]`。そうでなければ `[SetScene(intro)]` を出して「配信開始の待ち」に入り、Intro への期待どおりの `SceneChanged` が来たら `[StartStream]` を出す。期待が期限切れか `ActionFailed` になったら待ちを消して `last_error` に記録し、配信は始めない（Live のカメラ映像のまま配信が始まるのを防ぐ） |
| StreamStop | 配信中 | `[StopStream]` |
| GoLive | 配信中、役割が Intro | 受信状態が OK なら `[RestartMedia, SetScene(live)]`、そうでなければ `[SetScene(fail)]` |
| PauseOn | 配信中、役割が Intro / Live / Fail | `[SetScene(pause)]` |
| PauseOff | 配信中、役割が Pause | 下記 |

条件を満たさない Command は `ErrInvalidState` を返し、web は 409 を返す。配信開始の待ちは、手動の `SceneChanged`、`OBSDisconnected` でも消す。

PauseOn は旧版と違い、Intro からも受け付ける（開始前の待機中に一時的に Pause を出せるようにするため）。

**Pause の直前の役割:** 役割が Pause に変わるたびに、必ず上書きする。直前の役割が Intro / Live / Fail ならその役割、それ以外（Other から来た、など）なら「不明」。OBS 側で手動で Pause にした場合も、`SceneChanged` から同じように扱う。次のときは「不明」に戻す: Pause から出たとき、`OBSConnected`（接続した時点で Pause だった場合を含む）、`StreamStateChanged{Streaming: false}`。

**PauseOff の戻り先:**
- 直前が Intro → `[SetScene(intro)]`
- 直前が Live か Fail、または不明 → その時点の受信状態が OK なら `[RestartMedia, SetScene(live)]`、そうでなければ `[SetScene(fail)]`

旧版は受信状態に関係なく直前のシーンへ戻していた。新版では、回線が悪いまま Live に戻ってすぐ Fail へ飛ぶのを避ける。

### OBS 未接続の間

`OBSDisconnected` から `OBSConnected` までは Action を出さず、`Approve` もすべて却下する。`OBSDisconnected` で期待をすべて消す（世代番号も進むので、切断前のバッチは再接続後に実行されない）。`OBSConnected` で配信状態と現在のシーンを取り直してから判定を再開する。

## 7. OBS 接続（obs）

```go
type Client interface {
    Events() <-chan switcher.Event          // OBSConnected / OBSDisconnected / SceneChanged / TransitionChanged / StreamStateChanged
    SetScene(name string) error             // SetCurrentProgramScene
    RestartMedia(input string) error
    StartStream() error
    StopStream() error
}
```

- `Run(ctx)` で接続を保つ。切れたら待ち時間を 1→2→4…秒と延ばし、上限5秒で再接続を続ける
- 接続したら、配信状態、program の現在のシーン、現在のトランジションとその長さを取得して `OBSConnected` を流す。あわせてシーン一覧と入力一覧を取得し、config のシーン名と `media_source` が存在しなければ警告を status に出す
- シーンの取得・設定・監視は program だけを対象にする（`GetCurrentProgramScene`、`SetCurrentProgramScene`、`CurrentProgramSceneChanged`）。Studio Mode の preview は扱わない
- トランジションの変更（`CurrentSceneTransitionChanged`、`CurrentSceneTransitionDurationChanged`）を `TransitionChanged` として流す
- goobs の要求は context を受け取らず、応答待ちの既定は10秒。ラッパーは `WithResponseTimeoutDuration` で応答待ちを5秒にする。タイムアウトは失敗として返す
- バッチ内の失敗の扱いは 6章「バッチ内の失敗」のとおり（実行役が行う）
- 中核ループは `Client` インターフェースだけに依存し、テストでは偽物に差し替える

## 8. MediaMTX の管理（supervisor）

### 生成する mediamtx.yml

起動のたびに config から生成して上書きする（先頭に「自動生成。編集しないこと」と書く）。要点:

- 既定値に頼らず、v1.21.4 にあるすべてのプロトコルと機能のスイッチを明示して書く。有効にするのは RTMP（`:rtmp_port`、`rtmpEncryption: "no"`）、SRT（`:srt_port`）、RTSP（`:rtsp_port`、`rtspTransports: [tcp]`、`rtspEncryption: "no"`）、API（`127.0.0.1:api_port`）だけ。HLS、WebRTC、MoQ、playback、metrics、pprof は `false` にする。RTSP の UDP とマルチキャスト（既定では 8000/8001 UDP などを開く）と MoQ（既定で有効、8892/8893）を止めるのは、事前確認していないポートで起動に失敗するのと、意図しない待ち受けを開くのを防ぐため
- path は `stream.name` の1本だけ。それ以外の path は受け付けない
- `authInternalUsers`（既定の匿名ユーザーは置き換えて残さない）:
  - publish: `stream.publish_user` / `stream.publish_pass`（RTMP と SRT で共通）、IP 制限なし
  - read: `stream.read_user` / `stream.read_pass`、IP 制限なし。パスワードなしの読み出しは作らない。トンネルやポートプロキシ、送信元を書き換える転送を使うと、インターネットからの接続も 127.0.0.1 や LAN のアドレスに見えるため、IP だけで許可すると生映像を誰でも読めてしまう
  - api: 起動のたびにランダムに作るユーザー名とパスワード、`127.0.0.1` と `::1` からだけ許可。Poller はこの資格情報で API を呼ぶ

送信用 URL（status と、起動時の標準出力に表示する。10章のログの規則を参照）:
- SRT: `srt://<host>:<srt_port>?streamid=publish:<name>:<user>:<pass>`
- RTMP: `rtmp://<host>:<rtmp_port>/<name>?user=<user>&pass=<pass>`

OBS メディアソース用 URL は spike ① の結果で決める。どれを選んでも read の資格情報を埋め込む（候補: `rtsp://<read_user>:<read_pass>@<host>:<rtsp_port>/<name>`、`rtmp://<host>:<rtmp_port>/<name>?user=<read_user>&pass=<read_pass>`、`srt://<host>:<srt_port>?streamid=read:<name>:<read_user>:<read_pass>`）。この URL も status と起動時の標準出力にだけ表示する。

### 起動と再起動

- 起動前に、`rtmp_port`（TCP）、`srt_port`（UDP）、`rtsp_port`（TCP）、`api_port`（TCP）を、MediaMTX と同じ形（全インターフェース、api だけ 127.0.0.1）で listen できるか確かめ、すぐ閉じる。これは補助の診断で、ポートの予約ではない（閉じてから MediaMTX が bind するまでに、別のプロセスが取ることはありうる）。使用中なら、どのポートかと「前回の mediamtx が残っていないか確認すること」を示して起動を止める
- MediaMTX が起動直後に落ちたときは、そのログに出た bind などのエラー行を `last_error` に出す。事前確認を通ったあとの起動失敗も、これで理由が見える
- MediaMTX の標準出力と標準エラーは、`component=mediamtx` を付けて本体のログに流す。その際、`publish_pass`、`read_pass`、api のパスワードの文字列を `***` に置き換える（MediaMTX のログに RTMP のクエリや SRT の streamid が出る場合に備える。4章で使える文字を制限しているので、URL エンコードで形が変わることはない）
- 落ちたら再起動する。待ち時間は 1→2→4…秒と延ばし、上限30秒。60秒安定して動けば待ち時間を1秒に戻す
- 落ちている間、Poller の API 呼び出しは失敗するので受信状態は `DOWN` になる。status では `mediamtx: "down"` として区別する
- 本体の終了時（Ctrl+C、SIGTERM）は MediaMTX を止めてから終了する
- 本体が強制終了されても子プロセスを残さない:
  - **Windows:** 本体の起動時に Job Object（`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`）を作り、本体自身をその Job に入れる。子は生成された時点で Job を継承するので、`Start` から Job への割り当てまでの間に本体が落ちて子が残る、という窓がない。Job のハンドルは子に継承させない
  - **Linux:** `SysProcAttr.Pdeathsig = SIGKILL`。Go の Pdeathsig は、子を fork した OS スレッドが終わった時点で発火する（プロセスの終了より前のことがある。go.dev/issue/27505）。そのため、子の起動から `Wait` が返るまでを、`runtime.LockOSThread()` した専用の goroutine 1本で行い、子が生きている間はその goroutine を終わらせない
  - **macOS:** 相当する仕組みがないため、正常終了時の停止だけとする（既知の制約）

## 9. 中核ループ（core）と API（web）

### 中核ループ

goroutine 1本が switcher の状態を持ち、次を `select` で受ける: ingest の `Sample`、obs の `Events()`、web からの Command、1秒ごとの `Tick`。

- `Sample` は Judge に通し、受信状態が変わったら `LinkChanged` を switcher に渡す
- switcher が返したバッチは obs で実行する。中核ループを止めないよう、実行は専用の実行役 goroutine 1本に渡す。実行役はバッチ内の Action を返された順に1つずつ実行し、バッチを1つずつ処理するので、バッチ同士は重ならない（`[RestartMedia, SetScene(live)]` の順序が保証される）
- **実行直前の承認:** 実行役は各 Action を送る直前に、`Approve{Gen, Action}` を中核ループに送って応答を待つ。中核ループは switcher に渡し、承認か却下を返す。却下なら、そのバッチの残りを捨てる。`SetScene` の期待は、この承認の時点で登録される（6章）。`RestartMedia.Wait` の待ちは RESTART の送出後に実行役が行い、続く `SetScene` の承認はその待ちのあとに取る
- 失敗は `ActionFailed{Gen, Action, Err}` として中核ループに戻す
- Command は応答用のチャネルを持ち、switcher の `cmdErr` を web に返す（Action の実行結果は待たない）
- 状態が変わるたびに `Snapshot` を作り、`atomic.Pointer` で公開する。web はこれを読むだけ

### API（第1段階）

すべて Basic 認証（ユーザー名 `admin`、パスワード `admin.password`）。POST は、さらに独自ヘッダー `X-IRL-Live-Keeper: 1` を必須とし、ない場合は 403 を返す。ブラウザが Basic 認証を覚えたあと、別のサイトのページからフォームで POST されて配信が止まる（CSRF）のを防ぐため。フォームの POST は独自ヘッダーを付けられない。

- `GET /api/status` → 200

```json
{
  "link":    {"state": "OK", "kbps": 4200.5, "online": true, "protocol": "srt",
              "srt": {"rtt_ms": 85.2, "loss_rate": 0.01, "recv_mbps": 4.2}},
  "obs":     {"connected": true, "streaming": true, "scene": "Live", "role": "live",
              "manual_hold": {"active": false, "since": null},
              "warnings": []},
  "mediamtx": "running",
  "urls":    {"publish_srt": "srt://…", "publish_rtmp": "rtmp://…", "obs_read": "rtsp://obs:…@…"},
  "last_error": null
}
```

- `POST /api/stream/start`、`/api/stream/stop`、`/api/scene/live`、`/api/pause/on`、`/api/pause/off`
  - 200: 受け付けた（Action の完了は待たない）
  - 401: 認証失敗
  - 403: 独自ヘッダーがない
  - 409: `ErrInvalidState`（理由を本文に入れる）
  - 503: `ErrNotConnected`（OBS に未接続）

## 10. エラー処理とログ

方針: **わからないときは待機側に倒す。ただし誤って何度も切り替えないよう歯止めをかける。**

| 事象 | 対応 |
|---|---|
| MediaMTX が落ちた | 8章のとおり再起動。受信状態は DOWN になり、配信中なら Fail へ |
| 起動時にポートが使用中 | どのポートかと、前回の mediamtx の残りを疑うよう示して起動を止める |
| MediaMTX が起動直後に落ち続ける | ログの bind などのエラー行を `last_error` に出し、8章の待ち時間で再起動を続ける |
| API の失敗・タイムアウト | その回は受信0。DOWN は `down_sec` 続いたときだけなので、1回では切り替わらない。差分の基準は変えない |
| 再接続（publisher が変わった、`inboundBytes` が減った） | その回は0として扱い、差分の基準を取り直す |
| 実行待ちのバッチが古くなった | 実行直前の承認で却下され、残りを捨てる |
| OBS と切断した | 自動切替を止め、手動操作は 503。再接続後に状態を取り直して再開 |
| OBS の操作が失敗した | ログと `last_error` に記録。条件が続く間、最短5秒あけて再送 |
| シーン名やメディアソース名が OBS にない | 接続時に照合し、status の `warnings` に出す |
| config.yaml がない | 既定値で生成し（`*` はランダム）、送信用 URL と管理 API のパスワードを表示して起動を続ける。`obs.password` が空のままでは、OBS 側で認証が有効なら接続に失敗し、status に認証失敗として出る |
| config.yaml が不正 | 項目名と理由を表示して起動を止める |

### ログ

- `log/slog` のテキスト形式で、標準出力と `<log.dir>/irl-live-keeper-YYYY-MM-DD.log` の両方に書く。日付が変わったら新しいファイルに切り替える
- 起動時に `keep_days` を過ぎたログファイルを消す。依存は増やさない
- 自動切替のたびに理由を残す。例: `msg="scene switch" from=live to=fail reason=low kbps=620 for=5s`
- パスワードと、パスワードを含む URL（送信用、OBS 用）はログファイルに書かない。起動時に標準出力にだけ1回表示する（config.yaml を生成したときは、生成した管理 API のパスワードもあわせて表示する）

## 11. テスト方針

### 単体テスト（`go test -race ./...`）

- **switcher:** 整合は状態機械の中心（`Handle`）だけで守り、入口ごとに確認を足さない。テストは「入口 × 状態」の表1つにまとめる。入口は各 Event と各 Command、状態は役割（Intro / Live / Fail / Pause / Other）× 受信状態（OK / LOW / DOWN）× 配信中か × OBS 接続 × 手動保持の有無 × 期待の有無 × Pause の直前の役割（Intro / Live / Fail / 不明）× 配信開始の待ちの有無。表の各行で、出るバッチ、`cmdErr` の型、世代番号が進むかを確かめる。時間に依存する状態（期待の期限、再送の5秒）は表に入れず、下の Event の列のテストで確かめる。時刻は Event の `At` で注入する。表とは別に、Event の列で確かめる必須のケース:
  - 自動切替は、配信中かつ Live / Fail のときだけ。Intro、Pause、Other、配信停止中、OBS 未接続では何も出さない
  - Fail → Live で `RestartMedia` が `SetScene(live)` より先に出る
  - PauseOff の戻り先（直前が Intro、Live、Fail、不明 × 受信状態）
  - OBS 側の手動の Pause でも直前の役割を覚える
  - 再送が5秒以上あく
  - 各 Command の 409 になる条件
  - 手動保持: 期待どおりの `SceneChanged` は本体の切替とみなし、期限切れや別のシーンへの変化は手動とみなす。手動の Live / Fail で保持に入り、再送を取り消す。OK と不良の間の変化で解除し、LOW と DOWN の間の変化では解除しない。Command、配信の停止、シーンか配信状態が変わっていた `OBSConnected` で解除し、何も変わっていない `OBSConnected` では解除しない
  - 期待: 承認の時点で登録される。期限はトランジションの長さ + 3秒（取れなければ10秒）。トランジションが長く、期限の直前に届いた本体の `SceneChanged` を手動とみなさない。一致しない `SceneChanged` で全部消える。`ActionFailed` で対応する期待が消える
  - 世代番号: 復帰のバッチ `[RestartMedia, SetScene(live)]` の待ち時間中に、受信状態の悪化、手動の Pause、配信の停止、OBS の切断のどれかが起きたら、`SetScene(live)` の `Approve` が却下される
  - 配信開始: Intro への切替が失敗または期限切れなら `StartStream` を出さない
  - バッチ内の失敗: `RestartMedia` の失敗では続け、それ以外の失敗では残りを捨てる
- **ingest.Judge:** 時刻は `Sample.At` と `Interval` で注入する。5章「実時刻の例」の表の全行を、そのままテストにする。あわせて、閾値ちょうどの値（`fail_kbps`、`recover_kbps` と等しい kbps）を確かめる
- **ingest.Poller:** `httptest` のサーバーで、spike ③ で録った MediaMTX v1.21.4 の JSON を返す。5章の Poller の例（P1〜P4）の全行、タイムアウト、SRT の統計の取り出しと、その読み取りだけの失敗。JSON の形を手書きで再現しない。録った実物と違う形への fallback（別のフィールド名も読む、など）も足さない。偽の形でテストが通り続ける穴になるため
- **config:** 既定値の生成、読み込み、4章の検証すべて
- **supervisor:** mediamtx.yml の生成をゴールデンファイルと比べる（全プロトコルのスイッチが明示されていること、RTSP が TCP だけであること、MoQ が無効であること、read に資格情報が要ることを含む）。再起動の待ち時間は、偽の子プロセス（Go のヘルパープロセスの手法）で確かめる。ポートの事前確認
- **core:** obs.Client の偽物を使い、Sample → 切替 → 承認 → Action の実行 → 失敗時の `ActionFailed` までを通しで確かめる。偽物の応答を遅らせ、承認の却下でバッチが捨てられることを確かめる
- **web:** `httptest` で認証、独自ヘッダーがない POST の 403、`cmdErr` の型と 503 / 409 の対応、status の JSON の形

goobs のラッパーは実際の OBS を相手に手動で確かめる。自動テストにはしない。

### 手動の結合テスト（チェックリスト）

1. ffmpeg で RTMP と SRT のそれぞれへ送り、status の kbps と protocol を確かめる
2. 送信を止めて、約2秒で Fail に切り替わる
3. 低いビットレート（例: 500kbps）で送って、約5秒で Fail に切り替わる
4. 送信を戻して、約10秒で Live に戻り、映像が出る
5. MediaMTX を kill して、再起動される（配信中なら Fail を経て Live に戻る）
6. OBS を再起動して、再接続し、判定が再開する
7. Pause の ON/OFF と、回線が悪いときの PauseOff の戻り先
8. 外部ツール（OBS の画面、IRL Link）から、回線が悪いときに Live、良いときに Fail へ切り替えて、打ち消されないこと。そのあと回線の状態を変えると、自動切替が再開すること
9. 長いトランジション（例: 5秒のフェード、スティンガー）で自動切替しても、手動保持に入らないこと
10. 本体を強制終了して、MediaMTX が残らない（Windows と Linux）
11. OBS 用 URL から read の資格情報を外すと読み出せないこと

## 12. 最初の spike（実装の前に行う。コードは捨てる前提）

① と ② には Windows の OBS 実機が必要で、ユーザーに操作してもらいながら進める。

1. **OBS からの読み出し:** rtsp / rtmp / srt のうち、遅延が小さく再接続が確実な URL と、メディアソースの設定（`reconnect_delay_sec`、バッファリング、ネットワークバッファ）を決める。決まった URL を 8章の OBS 用 URL とする
2. **Fail → Live の復帰:** 待機シーン中はメディアソースが非表示になる。`restart_on_activate`（既定 ON）だと、非表示中に再接続しても表示されるまで再生を始めない。この挙動のもとで、切替の前に `RESTART` を送ると映像がすぐ出るか、RESTART と切替の順序と待ち時間をどうするかを決める。結果は 6章の定数に反映する
3. **テスト用の JSON:** 実際の API の応答（`/v3/paths/get/{name}`、`/v3/rtmp/conns/list`、`/v3/srt/conns/list`）を、RTMP と SRT それぞれ送信中・未送信の状態で録り、`internal/ingest/testdata/` に置く。TCP 接続が残ったままバイトが止まる状況を再現できれば、それも録る。あわせて、MediaMTX のログに publish の資格情報が出るかを確かめる（8章の置き換えが効いているかの確認）

## 13. 既知の制約

- macOS では、本体が強制終了されると MediaMTX が残ることがある
- 本体と外部ツールがほぼ同時にシーンを切り替えると、最後に届いた切替が残る。本体は SetScene を送る直前に承認を取る（9章）ので、人の操作が先に届いていればバッチは失効する。残る窓は「承認から OBS が要求を受け取るまで」の間だけ。その間に人が切り替えた場合は、人の選択が上書きされることがある。もう一度切り替えれば手動保持に入って定着する
- 外部ツール（IRL Link など）にもビットレートによる自動切替の機能があり、それを有効にすると二重に切り替わる。自動切替はどちらか一方だけで使う
- 管理 API を LAN に公開した場合、通信は平文の HTTP と Basic 認証なので、LAN 内で盗聴されるとパスワードと操作を奪われうる。操作はシーンの切替と配信の開始・停止に限られるため、第1段階では TLS を入れず、利用者の判断で LAN 公開を選ぶ形にする
- 本体が要求したのと同じシーンを、人がほぼ同時に選んだ場合は、本体の切替と区別できない（obs-websocket のイベントに発信者が含まれないため）。その場合は手動保持に入らない
- SRT の streamid と RTMP のクエリに含めた資格情報は暗号化されない（SRT のパスフレーズは第1段階では使わない）
- MediaMTX の受信サーバーの設定（ポートなど）や path の認証設定を変えると、配信が切れる。第2段階の設定編集では「配信中に変えると切れる」設定として扱う

## 変更履歴

| 日付 | 版 | 変更 | 理由 | commit |
|---|---|---|---|---|
| 2026-11-18 | v1.0 | 初版。設計セクション1〜5と、外部ツールからの手動切替を尊重する手動保持（6章） | brainstorming での承認。手動保持は IRL Link との競合についてのユーザーの指摘 | 764e12a |
| 2026-11-18 | v1.1 | 管理 API の待ち受けの既定を `127.0.0.1` にし、LAN 公開時の警告を追加（2章、4章、13章）。Judge に実時刻の例の表を追加（5章）。switcher のテストを「入口 × 状態」の表にし、テスト用 JSON に fallback を足さないことを明記（11章）。変更履歴の表を追加 | agentic-fx での裁定と教訓（既定は安全側、整合は状態機械の中心で守る、時刻の契約は実時刻の例で固定する、フィクスチャは実物から作る）を、ユーザーが選んで取り込んだ | 39c8ea4 |
| 2026-11-18 | v1.2 | 読み出しにも認証を必須にした（4章、8章）。実行待ちのバッチを世代番号で失効させ、実行直前に承認を取る（6章、9章）。期待シーンを送信時に登録し、期限をトランジション時間から決め、列で持つ（6章）。配信開始は Intro になってから始める。バッチは失敗した時点で残りを捨てる（6章、7章）。手動保持を再接続で解除しない条件、配信停止で消すこと、Pause の直前の役割の寿命を定義（6章）。Judge の評価順、Kbps の分母、新しい publisher の検出を定義（5章）。MediaMTX の全プロトコルのスイッチを明示し、RTSP を TCP だけにした（8章）。Pdeathsig のスレッド固定、Job Object を本体自身に掛ける方式に変更、ポートの事前確認を補助の診断にした（8章）。名前とパスワードの使える文字を制限（4章）。`cmdErr` の型で 503 と 409 を分け、POST に独自ヘッダーを必須にした（9章）。テスト表の状態の軸を追加（11章）。13章の2項目が変更履歴の後ろにずれていたのを戻した | 設計レビュー1周目（Fable: Critical 1 / Important 8 / Minor 8、codex sol: Critical 1 / Important 10 / Minor 2）。読み出しの認証はユーザーの裁定 | この版の commit |
