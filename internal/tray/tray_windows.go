//go:build windows

package tray

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	winapi "golang.org/x/sys/windows"
)

const (
	nimAdd        = 0x00000000
	nimDelete     = 0x00000002
	nimSetFocus   = 0x00000003
	nimSetVersion = 0x00000004

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifShowTip = 0x00000080

	notifyIconVersion4 = 4

	wmNull        = 0x0000
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmContextMenu = 0x007b
	wmCommand     = 0x0111
	wmUser        = 0x0400
	wmApp         = 0x8000
	ninSelect     = wmUser
	ninKeySelect  = wmUser + 1

	trayCallbackMessage = wmApp + 1

	mfString    = 0x0000
	mfGrayed    = 0x0001
	mfDisabled  = 0x0002
	mfSeparator = 0x0800

	tpmLeftAlign   = 0x0000
	tpmRightAlign  = 0x0008
	tpmTopAlign    = 0x0000
	tpmBottomAlign = 0x0020
	tpmVertical    = 0x0040
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100
	tpmRightButton = 0x0002
	tpmWorkArea    = 0x10000

	monitorDefaultToNearest = 0x00000002
	swShowNormal            = 1

	idiApplication = 32512
	idcArrow       = 32512
	appIconID      = 1

	menuOpenLogs       = 1001
	menuOpenUsage      = 1002
	menuOpenConfig     = 1003
	menuExitCouchPilot = 1004
)

var (
	kernel32 = winapi.NewLazySystemDLL("kernel32.dll")
	shell32  = winapi.NewLazySystemDLL("shell32.dll")
	user32   = winapi.NewLazySystemDLL("user32.dll")

	procGetModuleHandleW       = kernel32.NewProc("GetModuleHandleW")
	procShellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW")
	procShellNotifyIconGetRect = shell32.NewProc("Shell_NotifyIconGetRect")
	procShellExecuteW          = shell32.NewProc("ShellExecuteW")
	procAppendMenuW            = user32.NewProc("AppendMenuW")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procDestroyMenu            = user32.NewProc("DestroyMenu")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procGetMonitorInfoW        = user32.NewProc("GetMonitorInfoW")
	procLoadCursorW            = user32.NewProc("LoadCursorW")
	procLoadIconW              = user32.NewProc("LoadIconW")
	procMonitorFromRect        = user32.NewProc("MonitorFromRect")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procRegisterWindowMsgW     = user32.NewProc("RegisterWindowMessageW")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procTrackPopupMenuEx       = user32.NewProc("TrackPopupMenuEx")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procUnregisterClassW       = user32.NewProc("UnregisterClassW")

	activeTray atomic.Pointer[nativeTray]
)

