# S2-19c 確かめ直し 2回目の結果（S2-19b で残った所だけ）

- 実施日: 2026-11-24 19:58〜20:17
- 本体: `irl-live-keeper.exe`（2026-11-24 19:54:04、15,317,504 バイト）。`windows-test2` の exe は、始める前にユーザーが入れ替えていた（`s2-19c` のものとハッシュが同じことを確かめた）
- `occupy-ports.ps1` は前回と同じ中身（ハッシュが同じ）
- config.yaml は前回のまま（`admin.lan: true`、`gui.close_action: quit`）。私は開いていない（C-1 で、パスワードがログに出ていないかを確かめるためだけに、スクリプトで値を読み、表示せずに比べた）
- Windows はダーク
- 項目は、本体の終了が要るものを後ろに回して、C-2 → C-1 → C-7 → C-4 → C-5 → C-3 → C-6 の順で行った
- 窓の中身は、Windows から部品の文字と位置として読み取った

## 判定のまとめ

| # | 判定 | 理由 |
|---|---|---|
| C-1 管理ページのログ | 合格 | `admin login`（`auth=passphrase` と `auth=password`）、`admin logout`、`scene selected from admin page`（`scene=…`）が出た。パスワードはログに出ていない |
| C-2 回線の行 | 合格 | `回線 メインカメラ (main)`、`回線 サブカメラ (sub)` と出た |
| C-3 起動の失敗の文 | 合格（気になる点あり） | 文は最後まで窓に収まった。「もう一度試す」で動き出し、`retry requested` と `restart complete` が出た。見出しのあとが改行されず、記号（`↑` のような文字）が出る |
| C-4 再起動のログ | 合格 | `restart requested`、`generation stopped reason=restart`、`restart complete` が出て、`shutdown complete` は出ない |
| C-5 終了の確認 | 合格 | 「終了」では確認が出て、「いいえ」で残った。「×」は確認なしで終わった |
| C-6 `--cli` の Ctrl+C | 合格 | Ctrl+C 1回で、`Shutdown requested` と `shutdown complete` が出て閉じた |
| C-7 管理ページの OBS の表示 | 合格（気になる点あり） | OBS を閉じるとシーンの一覧が消えて「OBS に未接続」が出て、起動し直すと戻った（PC・スマホ）。スマホのシーン一覧の「OBS に未接続」の文言は変える余地がある |

## C-2 回線の行

- 19:58:48 ユーザーが本体を起動（PID 44696、MediaMTX PID 54548）。19:58:54 OBS 起動（PID 33972）、19:59:02 `connected to OBS`
- 窓（540×431）の文字:

```
"管理ページを開く" | "終了" | [隠れている] "ログを開く" | [隠れている] "もう一度試す" | ""
"回線 メインカメラ (main)  ● DOWN  0 kbps"
"回線 サブカメラ (sub)  ● DOWN  0 kbps"
"OBS  ● 接続  配信停止中  ⚠回線待機中"
"待機  なし"
"MediaMTX  ● 動作中"
```

- 判断: **合格**

## C-1 管理ページのログ（前回の R-9）

- ユーザーが PC で、窓の「管理ページを開く」で開き → ログアウト → パスワードでログイン → 管理ページからシーンを2回切り替えた
- ユーザーの言葉: 「おわりました」
- ログ:

```
19:59:48.891 level=INFO msg="admin login" source=local auth=passphrase
19:59:53.007 level=INFO msg="admin logout" source=local auth=passphrase
20:00:01.920 level=INFO msg="admin login" source=local auth=password
20:00:22.702 level=INFO msg="scene selected from admin page" scene=Camera2 scene_uuid=e79d4275-e3fe-47cf-900d-a557c68fddeb source=local
20:00:23.016 level=INFO msg="scene changed by user" from=⚠回線待機中 to=Camera2
20:00:25.799 level=INFO msg="scene selected from admin page" scene=Camera scene_uuid=c374bdee-8b72-4634-8c24-319a1856734d source=local
20:00:26.116 level=INFO msg="scene changed by user" from=Camera2 to=Camera
```

- config.yaml のパスワードの値（2つ）が、この間のログの行に含まれていないことを、スクリプトで確かめた（値は表示していない。含む行は0）
- 判断: **合格**
- メモ:
  - 管理ページから切り替えると、約0.3秒後に `scene changed by user`（OBS で人が切り替えたときと同じ行）も出て、1回の操作で2行になる
  - ログアウトの行は、ログインした方法（`auth=passphrase`）を示している

## C-7 管理ページの OBS の表示

- 管理ページを PC とスマホで開いたまま、ユーザーが OBS を終了し、起動し直した
- ログ:

```
20:05:13.497 level=INFO msg="admin login" source=lan auth=password    ← スマホ
20:05:47.961 level=WARN msg="OBS connection lost"
20:07:04.977 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
```

- OBS は PID 57732（20:06:58）に替わった
- ユーザーの言葉: 「1、2 とも問題なし」。気になる点: 「スマホ版シーン一覧に OBS 停止中に OBS に未接続の文言、変更の余地あり」
- 判断: **合格（気になる点あり）**。OBS を閉じるとシーンの一覧が消えて「OBS に未接続」が出て、詳しい状態の OBS の行も未接続を示し、2か所に重ならない。起動し直すと一覧が戻る。スマホのシーン一覧に出る文言は、あとで直す余地がある

