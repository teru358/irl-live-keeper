# S2-0 spike の確認結果（小さな窓・Job の外で開く・CLI・ダークモード・アイコン）

- 実施日: 2026-11-23 19:47〜20:16
- 試しのフォルダ: `C:\Users\terus\Desktop\windows-test2\s2-0`
  - `spike.exe`、`spike-console.exe`: どちらも 3,405,824 バイト（19:41:54）
  - `A2.ico`（28,582 バイト）、`C2.ico`（37,425 バイト）、`icons-preview2.png`（63,039 バイト）
- 始める前の Windows の設定は **ダーク**（`AppsUseLightTheme=0`）。そのため、最初に起動した窓を W-5（ダーク）として見てもらい、W-1（ライト）は、ユーザーにライトへ切り替えてもらってから行った
- 本体、OBS、MediaMTX には触れていない
- ブラウザは Chrome（ユーザーの既定のブラウザ）

## 判定のまとめ

| # | 判定 | 理由 |
|---|---|---|
| W-1 窓（ライト） | 気になる点 | 窓は開き、コンソールの窓は出ない。地は明るく、文字は黒で読みやすい。題名の帯は白。ただし、カメラ3台と OBS の4行が空のままで、1秒ごとの更新が動いていない。ボタンは古い形 |
| W-2 Job の外で開く（ブラウザが動いているとき） | 合格 | 「ブラウザ A」「ブラウザ B」「フォルダ」の3つとも開いた（Chrome、エクスプローラー） |
| W-3 終了 | 合格 | 終了のあと、spike と ping は消え、Chrome のタブ（A と B）とエクスプローラーは残った |
| W-4 Job の外で開く（ブラウザが動いていないとき） | 合格（A）／ B は不可 | A で開いた Chrome は、spike の終了のあとも残った。B で開いた Chrome は、spike の終了と一緒に閉じた |
| W-5 窓（ダーク） | 不合格 | 暗い灰色の地に黒い文字で「とても見づらい」。ボタンは白いまま（古い形）。`DwmSetWindowAttribute` は2つとも `HRESULT 0x80070006` で失敗。4行が空なのはライトと同じ |
| W-6 CLI（`--cli-alloc`） | 合格（文字化けあり） | 新しいコンソールが開き、10行出て終わった。途中の Ctrl+C で止まった（`console stop control=0`）。元の PowerShell はすぐ戻った。ただし、新しいコンソールの文字が化けた |
| W-7 CLI（`--cli-attach`） | 不合格 | 行は同じ端末に出たが、プロンプトが先に戻って出力と混ざった。Ctrl+C はシェルだけが受け取り、spike は 10/10 まで出し続けた。cmd.exe では文字化けした |
| W-8 コンソールの版 | 記録 | 新しいコンソールの窓は出ず、打った PowerShell をそのまま使った。spike を閉じるまでプロンプトは戻らなかった。「終了」で spike の窓は閉じ、PowerShell は残った |
| W-9 アイコン | 記録 | ユーザーの好みは A2。「詳細」の表示でも見分けられる。ほかの案も見たい |

## W-5 窓（ダーク）（最初に起動した窓）

- 19:47:59 `Start-Process .\spike.exe`（PID 56984）。ダークのままだと気づく前に起動したので、この窓をダークの確認に使った
- 19:48:12 `.\spike.exe --theme`（PID 57224）。ログに値を出して、すぐ終わった
- ログ:

```
2026-11-23 19:47:59 AppsUseLightTheme=0; dark=true
2026-11-23 19:47:59 Job created; self assigned: <nil>; ping PID 56760 assigned=<nil>
2026-11-23 19:47:59 DwmSetWindowAttribute(20)=HRESULT 0x80070006
2026-11-23 19:47:59 DwmSetWindowAttribute(19)=HRESULT 0x80070006
2026-11-23 19:48:12 theme AppsUseLightTheme=0
```

