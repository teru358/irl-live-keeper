# S2-33 区切り2の実機の確認（Linux）

2026-11-26、この開発機（Linux、画面なしの使い方を想定）で確かめた。systemd には登録せず、本体を直接動かした（journal に出るのは本体の標準出力と標準エラーと同じ中身なので、それを読んだ）。1935 番は system の nginx が使っているので、`--dir` の確かめ用の置き場所でポートを替え（管理ページ 28787 など）、`admin.lan: true`、`admin.password` を入れた。「LAN の機器から」は、この PC の LAN のアドレスに Basic 認証で要求を送って代わりにした（相手のアドレスが loopback でないので `source=lan` になる）。終わったあと、置き場所は消した。

| 項目 | 結果 |
|---|---|
| LAN から設定を読む（`GET /api/settings`） | 合格。200、`local=false` |
| LAN からすぐ反映の項目を保存（`switch.low_sec`） | 合格。202 `apply=immediate`。再起動しない。ログ `settings saved paths=[switch.low_sec] apply=immediate source=lan` |
| LAN から同じ PC だけの項目（`log.dir`） | 合格。403 `settings.local_only`。ログ `settings operation failed code=settings.local_only source=lan` |
| 使用中のポート（nginx の 1935）に変える保存 | 合格。422、`fields` の `mediamtx.rtmp_port` に理由 |
| LAN から再起動して反映の項目を保存（`mediamtx.rtsp_port`） | 合格。202 `apply=restart`。ログ `settings saved … apply=restart` → `restart requested` → `generation stopped reason=restart` → `restart complete kind=restart` |
| 世代の作成の失敗（config.yaml を直接 1935 に書き換えて `POST /api/restart`） | 合格。ログ（英語）に `port precheck failed` と `restart failed` の理由、続けてコンソールに日本語の理由と「config.yaml.bak から戻すには、config.yaml にコピーしてから再起動してください。」。**終了コード 1** で終わった |

あわせて（2026-11-26、chromium-devtools で）: すぐ反映の保存のあと「保存して反映しました。」が、状態が届くたびの全体の警告で上書きされて見えなかった不具合を直した（eb8b8e5。S2-33c の「保存のあと何も知らせが出ない」の原因）。
