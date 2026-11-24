# S2-19b 確かめ直しの結果（直した所だけ）

- 実施日: 2026-11-24 19:07〜19:43
- 本体: `irl-live-keeper.exe`（2026-11-23 23:14:27、15,310,336 バイト）。`windows-test2` の exe は、始める前にユーザーが入れ替えていた（`s2-19b` のものとハッシュが同じことを確かめた）。`ctl.exe` は前回のまま
- `occupy-ports.ps1` は前回（`s2-19`）と同じ中身（ハッシュが同じ）
- config.yaml は前回のまま（`admin.lan: true`、`gui.close_action: quit`）。私は開いていない
- Windows はダーク。Chrome はユーザーの既定のブラウザ
- スクリーンショット: ユーザーが `Pictures\Screenshots` に保存したものを、`s2-19b` に写した（`R-1-190804.png`、`R-4-起動の失敗.png`）
- 窓の中身は、Windows から部品の文字と位置として読み取った

## 判定のまとめ

| # | 判定 | 理由 |
|---|---|---|
| R-1 窓のボタン | 合格（気になる点あり） | 4つのボタンとも反応した（R-2〜R-4 で使った）。Tab で選択の枠が移り、Enter でも押せた。左上のアイコンは A3。kbps に桁区切りが付き、DOWN のときの空の `()` は出ない。ただし、回線の行に camera id（`(main)`）は出ていない |
| R-2 管理ページを開く | 合格 | マウスでも、Tab と Enter でも、ログインを通らずに状態の画面が開いた |
| R-3 終了とブラウザ | 合格 | Chrome を全部閉じてから、ボタンで Chrome を起動させ、「終了」で本体を終えた。Chrome は残った。本体と MediaMTX は残らず、`crash-*.log` は終了のときに消えた |
| R-4 起動の失敗 | 合格（気になる点あり） | 窓が残り、見出し・文が直っていた。「ログを開く」でログのフォルダが開いた。ポートを空けて「もう一度試す」で動き出した。案内の文が長く、窓の下端で切れる |
| R-5 `--cli` の Ctrl+C | 合格（気になる点あり） | Ctrl+C でも、コンソールの「×」でも、`Shutdown requested` と `shutdown complete` がログに出た。MediaMTX は残らない。Ctrl+C は2回押さないと閉じなかったかもしれない（ユーザー） |
| R-6 再起動のログ | 合格 | `restart requested` と `restart complete` が出た。間に `shutdown complete code=0` も出る |
| R-7 二重の起動のログ | 合格 | 前の窓が前に出て、`second instance blocked` が出た |
| R-8 管理ページの直し | 合格（気になる点あり） | 「OBS に未接続」は、OBS を閉じると左上に出て、起動すると消えた（PC・スマホ）。言語の印、住所を替えても画面がずれない、住所の候補は2つだけ。OBS を閉じてもシーンの一覧が残る |
| R-9 管理ページのログ | 不合格 | `scene selected from admin page`、`admin logout`、`admin login` が、ログに出なかった |

## R-1 窓のボタン

- 19:07:05 ユーザーが起動（本体 PID 39532、MediaMTX PID 39696）。19:07:09 OBS 起動（PID 39896）、19:07:18 `connected to OBS`
- ログ: `19:07:06.425 level=WARN msg="admin.lan is true. The admin API and page are open to the LAN. …"`
- `logs\crash-20261124-190706.log`（0 バイト）があった。前日の `crash-*.log` は残っていなかった
- 窓の文字:

```
"管理ページを開く" | "終了" | [隠れている] "ログを開く" | [隠れている] "もう一度試す" | ""
"回線 メインカメラ  ● DOWN  0 kbps"
"回線 サブカメラ  ● DOWN  0 kbps"
"OBS  ● 接続  配信停止中  Camera"
"待機  なし"
"MediaMTX  ● 動作中"
```

