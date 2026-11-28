---
id: template-compile-quadratic
title: テンプレートの Compile が本文の長さの2乗に近い時間がかかる
status: 設計待ち
priority: 低
kind: 保守
opened: 2026-11-28
closed: null
related: []
---
# [template-compile-quadratic] テンプレートの Compile が本文の長さの2乗に近い時間がかかる

**状態**: 設計待ち / **優先**: 低 / **種別**: 保守

## 現象

S2-77 の閉鎖確認（Fable N4）。{if cam1.online}a{end} を 64 KiB 並べると保存の検査が約 150 ms。template.Compile だけで 16.5k/33k/66k バイトが 4.3/16.6/56.8 ms。保存の中で複数回呼ばれる。目標の 50 ms を超えるが、ふつうの本文では問題ない

## 原因

## 処置案・裁定

## 修正内容

## 経緯

- 2026-11-28: 起票。
