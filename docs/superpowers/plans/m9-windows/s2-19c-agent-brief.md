# S2-19 の確かめ直し 2回目（S2-19b で残った所だけ。Windows の Claude Code 向けの指示書）

前回（`../s2-19b/s2-19b-agent-brief.md` と `s2-19b-result.md`）の続き。**前提・守ることは `../s2-19/s2-19-agent-brief.md` のとおり**（パスワードを書かない、config.yaml はあなたは書き換えない、止めてよいのは自分が起動したものだけ〔PID で〕、本体と OBS はユーザーが起動・終了する）。ユーザーへの文は日本語。

ユーザーに、本体を終了してから、このフォルダ（`s2-19c`）の `irl-live-keeper.exe` を本体のフォルダの exe と入れ替えてもらう。

## 確かめること

| # | 手順 | 確かめること |
|---|---|---|
| C-1 管理ページのログ（前回の R-9） | 管理ページ（PC）でログアウトしてログインし直し、シーンを2回切り替えてもらう | ログに `admin logout`、`admin login`（`auth=password`）、`scene selected from admin page`（`scene=<シーンの名前>`）が出ること。窓の「管理ページを開く」で開いたときは `admin login`（`auth=passphrase`）。パスワードや合言葉が出ないこと |
| C-2 回線の行 | 窓を見てもらう | 「メインカメラ (main)」のように名前と id が出ること |
| C-3 起動の失敗の文 | `powershell -ExecutionPolicy Bypass -File .\occupy-ports.ps1 -Target rtmp` を裏で起動（PID を控える）してから本体を起動してもらう | 案内の文が窓の下端で切れず、窓が文に合わせて広がること。スクリプトを PID で止めて「もう一度試す」→ 動き出し、窓が元の高さに戻ること。ログに `retry requested` と `restart complete` |
| C-4 再起動のログ | 本体が動いている状態で再起動（前回の R-6 のとおり） | ログに `restart requested`、`generation stopped`（`reason=restart`）、`restart complete` が出て、`shutdown complete` が**出ない**こと |
| C-5 終了の確認 | config.yaml の `gui.close_action` は `quit` のまま。窓の「終了」を押してもらう | 「終了」では**確認が出る**こと（「いいえ」で残る）。「×」は確認なしで終わること（`quit` の動き） |
| C-6 `--cli` の Ctrl+C | PowerShell から `.\irl-live-keeper.exe --cli`。動き出したら、開いたコンソールの窓で Ctrl+C を**1回だけ**押し、**5秒待って**もらう | 1回で、`Shutdown requested` と `shutdown complete` の行が出てから閉じるか。閉じるまでの秒数（ユーザーに数えてもらう）。閉じなければ、そのとき出ていた最後の行 |
| C-7 管理ページの OBS の表示 | 管理ページを開いたまま、OBS を終了してもらい、また起動してもらう | OBS を閉じたら、シーンの一覧が消えて「OBS に未接続」が出ること。詳しい状態の OBS の行が、文字と色で未接続を示すこと。「OBS に未接続」が2か所に出ないこと。OBS を起動し直したら一覧が戻ること（スマホでも） |

## 記録

このフォルダに `s2-19c-result.md` を作り、項目ごとに、ユーザーの言葉、ログの行、あなたの判断を書く。ユーザーに、できたことを伝える。
