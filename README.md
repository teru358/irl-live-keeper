# irl-live-keeper

屋外配信（IRL）で回線が不安定になったら、OBS を待機画面のシーンへ自動で切り替え、回復したら自動で元に戻すツールです。

スマホなどから送られてくる映像の受信（RTMP / SRT）、OBS への取り込み、シーンの自動切替を、1つのプログラムにまとめています。zip を展開して実行するだけで使えることを目指しています。

> **開発中です。** 現在は第1段階（コア）を作っています。下の「できること」のうち、段階の印がついたものはまだ使えません。

## できること

- **受信:** スマホの配信アプリ（Larix Broadcaster、IRL Pro、Moblin など）や OBS から、RTMP と SRT の両方で映像を受け取る。受信には、同梱の [MediaMTX](https://github.com/bluenviron/mediamtx) を使う
- **自動切替:** 受信のビットレートを1秒ごとに見て、カメラ映像を使うシーン（監視対象のシーン）にいる間、次のように切り替える
  - 映像が途切れたら（約2秒）、待機シーンへ
  - ビットレートが低い状態が続いたら（既定は 800kbps 未満が5秒）、待機シーンへ
  - 回復して安定したら（既定は 1500kbps 以上が10秒）、待機シーンに入る前のシーンへ戻す。IRL Link などでカメラのシーンを切り替えていた場合も、そのとき使っていたシーンへ戻る
- **外部ツールとの併用:** 普段のシーン操作は IRL Link や OBS で行う前提。回線が悪い間にカメラのシーンへ切り替えると、待機シーンへ送り、回復したらそのシーンへ戻す。あえて乱れた映像を出したいときは、自動切替を OFF にする
- **配信開始時の切替**（任意、既定は OFF）: 配信が始まったら、指定したシーン（配信開始画面など）へ切り替える
- **テスト用の手動操作:** 配信の開始・停止と、手動の待機 ON/OFF
- **管理ページ**（第2段階）: 状態とビットレートのグラフ、配信の開始・停止、シーンの操作、設定の編集、送信用 URL の表示
- **オーバーレイ**（第3段階）: OBS のテキストソースを書き換えて、通信状況とビットレートを配信画面に出す
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
3. OBS に待機シーンを作り、`config.yaml` の `obs.standby_scene` にその名前を書く。カメラ映像を使うシーンの名前は `obs.monitored_scenes` に並べる（第2段階では、管理ページで OBS のシーン一覧から選べるようにする）
   - 待機シーンの名前は「⚠回線待機中」のように、見てすぐわかる名前にしておくと、IRL Link の画面で待機に入ったことに気づける
4. カメラ映像を使うシーンにメディアソース（名前は `IRL`）を置き、表示された OBS 用 URL を入力する
   - 「ローカルファイル」は外し、FFmpeg のオプションに `rtsp_transport=tcp` を入れる
   - 「アクティブになったときに再生を再開」は解除する（有効のままだと、回線の回復で本ツールがカメラのシーンへ戻した直後に再生がやり直され、2秒ほど黒い画面が出る。解除しても、IRL Link などで人がカメラのシーンへ切り替えたときに映像はすぐ出る）
   - 「再接続までの遅延」は2秒を推奨する。カメラの接続が確立してから OBS の画面に映像が出るまでの時間は、この値に依存する（回線の回復で本ツールが自動で戻すときは、切り替える前にメディアソースを再起動するので、この値によらない）
5. スマホの配信アプリに、表示された送信用 URL を設定する。URL の host は、起動時にこの PC の LAN のアドレスを自動で調べて入れる。屋外から送るときは、host をポート転送やトンネル、VPN のアドレスに置き換える。`config.yaml` の `stream.publish_host` に書けば、そのアドレスで固定できる
   - SRT: `srt://<この PC のアドレス>:8890?streamid=publish:live:<ユーザー>:<パスワード>`
   - RTMP: `rtmp://<この PC のアドレス>:1935/live?user=<ユーザー>&pass=<パスワード>`
6. 外から送る場合は、ルーターで 1935/TCP と 8890/UDP だけを、この PC へ転送する

## LAN の中継 PC に置く場合

- 基本の置き方は、OBS と同じ配信 PC に本ツールと MediaMTX を置くこと。本ツールと MediaMTX を LAN の別の PC（中継 PC）に置き、OBS は配信 PC に置く構成にも対応する
- `config.yaml`（中継 PC 側）の設定
  - `obs.host`: 配信 PC のアドレス（obs-websocket は 4455）
  - `stream.obs_read_host`: 中継 PC の LAN のアドレス（配信 PC の OBS が RTSP を読みに行く先）
  - ルーターのポート転送で外側のポートを変えたときは、`stream.publish_rtmp_port` / `stream.publish_srt_port` に外側のポートを書く
- ファイアウォール（どちらも LAN からだけ許可する）
  - 配信 PC: obs-websocket の 4455/TCP
  - 中継 PC: RTSP の 8554/TCP
- LAN の通信は暗号化されない。obs-websocket と、OBS の RTSP の読み出し（read の資格情報を含む）は、LAN 内で盗聴されうる。信頼できない人がいる LAN では使わない
- 外部のサーバー（VPS など）に置くことは、VPN（Tailscale など）越しなら技術的には可能だが、正式には対応しない。インターネットへ直接は通さない

## 設定（config.yaml の主な項目）

| 項目 | 既定値 | 意味 |
|---|---|---|
| `switch.fail_kbps` / `low_sec` | 800 / 5 | これ未満が何秒続いたら待機画面へ |
| `switch.recover_kbps` / `recover_sec` | 1500 / 10 | これ以上が何秒続いたら配信画面へ戻す |
| `switch.down_sec` | 2 | 受信が途切れて何秒で待機画面へ |
| `stream.publish_host` | 空 | 送信用 URL の host。空なら LAN のアドレスを自動で調べる。転送先やトンネルのアドレスが決まっていれば書く |
| `stream.obs_read_host` | 空 | OBS 用 URL の host。空なら `127.0.0.1`（配信 PC に置く場合）。LAN の中継 PC に置くときは中継 PC の LAN のアドレスを書く |
| `stream.publish_rtmp_port` / `publish_srt_port` | `0` / `0` | 送信用 URL に出す RTMP と SRT のポート。0 なら受信のポート（`mediamtx.rtmp_port` / `srt_port`）と同じ。ルーターのポート転送で外側のポートを変えたときに書く |
| `admin.listen` | `127.0.0.1:8787` | 管理 API の待ち受け。別の PC から操作するときだけ LAN のアドレスにする |
| `obs.standby_scene` | `Standby` | 回線が悪い間に出す待機シーン |
| `obs.monitored_scenes` | `[Camera]` | カメラ映像を使うシーン（自動切替の対象） |
| `switch.auto_enabled` | `true` | 自動切替の ON/OFF |
| `on_stream_start.enabled` / `scene` | `false` / 空 | 配信開始時に切り替えるか、どのシーンへ |

全項目は、初回に作られる `config.yaml` のコメントを見てください。

## セキュリティ

- 送信と読み出しには、どちらもユーザー名とパスワードが必要です（初回に乱数で作ります）
- ルーターで転送するのは 1935/TCP と 8890/UDP だけにしてください。管理 API（8787）、RTSP（8554）、MediaMTX の API（9997）は転送しないでください
- 管理 API を LAN に公開した場合、通信は暗号化されません（起動時に警告を出します）
- SRT の streamid と RTMP の URL に含まれるパスワードは、暗号化されずに送られます

## 既知の制約

- OBS の WebSocket の接続はできたが、認証が終わる前に失敗する状態（OBS が固まって応答しない、認証の途中で接続が切れる）が続くと、再接続のたびに本ツールの中に小さな処理（goroutine）が1つずつ残る（1時間に数百、数 MB）。使っているライブラリ goobs の不具合で、上流に報告済み（[andreykaipov/goobs#277](https://github.com/andreykaipov/goobs/issues/277)）。OBS が起動していないときや、パスワードが違うときは起きない。気になる場合は本ツールを再起動する
- 詳しくは設計の文書の 13章を見てください

## 開発

```sh
go test -race ./...
```

設計は `docs/superpowers/specs/2026-11-18-core-design.md`、作業の一覧は `docs/superpowers/plans/` にあります。

## ライセンス

[MIT License](LICENSE)

同梱する MediaMTX（MIT License、著作権者 aler9）と、利用しているライブラリのライセンスは [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) を見てください。irl-live-keeper は MediaMTX の作者による公式のソフトウェアではありません。