- `Job created; self assigned: <nil>; ping PID 56760 assigned=<nil>`: Job ができ、spike 自身と ping を Job に入れることに成功した（エラーが `nil`）
- `DwmSetWindowAttribute(20)` と `(19)`（暗い題名の帯）は、どちらも `0x80070006`（`E_HANDLE`、ハンドルが無効）で失敗した。窓ができる前のハンドルで呼んでいるように見える
- ユーザーの言葉:
  1. 窓は開いた。コンソールの窓は出ていない
  2. 「カメラ、OBS は表示されていない。待機・MediaMTX・起動していますとボタン関係があります」
  3. 1秒ごとに変わるか、色が変わるかは「わからない」
  4. 「暗めの灰色の背景に黒文字で描かれている。とても見づらい」
  5. ボタンは「古い形」。「黒画面に白いボタン」
  6. 文字は読める（にじみ・切れはない）
  7. 案内の欄は「わからない」
  - ユーザーはスクリーンショットを撮った（試しのフォルダには置かれていない）
- 窓の中身を、Windows から部品の文字と位置として読み取った（画面は写していない）。2秒あけて2回読み、同じだった:

```
top rect 1474,276 456x519 client 440x480
Static at 26,51  400x24 ""                          ← カメラ（空）
Static at 26,78  400x24 ""                          ← カメラ（空）
Static at 26,105 400x24 ""                          ← カメラ（空）
Static at 26,132 400x24 ""                          ← OBS（空）
Static at 26,163 400x22 "待機: 自動切替のデモ中"
Static at 26,189 400x22 "MediaMTX: ping PID 56760"
Static at 26,219 400x105 "起動しています"            ← 案内の欄
Button at 26,335 180x26 "管理ページを開く"
Button at 218,335 100x26 "終了"
Button at 26,375 120x26 "ブラウザ A"
Button at 158,375 120x26 "ブラウザ B"
Button at 290,375 120x26 "フォルダ"
```

- 判断: **不合格**
  - カメラ3台と OBS の行の文字の欄はあるが、中身が空で、1秒ごとの更新が動いていない（待機・MediaMTX・案内の文字も、起動したときのまま変わらない）。色の変化も確かめられなかった
  - ダークで、地は暗くなるが文字が黒のままで読みにくい。ボタンは白いまま浮く
  - 題名の帯を暗くする DWM の呼び出しが失敗している。案内の欄にも DWM の結果は出ていない（「起動しています」のまま）

## W-1 窓（ライト）

- ユーザーに、Windows をライトに切り替えてもらった（`AppsUseLightTheme=1` を確かめた）
- 20:02:02 `Start-Process .\spike.exe`（PID 60940、ping は PID 27324）
- ログ:

```
2026-11-23 20:02:02 AppsUseLightTheme=1; dark=false
2026-11-23 20:02:02 Job created; self assigned: <nil>; ping PID 27324 assigned=<nil>
```

- ライトでは DWM の行は出ない
- 部品の文字を2秒あけて2回読んだ: `"" | "" | "" | "" | "待機: 自動切替のデモ中" | "MediaMTX: ping PID 27324" | "起動しています"`。ライトでも4行は空のまま
- ユーザーの言葉:
  1. コンソールの窓は「出ていない」
  2. 「黒文字のため読みやすくはある」
  3. 題名の帯は「白」
  4. 文字のにじみ・切れは「ない」
- ユーザーが「終了」→「はい」で閉じた。spike と ping（PID 27324）は消えた
- 判断: **気になる点**。ライトの見た目は読みやすいが、4行が空で、1秒ごとの更新が動いていないのはダークと同じ。ボタンは古い形

## W-2 Job の外で開く（ブラウザが動いているとき）

- W-5 の窓（PID 56984）で、Chrome が開いている状態で、ユーザーに「ブラウザ A」→「ブラウザ B」→「フォルダ」を押してもらった
- ユーザーの言葉: 「全てで開いた（chrome）」
- ログ:

```
2026-11-23 19:55:01 ブラウザ A: 成功 PID 52524
2026-11-23 19:55:07 ブラウザ B: 成功 (PID は取得できません)
2026-11-23 19:55:11 フォルダ: 成功 PID 58844
```

