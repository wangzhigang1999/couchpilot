package winplatform

import (
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/desktop"
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
	vkBackspace          = 0x08
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
	windowSwitching bool
}

var _ desktop.Driver = (*Desktop)(nil)

func NewDesktop(voiceKey string) (*Desktop, error) {
	key, err := virtualKey(voiceKey)
	if err != nil {
		return nil, err
	}
	return &Desktop{voiceVirtualKey: key}, nil
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

func (d *Desktop) TapKey(key desktop.Key) error {
	virtualKey, err := windowsKey(key)
	if err != nil {
		return err
	}
	return tapKey(virtualKey, 25*time.Millisecond)
}

func (d *Desktop) TapChord(modifiers []desktop.Modifier, key desktop.Key) error {
	keys := make([]uint16, 0, len(modifiers)+1)
	for _, modifier := range modifiers {
		virtualKey, err := windowsModifier(modifier)
		if err != nil {
			return err
		}
		keys = append(keys, virtualKey)
	}
	virtualKey, err := windowsKey(key)
	if err != nil {
		return err
	}
	return tapHotkey(append(keys, virtualKey)...)
}

func (d *Desktop) Mouse(button desktop.MouseButton, state desktop.ButtonState) error {
	flags, err := windowsMouseFlags(button, state)
	if err != nil {
		return err
	}
	return sendMouse(mouseInputData{Flags: flags})
}

func (d *Desktop) NavigateBack() error {
	return tapHotkey(vkAlt, vkLeft)
}

func (d *Desktop) SwitchWindow(direction desktop.Direction) error {
	if direction == desktop.Previous {
		return tapHotkey(vkAlt, vkShift, vkTab)
	}
	return tapHotkey(vkAlt, vkTab)
}

func (d *Desktop) CycleWindow(direction desktop.Direction) error {
	return d.cycleWindow(direction == desktop.Previous)
}

func (d *Desktop) CommitWindowSwitch() error {
	return d.commitWindowSwitch()
}

func (d *Desktop) Voice(event desktop.VoiceEvent) error {
	switch event {
	case desktop.VoiceTap:
		if err := physicalKeyEvent(d.voiceVirtualKey, true); err != nil {
			return err
		}
		time.Sleep(55 * time.Millisecond)
		return physicalKeyEvent(d.voiceVirtualKey, false)
	case desktop.VoiceDown:
		return physicalKeyEvent(d.voiceVirtualKey, true)
	case desktop.VoiceUp:
		return physicalKeyEvent(d.voiceVirtualKey, false)
	default:
		return fmt.Errorf("unsupported Windows voice event %q", event)
	}
}

func (d *Desktop) Media(key desktop.MediaKey) error {
	virtualKey, err := windowsMediaKey(key)
	if err != nil {
		return err
	}
	return tapKey(virtualKey, 25*time.Millisecond)
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
	if err := keyEvent(vkAlt, false); err != nil {
		return err
	}
	d.windowSwitching = false
	return nil
}

func (d *Desktop) ForegroundApplication() core.AppIdentity {
	path, err := foregroundProcessPath()
	if err != nil {
		return core.AppIdentity{}
	}
	return core.AppIdentity{ProcessName: processNameFromPath(path), ExecutablePath: path}
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
	return tapHotkeyWith(keyEvent, time.Sleep, keys...)
}

type keyEventPoster func(virtualKey uint16, down bool) error

func tapHotkeyWith(post keyEventPoster, sleep func(time.Duration), keys ...uint16) (resultErr error) {
	if len(keys) == 0 {
		return nil
	}
	modifiers, key := keys[:len(keys)-1], keys[len(keys)-1]
	pressed := make([]uint16, 0, len(modifiers))
	defer func() {
		for index := len(pressed) - 1; index >= 0; index-- {
			if err := post(pressed[index], false); resultErr == nil && err != nil {
				resultErr = err
			}
		}
	}()
	for _, modifier := range modifiers {
		if err := post(modifier, true); err != nil {
			return err
		}
		pressed = append(pressed, modifier)
	}
	if err := post(key, true); err != nil {
		return err
	}
	sleep(25 * time.Millisecond)
	return post(key, false)
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
	case "platform_default", "right_alt", "alt_right":
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

func processNameFromPath(path string) string {
	normalized := strings.ReplaceAll(path, "/", `\`)
	if index := strings.LastIndex(normalized, `\`); index >= 0 {
		return normalized[index+1:]
	}
	return normalized
}

func windowsKey(key desktop.Key) (uint16, error) {
	keys := map[desktop.Key]uint16{
		desktop.KeyEscape: vkEscape, desktop.KeyArrowUp: vkUp,
		desktop.KeyArrowDown: vkDown, desktop.KeyArrowLeft: vkLeft,
		desktop.KeyArrowRight: vkRight, desktop.KeyBackspace: vkBackspace,
		desktop.KeyEnter: vkEnter, desktop.KeyTab: vkTab,
		desktop.KeyPageUp: vkPageUp, desktop.KeyPageDown: vkPageDown,
		desktop.KeyLeftBracket: vkOEM4, desktop.KeyRightBracket: vkOEM6,
		desktop.KeyGrave: vkOEM3, desktop.KeyF: 'F', desktop.KeyK: 'K',
		desktop.KeyL: 'L', desktop.KeyN: 'N', desktop.KeyP: 'P', desktop.KeyT: 'T',
	}
	if virtualKey, ok := keys[key]; ok {
		return virtualKey, nil
	}
	return 0, fmt.Errorf("unsupported Windows key %q", key)
}

func windowsModifier(modifier desktop.Modifier) (uint16, error) {
	switch modifier {
	case desktop.ModifierPrimary, desktop.ModifierControl:
		return vkControl, nil
	case desktop.ModifierShift:
		return vkShift, nil
	default:
		return 0, fmt.Errorf("unsupported Windows modifier %q", modifier)
	}
}

func windowsMouseFlags(button desktop.MouseButton, state desktop.ButtonState) (uint32, error) {
	flags := map[desktop.MouseButton]map[desktop.ButtonState]uint32{
		desktop.MouseLeft:  {desktop.ButtonDown: mouseLeftDown, desktop.ButtonUp: mouseLeftUp},
		desktop.MouseRight: {desktop.ButtonDown: mouseRightDown, desktop.ButtonUp: mouseRightUp},
	}
	if value, ok := flags[button][state]; ok {
		return value, nil
	}
	return 0, fmt.Errorf("unsupported Windows mouse event %q %q", button, state)
}

func windowsMediaKey(key desktop.MediaKey) (uint16, error) {
	keys := map[desktop.MediaKey]uint16{
		desktop.MediaPrevious:  vkMediaPreviousTrack,
		desktop.MediaNext:      vkMediaNextTrack,
		desktop.MediaPlayPause: vkMediaPlayPause,
		desktop.MediaMute:      vkVolumeMute,
	}
	if virtualKey, ok := keys[key]; ok {
		return virtualKey, nil
	}
	return 0, fmt.Errorf("unsupported Windows media key %q", key)
}

func callError(name string, err error) error {
	if err == nil || err == winapi.ERROR_SUCCESS {
		return fmt.Errorf("%s failed", name)
	}
	return fmt.Errorf("%s: %w", name, err)
}
