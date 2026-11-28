---
id: html-nul-code-mismatch
title: 閉じない属性値の中の NUL で、Go と JS の誤りの code が違う
status: 設計待ち
priority: 低
kind: 保守
opened: 2026-11-28
closed: null
related: []
---
# [html-nul-code-mismatch] 閉じない属性値の中の NUL で、Go と JS の誤りの code が違う

**状態**: 設計待ち / **優先**: 低 / **種別**: 保守

## 現象

S2-77 の閉鎖確認（Fable N3）。rendered の入力 <div dir="\0{cam1.nam></div> などで Go は html_nul、JS は html_lex。どちらも拒否なので害はない。設計の「NUL は常に拒否」に合わせて JS を html_nul にそろえる

## 原因

## 処置案・裁定

## 修正内容

## 経緯

- 2026-11-28: 起票。
