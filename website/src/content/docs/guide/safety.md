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

This rule concerns A without a trigger. RT+A is an explicit immediate Enter in every app, with no voice delay. It may submit or confirm the focused control, so hold RT only when that is your intent.

## X never stops Codex

In Codex, <kbd>X</kbd> stays the right mouse button and never sends Escape, so it cannot stop a response in progress.

This applies to X alone. RT+X deliberately sends Escape and can cancel an operation or stop a response, depending on the app. RT editing chords also clear the temporary voice-edit state so a later plain A returns to its mouse action.

## Emergency exit is always available

Hold <kbd>Back + Start</kbd> together for 1.5 seconds to stop CouchPilot immediately.
