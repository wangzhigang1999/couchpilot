---
title: Haptic feedback
description: CouchPilot's four haptic levels and configuration options.
sidebar:
  order: 3
---

Haptics are low-distraction confirmation that an action really happened, not decoration.

| Feedback | Typical actions | Feel |
| --- | --- | --- |
| Light tick | A / X click | A very short tick |
| Navigation | Direction, tab, menu | A clear but restrained pulse |
| Confirmation | Start voice input | A more noticeable confirmation |
| Strong confirmation | Window switching and commit | The strongest feedback level |

## Configuration

```json
{
  "haptics_enabled": true,
  "haptic_strength": 1.0
}
```

`haptic_strength` accepts values from `0.0` to `2.0`. Set `haptics_enabled` to `false` to disable all vibration.

Running `couchpilot doctor` sends one noticeable pulse so you can verify that the connected controller supports feedback.