- PID 52524 と 58844 は、確かめた時にはもう終わっていた（中継ぎの短いプロセス）
- 判断: **合格**

## W-3 終了

- ユーザーが、一度ブラウザ（またはエクスプローラー）を閉じてしまったので、もう一度3つのボタンを押して開き直した:

```
2026-11-23 19:56:06 ブラウザ A: 成功 PID 60632
2026-11-23 19:56:07 ブラウザ B: 成功 (PID は取得できません)
2026-11-23 19:56:07 フォルダ: 成功 PID 60296
```

- そのあと「終了」→「はい」
- ユーザーの言葉: 「全て開いたままです」（Chrome のタブ A と B、エクスプローラー）
- `Get-Process -Id 56984`（spike）と `Get-Process -Id 56760`（ping）は、どちらもなかった
- 「終了」を押したときの行は、ログに出ない
- 判断: **合格**

## W-4 Job の外で開く（ブラウザが動いていないとき）

- ユーザーに、Chrome をすべて閉じてよいか聞き、よいとのことで、Chrome のメニューの「終了」で閉じてもらった。毎回、`Get-Process chrome` が0であることを確かめてから spike を起動した

### A（Job の外で開く）

- 19:58:06 `spike.exe` を起動（PID 56496、ping は PID 58032）
- 「ブラウザ A」→ Chrome が開いた →「終了」→「はい」

```
2026-11-23 19:58:06 AppsUseLightTheme=0; dark=true
2026-11-23 19:58:06 Job created; self assigned: <nil>; ping PID 58032 assigned=<nil>
2026-11-23 19:58:30 ブラウザ A: 成功 PID 52360
```

- ユーザーの言葉: 「残りました」
- spike（PID 56496）と ping（PID 58032）は消えた。Chrome のプロセスは25個残っていた。Chrome の本体（`--type=` のないプロセス、PID 50948、19:58:30 起動）の親は PID 52360（ログの A の PID）だった

### B（ふつうに開く）

- Chrome をもう一度閉じてもらい、0であることを確かめた
- 19:59:18 `spike.exe` を起動（PID 49912、ping は PID 28096）
- 「ブラウザ B」→ Chrome が開いた →「終了」→「はい」

```
2026-11-23 19:59:18 AppsUseLightTheme=0; dark=true
2026-11-23 19:59:18 Job created; self assigned: <nil>; ping PID 28096 assigned=<nil>
2026-11-23 20:00:02 ブラウザ B: 成功 (PID は取得できません)
```

- ユーザーの言葉: 「消えました」
- spike（PID 49912）、ping（PID 28096）とも消えた。Chrome のプロセスも0になった

- 判断: **A は合格。B は、ブラウザが動いていないときに、spike の終了と一緒にブラウザを閉じてしまう**（Job の中で起動されたため）

## W-6 CLI（`--cli-alloc`）

- 私の PowerShell から `.\spike.exe --cli-alloc` を2回動かした。どちらも 0.01 秒で PowerShell に戻った（GUI の exe なので待たない）
- 1回目（PID 59340）: Ctrl+C を押さずに最後まで
  - ユーザーの言葉: 「表示されていました。行数までは数えられませんでした。その後とじました」
  - ログ:

```
2026-11-23 20:04:12 AllocConsole called
2026-11-23 20:04:12 stdout reopened as CONOUT$
2026-11-23 20:04:12 stdin reopened as CONIN$
2026-11-23 20:04:12 cli status 1/10
（1秒ごと）
2026-11-23 20:04:22 cli status 10/10
```

- 2回目（PID 58908）: 途中で Ctrl+C
  - ユーザーの言葉: 「文字化けしていましたが、違う文字が表示されたことは確認できました。窓もとじました」
  - ログ:

```
2026-11-23 20:05:04 AllocConsole called
2026-11-23 20:05:04 stdout reopened as CONOUT$
2026-11-23 20:05:04 stdin reopened as CONIN$
2026-11-23 20:05:04 cli status 1/10
2026-11-23 20:05:05 cli status 2/10
2026-11-23 20:05:06 cli status 3/10
2026-11-23 20:05:07 cli status 4/10
2026-11-23 20:05:08 console stop control=0
```

