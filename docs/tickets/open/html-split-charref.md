---
id: html-split-charref
title: 文字参照を差し込みや if で割ると、保存できたのに表示で誤りになる
status: 設計待ち
priority: 低
kind: 不具合
opened: 2026-11-28
closed: null
related: []
---
# [html-split-charref] 文字参照を差し込みや if で割ると、保存できたのに表示で誤りになる

**状態**: 設計待ち / **優先**: 低 / **種別**: 不具合

## 現象

S2-77 の閉鎖確認（Fable N2）。<div title="&#{if cam1.online}a{end}0;"> や <p>&#{cam1.name}0;</p> は保存の検査を通るが、値によって出力が &#0; になり、組み立てたあとの検査（Go と JS）が html_nul で拒否する（表示が止まる）。危なくはない。保存の検査で & の続きに差し込みや制御タグを置けなくすると閉じる

## 原因

## 処置案・裁定

## 修正内容

## 経緯

- 2026-11-28: 起票。
