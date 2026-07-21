---
title: 随 Windows 启动
description: 登录 Windows 时自动启动 CouchPilot，并在异常退出后自动恢复。
sidebar:
  order: 2
---

在 CouchPilot 文件夹中执行一次安装命令：

```powershell
.\bin\couchpilot.exe install
```

CouchPilot 会立即启动，以后每次登录 Windows 时也会自动启动。如果进程异常退出，Windows 会每分钟重试一次，最多重试 10 次。

运行期间，Windows 通知区域会显示一个小手柄图标。右键点击它可以打开日志、查看易读的本地按键报告、打开配置目录，或安全退出 CouchPilot。Windows 首次看到新图标时，可能会把它放在通知区域的折叠箭头中。

也可以在资源管理器中双击 `couchpilot.exe`：程序会直接进入后台，不会留下黑色终端窗口；从 PowerShell 启动时仍会保留正常的终端行为和输出。

可以继续使用普通命令检查或停止当前进程：

```powershell
.\bin\couchpilot.exe status
.\bin\couchpilot.exe stop
```

正常停止或按住 <kbd>Back + Start</kbd> 紧急退出不会触发重试。开机启动任务仍会保留，供下次登录 Windows 时使用。

如需停止 CouchPilot 并移除开机启动任务：

```powershell
.\bin\couchpilot.exe uninstall
```

如果移动了 `couchpilot.exe` 或 `config.json`，请重新运行 `install`，让计划任务记录新的绝对路径。