- 判断: **合格（文字化けあり）**
  - 新しいコンソールの窓が開き、10行出て終わった。Ctrl+C で止まった（`control=0` は CTRL_C_EVENT）
  - 新しいコンソールで、日本語が化けた。新しいコンソールは、日本語の Windows の既定の文字コード（932）で開き、spike が UTF-8 で書いているためと見られる

## W-7 CLI（`--cli-attach`）

- ユーザーが自分の端末で動かした

### PowerShell

- 1回目と2回目（20:07:24、20:07:39）: ユーザーは、10行が出終わったあとで Ctrl+C を押していた（シェルの新しいプロンプトが出ただけ）
- ユーザーが貼った2回目の出力（プロンプトのあとに行が続いた）:

```
PS C:\Users\terus\Desktop\windows-test2\s2-0> 20:07:39 status 1/10: メインカメラ ● OK 2,450 kbps (SRT)
20:07:40 status 2/10: メインカメラ ● OK 2,450 kbps (SRT)
…
20:07:49 status 10/10: メインカメラ ● OK 2,450 kbps (SRT)
```

- ユーザーの言葉: 「ログが流れて停止させたあと、入力位置が出力されたログに重なっていました」
- 3回目（20:11:41）: 3〜4行目で Ctrl+C を押してもらった。ユーザーの言葉: 「止まりませんでした」。ユーザーが貼った出力:

```
PS C:\Users\terus\Desktop\windows-test2\s2-0> 20:11:41 status 1/10: メインカメラ ● OK 2,450 kbps (SRT)
20:11:42 status 2/10: メインカメラ ● OK 2,450 kbps (SRT)
20:11:43 status 3/10: メインカメラ ● OK 2,450 kbps (SRT)
20:11:44 status 4/10: メインカメラ ● OK 2,450 kbps (SRT)
                                              ^C
PS C:\Users\terus\Desktop\windows-test2\s2-0> 20:11:45 status 5/10: メインカメラ ● OK 2,450 kbps (SRT)
20:11:46 status 6/10: メインカメラ ● OK 2,450 kbps (SRT)
…
20:11:50 status 10/10: メインカメラ ● OK 2,450 kbps (SRT)
```

- ログでは、3回とも `AttachConsole(ATTACH_PARENT_PROCESS) succeeded` のあと `cli status 10/10` まで出ていて、`console stop` の行はない
- PowerShell では文字化けしなかった

### cmd.exe

- 20:09:09 に動かした。ユーザーの言葉: 「停止できた。文字化けしている」。ユーザーが貼った出力:

```
C:\Users\terus\Desktop\windows-test2\s2-0>20:09:09 status 1/10: 繝｡繧､繝ｳ繧ｫ繝｡繝ｩ 笳・OK 2,450 kbps (SRT)
…
20:09:17 status 9/10: 繝｡繧､繝ｳ繧ｫ繝｡繝ｩ 笳・OK 2,450 kbps (SRT)
```

- ログでは、20:09:18 に `cli status 10/10` まで出ていて、止める操作の行はない。止まったように見えたのは、出力が終わったためと見られる

- 判断: **不合格**
  - プロンプトが先に戻り、出力がプロンプトのあとに混ざる。出力のあと、入力の位置が出力に重なる
  - Ctrl+C はシェルだけが受け取り、spike には届かない（止められない）
  - cmd.exe（文字コード 932）では文字化けする。PowerShell では化けなかった

## W-8 コンソールの版

- ユーザーが自分の PowerShell から `.\spike-console.exe` を動かした（20:14:00 と 20:15:07 の2回。ログに出ている）
- ユーザーの言葉:
  - 「窓しか出ない」（新しいコンソールの窓は出ず、打った PowerShell をそのまま使った）
  - 「戻らない。窓を閉じると戻る」
  - 「窓は閉じる。コマンドを打ったコンソールはそのまま」
