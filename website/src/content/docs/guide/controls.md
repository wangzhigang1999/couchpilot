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
| <kbd>D-pad</kbd> | Arrow keys | Up, down, left, right |
| <kbd>LT</kbd> | Precision movement | Reduces pointer speed |
| <kbd>RT</kbd> | Boost movement | Increases pointer speed |
| <kbd>LT + LB</kbd> | Previous window | Alt + Shift + Tab |
| <kbd>LT + RB</kbd> | Next window | Alt + Tab |
| <kbd>Back + Start</kbd> | Emergency exit | Hold for 1.5 seconds |

:::tip[Hold to drag]
Hold A while moving the left stick. CouchPilot keeps the left mouse button down, so you can select text, move windows, or marquee-select a region.
:::

## Voice editing in whitelisted apps

Codex, QQ/WeChat, Claude, and Cherry Studio add a temporary voice-edit state. Press <kbd>Y</kbd> to dictate, tap or hold <kbd>B</kbd> to delete characters, then press <kbd>A</kbd> to send. Moving the pointer cancels the state and restores the normal A/B bindings immediately.

See each app page for the exact behavior and safety limits. Browser pages, documents, and terminals do not enable voice sending.

## How app profiles override controls

When CouchPilot matches the foreground app to a profile, it replaces only the bindings declared by that profile. For example, RB moves to the next tab in a browser, while pointer movement, scrolling, right click, voice input, and window switching remain global.
