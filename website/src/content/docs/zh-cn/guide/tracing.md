---
title: 本地诊断 trace
description: 用于排查手柄映射和动作派发失败的最小本地证据。
sidebar:
  order: 2
---

CouchPilot 会保留一份很小的本地 trace，把每条诊断事实作为一行 JSON 追加到 `trace/trace.jsonl`。每行只记录时间、前台程序的可执行文件名、命中的 profile、物理按键、解析后的动作和派发结果。这是内部诊断证据，不是面向用户的统计功能。

Trace 不会记录输入文字、窗口标题、完整进程路径、手柄编号或鼠标坐标；数据只留在本机，绝不会上传。它没有聚合存储、恢复日志、HTML 报表、统计 UI 或推荐引擎。唯一的文件上限为 8 MiB；到达上限后会清空并重新开始，不生成轮转文件或备份。

## 找到 trace 文件

Trace 默认开启。`trace/trace.jsonl` 相对于当前使用的配置目录，不一定在可执行文件所在的目录。打开 Windows 托盘或 macOS 菜单栏菜单，选择**打开配置目录**，其中的 `trace` 文件夹就是记录所在的位置。

如果从命令行通过 `--config` 指定了配置，就以该配置文件所在的目录为准。双击 macOS 的 `CouchPilot.app` 时，默认 trace 位置为 `~/Library/Application Support/CouchPilot/trace/trace.jsonl`。

## 关闭记录

1. 在上述目录中打开当前使用的配置文件，通常是 `config.json`。
2. 将顶层的 `local_trace_enabled` 属性设为 `false`。如果没有这个属性，请在保持 JSON 格式合法的前提下添加。保留其他设置，不要用这一个属性替换整个配置文件。
3. 保存文件，然后退出并重新启动 CouchPilot，继续使用同一份配置。设置只在启动时读取，仅保存文件不会让正在运行的实例停止记录。

关闭记录会在重启后停止追加，但不会删除已有的 trace 数据。如果要清除旧记录，请先退出 CouchPilot，再仅删除当前配置目录中的 `trace/trace.jsonl`。若不希望下次启动重新生成文件，请保持记录关闭。需要恢复记录时，将 `local_trace_enabled` 改回 `true` 并再次重启。
