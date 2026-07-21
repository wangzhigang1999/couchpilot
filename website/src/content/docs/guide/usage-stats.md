---
title: Local key-strategy tracing
description: Collect privacy-preserving evidence for improving buttons, chords, and app-specific mappings.
sidebar:
  order: 2
---

CouchPilot keeps local-only aggregate facts so mapping decisions can be based on real use rather than guesswork. The report is designed to reveal which controls matter in each app, whether a chord appears difficult to reach, which mappings receive no exposure, and where a before/after remapping experiment may be worthwhile. It records evidence; it does not change mappings automatically.

## What is recorded

For each relevant controller interaction, CouchPilot keeps the day, foreground executable base name (for example `ChatGPT.exe` or `chrome.exe`), active mapping profile, physical control, normalized gesture, binding profile that actually won, logical action, dispatch result, and a stable revision for the effective mapping strategy. Default-profile fallbacks are kept separate from app-specific bindings, so an analysis will not credit the wrong mapping or mix observations from before and after a remap.

The recorder also derives privacy-safe, coarse tracing facts:

- physical overlaps and whether they resolved to a bound, disabled, unbound, or fallback gesture;
- broad hold-duration and transition-time buckets, rather than exact timestamps;
- interaction, voice-compose, window-switching, and repeat episodes and how they ended;
- short follow-up patterns that can be reviewed as suspected corrections.

The snapshot includes the current control and binding inventory. This lets the report distinguish a disabled binding from an enabled mapping that has not been observed, while per-app interaction exposure keeps “unused” from being confused with “never had an opportunity to use.”

“Dispatch succeeded” and “dispatch failed” describe only whether CouchPilot successfully sent the requested system action. They do not prove that the user's task or intent succeeded.

## How to interpret the leads

A **chord near-miss** means that physical controls overlapped in a chord-like way but did not resolve to that chord. It may indicate reach or timing friction, but it may also be an intentional single-button action or accidental overlap. It is not proof that the user intended the chord. Because LT and RT also modify pointer speed, probes observed in pointer or stick activity are marked ambiguous, shown as raw evidence, and excluded from the recommendation denominator.

A **suspected correction** means that a coarse follow-up pattern—such as rapidly reversing direction—looks like a correction. It is useful for finding candidates to inspect, but it is not confirmation that the first action was a mistake. Compare these leads across apps, days, exposure, and strategy revisions before changing a binding.

The report never recommends a remap from an isolated event. Within the same strategy, app, and profile, a chord candidate must have enough eligible exposure, cross a minimum rate, and recur on at least two active days. Suspected corrections and quick repeats likewise require an origin-transition denominator, occurrence count, rate, and cross-day evidence. The exact gates are printed in the report.

## What is not recorded

The local input-usage files do not record typed text, keyboard activity outside CouchPilot's own gamepad actions, window titles, full process paths, controller identifiers, pointer coordinates, or a precise input timeline. They keep only the executable base name—not its location—and store daily aggregate facts with broad timing buckets rather than a sequence of timestamped presses. While recording is enabled, compaction maintains a rolling window of 90 days; turning recording off freezes the existing local files until you remove them manually. Nothing from these records is uploaded or sent over the network. Normal diagnostic logs remain separate, and `--verbose` can include action-level troubleshooting messages.

## Find the data

Right-click the CouchPilot tray icon and choose **查看按键报告** to open the readable local report, or run this command to print the same live summary:

```powershell
.\bin\couchpilot.exe usage
```

The files live beside `config.json`:

```text
usage/
  usage-v1-report.html
  usage-v1.snapshot.json
  usage-v1.wal.jsonl
```

The snapshot is the durable aggregate view and catches up automatically about once a minute while new data is pending. The JSONL file holds the most recent bounded crash-recovery journal. Together they are the current record; CouchPilot compacts the journal automatically. You may also see an internal `usage-v1.snapshot.json.bak` recovery copy. Do not edit these files while CouchPilot is running.

## Acceptance check

1. Open the report and note the current count for one button.
2. Press that button three separate times. Do not hold it.
3. Wait about ten seconds (up to fifteen seconds), then refresh or reopen the report. Its count should increase by three and it should no longer appear under controls not observed for the current strategy.
4. Repeat with a defined chord such as `LT+RB`. The chord row should increase while held frames and releases add no extra attempts.
5. Confirm that dispatch failures and dropped events remain zero during normal use.

This verifies collection only. Chord near-misses and suspected corrections need a little normal use before they become useful and remain hypotheses, not confirmed intent or mistakes. The report does not change bindings automatically.

## Turn recording off

Local recording is enabled by default. Set this field in `config.json`, then restart CouchPilot:

```json
{
  "local_usage_stats_enabled": false
}
```

Turning it off does not delete existing data, and the existing report remains available from the tray. To remove the records, stop CouchPilot first and then delete its `usage` folder. This switch only controls local aggregate records and can never enable network upload.
