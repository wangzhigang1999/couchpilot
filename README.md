# CouchPilot

[![CI](https://github.com/wangzhigang1999/couchpilot/actions/workflows/ci.yml/badge.svg)](https://github.com/wangzhigang1999/couchpilot/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Pilot your desktop from a gamepad. Fast, configurable, and cross-platform by design.**

CouchPilot is a small, portable Go program that turns a gamepad into a desktop controller. Windows and XInput are supported today; the core and mapping engine are platform-neutral so additional desktop and device adapters can be added cleanly.

The runtime is a single executable with no Python, C toolchain, or external runtime dependency.

## Run

```powershell
cd D:\couchpilot
.\bin\couchpilot.exe doctor
.\bin\couchpilot.exe start --verbose
.\bin\couchpilot.exe status
```

Stop the background process cleanly:

```powershell
.\bin\couchpilot.exe stop
```

Holding **Back + Start** for 1.5 seconds is still the emergency exit.

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

Codex keeps its task, command-menu, terminal and Back mappings. X remains right click in Codex so it cannot accidentally stop a response. Chrome keeps its tab, address-bar and new-tab mappings. The LT window shortcuts take priority without changing a shoulder button pressed by itself.

For multiple windows, keep LT held, tap M1/RB or M2/LB repeatedly to move through the native window switcher, then release LT to select the highlighted window.

## Configuration and future UI

`config.json` is the stable configuration contract for the CLI and a future UI. A UI only needs to validate and edit this file; the engine remains unchanged.

Bindings are optional overrides grouped by foreground-app profile:

```json
{
  "bindings": {
    "default": {
      "a": "click_left",
      "lt+rb": "window_next"
    },
    "chrome": {
      "rb": "chrome_next_tab"
    }
  }
}
```

Set an action to an empty string to disable that exact binding. Run the following command to list valid action names:

```powershell
.\bin\couchpilot.exe actions
```

The current gesture names are `a`, `b`, `x`, `y`, `lb`, `rb`, `l3`, `r3`, `dpad_up`, `dpad_down`, `dpad_left`, and `dpad_right`. Prefix a gesture with `lt+` or `rt+` for a trigger chord.

## Architecture

- `internal/core`: platform-neutral device state, actions and narrow interfaces.
- `internal/engine`: pointer math, edge detection, profiles and configurable binding resolution.
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
