# CouchPilot

<p align="center">
  <img src="assets/couchpilot-banner-v2.png" alt="CouchPilot — pilot your desktop from a gamepad" width="100%">
</p>

[![CI](https://github.com/wangzhigang1999/couchpilot/actions/workflows/ci.yml/badge.svg)](https://github.com/wangzhigang1999/couchpilot/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[Open the searchable CouchPilot Field Guide →](https://wangzhigang1999.github.io/couchpilot/)**

**Pilot your desktop from a gamepad. Fast, configurable, and cross-platform by design.**

CouchPilot is a small, portable Go program that turns a gamepad into a desktop controller. Windows and XInput are supported today; the core and mapping engine are platform-neutral so additional desktop and device adapters can be added cleanly.

The runtime is a single executable with no Python, C toolchain, or external runtime dependency.

## Run

```powershell
cd <path-to-couchpilot>
.\bin\couchpilot.exe doctor
.\bin\couchpilot.exe start --verbose
.\bin\couchpilot.exe status
```

When CouchPilot is running on Windows, its small controller icon appears in
the notification area. Right-click it to view the local key-strategy report,
open logs or configuration, or exit CouchPilot cleanly. Windows may
place a new icon under the notification-area overflow arrow the first time it
runs.

You can also double-click `couchpilot.exe`: Explorer launches it directly in
the background without leaving a console window open. Running the same file
from PowerShell keeps the normal terminal behavior and output.

Stop the background process cleanly:

```powershell
.\bin\couchpilot.exe stop
```

Install CouchPilot to start automatically when you sign in to Windows. The scheduled task also retries the process every minute after an unexpected failure, up to 10 times:

```powershell
.\bin\couchpilot.exe install
```

Installation starts CouchPilot immediately. A normal `stop` does not trigger a retry. To stop CouchPilot and remove the startup task:

```powershell
.\bin\couchpilot.exe uninstall
```

Holding **Back + Start** for 1.5 seconds is still the emergency exit.

## Local key-strategy tracing

CouchPilot records small, local-only aggregate facts to make later binding
decisions evidence-based. Besides button and recognized-chord counts, it keeps
the foreground executable's base name (for example `ChatGPT.exe` or
`chrome.exe`), the active and winning mapping profiles, the selected action,
and the current mapping-strategy revision. Coarse tracing also distinguishes
physical overlap from the gesture that was finally resolved, groups hold and
transition timing into broad buckets, and summarizes interaction, compose,
window-switching, and repeat episodes. This is meant to answer which bindings
are useful in each app, which chords may be awkward, and where a remap deserves
an experiment. CouchPilot still does not change bindings automatically.

The recorder does **not** install a keyboard hook or save typed text, window
titles, full process paths, controller identifiers, pointer coordinates, or
an exact timestamped input timeline. It keeps only the executable base name,
never its location. Timing and episode signals are stored only as daily local
aggregate facts in coarse buckets. While recording is enabled, compaction
keeps a rolling 90-day window; turning recording off freezes the existing
local files until they are removed manually. Nothing is uploaded. Data stays
beside `config.json` in
`usage\usage-v1.snapshot.json`; the neighboring JSONL file is a bounded crash-
recovery journal, not a browsing history.

While new observations are pending, the snapshot refreshes automatically
about once a minute. The snapshot and the recent journal together form the
current durable record; `usage-v1.snapshot.json.bak` is an internal recovery
copy.

Right-click the tray icon and choose **查看按键报告** to open a readable local
HTML report, or print the same live summary in a terminal:

```powershell
.\bin\couchpilot.exe usage
```

For a quick acceptance check, note the current count for one button, press it
three times, wait about ten seconds (up to fifteen), then refresh the report.
The count should increase by three and the button should disappear from the
current-strategy not-observed list. Test a defined chord such as `LT+RB` the
same way. After a
little normal use, the strategy section can also surface chord near-misses and
suspected corrections. A near-miss means physical controls overlapped without
resolving to that chord; a suspected correction means a coarse follow-up
pattern looked corrective. Trigger probes observed during pointer or stick
activity are shown as ambiguous and excluded from the recommendation
denominator. Recommendations also require a denominator, a minimum rate, and
evidence on more than one day. None of these signals proves the user's intent
or an actual mistake, so treat them as leads to investigate rather than
automatic remapping rules. Likewise, **dispatch succeeded/failed** describes
only whether CouchPilot sent the system action, not whether the user's task
succeeded.

Local aggregate recording is enabled by default. Set
`local_usage_stats_enabled` to `false` to stop adding observations. Existing
records and the tray report remain available until you stop CouchPilot and
remove the `usage` folder.

## Controls

| Control | Default action |
|---|---|
| Left stick | Immediate, continuous pointer movement |
| Right stick | Scroll |
| D-pad | Arrow keys |
| A | Left click; hold while moving the left stick to drag or select |
| B | Back (`Alt+Left`) |
| X | Right click; supports right-button drag |
| Y | Tap physical right Alt for voice input |
| LT | Precision pointer speed |
| RT | Boost pointer speed |
| LT + M1 / RB | Next Windows window |
| LT + M2 / LB | Previous Windows window |

Haptic feedback is enabled by default: clicks use a light tick, navigation uses a short pulse, voice activation is more noticeable, and window switching/commit uses the strongest confirmation. Controller connection also produces one short pulse.

Codex keeps its task, command-menu, terminal and Back mappings. X remains right click in Codex so it cannot accidentally stop a response. Browsers keep tab, address-bar and new-tab mappings. The LT window shortcuts take priority without changing a shoulder button pressed by itself.

For multiple windows, keep LT held, tap M1/RB or M2/LB repeatedly to move through the native window switcher, then release LT to select the highlighted window.

### Built-in app profiles

CouchPilot identifies the foreground executable and applies a small, safe profile. Mouse movement, scrolling, right-click, voice input, and LT window switching remain available everywhere.

| Apps | LB / RB | L3 | R3 | Special |
|---|---|---|---|---|
| Codex | Previous / next task | Command menu | Terminal | B goes back; X stays right-click |
| Chrome, Edge, Firefox | Previous / next tab | Address bar | New tab | B navigates back |
| Raycast | Selection up / down | — | — | A confirms; B dismisses |
| Typora, Obsidian | Previous / next tab | Find | New document | No automatic input focus |
| VS Code | Previous / next tab | Command palette | Quick open | B dismisses |
| PyCharm, IntelliJ, GoLand | — | Find | — | B dismisses |
| QQ, WeChat | — | Find | — | Voice edit whitelist: A sends, B deletes after Y |
| Claude, Cherry Studio | — | Find | — | Voice edit whitelist: A sends, B deletes after Y |
| QQ Music, Spotify, VLC | Previous / next track | Mute | Play / pause | Uses Windows media keys |
| Acrobat, Word, Excel, PowerPoint | Page up / down | Find | — | — |
| Windows Terminal | Previous / next tab | Command palette | New tab | B dismisses |
| Typeless | — | — | — | B dismisses; Y still uses right Alt |

## Configuration and future UI

`config.json` is the stable configuration contract for the CLI and a future UI. A UI only needs to validate and edit this file; the engine remains unchanged.

Set `haptics_enabled` to `false` to disable vibration, or adjust `haptic_strength` from `0.0` to `2.0`. The default is `1.0`.

Set `local_usage_stats_enabled` to `false` to disable the local aggregate
key-strategy facts described above. This setting never enables network upload.

`voice_submit_timeout_seconds` controls how long an app-specific voice compose mode remains armed. Codex, QQ/WeChat, Claude, and Cherry Studio are currently whitelisted: after `Y`, tap `A` to submit, tap `B` to delete one character, hold `B` to keep deleting, or move the pointer to restore normal mouse behavior. `RT+A` always submits in a whitelisted profile. Chat apps currently assume Enter-to-send.

Bindings are optional overrides grouped by foreground-app profile. `app_profiles` controls which executable selects each profile; matching is case-insensitive, list items are alternatives, and `process_names` plus `path_contains` can disambiguate executables with the same name. Earlier rules win.

```json
{
  "app_profiles": [
    {
      "name": "notes",
      "process_names": ["Typora.exe", "Obsidian.exe"]
    }
  ],
  "bindings": {
    "default": {
      "a": "click_left",
      "lt+rb": "window_next"
    },
    "notes": {
      "rb": "tab_next",
      "l3": "find"
    },
    "codex": {
      "voice+a": "enter",
      "voice+b": "backspace",
      "rt+a": "enter"
    }
  }
}
```

Set an action to an empty string to disable that exact binding. Run the following command to list valid action names:

```powershell
.\bin\couchpilot.exe actions
```

The current gesture names are `a`, `b`, `x`, `y`, `lb`, `rb`, `l3`, `r3`, `dpad_up`, `dpad_down`, `dpad_left`, and `dpad_right`. Prefix a gesture with `lt+` or `rt+` for a trigger chord. `voice+a` and `voice+b` are contextual gestures available after voice input; the whitelisted Codex, chat, and assistant profiles use them for submit and repeatable Backspace. The supplied `config.json` contains the full editable profile list.

To check which profile CouchPilot sees for the foreground app, focus that app and run:

```powershell
.\bin\couchpilot.exe profile
```

## Architecture

- `internal/core`: platform-neutral device state, actions and narrow interfaces.
- `internal/engine`: pointer math, edge detection, profiles and configurable binding resolution.
- `internal/usage`: 90-day local key-strategy aggregates with crash-recovery journaling.
- `internal/platform/windows`: XInput and Windows desktop output.
- `internal/platform`: platform factory; a future macOS adapter can implement the same interfaces.
- `internal/config`: versioned JSON schema shared by the CLI and future UI.

This is intentionally an adapter boundary, not a plugin framework. Additional gamepads or desktop platforms can be added without changing the mapping engine.

## Build

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1
```

The script uses project-local Go caches, runs tests and static checks, then creates `bin\couchpilot.exe`. The executable has no Go, Python, or C runtime dependency to install.

## Contributing and license

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing platform boundaries or the public configuration schema.

Released under the [MIT License](LICENSE).
