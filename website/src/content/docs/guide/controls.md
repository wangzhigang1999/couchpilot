---
title: Global controls
description: CouchPilot controls available in every application.
sidebar:
  order: 1
---

These controls work in every app. An app profile overrides only the controls explicitly listed on that app's page.

| Gamepad control | Default action | Notes |
| --- | --- | --- |
| <kbd>Left stick</kbd> | Move pointer | Immediate, continuous movement |
| <kbd>Right stick</kbd> | Scroll | Vertical scrolling |
| <kbd>A</kbd> | Left mouse button | Hold to drag or select |
| <kbd>X</kbd> | Right mouse button | Hold for right-button drag |
| <kbd>Y</kbd> | Voice input | Taps physical right Alt |
| <kbd>Y</kbd>, then <kbd>A</kbd> | Press Enter | Available in every app |
| <kbd>D-pad</kbd> | Arrow keys | Up, down, left, right |
| <kbd>LT</kbd> | Precision movement | Reduces pointer speed |
| <kbd>RT</kbd> | Boost movement | Increases pointer speed |
| <kbd>RT + A</kbd> | Enter / confirm | Immediate, in every app |
| <kbd>RT + B</kbd> | Backspace | Once per press, no repeat |
| <kbd>RT + X</kbd> | Escape / cancel | May stop an app's current operation |
| <kbd>RT + Y</kbd> | Find | Ctrl+F on Windows, Command+F on macOS |
| <kbd>LT + LB</kbd> | Previous window | Alt + Shift + Tab |
| <kbd>LT + RB</kbd> | Next window | Alt + Tab |
| <kbd>Back + Start</kbd> | Emergency exit | Hold for 1.5 seconds |

:::tip[Hold to drag]
Hold A while moving the left stick. CouchPilot keeps the left mouse button down, so you can select text, move windows, or marquee-select a region.
:::

## Voice submit in every app

Press <kbd>Y</kbd> to start voice input, speak, and pause briefly before pressing <kbd>A</kbd>. By default, A can submit only two seconds after Y. CouchPilot triggers the operating system's voice input but does not receive microphone or VAD events, so this configurable minimum delay prevents an immediate accidental send. Pressing A too early is safely ignored instead of clicking or sending. Moving the pointer, changing apps, pressing another control, submitting, or reaching the timeout restores A to the normal left mouse button.

Codex additionally lets you tap or hold <kbd>B</kbd> to delete characters before submitting. See the Codex page for those app-specific editing controls.

App changes are checked while a voice sequence is active, even if you are not touching the gamepad. Leaving the app cancels the sequence; returning does not reactivate it. If the foreground app can no longer be identified, an existing identified sequence is also cancelled.

## RT editing shortcuts

Hold RT before pressing A, B, X, or Y. These editing shortcuts work in every profile, run once per press, and do not auto-repeat. RT+A sends Enter immediately without starting voice input or waiting two seconds. RT+B performs one Backspace even after voice input; Codex's unmodified voice-then-B still supports repeat deletion.

An explicit trigger chord leaves CouchPilot's temporary voice-edit state, so opening Find with RT+Y will not turn a later plain A into an unexpected Enter. This does not turn off the operating system's voice input. The actual effect of Enter, Escape, and Find depends on the focused application.

RT still boosts pointer speed; LT wins when both triggers are held. The chord is selected when the button is pressed. Changing a trigger while a button remains held does not trigger a second action or change an ongoing mouse drag.

## How app profiles override controls

When CouchPilot matches the foreground app to a profile, it replaces only the bindings declared by that profile. For example, RB moves to the next tab in a browser, while pointer movement, scrolling, right click, voice input, and window switching remain global.

Configuration accepts `a`, `b`, `x`, `y`, `lb`, `rb`, `l3`, `r3`, `dpad_up`, `dpad_down`, `dpad_left`, and `dpad_right`, optionally with one `lt+` or `rt+` prefix. `voice+a` and `voice+b` are separate contextual sequences and cannot take another prefix. Back and Start remain reserved for emergency exit. Unsupported names cause a configuration error rather than an ineffective binding.

You can change the action assigned to `voice+b`. Only `backspace` repeats while held; other actions run once per press, mouse actions retain hold/release behavior, and voice uses the configured voice mode.

Use an empty action to disable an exact binding, including a trigger chord. A disabled chord does nothing; only an unassigned chord falls back to the base button.
