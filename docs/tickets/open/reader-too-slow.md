---
id: reader-too-slow
title: OBS の読み取りが追いつかない（reader is too slow）を status に出す
status: 設計待ち
priority: 中
kind: 機能
opened: 2026-11-22
closed: null
related: []
---
# [reader-too-slow] OBS の読み取りが追いつかない（reader is too slow）を status に出す

**状態**: 設計待ち / **優先**: 中 / **種別**: 機能

## 現象

T-15 の1日目（2026-11-22）: IRL Pro（Android）を 1080p・ビットレート自動（約 7,200 kbps）で送ると、MediaMTX が OBS の読み手（RTSP の session）について reader is too slow, discarding N frames を2分で168行出し、OBS の映像がぼやけてノイズが出た。本体は受信のビットレートだけで健康を決めるので OK のままだった。720p・3,000 kbps にすると、ほぼ出なくなった。F-3（2026-11-21、Android 3.8 Mbps、ハードウェアデコードなし）も同じ型

## 原因

## 処置案・裁定

## 修正内容

## 経緯

- 2026-11-22: 起票。
- 2026-11-22: 状態: 設計待ち — 案: MediaMTX のログの行（component=mediamtx の WAR で reader is too slow）を数え、どのカメラの path の読み手かを session から引いて、status の warnings にカメラごとに出す（例:「メインカメラ の映像を OBS が読み切れていません（ビットレートを下げるか、OBS のメディアソースのハードウェアデコードを入れてください）」）。健康（待機へ送るか）には入れない案と、入れる案があり、ユーザーの判断が要る
