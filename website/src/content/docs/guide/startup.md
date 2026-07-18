---
title: Start with Windows
description: Start CouchPilot automatically at sign-in and recover from unexpected failures.
sidebar:
  order: 2
---

From the CouchPilot folder, install the Windows startup task once:

```powershell
.\bin\couchpilot.exe install
```

CouchPilot starts immediately and starts automatically whenever you sign in to Windows. If the process fails unexpectedly, Windows retries it every minute, up to 10 times.

Use the normal commands to check or stop the current process:

```powershell
.\bin\couchpilot.exe status
.\bin\couchpilot.exe stop
```

A normal stop or the <kbd>Back + Start</kbd> emergency exit does not trigger a retry. The startup task remains installed for your next Windows sign-in.

To stop CouchPilot and remove the startup task:

```powershell
.\bin\couchpilot.exe uninstall
```

Run `install` again after moving `couchpilot.exe` or `config.json` so the task uses their new absolute paths.