type point struct {
	X int32
	Y int32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type notifyIconIdentifier struct {
	Size   uint32
	Window winapi.Handle
	ID     uint32
	GUID   winapi.GUID
}

type trackPopupMenuParams struct {
	Size    uint32
	Exclude rect
}

type message struct {
	Window  winapi.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}

type windowClassEx struct {
	Size        uint32
	Style       uint32
	WindowProc  uintptr
	ClassExtra  int32
	WindowExtra int32
	Instance    winapi.Handle
	Icon        winapi.Handle
	Cursor      winapi.Handle
	Background  winapi.Handle
	MenuName    *uint16
	ClassName   *uint16
	IconSmall   winapi.Handle
}

type notifyIconData struct {
	Size            uint32
	Window          winapi.Handle
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            winapi.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            winapi.GUID
	BalloonIcon     winapi.Handle
}

type nativeTray struct {
	cancel          context.CancelFunc
	configDirectory string
	logDirectory    string
	usageReportPath string
	instance        winapi.Handle
	window          winapi.Handle
	menu            winapi.Handle
	className       *uint16
	taskbarCreated  uint32
	notify          notifyIconData
	classRegistered bool
	iconAdded       bool
}

// Start creates the Windows notification icon on its own OS thread. The
// returned channel completes after the icon and hidden window have both been
// removed. Calling cancel, whether from the tray menu or another worker exit
// path, begins that cleanup.
func Start(ctx context.Context, cancel context.CancelFunc, logPath, configPath, usageReportPath string) (<-chan error, error) {
	ready := make(chan error, 1)
	done := make(chan error, 1)
	tray := &nativeTray{
		cancel:          cancel,
		configDirectory: filepath.Dir(configPath),
		logDirectory:    filepath.Dir(logPath),
		usageReportPath: usageReportPath,
	}
	go func() {
		err := tray.run(ctx, ready)
		done <- err
		close(done)
	}()
	if err := <-ready; err != nil {
		<-done
		return nil, err
	}
	return done, nil
}

func (t *nativeTray) run(ctx context.Context, ready chan<- error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := t.initialise(); err != nil {
		ready <- err
		t.cleanup()
		return err
	}
	ready <- nil

	watcherDone := make(chan struct{})
	watcherExited := make(chan struct{})
	window := t.window
	go func() {
		defer close(watcherExited)
		select {
		case <-ctx.Done():
			postMessage(window, wmClose, 0, 0)
		case <-watcherDone:
		}
	}()

	err := t.messageLoop()
	close(watcherDone)
	<-watcherExited
	t.cleanup()
	if err != nil {
		t.cancel()
	}
	return err
}

func (t *nativeTray) initialise() error {
	instance, _, callErr := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return callFailure("GetModuleHandleW", callErr)
	}
	t.instance = winapi.Handle(instance)

	className, err := winapi.UTF16PtrFromString(fmt.Sprintf("CouchPilotTrayWindow.%d", os.Getpid()))
	if err != nil {
		return err
	}
	t.className = className

	icon, err := loadApplicationIcon(t.instance)
	if err != nil {
		return err
	}
	cursor, _, callErr := procLoadCursorW.Call(0, idcArrow)
	if cursor == 0 {
		return callFailure("LoadCursorW", callErr)
	}

	class := windowClassEx{
		WindowProc: windowsCallback,
		Instance:   t.instance,
		Icon:       winapi.Handle(icon),
		Cursor:     winapi.Handle(cursor),
		ClassName:  t.className,
		IconSmall:  winapi.Handle(icon),
	}
	class.Size = uint32(unsafe.Sizeof(class))
	registered, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	if registered == 0 {
		return callFailure("RegisterClassExW", callErr)
	}
	t.classRegistered = true

	taskbarCreatedName, _ := winapi.UTF16PtrFromString("TaskbarCreated")
	taskbarCreated, _, callErr := procRegisterWindowMsgW.Call(uintptr(unsafe.Pointer(taskbarCreatedName)))
	if taskbarCreated == 0 {
		return callFailure("RegisterWindowMessageW", callErr)
	}
	t.taskbarCreated = uint32(taskbarCreated)

	activeTray.Store(t)
	windowName, _ := winapi.UTF16PtrFromString("CouchPilot")
	window, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(t.className)),
		uintptr(unsafe.Pointer(windowName)),
		0,
		0, 0, 0, 0,
		0, 0,
		uintptr(t.instance),
		0,
	)
	if window == 0 {
		activeTray.CompareAndSwap(t, nil)
		return callFailure("CreateWindowExW", callErr)
	}
	t.window = winapi.Handle(window)

	menu, _, callErr := procCreatePopupMenu.Call()
	if menu == 0 {
		return callFailure("CreatePopupMenu", callErr)
	}
	t.menu = winapi.Handle(menu)
	if err := t.buildMenu(); err != nil {
		return err
	}

	t.notify = notifyIconData{
		Window:          t.window,
		ID:              1,
		Flags:           nifMessage | nifIcon | nifTip | nifShowTip,
		CallbackMessage: trayCallbackMessage,
		Icon:            winapi.Handle(icon),
		Version:         notifyIconVersion4,
	}
	t.notify.Size = uint32(unsafe.Sizeof(t.notify))
	copyUTF16(t.notify.Tip[:], "CouchPilot 正在运行")
	if err := t.addIcon(); err != nil {
		return err
	}
	return nil
}

func loadApplicationIcon(instance winapi.Handle) (winapi.Handle, error) {
	if icon, _, _ := procLoadIconW.Call(uintptr(instance), appIconID); icon != 0 {
		return winapi.Handle(icon), nil
	}
	icon, _, callErr := procLoadIconW.Call(0, idiApplication)
	if icon == 0 {
		return 0, callFailure("LoadIconW", callErr)
	}
	return winapi.Handle(icon), nil
}

func (t *nativeTray) buildMenu() error {
	if err := appendMenu(t.menu, mfString|mfDisabled|mfGrayed, 0, "CouchPilot 正在运行"); err != nil {
		return err
	}
	if err := appendMenu(t.menu, mfSeparator, 0, ""); err != nil {
		return err
	}
	if err := appendMenu(t.menu, mfString, menuOpenLogs, "打开日志"); err != nil {
		return err
	}
	if t.usageReportPath != "" {
		if err := appendMenu(t.menu, mfString, menuOpenUsage, "查看按键报告"); err != nil {
			return err
		}
	}
	if err := appendMenu(t.menu, mfString, menuOpenConfig, "打开配置目录"); err != nil {
		return err
	}
	if err := appendMenu(t.menu, mfSeparator, 0, ""); err != nil {
		return err
	}
	return appendMenu(t.menu, mfString, menuExitCouchPilot, "退出 CouchPilot")
}