- ログ:

```
2026-11-23 20:14:00 AppsUseLightTheme=0; dark=true
2026-11-23 20:14:00 Job created; self assigned: <nil>; ping PID 65252 assigned=<nil>
2026-11-23 20:14:00 DwmSetWindowAttribute(20)=HRESULT 0x80070006
2026-11-23 20:14:00 DwmSetWindowAttribute(19)=HRESULT 0x80070006
2026-11-23 20:15:07 AppsUseLightTheme=0; dark=true
2026-11-23 20:15:07 Job created; self assigned: <nil>; ping PID 62992 assigned=<nil>
2026-11-23 20:15:07 DwmSetWindowAttribute(20)=HRESULT 0x80070006
2026-11-23 20:15:07 DwmSetWindowAttribute(19)=HRESULT 0x80070006
```

- この時、ユーザーは Windows をダークに戻していた
- 判断: **記録**。エクスプローラーからダブルクリックしたときの動き（新しいコンソールの窓が出るか）は確かめていない

## W-9 アイコン

- `icons-preview2.png` の中身:
  - A2: 紺の角丸に、緑の電波の棒3本と、左上に赤い丸
  - C2: 紺の角丸に、緑の電波の弧2本と、下に赤い丸
  - 明るい地と暗い地、大きさ5段
- ユーザーの言葉: 「A2が好み、詳細でも見分けられる、赤点と電波の棒が離れているので見やすい、他案があれば見たい」
- 判断: **記録**。A2

## 片づけ

- `Get-Process spike, spike-console` は空
- ログに出た ping の PID 6つ（56760、58032、28096、27324、65252、62992）は、すべて消えていた。私が止めたプロセスはない（spike の終了で、Job ごと消えた）
- Windows のテーマ: 最初はダーク。W-1 のためにユーザーがライトにし、W-8 の前にユーザーがダークに戻した（最後の `AppsUseLightTheme=0`）

## 意見

### 1. windigo の窓で、本番の小さな窓（spec の図）を作れそうか

作れると考える。ただし、直すべき所が4つある。

- **1秒ごとの更新が動いていない。** カメラ3台と OBS の4行が空のまま、待機・MediaMTX・案内の欄も起動したときのまま変わらなかった（ライトでもダークでも）。タイマーが動いていないか、文字を入れる処理が UI のスレッドに届いていないように見える。本番の窓のいちばん大事な所なので、先に直して確かめる必要がある
- **ボタンが古い形。** Windows の今の形（Common Controls v6）にするには、exe にマニフェストを入れる必要があるはず
- **ダークが読めない。** 地は暗くなるが、文字は黒のまま（`WM_CTLCOLORSTATIC` などで文字の色を変えていない）。ボタンは白いまま浮く。Win32 の標準のボタンは、正式にはダークにならない。ダークは、地と文字だけをそろえてボタンは明るいままにするか、ボタンを自分で描くかを決める必要がある
- **題名の帯を暗くする DWM の呼び出しが失敗している**（`0x80070006`、ハンドルが無効）。窓ができる前に呼んでいるように見える。窓ができたあと（`WM_CREATE` のあとなど）に呼べば効くと思う

ライトの見た目は「黒文字のため読みやすくはある」で、文字のにじみや切れもなかった。窓の大きさ（456×519）と部品の並びはそのまま使えそう。

### 2. ブラウザとフォルダを開く方法は A（Job の外）でよいか

A でよい。B は使えない。

- ブラウザが動いていないときに B で開くと、ブラウザが Job の中で起動され、spike（本番では本体）の終了と一緒に閉じた。ユーザーの作業を壊す
- A は、ブラウザが動いている・いないのどちらでも、終了のあとも残った
- フォルダ（エクスプローラー）は、ブラウザが動いているときに A の方法で開き、終了のあとも残った

### 3. CLI は `--cli-alloc` と `--cli-attach` のどちらがよいか

`--cli-alloc` がよい。

