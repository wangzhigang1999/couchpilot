package winplatform

import (
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	winapi "golang.org/x/sys/windows"
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	keyUp       = 0x0002
	scanCode    = 0x0008
	extendedKey = 0x0001

	mouseLeftDown  = 0x0002
	mouseLeftUp    = 0x0004
	mouseRightDown = 0x0008
	mouseRightUp   = 0x0010
	mouseWheel     = 0x0800

	vkShift              = 0x10
	vkControl            = 0x11
	vkAlt                = 0x12
	vkEscape             = 0x1B
	vkEnter              = 0x0D
	vkPageUp             = 0x21
	vkPageDown           = 0x22
	vkLeft               = 0x25
	vkUp                 = 0x26
	vkRight              = 0x27
	vkDown               = 0x28
	vkTab                = 0x09
	vkLeftAlt            = 0xA4
	vkRightAlt           = 0xA5
	vkOEM3               = 0xC0
	vkOEM4               = 0xDB
	vkOEM6               = 0xDD
	vkMediaNextTrack     = 0xB0
	vkMediaPreviousTrack = 0xB1
	vkMediaPlayPause     = 0xB3
	vkVolumeMute         = 0xAD

	processQueryLimitedInformation = 0x1000
)

type point struct {
	X int32
	Y int32
}

