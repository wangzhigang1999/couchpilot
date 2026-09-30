---
title: Codex
description: Task navigation and development shortcuts for the ChatGPT and Codex desktop apps.
sidebar:
  order: 2
  badge: Frequent
---

<span class="process-chip">ChatGPT.exe · OpenAI.Codex</span>

Task switching, the command menu, the terminal, and the complete voice-to-send flow all live on easy-to-reach controls.

| Control | Action | Keyboard shortcut |
| --- | --- | --- |
| <kbd>B</kbd> | Back | Ctrl + [ |
| <kbd>LB</kbd> | Previous task | Ctrl + Shift + [ |
| <kbd>RB</kbd> | Next task | Ctrl + Shift + ] |
| <kbd>L3</kbd> | Command menu | Ctrl + K |
| <kbd>R3</kbd> | Open terminal | Ctrl + ` |
| <kbd>X</kbd> | Right mouse button | Does not send Escape |
| <kbd>Y</kbd>, then <kbd>A</kbd> | Dictate, review, and send | Right Alt, then Enter |
| After <kbd>Y</kbd>, tap or hold <kbd>B</kbd> | Delete one or keep deleting | Backspace |
| <kbd>RT</kbd> + <kbd>A</kbd> | Send at any time | Enter |

Y then A is the same global voice-submit flow available in every app. Pause for the configured minimum delay after Y before pressing A; an earlier A press is ignored. Codex additionally arms B as **Backspace**: a short press deletes one character, while holding B starts a steady repeat after a short delay and stops immediately on release. Moving the left stick cancels the voice-edit mode silently and restores the normal mouse controls. The mode also clears when the foreground app changes or after the configured timeout.

:::caution[Safety rule]
X without a trigger remains right click. RT+X deliberately sends Escape and may stop a response in progress. The global RT+A/B/X/Y editing shortcuts also apply here; RT+B deletes only once per press, unlike voice-then-B. CouchPilot never searches for or clicks the send button, and it never forces focus into the composer.
:::