func (t *nativeTray) addIcon() error {
	if err := shellNotifyIcon(nimAdd, &t.notify); err != nil {
		return err
	}
	t.iconAdded = true
	if err := shellNotifyIcon(nimSetVersion, &t.notify); err != nil {
		_ = shellNotifyIcon(nimDelete, &t.notify)
		t.iconAdded = false
		return err
	}
	return nil
}

func (t *nativeTray) messageLoop() error {
	var msg message
	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(result) {
		case -1:
			return callFailure("GetMessageW", callErr)
		case 0:
			return nil
		default:
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

func (t *nativeTray) cleanup() {
	t.removeIcon()
	if t.menu != 0 {
		procDestroyMenu.Call(uintptr(t.menu))
		t.menu = 0
	}
	if t.window != 0 {
		procDestroyWindow.Call(uintptr(t.window))
		t.window = 0
	}
	activeTray.CompareAndSwap(t, nil)
	if t.classRegistered {
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(t.className)), uintptr(t.instance))
		t.classRegistered = false
	}
}

func (t *nativeTray) windowProc(window winapi.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch {
	case message == t.taskbarCreated:
		t.iconAdded = false
		if err := t.addIcon(); err != nil {
			fmt.Fprintln(os.Stderr, "tray: restore icon:", err)
		}
		return 0
	case message == trayCallbackMessage:
		event := uint32(lParam & 0xffff)
		if event == wmContextMenu || event == ninSelect || event == ninKeySelect {
			if err := t.showMenu(); err != nil {
				fmt.Fprintln(os.Stderr, "tray: show menu:", err)
			}
		}
		return 0
	case message == wmCommand:
		t.handleCommand(uint32(wParam & 0xffff))
		return 0
	case message == wmClose:
		t.removeIcon()
		t.window = 0
		procDestroyWindow.Call(uintptr(window))
		return 0
	case message == wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	default:
		result, _, _ := procDefWindowProcW.Call(uintptr(window), uintptr(message), wParam, lParam)
		return result
	}
}

func (t *nativeTray) removeIcon() {
	if !t.iconAdded {
		return
	}
	_ = shellNotifyIcon(nimDelete, &t.notify)
	t.iconAdded = false
}

func (t *nativeTray) showMenu() error {
	iconRect, err := t.iconRect()
	if err != nil {
		var cursor point
		result, _, callErr := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
		if result == 0 {
			return fmt.Errorf("locate notification icon: %v; %w", err, callFailure("GetCursorPos", callErr))
		}
		iconRect = rect{Left: cursor.X, Top: cursor.Y, Right: cursor.X + 1, Bottom: cursor.Y + 1}
	}

	anchor, alignment := popupAnchor(iconRect)
	parameters := trackPopupMenuParams{Exclude: iconRect}
	parameters.Size = uint32(unsafe.Sizeof(parameters))

	// TrackPopupMenuEx requires its top-level owner to be foreground for the
	// menu to stay above Shell flyouts and dismiss correctly on outside clicks.
	procSetForegroundWindow.Call(uintptr(t.window))
	command, _, _ := procTrackPopupMenuEx.Call(
		uintptr(t.menu),
		uintptr(alignment|tpmVertical|tpmNoNotify|tpmReturnCmd|tpmRightButton|tpmWorkArea),
		uintptr(anchor.X),
		uintptr(anchor.Y),
		uintptr(t.window),
		uintptr(unsafe.Pointer(&parameters)),
	)
	if t.window == 0 || !t.iconAdded {
		return nil
	}
	postMessage(t.window, wmNull, 0, 0)
	// NIM_SETFOCUS is best-effort cleanup after the menu closes. Explorer can
	// reject it when the icon lives in the overflow flyout even though the menu
	// was shown and dismissed successfully, so it must not turn a successful
	// interaction into a tray error.
	_ = shellNotifyIcon(nimSetFocus, &t.notify)
	if command != 0 {
		t.handleCommand(uint32(command))
	}
	return nil
}

func (t *nativeTray) iconRect() (rect, error) {
	identifier := notifyIconIdentifier{
		Window: t.window,
		ID:     t.notify.ID,
	}
	identifier.Size = uint32(unsafe.Sizeof(identifier))

	var iconRect rect
	result, _, _ := procShellNotifyIconGetRect.Call(
		uintptr(unsafe.Pointer(&identifier)),
		uintptr(unsafe.Pointer(&iconRect)),
	)
	runtime.KeepAlive(&identifier)
	if uint32(result) != 0 {
		return rect{}, fmt.Errorf("Shell_NotifyIconGetRect failed: HRESULT 0x%08X", uint32(result))
	}
	return iconRect, nil
}

