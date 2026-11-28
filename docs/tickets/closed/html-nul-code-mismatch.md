---
id: html-nul-code-mismatch
title: 閉じない属性値の中の NUL で、Go と JS の誤りの code が違う
status: 是正済
priority: 低
kind: 保守
opened: 2026-11-28
closed: 2026-11-28
related: []
---
# [html-nul-code-mismatch] 閉じない属性値の中の NUL で、Go と JS の誤りの code が違う

**状態**: 是正済 / **優先**: 低 / **種別**: 保守

## 現象

S2-77 の閉鎖確認（Fable N3）。rendered の入力 <div dir="\0{cam1.nam></div> などで Go は html_nul、JS は html_lex。どちらも拒否なので害はない。設計の「NUL は常に拒否」に合わせて JS を html_nul にそろえる

## 原因

## 処置案・裁定

## 修正内容

- 2026-11-28: html.js の validateRendered で、閉じない属性値の残りに NUL があれば html_nul(Go は属性値を読む途中で NUL を見るのと同じ段階)。共通入力表 H514-H516 を Go と JS の両方で確認

## 経緯

- 2026-11-28: 起票。
- 2026-11-28: 是正内容 — html.js の validateRendered で、閉じない属性値の残りに NUL があれば html_nul(Go は属性値を読む途中で NUL を見るのと同じ段階)。共通入力表 H514-H516 を Go と JS の両方で確認
