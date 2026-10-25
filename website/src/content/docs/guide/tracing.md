---
title: Local diagnostic trace
description: Minimal local evidence for diagnosing controller mappings and dispatch failures.
sidebar:
  order: 2
---

CouchPilot keeps a small local trace by appending one JSON object per diagnostic fact to `trace/trace.jsonl`. Each line records the timestamp, foreground executable base name, matched profile, physical control, resolved action, and dispatch result. The trace is implementation evidence, not a user-facing statistics feature.

Trace data never includes typed text, window titles, full process paths, controller identifiers, or pointer coordinates. It stays on the device and is never uploaded. There is no aggregate store, recovery journal, HTML report, statistics UI, or recommendation engine. The one file is capped at 8 MiB; CouchPilot clears it and starts again at the limit instead of creating rotations or backups.

## Find the trace file

Tracing is enabled by default. The path `trace/trace.jsonl` is relative to the active configuration directory, not necessarily the folder containing the executable. Open the Windows tray or macOS menu-bar menu and choose **打开配置目录** (open configuration folder); the `trace` folder is inside that directory.

If you launch from the command line with `--config`, use the directory containing that configuration file. When you double-click `CouchPilot.app` on macOS, the default trace location is `~/Library/Application Support/CouchPilot/trace/trace.jsonl`.

## Turn tracing off

1. Open the active configuration file, usually `config.json`, in the directory above.
2. Set its top-level `local_trace_enabled` property to `false`. If the property is absent, add it while keeping the JSON valid. Preserve the other settings; do not replace the whole file with this one property.
3. Save the file, then quit and restart CouchPilot using the same configuration. Settings are loaded at startup, so saving alone does not stop the running instance from recording.

Disabling tracing stops new records after restart but does not delete existing trace data. To remove old records, quit CouchPilot first and delete only `trace/trace.jsonl` from the active configuration directory. Leave tracing disabled if you do not want the file recreated on the next launch. To resume recording, set `local_trace_enabled` to `true` and restart again.
