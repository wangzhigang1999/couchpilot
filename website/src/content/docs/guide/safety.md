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

<kbd>A</kbd> stays the left mouse button during normal use. After <kbd>Y</kbd> starts voice input, CouchPilot waits for the configured minimum delay before mapping <kbd>A</kbd> to Enter in every app. An earlier A press is ignored, so it cannot send empty input or click away from the focused field. Moving the pointer, changing apps, pressing another control, sending, or reaching the configured timeout restores the normal mouse binding.

CouchPilot still never focuses a text field for you: Enter goes only to the control that already has focus.

## X never stops Codex

In Codex, <kbd>X</kbd> stays the right mouse button and never sends Escape, so it cannot stop a response in progress.

## Emergency exit is always available

Hold <kbd>Back + Start</kbd> together for 1.5 seconds to stop CouchPilot immediately.
