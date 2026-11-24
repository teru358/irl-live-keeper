# S2-19 区切り1の実機の確認（Linux）

2026-11-24、この開発機（Linux、systemd のユーザーのサービス）で確かめた。1935 番は system の nginx が使っているので、`--dir ~/irl-svc-test` に config.yaml を作り、ポートを 28787 / 21935 / 28890 / 28554 / 29997 に替えた（mediamtx の実行ファイルは置かず、OBS も動かしていない。どちらも「つながらない」警告が出るだけで、本体は動き続けた）。終わったあと、サービスと置き場所は消した。

| 項目 | 結果 |
|---|---|
| `service install --user --dir` | 合格。`~/.config/systemd/user/irl-live-keeper.service` を書き出した（`ExecStart=... --dir ...`、`Restart=on-failure`、`KillSignal=SIGTERM`、`WantedBy=default.target`）。案内（`systemctl --user` の手順、linger、管理ページの URL は config の `admin.port`、`admin.lan`、VPN）が出た |
| 常駐（`enable --now`） | 合格。`active`。journal に起動の表示が出て、systemd から起動したので URL のパスワードは `***`、「管理ページか config.yaml で確かめる」の案内（S2-7） |
| journal にパスワードが出ない | 合格。config.yaml の `admin.password`、`publish_pass`、`read_pass` の値が、journal に0回（値は表示せずに数えた）。ログの文は英語（S2-56） |
| `systemctl --user restart` | 合格。`Shutdown requested` → `shutdown complete` を通って止まり、新しい PID で起動した |
| 強制終了（SIGKILL）からの起動し直し | 合格。`Restart=on-failure` で起動し直した（`NRestarts=1`） |
| `systemctl --user stop` | 合格。止める手順のログが出た |
| `service uninstall --user` | 合格。定義と自動起動のリンクが消え、`Unit irl-live-keeper.service could not be found` |
| `--dir` | 合格。config.yaml・logs が置き場所に、mediamtx は実行ファイルのフォルダを見た（S2-7） |
| `service install --system --run-as` | 未実施（`sudo` が要る。書き出す定義と、ユーザーの決め方・root のエラーは単体テストで確かめている） |

気になった文言（あとで直す一覧に足す）: 起動の表示の「屋外から送るときは、host をポート転送やトンネルのアドレスに置き換える」は古い。今は Tailscale と、管理ページの送信の設定で住所を選ぶ形を勧めている（spec 第14節）。