- `--cli-attach` は、プロンプトが先に戻って出力と混ざり、Ctrl+C が spike に届かず、止められなかった。CLI としては使いにくい
- `--cli-alloc` は、別の窓になるが、出力が混ざらず、Ctrl+C で止まった
- どちらの方法でも、コンソールの文字コードが 932 のときに日本語が化けた（alloc の新しい窓、attach の cmd.exe）。`SetConsoleOutputCP(65001)` を呼ぶか、`WriteConsoleW` で書くとよい
- ほかの道として、W-8 のコンソールの版は、打った端末をそのまま使い、閉じるまでプロンプトが戻らなかった（ふつうのコマンドと同じ動き）。CLI 用に、コンソールの版の exe を別に置く方法も考えられる

### 4. アイコンはどちらの案か

A2。ユーザーの好みで、「詳細」の表示（小さいとき）でも見分けられる。赤い点と電波の棒が離れていて見やすい、とのこと。ユーザーは、ほかの案があれば見たい。

## spike-log.txt の全文

```
2026-11-23 19:47:59 AppsUseLightTheme=0; dark=true
2026-11-23 19:47:59 Job created; self assigned: <nil>; ping PID 56760 assigned=<nil>
2026-11-23 19:47:59 DwmSetWindowAttribute(20)=HRESULT 0x80070006
2026-11-23 19:47:59 DwmSetWindowAttribute(19)=HRESULT 0x80070006
2026-11-23 19:48:12 theme AppsUseLightTheme=0
2026-11-23 19:55:01 ブラウザ A: 成功 PID 52524
2026-11-23 19:55:07 ブラウザ B: 成功 (PID は取得できません)
2026-11-23 19:55:11 フォルダ: 成功 PID 58844
2026-11-23 19:56:06 ブラウザ A: 成功 PID 60632
2026-11-23 19:56:07 ブラウザ B: 成功 (PID は取得できません)
2026-11-23 19:56:07 フォルダ: 成功 PID 60296
2026-11-23 19:58:06 AppsUseLightTheme=0; dark=true
2026-11-23 19:58:06 Job created; self assigned: <nil>; ping PID 58032 assigned=<nil>
2026-11-23 19:58:06 DwmSetWindowAttribute(20)=HRESULT 0x80070006
2026-11-23 19:58:06 DwmSetWindowAttribute(19)=HRESULT 0x80070006
2026-11-23 19:58:30 ブラウザ A: 成功 PID 52360
2026-11-23 19:59:18 AppsUseLightTheme=0; dark=true
2026-11-23 19:59:18 Job created; self assigned: <nil>; ping PID 28096 assigned=<nil>
2026-11-23 19:59:18 DwmSetWindowAttribute(20)=HRESULT 0x80070006
2026-11-23 19:59:18 DwmSetWindowAttribute(19)=HRESULT 0x80070006
2026-11-23 20:00:02 ブラウザ B: 成功 (PID は取得できません)
2026-11-23 20:02:02 AppsUseLightTheme=1; dark=false
2026-11-23 20:02:02 Job created; self assigned: <nil>; ping PID 27324 assigned=<nil>
2026-11-23 20:04:12 AllocConsole called
2026-11-23 20:04:12 stdout reopened as CONOUT$
2026-11-23 20:04:12 stdin reopened as CONIN$
2026-11-23 20:04:12 cli status 1/10
2026-11-23 20:04:13 cli status 2/10
2026-11-23 20:04:14 cli status 3/10
2026-11-23 20:04:15 cli status 4/10
2026-11-23 20:04:16 cli status 5/10
2026-11-23 20:04:17 cli status 6/10
2026-11-23 20:04:18 cli status 7/10
2026-11-23 20:04:20 cli status 8/10
2026-11-23 20:04:21 cli status 9/10
2026-11-23 20:04:22 cli status 10/10
2026-11-23 20:05:04 AllocConsole called
2026-11-23 20:05:04 stdout reopened as CONOUT$
2026-11-23 20:05:04 stdin reopened as CONIN$
2026-11-23 20:05:04 cli status 1/10
2026-11-23 20:05:05 cli status 2/10
2026-11-23 20:05:06 cli status 3/10
2026-11-23 20:05:07 cli status 4/10
2026-11-23 20:05:08 console stop control=0
2026-11-23 20:07:24 AttachConsole(ATTACH_PARENT_PROCESS) succeeded
2026-11-23 20:07:24 stdout reopened as CONOUT$
2026-11-23 20:07:24 stdin reopened as CONIN$
2026-11-23 20:07:24 cli status 1/10
2026-11-23 20:07:25 cli status 2/10
2026-11-23 20:07:26 cli status 3/10
2026-11-23 20:07:27 cli status 4/10
2026-11-23 20:07:28 cli status 5/10
2026-11-23 20:07:29 cli status 6/10
2026-11-23 20:07:30 cli status 7/10
2026-11-23 20:07:31 cli status 8/10
2026-11-23 20:07:32 cli status 9/10
2026-11-23 20:07:33 cli status 10/10
2026-11-23 20:07:39 AttachConsole(ATTACH_PARENT_PROCESS) succeeded
2026-11-23 20:07:39 stdout reopened as CONOUT$
2026-11-23 20:07:39 stdin reopened as CONIN$
2026-11-23 20:07:39 cli status 1/10
2026-11-23 20:07:40 cli status 2/10
2026-11-23 20:07:41 cli status 3/10
2026-11-23 20:07:42 cli status 4/10
2026-11-23 20:07:43 cli status 5/10
2026-11-23 20:07:44 cli status 6/10
2026-11-23 20:07:45 cli status 7/10
2026-11-23 20:07:46 cli status 8/10
2026-11-23 20:07:47 cli status 9/10
2026-11-23 20:07:49 cli status 10/10
2026-11-23 20:09:09 AttachConsole(ATTACH_PARENT_PROCESS) succeeded
2026-11-23 20:09:09 stdout reopened as CONOUT$
2026-11-23 20:09:09 stdin reopened as CONIN$
2026-11-23 20:09:09 cli status 1/10
2026-11-23 20:09:10 cli status 2/10
2026-11-23 20:09:11 cli status 3/10
2026-11-23 20:09:12 cli status 4/10
2026-11-23 20:09:13 cli status 5/10
2026-11-23 20:09:14 cli status 6/10
2026-11-23 20:09:15 cli status 7/10
2026-11-23 20:09:16 cli status 8/10
2026-11-23 20:09:17 cli status 9/10
2026-11-23 20:09:18 cli status 10/10
2026-11-23 20:11:41 AttachConsole(ATTACH_PARENT_PROCESS) succeeded
2026-11-23 20:11:41 stdout reopened as CONOUT$
2026-11-23 20:11:41 stdin reopened as CONIN$
2026-11-23 20:11:41 cli status 1/10
2026-11-23 20:11:42 cli status 2/10
2026-11-23 20:11:43 cli status 3/10
2026-11-23 20:11:44 cli status 4/10
2026-11-23 20:11:45 cli status 5/10
2026-11-23 20:11:46 cli status 6/10
2026-11-23 20:11:47 cli status 7/10
2026-11-23 20:11:48 cli status 8/10
2026-11-23 20:11:49 cli status 9/10
2026-11-23 20:11:50 cli status 10/10
2026-11-23 20:14:00 AppsUseLightTheme=0; dark=true
2026-11-23 20:14:00 Job created; self assigned: <nil>; ping PID 65252 assigned=<nil>
2026-11-23 20:14:00 DwmSetWindowAttribute(20)=HRESULT 0x80070006
2026-11-23 20:14:00 DwmSetWindowAttribute(19)=HRESULT 0x80070006
2026-11-23 20:15:07 AppsUseLightTheme=0; dark=true
2026-11-23 20:15:07 Job created; self assigned: <nil>; ping PID 62992 assigned=<nil>
2026-11-23 20:15:07 DwmSetWindowAttribute(20)=HRESULT 0x80070006
2026-11-23 20:15:07 DwmSetWindowAttribute(19)=HRESULT 0x80070006
```
