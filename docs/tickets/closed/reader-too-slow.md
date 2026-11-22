---
id: reader-too-slow
title: OBS の読み取りが追いつかない（reader is too slow）を status に出す
status: 是正済
priority: 中
kind: 機能
opened: 2026-11-22
closed: 2026-11-22
related: []
---
# [reader-too-slow] OBS の読み取りが追いつかない（reader is too slow）を status に出す

**状態**: 是正済 / **優先**: 中 / **種別**: 機能

## 現象

T-15 の1日目（2026-11-22）: IRL Pro（Android）を 1080p・ビットレート自動（約 7,200 kbps）で送ると、MediaMTX が OBS の読み手（RTSP の session）について reader is too slow, discarding N frames を2分で168行出し、OBS の映像がぼやけてノイズが出た。本体は受信のビットレートだけで健康を決めるので OK のままだった。720p・3,000 kbps にすると、ほぼ出なくなった。F-3（2026-11-21、Android 3.8 Mbps、ハードウェアデコードなし）も同じ型

## 原因

## 処置案・裁定

## 修正内容

- 2026-11-22: supervisor が MediaMTX の行（is reading from path / destroyed・closed / reader is too slow）から読み手の id と path を引いて数え、30秒に3回以上で「追いつかない」、最後から60秒で解くようにした。core が path をカメラに引き、status の obs.warnings にカメラごとに1行出す（既存の警告と並べる）。健康・switcher には渡さない（ユーザーの裁定 2026-11-22）。出たときと解けたときに obs reader too slow / caught up を INFO で出す。go vet（Linux・Windows）と go test -race ./... が通った

## 経緯

- 2026-11-22: 起票。
- 2026-11-22: 状態: 設計待ち — 案: MediaMTX のログの行（component=mediamtx の WAR で reader is too slow）を数え、どのカメラの path の読み手かを session から引いて、status の warnings にカメラごとに出す（例:「メインカメラ の映像を OBS が読み切れていません（ビットレートを下げるか、OBS のメディアソースのハードウェアデコードを入れてください）」）。健康（待機へ送るか）には入れない案と、入れる案があり、ユーザーの判断が要る
- 2026-11-22: 是正内容 — supervisor が MediaMTX の行（is reading from path / destroyed・closed / reader is too slow）から読み手の id と path を引いて数え、30秒に3回以上で「追いつかない」、最後から60秒で解くようにした。core が path をカメラに引き、status の obs.warnings にカメラごとに1行出す（既存の警告と並べる）。健康・switcher には渡さない（ユーザーの裁定 2026-11-22）。出たときと解けたときに obs reader too slow / caught up を INFO で出す。go vet（Linux・Windows）と go test -race ./... が通った