type mouseInputData struct {
	DX        int32
	DY        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type keyboardInputData struct {
	VirtualKey uint16
	Scan       uint16
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
}

type input struct {
	Type uint32
	_    uint32
	Data [32]byte
}

var (
	user32                        = winapi.NewLazySystemDLL("user32.dll")
	procGetCursorPos              = user32.NewProc("GetCursorPos")
	procSetCursorPos              = user32.NewProc("SetCursorPos")
	procSendInput                 = user32.NewProc("SendInput")
	procMapVirtualKeyW            = user32.NewProc("MapVirtualKeyW")
	procGetForegroundWindow       = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessID  = user32.NewProc("GetWindowThreadProcessId")
	kernel32                      = winapi.NewLazySystemDLL("kernel32.dll")
	procQueryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
)

type Desktop struct {
	voiceVirtualKey uint16
	appProfiles     []core.AppProfile
	windowSwitching bool
}

func NewDesktop(voiceKey string, appProfiles []core.AppProfile) (*Desktop, error) {
	key, err := virtualKey(voiceKey)
	if err != nil {
		return nil, err
	}
	return &Desktop{voiceVirtualKey: key, appProfiles: appProfiles}, nil
}

func (d *Desktop) MovePointer(dx, dy int) error {
	var cursor point
	result, _, callErr := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	if result == 0 {
		return callError("GetCursorPos", callErr)
	}
	result, _, callErr = procSetCursorPos.Call(uintptr(int64(cursor.X)+int64(dx)), uintptr(int64(cursor.Y)+int64(dy)))
	if result == 0 {
		return callError("SetCursorPos", callErr)
	}
	return nil
}

func (d *Desktop) Scroll(amount int) error {
	return sendMouse(mouseInputData{MouseData: uint32(amount), Flags: mouseWheel})
}

func (d *Desktop) Perform(action core.Action) error {
	switch action {
	case core.ClickLeft:
		return d.click(false)
	case core.ClickRight:
		return d.click(true)
	case core.MouseLeftDown:
		return sendMouse(mouseInputData{Flags: mouseLeftDown})
	case core.MouseLeftUp:
		return sendMouse(mouseInputData{Flags: mouseLeftUp})
	case core.MouseRightDown:
		return sendMouse(mouseInputData{Flags: mouseRightDown})
	case core.MouseRightUp:
		return sendMouse(mouseInputData{Flags: mouseRightUp})
	case core.NavigateBack:
		return tapHotkey(vkAlt, vkLeft)
	case core.Escape:
		return tapKey(vkEscape, 25*time.Millisecond)
	case core.ArrowUp:
		return tapKey(vkUp, 25*time.Millisecond)
	case core.ArrowDown:
		return tapKey(vkDown, 25*time.Millisecond)
	case core.ArrowLeft:
		return tapKey(vkLeft, 25*time.Millisecond)
	case core.ArrowRight:
		return tapKey(vkRight, 25*time.Millisecond)
	case core.Enter:
		return tapKey(vkEnter, 25*time.Millisecond)
	case core.TabPrevious:
		return tapHotkey(vkControl, vkShift, vkTab)
	case core.TabNext:
		return tapHotkey(vkControl, vkTab)
	case core.TabNew:
		return tapHotkey(vkControl, uint16('T'))
	case core.FocusLocation:
		return tapHotkey(vkControl, uint16('L'))
	case core.Find:
		return tapHotkey(vkControl, uint16('F'))
	case core.NewDocument:
		return tapHotkey(vkControl, uint16('N'))
	case core.PageUp:
		return tapKey(vkPageUp, 25*time.Millisecond)
	case core.PageDown:
		return tapKey(vkPageDown, 25*time.Millisecond)
	case core.CommandPalette:
		return tapHotkey(vkControl, vkShift, uint16('P'))
	case core.QuickOpen:
		return tapHotkey(vkControl, uint16('P'))
	case core.MediaPreviousTrack:
		return tapKey(vkMediaPreviousTrack, 25*time.Millisecond)
	case core.MediaNextTrack:
		return tapKey(vkMediaNextTrack, 25*time.Millisecond)
	case core.MediaPlayPause:
		return tapKey(vkMediaPlayPause, 25*time.Millisecond)
	case core.VolumeMute:
		return tapKey(vkVolumeMute, 25*time.Millisecond)
	case core.VoiceTap:
		if err := physicalKeyEvent(d.voiceVirtualKey, true); err != nil {
			return err
		}
		time.Sleep(55 * time.Millisecond)
		return physicalKeyEvent(d.voiceVirtualKey, false)
	case core.VoiceDown:
		return physicalKeyEvent(d.voiceVirtualKey, true)
	case core.VoiceUp:
		return physicalKeyEvent(d.voiceVirtualKey, false)
	case core.WindowPrevious:
		return tapHotkey(vkAlt, vkShift, vkTab)
	case core.WindowNext:
		return tapHotkey(vkAlt, vkTab)
	case core.WindowCyclePrevious:
		return d.cycleWindow(true)
	case core.WindowCycleNext:
		return d.cycleWindow(false)
	case core.WindowCycleCommit:
		return d.commitWindowSwitch()
	case core.CodexBack:
		return tapHotkey(vkControl, vkOEM4)
	case core.CodexPreviousTask:
		return tapHotkey(vkControl, vkShift, vkOEM4)
	case core.CodexNextTask:
		return tapHotkey(vkControl, vkShift, vkOEM6)
	case core.CodexCommandMenu:
		return tapHotkey(vkControl, uint16('K'))
	case core.CodexTerminal:
		return tapHotkey(vkControl, vkOEM3)
	case core.ChromePreviousTab:
		return tapHotkey(vkControl, vkShift, vkTab)
	case core.ChromeNextTab:
		return tapHotkey(vkControl, vkTab)
	case core.ChromeAddressBar:
		return tapHotkey(vkControl, uint16('L'))
	case core.ChromeNewTab:
		return tapHotkey(vkControl, uint16('T'))
	default:
		return fmt.Errorf("unsupported Windows action %q", action)
	}
}

func (d *Desktop) cycleWindow(previous bool) error {
	if !d.windowSwitching {
		if err := keyEvent(vkAlt, true); err != nil {
			return err
		}
		d.windowSwitching = true
	}
	if previous {
		if err := keyEvent(vkShift, true); err != nil {
			_ = d.commitWindowSwitch()
			return err
		}
	}
	err := tapKey(vkTab, 25*time.Millisecond)
	if previous {
		if releaseErr := keyEvent(vkShift, false); err == nil {
			err = releaseErr
		}
	}
	if err != nil {
		_ = d.commitWindowSwitch()
	}
	return err
}

func (d *Desktop) commitWindowSwitch() error {
	if !d.windowSwitching {
		return nil
	}
	d.windowSwitching = false
	return keyEvent(vkAlt, false)
}

func (d *Desktop) ForegroundProfile() string {
	path, err := foregroundProcessPath()
	if err != nil {
		return "default"
	}
	return matchProfile(path, d.appProfiles)
}

func (d *Desktop) click(right bool) error {
	down, up := uint32(mouseLeftDown), uint32(mouseLeftUp)
	if right {
		down, up = mouseRightDown, mouseRightUp
	}
	if err := sendMouse(mouseInputData{Flags: down}); err != nil {
		return err
	}
	return sendMouse(mouseInputData{Flags: up})
}

func sendMouse(data mouseInputData) error {
	var item input
	item.Type = inputMouse
	*(*mouseInputData)(unsafe.Pointer(&item.Data[0])) = data
	return send(item)
}

func sendKeyboard(data keyboardInputData) error {
	var item input
	item.Type = inputKeyboard
	*(*keyboardInputData)(unsafe.Pointer(&item.Data[0])) = data
	return send(item)
}

func send(item input) error {
	result, _, callErr := procSendInput.Call(1, uintptr(unsafe.Pointer(&item)), unsafe.Sizeof(item))
	if result != 1 {
		return callError("SendInput", callErr)
	}
	return nil
}

func keyEvent(virtualKey uint16, down bool) error {
	flags := uint32(0)
	if !down {
		flags = keyUp
	}
	return sendKeyboard(keyboardInputData{VirtualKey: virtualKey, Flags: flags})
}

func tapKey(virtualKey uint16, duration time.Duration) error {
	if err := keyEvent(virtualKey, true); err != nil {
		return err
	}
	if duration > 0 {
		time.Sleep(duration)
	}
	return keyEvent(virtualKey, false)
}

func tapHotkey(keys ...uint16) error {
	if len(keys) == 0 {
		return nil
	}
	modifiers, key := keys[:len(keys)-1], keys[len(keys)-1]
	for _, modifier := range modifiers {
		if err := keyEvent(modifier, true); err != nil {
			return err
		}
	}
	err := tapKey(key, 25*time.Millisecond)
	for index := len(modifiers) - 1; index >= 0; index-- {
		if releaseErr := keyEvent(modifiers[index], false); err == nil {
			err = releaseErr
		}
	}
	return err
}

func physicalKeyEvent(virtualKey uint16, down bool) error {
	mapped, _, _ := procMapVirtualKeyW.Call(uintptr(virtualKey), 4)
	if mapped == 0 {
		return fmt.Errorf("no scan code for virtual key 0x%02X", virtualKey)
	}
	flags := uint32(scanCode)
	if !down {
		flags |= keyUp
	}
	if mapped&0xE000 != 0 {
		flags |= extendedKey
		mapped &= 0xFF
	}
	return sendKeyboard(keyboardInputData{Scan: uint16(mapped), Flags: flags})
}

func virtualKey(name string) (uint16, error) {
	switch strings.ToLower(name) {
	case "right_alt", "alt_right":
		return vkRightAlt, nil
	case "left_alt", "alt_left":
		return vkLeftAlt, nil
	default:
		return 0, fmt.Errorf("unsupported Windows voice_key %q", name)
	}
}

func foregroundProcessPath() (string, error) {
	window, _, callErr := procGetForegroundWindow.Call()
	if window == 0 {
		return "", callError("GetForegroundWindow", callErr)
	}
	var processID uint32
	procGetWindowThreadProcessID.Call(window, uintptr(unsafe.Pointer(&processID)))
	if processID == 0 {
		return "", fmt.Errorf("foreground window has no process")
	}
	process, err := winapi.OpenProcess(processQueryLimitedInformation, false, processID)
	if err != nil {
		return "", err
	}
	defer winapi.CloseHandle(process)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	result, _, callErr := procQueryFullProcessImageName.Call(
		uintptr(process), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)),
	)
	if result == 0 {
		return "", callError("QueryFullProcessImageNameW", callErr)
	}
	return winapi.UTF16ToString(buffer[:size]), nil
}

func matchProfile(path string, profiles []core.AppProfile) string {
	normalized := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	processName := normalized
	if index := strings.LastIndex(normalized, `\`); index >= 0 {
		processName = normalized[index+1:]
	}
	for _, profile := range profiles {
		if !matchesAny(processName, profile.ProcessNames, func(value, candidate string) bool {
			return value == strings.ToLower(candidate)
		}) {
			continue
		}
		if !matchesAny(normalized, profile.PathContains, func(value, candidate string) bool {
			return strings.Contains(value, strings.ToLower(strings.ReplaceAll(candidate, "/", `\`)))
		}) {
			continue
		}
		return profile.Name
	}
	return "default"
}

func matchesAny(value string, candidates []string, match func(string, string) bool) bool {
	if len(candidates) == 0 {
		return true
	}
	for _, candidate := range candidates {
		if match(value, candidate) {
			return true
		}
	}
	return false
}

func callError(name string, err error) error {
	if err == nil || err == winapi.ERROR_SUCCESS {
		return fmt.Errorf("%s failed", name)
	}
	return fmt.Errorf("%s: %w", name, err)
}
