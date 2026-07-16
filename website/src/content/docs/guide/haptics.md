---
title: 震动反馈
description: CouchPilot 的四级震动语义与配置方式。
sidebar:
  order: 3
---

震动不是装饰，而是操作是否生效的低干扰确认。

| 反馈 | 常见操作 | 感受 |
| --- | --- | --- |
| 轻触 | A / X 点击 | 很短的 tick |
| 导航 | 方向、标签、菜单 | 清楚但克制的短脉冲 |
| 确认 | 启动语音输入 | 更明显的一次确认 |
| 强确认 | 窗口切换与落定 | 最强的一档反馈 |

## 配置

```json
{
  "haptics_enabled": true,
  "haptic_strength": 1.0
}
```

`haptic_strength` 的有效范围是 `0.0`–`2.0`。不想使用震动时，将 `haptics_enabled` 设为 `false`。

运行 `couchpilot doctor` 时会触发一次明显震动，可用来确认当前连接的手柄是否支持反馈。