- ユーザーの言葉: 「tab の挙動確認」「ウインドウ左上のアイコン確認」
- `R-1-190804.png`: 左上に A3 のアイコン。ボタン「管理ページを開く」「終了」（ダークで白いボタン。「終了」に選択の枠）
- 桁区切り: ユーザーが iPhone から sub を送った（19:41:02 `DOWN → LOW kbps=1479.4`、19:41:15 `LOW → OK kbps=2004`）。ユーザーの言葉: 「桁区切り記号ついています」
- 判断: **合格（気になる点あり）**
  - ボタンが反応しない問題は直った（R-2 で「管理ページを開く」をマウスと Tab・Enter で、R-3 で「終了」を、R-4 で「ログを開く」と「もう一度試す」を押した）
  - 空の `()` は出なくなった
  - 回線の行は `メインカメラ` だけで、指示書の例の `メインカメラ (main)` の形ではない

## R-2 管理ページを開く（K-5）

- 窓の「管理ページを開く」をマウスで押した → ログインなしで状態の画面が開いた
- Tab で「管理ページを開く」に枠を合わせて Enter → もう1つ開いた
- ユーザーの言葉: 「ログインなしで管理画面が開きました」「tab+enter でも開きました」
- このとき Chrome はすでに動いていた（18:41:24 起動、PID 5828）ので、タブが足されただけ。ブラウザが残るかは、R-3 で Chrome を閉じてから確かめた
- 管理ページを開いたことは、ログに出ない
- 判断: **合格**

## R-3 終了とブラウザ（K-8）

- ユーザーに、Chrome をメニューの「終了」ですべて閉じてもらい、`Get-Process chrome` が0であることを確かめた
- 窓の「管理ページを開く」で Chrome を起動させた（Chrome の本体 PID 29972、19:34:06、親は PID 41232〔ボタンが起動した中継ぎ〕）
- 窓の「終了」を押した。`gui.close_action: quit` なので、確認は出なかった（ユーザー「終了モードは quit だったので、確認は出ていません」）
- ログ:

```
19:34:14.942 level=INFO msg="Shutdown requested"
19:34:14.948 level=INFO msg="shutdown complete" code=0
```

- ユーザーの言葉: 「残りました」（Chrome）
- 終了のあと: 本体も MediaMTX もない。Chrome のプロセスは25。**`crash-*.log` は1つもなかった**（終了のときに消えた）
- 判断: **合格**
- メモ: `quit` のときは、窓の「終了」ボタンでも確認が出ない（「×」と同じ扱い）。指示書の「確認で『はい』」とは違うが、設定どおりと見られる

## R-4 起動の失敗（K-9）

- 19:35:09 `powershell -ExecutionPolicy Bypass -File .\occupy-ports.ps1 -Target rtmp` を裏で起動（PID 34152）。`0.0.0.0:1935` を PID 34152 が待ち受けていることを確かめた
- 19:35:53 ユーザーが本体を起動（PID 44732）
- ログ:

```
19:35:53.875 level=ERROR msg="port precheck failed" error="Cannot use mediamtx.rtmp_port port 1935 (tcp4). Check whether another program (such as a previous irl-live-keeper or mediamtx) is using this port: listen tcp4 0.0.0.0:1935: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted."
```

- 窓の文字と位置（窓は 540×408）:

```
at 20,43  "回線  —" / "OBS  —" / "待機  —" / "MediaMTX  —"
at 20,159 480x26 "管理ページを開く"（灰色、押せない）
at 20,189 480x26 "終了"
at 20,219 480x26 "ログを開く"
at 20,249 480x26 "もう一度試す"
at 20,283 480x100 "起動できませんでした / mediamtx.rtmp_port のポート 1935（tcp4）を使えません。ほかのプログラム（前回の irl-live-keeper や mediamtx など）がこのポートを使っていないか確かめてください: listen tcp4 0.0.0.0:1935: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted."
```

