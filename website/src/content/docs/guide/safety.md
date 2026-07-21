---
title: Safety rules
description: High-risk automation behavior CouchPilot deliberately avoids.
sidebar:
  order: 4
---

These constraints came from real usage failures, so CouchPilot treats them as product rules.

## Never steals input focus

Switching apps or moving the pointer does not make CouchPilot focus a text field automatically.

## A sends only in an explicit voice-edit state

<kbd>A</kbd> stays the left mouse button during normal use. After <kbd>Y</kbd> starts voice input in Codex, CouchPilot temporarily maps <kbd>A</kbd> to Enter so you can send deliberately. Moving the pointer, changing apps, sending, or reaching the configured timeout restores the normal mouse binding.

Browsers and every app other than Codex never enter this state.

## X never stops Codex

In Codex, <kbd>X</kbd> stays the right mouse button and never sends Escape, so it cannot stop a response in progress.

## Emergency exit is always available

Hold <kbd>Back + Start</kbd> together for 1.5 seconds to stop CouchPilot immediately.
