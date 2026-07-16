---
title: Codex
description: ChatGPT 与 Codex 桌面版的任务导航和开发快捷键。
sidebar:
  order: 2
  badge: 高频
---

<span class="process-chip">ChatGPT.exe · OpenAI.Codex</span>

任务切换、命令菜单、终端和完整的语音发送流程都放在最顺手的位置。

| 按键 | 动作 | 键盘快捷键 |
| --- | --- | --- |
| <kbd>B</kbd> | 返回 | Ctrl + [ |
| <kbd>LB</kbd> | 上一个任务 | Ctrl + Shift + [ |
| <kbd>RB</kbd> | 下一个任务 | Ctrl + Shift + ] |
| <kbd>L3</kbd> | 命令菜单 | Ctrl + K |
| <kbd>R3</kbd> | 打开终端 | Ctrl + ` |
| <kbd>X</kbd> | 鼠标右键 | 不发送 Escape |
| <kbd>Y</kbd>，然后 <kbd>A</kbd> | 语音输入、检查并发送 | 右 Alt，然后 Enter |
| 按过 <kbd>Y</kbd> 后，轻按或按住 <kbd>B</kbd> | 删除一个或连续删除 | Backspace |
| <kbd>RT</kbd> + <kbd>A</kbd> | 随时发送 | Enter |

Y 启动语音输入后，CouchPilot 会临时把 A 变成**发送**、把 B 变成 **Backspace**。轻按 B 删除一个字符；按住 B 会在短暂等待后稳定连续删除，松开立即停止。移动左摇杆会静默退出语音编辑状态并恢复普通鼠标操作；切换前台 App 或超过配置的等待时间也会自动退出。

:::caution[安全设计]
X 永远保持右键，避免误停正在生成的回答。CouchPilot 不会搜索或点击发送按钮，也不会强制把焦点抢到输入框。
:::