- ユーザーの言葉: 1.「窓は残ります」 2.「その通りの内容です」 3.「ログフォルダが開きました」
- ユーザーの言葉（気になる点）: 「表示された文字が長く、ウインドウの下端が見切れています」
  - `R-4-起動の失敗.png`: 最後の行（`address/port) is normally permitted.`）が窓の下端で切れている。文の欄（y=283〜383）が、窓の中に収まっていない
- 19:38:44 スクリプト（PID 34152）を PID で止めた。1935 の待ち受けはなくなった
- ユーザーが「もう一度試す」を押した → ユーザーの言葉: 「表示が戻り、通常起動時と同じ表示になりました」
- ログ:

```
19:39:12.268 level=WARN msg="admin.lan is true. …"
19:39:12.431 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
19:39:12.717 INF MediaMTX v1.21.4, windows, amd64
```

- MediaMTX PID 47132（19:39:12）。本体は PID 44732 のまま
- 判断: **合格（気になる点あり）**
  - 案内の文が窓の下端で切れる
  - 「もう一度試した」ことを示すログの行がない

## R-5 `--cli` の Ctrl+C（K-11）

- 19:41:48 GUI の本体（44732）をユーザーが「×」で閉じた（`Shutdown requested` → `shutdown complete code=0`）
- ユーザーが自分の PowerShell から `.\irl-live-keeper.exe --cli` を2回動かした
- 1回目（Ctrl+C）:

```
19:42:08.419 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
19:42:08.469 INF MediaMTX v1.21.4, windows, amd64
19:42:26.479 level=INFO msg="Shutdown requested"
19:42:26.483 level=INFO msg="shutdown complete" code=0
```

- 2回目（コンソールの窓の「×」）:

```
19:42:37.552 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
19:42:37.603 INF MediaMTX v1.21.4, windows, amd64
19:42:45.378 level=INFO msg="Shutdown requested"
19:42:45.383 level=INFO msg="shutdown complete" code=0
```

- ユーザーの言葉: 「2回、指示通りに実施。ctrlC は2回打たないととじなかったような気がします」
- 終わったあと、本体も MediaMTX も残っていない。`crash-*.log` も残っていない
- 判断: **合格（気になる点あり）**。Ctrl+C が1回で閉じない（2回要る）かもしれない。1回目で `Shutdown requested` が出て、窓が閉じるのを待つ形なのかは、確かめていない

## R-6 再起動のログ

- 管理ページに再起動のボタンはないので、ユーザーの PowerShell 7 で:

```powershell
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8787/api/restart -Headers @{'X-IRL-Live-Keeper'='1'} -Credential (Get-Credential admin) -Authentication Basic -AllowUnencryptedAuthentication
```

- 応答: `ok: True`
- ログ:

```
19:12:58.020 level=INFO msg="restart requested"
19:12:58.028 level=INFO msg="shutdown complete" code=0
19:12:58.029 level=WARN msg="admin.lan is true. …"
19:12:58.096 level=INFO msg="restart complete"
19:12:58.101 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
19:12:58.172 INF MediaMTX v1.21.4, windows, amd64
```

- 本体は PID 39532 のまま、MediaMTX は 39696 → 48828
- 判断: **合格**。ただし、`restart requested` と `restart complete` の間に `shutdown complete code=0` も出ていて、本体が止まったようにも読める

## R-7 二重の起動のログ

- 窓を最小化して、もう一度 exe をダブルクリック
- ユーザーの言葉: 「最小化後、実行ファイルをダブルクリックで全面に表示されます」
- ログ: `19:11:02.533 level=INFO msg="second instance blocked"`
- 本体は PID 39532 の1つだけ
- 判断: **合格**

## R-8 管理ページの直し

- ユーザーの言葉:
  1. 「OBS を起動しているので、OBS に未接続が根本的に消えているのか、つながっていて消えているのか区別できません。表示は PC でもスマホでもありません」
  2. 「PC、スマホどちらも印がついた」（言語の切り替え）
  3. 「どちらもずれません」（送信の設定で住所を替えたとき）
  4. 「どちらも2つだけです」（住所の候補。LAN と Tailscale）
