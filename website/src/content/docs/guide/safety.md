---
title: Safety rules
description: High-risk automation behavior CouchPilot deliberately avoids.
sidebar:
  order: 4
---

These constraints came from real usage failures, so CouchPilot treats them as product rules.

## Never steals input focus

Switching apps or moving the pointer does not make CouchPilot focus a text field automatically.

## A never auto-sends chat messages

In QQ, WeChat, and other chat apps, <kbd>A</kbd> stays the left mouse button. It is never mapped to Enter.

## X never stops Codex

In Codex, <kbd>X</kbd> stays the right mouse button and never sends Escape, so it cannot stop a response in progress.

## Emergency exit is always available

Hold <kbd>Back + Start</kbd> together for 1.5 seconds to stop CouchPilot immediately.
