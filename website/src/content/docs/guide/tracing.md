---
title: Local diagnostic trace
description: Minimal local evidence for diagnosing controller mappings and dispatch failures.
sidebar:
  order: 2
---

CouchPilot keeps a small local trace by appending one JSON object per diagnostic fact to `trace/trace.jsonl`. Each line records the timestamp, foreground executable base name, matched profile, physical control, resolved action, and dispatch result. The trace is implementation evidence, not a user-facing statistics feature.

Trace data never includes typed text, window titles, full process paths, controller identifiers, or pointer coordinates. It stays on the device and is never uploaded. There is no aggregate store, recovery journal, HTML report, statistics UI, or recommendation engine. The one file is capped at 8 MiB; CouchPilot clears it and starts again at the limit instead of creating rotations or backups.