func popupAnchor(iconRect rect) (point, uint32) {
	// The defaults match the common bottom-right taskbar placement and remain a
	// safe fallback if monitor information is temporarily unavailable.
	anchor := point{X: iconRect.Right, Y: iconRect.Top}
	alignment := uint32(tpmRightAlign | tpmBottomAlign)

	monitor, _, _ := procMonitorFromRect.Call(
		uintptr(unsafe.Pointer(&iconRect)),
		monitorDefaultToNearest,
	)
	if monitor == 0 {
		return anchor, alignment
	}
	info := monitorInfo{}
	info.Size = uint32(unsafe.Sizeof(info))
	result, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		return anchor, alignment
	}
	return popupAnchorForWorkArea(iconRect, info.Work)
}

func popupAnchorForWorkArea(iconRect, workArea rect) (point, uint32) {
	anchor := point{X: iconRect.Right, Y: iconRect.Top}
	alignment := uint32(tpmRightAlign | tpmBottomAlign)

	if iconRect.Right <= workArea.Left {
		anchor.X = iconRect.Right
		alignment &^= tpmRightAlign
		alignment |= tpmLeftAlign
	} else if iconRect.Left >= workArea.Right {
		anchor.X = iconRect.Left
		alignment |= tpmRightAlign
	} else if iconRect.Left-workArea.Left >= workArea.Right-iconRect.Right {
		anchor.X = iconRect.Right
		alignment |= tpmRightAlign
	} else {
		anchor.X = iconRect.Left
		alignment &^= tpmRightAlign
		alignment |= tpmLeftAlign
	}

	if iconRect.Bottom <= workArea.Top {
		anchor.Y = iconRect.Bottom
		alignment &^= tpmBottomAlign
		alignment |= tpmTopAlign
	} else if iconRect.Top >= workArea.Bottom {
		anchor.Y = iconRect.Top
		alignment |= tpmBottomAlign
	} else if iconRect.Top-workArea.Top >= workArea.Bottom-iconRect.Bottom {
		anchor.Y = iconRect.Top
		alignment |= tpmBottomAlign
	} else {
		anchor.Y = iconRect.Bottom
		alignment &^= tpmBottomAlign
		alignment |= tpmTopAlign
	}

	return anchor, alignment
}

func (t *nativeTray) handleCommand(command uint32) {
	var err error
	switch command {
	case menuOpenLogs:
		err = openDirectory(t.logDirectory)
	case menuOpenUsage:
		err = openFile(t.usageReportPath)
	case menuOpenConfig:
		err = openDirectory(t.configDirectory)
	case menuExitCouchPilot:
		t.cancel()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tray:", err)
	}
}

func appendMenu(menu winapi.Handle, flags uint32, id uint32, title string) error {
	var titlePointer *uint16
	if title != "" {
		var err error
		titlePointer, err = winapi.UTF16PtrFromString(title)
		if err != nil {
			return err
		}
	}
	result, _, callErr := procAppendMenuW.Call(
		uintptr(menu),
		uintptr(flags),
		uintptr(id),
		uintptr(unsafe.Pointer(titlePointer)),
	)
	if result == 0 {
		return callFailure("AppendMenuW", callErr)
	}
	return nil
}

func shellNotifyIcon(operation uint32, data *notifyIconData) error {
	result, _, callErr := procShellNotifyIconW.Call(uintptr(operation), uintptr(unsafe.Pointer(data)))
	runtime.KeepAlive(data)
	if result == 0 {
		return callFailure("Shell_NotifyIconW", callErr)
	}
	return nil
}

func postMessage(window winapi.Handle, message uint32, wParam, lParam uintptr) {
	procPostMessageW.Call(uintptr(window), uintptr(message), wParam, lParam)
}

func openDirectory(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}
	if err := exec.Command("explorer.exe", path).Start(); err != nil {
		return fmt.Errorf("open directory %s: %w", path, err)
	}
	return nil
}

func openFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("locate file %s: %w", path, err)
	}
	verb, err := winapi.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := winapi.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	result, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0,
		0,
		swShowNormal,
	)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(file)
	if result <= 32 {
		return fmt.Errorf("open file %s: ShellExecuteW code %d", path, result)
	}
	return nil
}

func copyUTF16(destination []uint16, value string) {
	if len(destination) == 0 {
		return
	}
	encoded, err := winapi.UTF16FromString(value)
	if err != nil {
		return
	}
	copy(destination, encoded)
	if len(encoded) > len(destination) {
		destination[len(destination)-1] = 0
	}
}

func callFailure(operation string, err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return fmt.Errorf("%s failed", operation)
	}
	if err == nil {
		return fmt.Errorf("%s failed", operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func globalWindowProc(window winapi.Handle, message uint32, wParam, lParam uintptr) uintptr {
	if tray := activeTray.Load(); tray != nil {
		return tray.windowProc(window, message, wParam, lParam)
	}
	result, _, _ := procDefWindowProcW.Call(uintptr(window), uintptr(message), wParam, lParam)
	return result
}

var windowsCallback = winapi.NewCallback(globalWindowProc)
