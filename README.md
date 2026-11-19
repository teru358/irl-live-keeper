# irl-live-keeper

屋外配信（IRL）で回線が不安定になったら、OBS を待機画面のシーンへ自動で切り替え、回復したら自動で元に戻すツールです。

スマホなどから送られてくる映像の受信（RTMP / SRT）、OBS への取り込み、シーンの自動切替を、1つのプログラムにまとめています。zip を展開して実行するだけで使えることを目指しています。

> **開発中です。** 現在は第1段階（コア）を作っています。下の「できること」のうち、段階の印がついたものはまだ使えません。

## できること

- **受信:** スマホの配信アプリ（Larix Broadcaster、IRL Pro、Moblin など）や OBS から、RTMP と SRT の両方で映像を受け取る。受信には、同梱の [MediaMTX](https://github.com/bluenviron/mediamtx) を使う
- **自動切替:** 受信のビットレートを1秒ごとに見て、次のように切り替える
  - 映像が途切れたら（約2秒）、待機画面（Fail）へ
  - ビットレートが低い状態が続いたら（既定は 800kbps 未満が5秒）、待機画面へ
  - 回復して安定したら（既定は 1500kbps 以上が10秒）、配信画面（Live）へ戻す
- **手動の操作を優先:** OBS の画面や IRL Link などの外部ツールで手動で切り替えたシーンは、打ち消さない。回線の状態が次に変わったときに、自動切替に戻る
- **一時停止:** 手動の待機画面（Pause）。解除すると、そのときの回線の状態に合うシーンへ戻る
- **管理ページ**（第2段階）: 状態とビットレートのグラフ、配信の開始・停止、シーンの操作、設定の編集、送信用 URL の表示
- **オーバーレイ**（第3段階）: 通信状況とビットレートを、OBS のブラウザソースで画面に表示
- **配布**（第4段階）: Windows、macOS、Linux 向けの zip

## 仕組み

```
[スマホの配信アプリ]
        │ RTMP :1935/TCP、SRT :8890/UDP（ルーターで転送するのはこの2つだけ）
        ▼
[MediaMTX]（同梱。irl-live-keeper が起動・監視する）── 映像 ──► [OBS のメディアソース]
        │ 受信のバイト数（API）
        ▼
[irl-live-keeper] ── シーンの切替（obs-websocket）──► [OBS]
```

irl-live-keeper は、MediaMTX が受け取ったバイト数の差分からビットレートを計算します。モバイル回線では、接続が残ったままデータだけが止まることがあります。そのため、切断の通知ではなく「データが届いているか」で途切れを判定します。

## 必要なもの

- OBS Studio 28 以降（obs-websocket v5 が組み込まれている版）
- OBS と同じ PC か、同じ LAN 内の PC（Windows を主に想定。macOS、Linux でも動く予定）

## 使い方（予定）

1. zip を展開し、`irl-live-keeper(.exe)` を実行する。初回は `config.yaml` ができ、送信用 URL とパスワードが表示される
2. OBS の「ツール → WebSocket サーバー設定」でサーバーを有効にし、パスワードを `config.yaml` の `obs.password` に書く
3. OBS に4つのシーンを作る（名前は `config.yaml` で変えられる）

   | シーン | 役割 |
   |---|---|
   | Intro | 配信開始前の画面 |
   | Live | カメラの映像（メディアソースを置く） |
   | Fail | 回線が切れたときの待機画面（自動で切り替わる） |
   | Pause | 手動の一時停止の画面 |

4. Live シーンにメディアソース（名前は `IRL`）を置き、表示された OBS 用 URL を入力する
5. スマホの配信アプリに、表示された送信用 URL を設定する
   - SRT: `srt://<この PC のアドレス>:8890?streamid=publish:live:<ユーザー>:<パスワード>`
   - RTMP: `rtmp://<この PC のアドレス>:1935/live?user=<ユーザー>&pass=<パスワード>`
6. 外から送る場合は、ルーターで 1935/TCP と 8890/UDP だけを、この PC へ転送する

## 設定（config.yaml の主な項目）

| 項目 | 既定値 | 意味 |
|---|---|---|
| `switch.fail_kbps` / `low_sec` | 800 / 5 | これ未満が何秒続いたら待機画面へ |
| `switch.recover_kbps` / `recover_sec` | 1500 / 10 | これ以上が何秒続いたら配信画面へ戻す |
| `switch.down_sec` | 2 | 受信が途切れて何秒で待機画面へ |
| `admin.listen` | `127.0.0.1:8787` | 管理 API の待ち受け。別の PC から操作するときだけ LAN のアドレスにする |
| `obs.scenes.*` | Intro / Live / Fail / Pause | 役割ごとの OBS のシーン名 |

全項目は、初回に作られる `config.yaml` のコメントを見てください。

## セキュリティ

- 送信と読み出しには、どちらもユーザー名とパスワードが必要です（初回に乱数で作ります）
- ルーターで転送するのは 1935/TCP と 8890/UDP だけにしてください。管理 API（8787）、RTSP（8554）、MediaMTX の API（9997）は転送しないでください
- 管理 API を LAN に公開した場合、通信は暗号化されません（起動時に警告を出します）
- SRT の streamid と RTMP の URL に含まれるパスワードは、暗号化されずに送られます

## 開発

```sh
go test -race ./...
```

設計は `docs/superpowers/specs/2026-11-18-core-design.md`、作業の一覧は `docs/superpowers/plans/` にあります。

## ライセンス

[MIT License](LICENSE)

同梱する MediaMTX（MIT License、著作権者 aler9）と、利用しているライブラリのライセンスは [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) を見てください。irl-live-keeper は MediaMTX の作者による公式のソフトウェアではありません。
