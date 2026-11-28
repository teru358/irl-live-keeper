---
id: template-compile-quadratic
title: テンプレートの Compile が本文の長さの2乗に近い時間がかかる
status: 是正済
priority: 低
kind: 保守
opened: 2026-11-28
closed: 2026-11-28
related: []
---
# [template-compile-quadratic] テンプレートの Compile が本文の長さの2乗に近い時間がかかる

**状態**: 是正済 / **優先**: 低 / **種別**: 保守

## 現象

S2-77 の閉鎖確認（Fable N4）。{if cam1.online}a{end} を 64 KiB 並べると保存の検査が約 150 ms。template.Compile だけで 16.5k/33k/66k バイトが 4.3/16.6/56.8 ms。保存の中で複数回呼ばれる。目標の 50 ms を超えるが、ふつうの本文では問題ない

## 原因

## 処置案・裁定

## 修正内容

- 2026-11-28: template.go の lex で行の頭の位置(lineStart)を読みながら覚え、制御タグのたびの strings.LastIndex をやめた。66KB の {if cam1.online}a{end} 並びで Compile 約37ms から約2ms、保存の検査全体(CompileHTML)約122msから約9.5ms。旧実装との差分比較(20万件のランダム入力)で結果・誤りが同じ

## 経緯

- 2026-11-28: 起票。
- 2026-11-28: 是正内容 — template.go の lex で行の頭の位置(lineStart)を読みながら覚え、制御タグのたびの strings.LastIndex をやめた。66KB の {if cam1.online}a{end} 並びで Compile 約37ms から約2ms、保存の検査全体(CompileHTML)約122msから約9.5ms。旧実装との差分比較(20万件のランダム入力)で結果・誤りが同じ