- 1 を確かめ直すため、ユーザーに OBS を閉じて起動し直してもらった（2回）。ログ:

```
19:25:25.184 level=WARN msg="OBS connection lost"
19:27:57.214 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
19:28:04.796 level=WARN msg="OBS connection lost"
19:28:31.807 level=INFO msg="connected to OBS" addr=127.0.0.1:4455
```

- ユーザーの言葉:
  - 「(OBS を閉じると) どちらでも、左上に表示されています」「obs 起動で非表示になりました。2回ほど実行しています」
  - 「管理画面を開いている状態で OBS をとじても、シーン一覧や未接続の表示が消えません。右上の OBS の状態は未接続になっています」
  - 「詳しい状態と OBS に未接続の表示という同じ内容が重複しているので、詳しい状態側のつながっている・つながっていない等の表示（文言は後で修整）の色を変えて表示し、未接続はオミットで良いのではと思います」
- 判断: **合格（気になる点あり）**
  - 「OBS に未接続」は、つながっていないときだけ出て、つながると消える。スマホでも下のバーに隠れない
  - OBS を閉じても、シーンの一覧が残る
  - OBS の状態が、詳しい状態の欄と「未接続」の表示の2か所に重なっている

## R-9 管理ページのログ

- ユーザーが PC の管理ページで、19:20 前後にログアウトしてログインし直し、19:21 にシーンを2回切り替えた
- ログに出たのは、次の2行だけ:

```
19:21:49.411 level=INFO msg="scene changed by user" from=Camera to=Camera2
19:21:52.627 level=INFO msg="scene changed by user" from=Camera2 to=⚠回線待機中
```

- `scene selected from admin page`、`admin logout`、`admin login` は出ていない。ログのファイルを `admin|login|logout|selected` で探しても、`admin.lan` の WARN しかない
- ほかに書かれたログのファイルはない（`logs\` は今日のログと `crash-*.log` だけ）
- exe の中には、`scene selected from admin page`、`admin logout`、`admin login` の文字がある。動いていた exe は `s2-19b` のものと同じ（ハッシュで確かめた）
- 判断: **不合格**。ログの段階（DEBUG など）で出ていないか、出す所が呼ばれていないと見られる

## 片づけ

- 私が起動したもの: `occupy-ports.ps1`（PID 34152）。19:38:44 に PID で止めた
- 本体は、R-5 の2回目（19:42:45）で終わっている。本体も MediaMTX も残っていない
- 設定はそのまま（config.yaml の `admin.lan: true`、`gui.close_action: quit`、Windows はダーク）
- OBS（PID 45772）と Chrome は、ユーザーのもの

## 残った気になる所（あとで直したい所）

1. **R-9: 管理ページのログアウト・ログイン・シーンの切り替えが、ログに出ない**（不合格）
2. 回線の行に camera id（`(main)`）が出ない（指示書の例と違う）
3. 起動の失敗の案内の文が長く、窓の下端で切れる
4. `--cli` の Ctrl+C が、1回で閉じないかもしれない（ユーザーの感じ）
5. 再起動のログに `shutdown complete code=0` も出て、本体が止まったように読める
6. 「もう一度試す」を押したことが、ログに出ない
7. 管理ページ: OBS を閉じても、シーンの一覧が残る
8. 管理ページ: OBS の状態が2か所に重なる。詳しい状態の側を色で分けて、「OBS に未接続」の表示はなくしてもよい（ユーザーの提案、文言はあとで直す）
9. `close_action: quit` のときは、「終了」ボタンでも確認が出ない（設定どおりなら、指示書の書き方を合わせる）
10. ダークのとき、ボタンが白いまま（前回からの点。`R-1-190804.png`）
