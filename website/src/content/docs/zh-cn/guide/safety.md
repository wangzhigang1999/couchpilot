---
title: 安全规则
description: CouchPilot 刻意不会执行的高风险自动化行为。
sidebar:
  order: 4
---

这些约束来自真实使用中踩过的坑，因此被当作产品规则保留下来。

## 不会自动抢输入框

切换 App 或移动鼠标时，CouchPilot 不会擅自改变输入焦点。

## A 只在明确的语音编辑状态发送

正常使用时，<kbd>A</kbd> 始终是鼠标左键。在白名单 App 中按 <kbd>Y</kbd> 启动语音输入后，CouchPilot 才会临时把 <kbd>A</kbd> 映射为 Enter，让你主动确认发送。移动鼠标、切换 App、完成发送或超过配置的等待时间，都会恢复普通鼠标操作。

浏览器页面、文档、终端以及白名单以外的所有 App 都不会进入这个状态。

## 不会让 X 停止 Codex

Codex 中 <kbd>X</kbd> 保持鼠标右键，不会发送 Escape，因此不会意外停止正在生成的回答。

## 随时可以紧急退出

同时按住 <kbd>Back + Start</kbd> 1.5 秒，CouchPilot 会立即停止。
