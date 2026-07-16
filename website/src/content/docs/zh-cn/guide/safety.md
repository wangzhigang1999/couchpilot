---
title: 安全规则
description: CouchPilot 刻意不会执行的高风险自动化行为。
sidebar:
  order: 4
---

这些约束来自真实使用中踩过的坑，因此被当作产品规则保留下来。

## 不会自动抢输入框

切换 App 或移动鼠标时，CouchPilot 不会擅自改变输入焦点。

## 不会用 A 自动发消息

在 QQ、微信等聊天应用中，<kbd>A</kbd> 始终是鼠标左键，不会映射为 Enter。

## 不会让 X 停止 Codex

Codex 中 <kbd>X</kbd> 保持鼠标右键，不会发送 Escape，因此不会意外停止正在生成的回答。

## 随时可以紧急退出

同时按住 <kbd>Back + Start</kbd> 1.5 秒，CouchPilot 会立即停止。
