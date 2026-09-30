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

Y 后按 A 是所有 App 都可用的全局语音回车流程。按 Y 后需要经过配置的最短等待时间再按 A；过早按 A 会被忽略。Codex 还会额外把 B 变成 **Backspace**：轻按删除一个字符；按住会在短暂等待后稳定连续删除，松开立即停止。移动左摇杆会静默退出语音编辑状态并恢复普通鼠标操作；切换前台 App 或超过配置的等待时间也会自动退出。

:::caution[安全设计]
不按扳机时，X 保持右键。RT+X 会明确发送 Escape，可能停止正在生成的回答。全局 RT+A/B/X/Y 编辑组合在这里同样生效；RT+B 每次按下只删除一次，与语音后长按 B 不同。CouchPilot 不会搜索或点击发送按钮，也不会强制把焦点抢到输入框。
:::
