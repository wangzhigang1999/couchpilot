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

After Y starts voice input, CouchPilot temporarily arms A as **Send** and B as **Backspace**. A short B press deletes one character; holding B starts a steady repeat after a short delay and stops immediately on release. Moving the left stick cancels the voice-edit mode silently and restores the normal mouse controls. The mode also clears when the foreground app changes or after the configured timeout.

:::caution[Safety rule]
X always remains right click so it cannot stop a response in progress. CouchPilot never searches for or clicks the send button, and it never forces focus into the composer.
:::