## C-4 再起動のログ

- ユーザーの PowerShell 7 で:

```powershell
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8787/api/restart -Headers @{'X-IRL-Live-Keeper'='1'} -Credential (Get-Credential admin) -Authentication Basic -AllowUnencryptedAuthentication
```

- ログ:

```
20:09:20.670 level=INFO msg="restart requested"
20:09:20.676 level=INFO msg="generation stopped" reason=restart
20:09:20.677 level=WARN msg="admin.lan is true. …"
20:09:20.729 level=INFO msg="restart complete"
20:09:20.733 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
20:09:20.796 INF MediaMTX v1.21.4, windows, amd64
```

- `shutdown complete` は出ていない
- 本体は PID 44696 のまま、MediaMTX は 54548 → 59092
- 判断: **合格**

## C-5 終了の確認

- `gui.close_action: quit` のまま
- ユーザーの言葉: 「1、2 確認通りです」（「終了」では確認が出て「いいえ」で残った。「×」は確認なしで終わった）
- ログ: `20:10:38.568 level=INFO msg="Shutdown requested"`、`20:10:38.579 level=INFO msg="shutdown complete" code=0`
- 終わったあと、本体も MediaMTX も残っていない。`crash-*.log` もない
- 判断: **合格**

## C-3 起動の失敗の文

- 20:11:03 `powershell -ExecutionPolicy Bypass -File .\occupy-ports.ps1 -Target rtmp` を裏で起動（PID 56292）。`0.0.0.0:1935` を PID 56292 が待ち受けていることを確かめた
- 20:11:59 ユーザーが本体を起動（PID 59968）
- ログ: `20:11:59.416 level=ERROR msg="port precheck failed" error="Cannot use mediamtx.rtmp_port port 1935 (tcp4). Check whether another program …"`
- 窓は 540×423（ふだんの起動のときは 540×431、前回の R-4 では 540×408 で文が切れていた）。案内の欄は y=283〜403
- ユーザーの言葉: 「最後まで表示されています。起動できませんでしたの後は改行したほうがいいと思います」
- スクリーンショット `C-3-201222.png`:
  - 文は最後の `address (protocol/network address/port) is normally permitted.` まで、窓の中に見えている
  - 「起動できませんでした」のすぐあとに、`↑` のような記号が1文字出て、改行されずに `mediamtx.rtmp_port のポート 1935（tcp4）を使えません。…` が続いている。前回（R-4）は見出しのあとで改行されていた
  - 「管理ページを開く」は灰色（押せない）
- 20:14:06 スクリプト（PID 56292）を PID で止めた。1935 の待ち受けはなくなった
- ユーザーが「もう一度試す」を押した。ユーザーの言葉: 「再開しました。窓の高さは変わっていないと思います」
- ログ:

```
20:14:57.896 level=INFO msg="retry requested"
20:14:57.897 level=WARN msg="admin.lan is true. …"
20:14:58.044 level=INFO msg="restart complete"
20:14:58.049 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
20:14:58.104 INF MediaMTX v1.21.4, windows, amd64
```

- 「もう一度試す」のあとの窓は 540×431（ふだんの高さ）。MediaMTX は PID 58932
- 判断: **合格（気になる点あり）**
  - 文は切れずに収まった。ただ、失敗のときの窓（423）は、ふだんの窓（431）より少し低い。「文に合わせて広がる」というより、ふだんの大きさの中に収まった形で、ユーザーには高さの変化がわからなかった
  - 見出しのあとの改行が、記号になって改行されない。窓の文字の欄は、改行に `\r\n` が要るためと見られる

## C-6 `--cli` の Ctrl+C

- 20:16:12 GUI の本体（59968）をユーザーが「×」で閉じた（`Shutdown requested` → `shutdown complete code=0`）
- ユーザーが自分の PowerShell から `.\irl-live-keeper.exe --cli` を動かし、Ctrl+C を1回押した
- ログ:

```
20:16:20.126 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
20:16:20.179 INF MediaMTX v1.21.4, windows, amd64
20:16:29.157 level=INFO msg="Shutdown requested"
20:16:29.161 level=INFO msg="shutdown complete" code=0
```

- ユーザーの言葉: 「1回でとじました」（秒数は数えていないが、5秒待たずに閉じた）
- 終わったあと、本体も MediaMTX も残っていない。`crash-*.log` もない
- 判断: **合格**

## 片づけ

- 私が起動したもの: `occupy-ports.ps1`（PID 56292）。20:14:06 に PID で止めた
- 本体は C-6 で終わっている。本体も MediaMTX も残っていない
- 設定はそのまま（config.yaml の `admin.lan: true`、`gui.close_action: quit`、Windows はダーク）
- OBS（PID 57732）は、ユーザーのもの

## 残った気になる所（あとで直したい所）

1. 起動の失敗の文で、見出し「起動できませんでした」のあとが改行されず、記号（`↑` のような文字）が出る（前回は改行されていた）
2. 起動の失敗のとき、窓は文に合わせて広がらない（ふだんの 431 より少し低い 423）。文は収まっている
3. スマホのシーン一覧で、OBS が止まっている間に出る「OBS に未接続」の文言は変える余地がある
4. 管理ページからシーンを切り替えると、`scene selected from admin page` のあとに `scene changed by user` も出て、1回の操作で2行になる
