//go:build darwin

package macplatform

/*
#include <stdlib.h>
#include "native.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

var ErrAccessibilityPermission = errors.New("allow CouchPilot in System Settings > Privacy & Security > Accessibility, then start it again")

const (
	keyA            = 0x00
	keyB            = 0x0B
	keyF            = 0x03
	keyK            = 0x28
	keyL            = 0x25
	keyN            = 0x2D
	keyP            = 0x23
	keyT            = 0x11
	keyReturn       = 0x24
	keyTab          = 0x30
	keyDelete       = 0x33
	keyEscape       = 0x35
	keyCommand      = 0x37
	keyShift        = 0x38
	keyOption       = 0x3A
	keyControl      = 0x3B
	keyRightOption  = 0x3D
	keyFunction     = 0x3F
	keyGrave        = 0x32
	keyLeftBracket  = 0x21
	keyRightBracket = 0x1E
	keyPageUp       = 0x74
	keyPageDown     = 0x79
	keyLeft         = 0x7B
	keyRight        = 0x7C
	keyDown         = 0x7D
	keyUp           = 0x7E
	mediaMute       = 7
	mediaPlayPause  = 16
	mediaNext       = 17
	mediaPrevious   = 18
	flagShift       = 1 << 17
	flagControl     = 1 << 18
	flagOption      = 1 << 19
	flagCommand     = 1 << 20
	flagSecondaryFn = 1 << 23
)

type voiceKeyBinding struct {
	key       uint16
	downFlags uint64
}

type Desktop struct {
	voiceKey               voiceKeyBinding
	appProfiles            []core.AppProfile
	accessibilityOK        bool
	accessibilityCheckedAt time.Time
	leftDown               bool
	rightDown              bool
	windowSwitching        bool
}

func NewDesktop(voiceKey string, appProfiles []core.AppProfile) (*Desktop, error) {
	key, err := macVoiceKey(voiceKey)
	if err != nil {
		return nil, err
	}
	return &Desktop{voiceKey: key, appProfiles: appProfiles}, nil
}

func (d *Desktop) Ready() error {
	d.accessibilityCheckedAt = time.Now()
	if C.cp_accessibility_trusted() != 0 {
		d.accessibilityOK = true
		return nil
	}
	d.accessibilityOK = false
	C.cp_request_accessibility()
	return ErrAccessibilityPermission
}

func (d *Desktop) MovePointer(dx, dy int) error {
	if err := d.ensureReady(); err != nil {
		return err
	}
	drag := 0
	if d.leftDown {
		drag = 1
	} else if d.rightDown {
		drag = 2
	}
	if C.cp_pointer_move(C.int(dx), C.int(dy), C.int(drag)) != 0 {
		return fmt.Errorf("post macOS pointer event")
	}
	return nil
}

func (d *Desktop) Scroll(amount int) error {
	if err := d.ensureReady(); err != nil {
		return err
	}
	lines := scrollLines(amount)
	if C.cp_scroll(C.int(lines)) != 0 {
		return fmt.Errorf("post macOS scroll event")
	}
	return nil
}

func (d *Desktop) ScrollSmooth(amount float64, phase core.SmoothScrollPhase) error {
	if err := d.ensureReady(); err != nil {
		return err
	}
	// Quartz uses the opposite sign from the portable Windows wheel convention.
	if C.cp_scroll_smooth(C.double(-amount), C.int(phase)) != 0 {
		return fmt.Errorf("post continuous macOS scroll event")
	}
	return nil
}

// Windows wheel deltas and Quartz scroll deltas have opposite signs for the
// same visual direction, so translate at the platform boundary.
func scrollLines(amount int) int {
	lines := -(amount / 120)
	if lines == 0 && amount != 0 {
		if amount > 0 {
			return -1
		}
		return 1
	}
	return lines
}

func (d *Desktop) Perform(action core.Action) error {
	if err := d.ensureReady(); err != nil {
		return err
	}
	switch action {
	case core.ClickLeft:
		return d.click(1)
	case core.ClickRight:
		return d.click(2)
	case core.MouseLeftDown:
		d.leftDown = true
		return mouseButton(1, true)
	case core.MouseLeftUp:
		d.leftDown = false
		return mouseButton(1, false)
	case core.MouseRightDown:
		d.rightDown = true
		return mouseButton(2, true)
	case core.MouseRightUp:
		d.rightDown = false
		return mouseButton(2, false)
	case core.NavigateBack:
		return tapHotkey(keyCommand, keyLeftBracket)
	case core.Escape:
		return tapKey(keyEscape)
	case core.ArrowUp:
		return tapKey(keyUp)
	case core.ArrowDown:
		return tapKey(keyDown)
	case core.ArrowLeft:
		return tapKey(keyLeft)
	case core.ArrowRight:
		return tapKey(keyRight)
	case core.Backspace:
		return tapKey(keyDelete)
	case core.Enter:
		return tapKey(keyReturn)
	case core.TabPrevious:
		return tapHotkey(keyControl, keyShift, keyTab)
	case core.TabNext:
		return tapHotkey(keyControl, keyTab)
	case core.TabNew:
		return tapHotkey(keyCommand, keyT)
	case core.FocusLocation:
		return tapHotkey(keyCommand, keyL)
	case core.Find:
		return tapHotkey(keyCommand, keyF)
	case core.NewDocument:
		return tapHotkey(keyCommand, keyN)
	case core.PageUp:
		return tapKey(keyPageUp)
	case core.PageDown:
		return tapKey(keyPageDown)
	case core.CommandPalette:
		return tapHotkey(keyCommand, keyShift, keyP)
	case core.QuickOpen:
		return tapHotkey(keyCommand, keyP)
	case core.MediaPreviousTrack:
		return mediaKey(mediaPrevious)
	case core.MediaNextTrack:
		return mediaKey(mediaNext)
	case core.MediaPlayPause:
		return mediaKey(mediaPlayPause)
	case core.VolumeMute:
		return mediaKey(mediaMute)
	case core.VoiceTap:
		if err := voiceKeyEvent(d.voiceKey, true); err != nil {
			return err
		}
		time.Sleep(55 * time.Millisecond)
		return voiceKeyEvent(d.voiceKey, false)
	case core.VoiceDown:
		return voiceKeyEvent(d.voiceKey, true)
	case core.VoiceUp:
		return voiceKeyEvent(d.voiceKey, false)
	case core.WindowPrevious:
		return tapHotkey(keyCommand, keyShift, keyTab)
	case core.WindowNext:
		return tapHotkey(keyCommand, keyTab)
	case core.WindowCyclePrevious:
		return d.cycleWindow(true)
	case core.WindowCycleNext:
		return d.cycleWindow(false)
	case core.WindowCycleCommit:
		return d.commitWindowSwitch()
	case core.CodexBack:
		return tapHotkey(keyCommand, keyLeftBracket)
	case core.CodexPreviousTask:
		return tapHotkey(keyCommand, keyShift, keyLeftBracket)
	case core.CodexNextTask:
		return tapHotkey(keyCommand, keyShift, keyRightBracket)
	case core.CodexCommandMenu:
		return tapHotkey(keyCommand, keyK)
	case core.CodexTerminal:
		return tapHotkey(keyCommand, keyGrave)
	default:
		return fmt.Errorf("unsupported macOS action %q", action)
	}
}

func (d *Desktop) ensureReady() error {
	// Accessibility can be revoked while CouchPilot is running. Recheck often
	// enough to stop claiming that actions were dispatched, without putting a
	// privacy API call in every 120 Hz pointer frame.
	if d.accessibilityOK && time.Since(d.accessibilityCheckedAt) < time.Second {
		return nil
	}
	d.accessibilityCheckedAt = time.Now()
	d.accessibilityOK = C.cp_accessibility_trusted() != 0
	if !d.accessibilityOK {
		return ErrAccessibilityPermission
	}
	return nil
}

func (d *Desktop) ForegroundContext() (string, string) {
	buffer := C.malloc(4096)
	if buffer == nil {
		return "default", ""
	}
	defer C.free(buffer)
	if C.cp_frontmost_executable((*C.char)(buffer), 4096) < 0 {
		return "default", ""
	}
	path := C.GoString((*C.char)(buffer))
	return matchProfile(path, d.appProfiles), filepath.Base(path)
}

func (d *Desktop) click(button int) error {
	if err := mouseButton(button, true); err != nil {
		return err
	}
	return mouseButton(button, false)
}

func (d *Desktop) cycleWindow(previous bool) error {
	if !d.windowSwitching {
		if err := keyEventWithFlags(keyCommand, true, flagCommand); err != nil {
			return err
		}
		d.windowSwitching = true
	}
	flags := uint64(flagCommand)
	if previous {
		flags |= flagShift
		if err := keyEventWithFlags(keyShift, true, flags); err != nil {
			_ = d.commitWindowSwitch()
			return err
		}
	}
	err := tapKeyWithFlags(keyTab, flags)
	if previous {
		if releaseErr := keyEventWithFlags(keyShift, false, flagCommand); err == nil {
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
	return keyEventWithFlags(keyCommand, false, 0)
}

func mouseButton(button int, down bool) error {
	value := 0
	if down {
		value = 1
	}
	if C.cp_mouse_button(C.int(button), C.int(value)) != 0 {
		return fmt.Errorf("post macOS mouse button event")
	}
	return nil
}

func keyEvent(key uint16, down bool) error {
	return keyEventWithFlags(key, down, 0)
}

func voiceKeyEvent(binding voiceKeyBinding, down bool) error {
	return keyEventWithFlags(binding.key, down, voiceKeyFlags(binding, down))
}

func voiceKeyFlags(binding voiceKeyBinding, down bool) uint64 {
	if down {
		return binding.downFlags
	}
	return 0
}

func keyEventWithFlags(key uint16, down bool, flags uint64) error {
	value := 0
	if down {
		value = 1
	}
	if C.cp_key_event(C.uint16_t(key), C.int(value), C.uint64_t(flags)) != 0 {
		return fmt.Errorf("post macOS key event")
	}
	return nil
}

func tapKey(key uint16) error {
	return tapKeyWithFlags(key, 0)
}

func tapKeyWithFlags(key uint16, flags uint64) error {
	if err := keyEventWithFlags(key, true, flags); err != nil {
		return err
	}
	time.Sleep(25 * time.Millisecond)
	return keyEventWithFlags(key, false, flags)
}

func tapHotkey(keys ...uint16) error {
	return tapHotkeyWith(keyEventWithFlags, time.Sleep, keys...)
}

type keyEventPoster func(key uint16, down bool, flags uint64) error

func tapHotkeyWith(post keyEventPoster, sleep func(time.Duration), keys ...uint16) (resultErr error) {
	if len(keys) == 0 {
		return nil
	}
	modifiers := keys[:len(keys)-1]
	pressed := make([]uint16, 0, len(modifiers))
	flags := uint64(0)
	defer func() {
		for index := len(pressed) - 1; index >= 0; index-- {
			modifier := pressed[index]
			flags &^= flagForModifier(modifier)
			if err := post(modifier, false, flags); resultErr == nil && err != nil {
				resultErr = err
			}
		}
	}()
	for _, modifier := range modifiers {
		flag := flagForModifier(modifier)
		if flag == 0 {
			return fmt.Errorf("unsupported macOS modifier keycode 0x%02X", modifier)
		}
		nextFlags := flags | flag
		if err := post(modifier, true, nextFlags); err != nil {
			return err
		}
		flags = nextFlags
		pressed = append(pressed, modifier)
	}
	key := keys[len(keys)-1]
	if err := post(key, true, flags); err != nil {
		return err
	}
	sleep(25 * time.Millisecond)
	return post(key, false, flags)
}

func flagsForModifiers(modifiers []uint16) uint64 {
	var flags uint64
	for _, modifier := range modifiers {
		flags |= flagForModifier(modifier)
	}
	return flags
}

func flagForModifier(modifier uint16) uint64 {
	switch modifier {
	case keyCommand:
		return flagCommand
	case keyShift:
		return flagShift
	case keyControl:
		return flagControl
	case keyOption, keyRightOption:
		return flagOption
	default:
		return 0
	}
}

func mediaKey(key int) error {
	if C.cp_media_key(C.int(key)) != 0 {
		return fmt.Errorf("post macOS media key event")
	}
	return nil
}

func macVoiceKey(name string) (voiceKeyBinding, error) {
	switch strings.ToLower(name) {
	case "platform_default", "fn", "function":
		return voiceKeyBinding{key: keyFunction, downFlags: flagSecondaryFn}, nil
	case "right_alt", "alt_right":
		return voiceKeyBinding{key: keyRightOption, downFlags: flagOption}, nil
	case "left_alt", "alt_left":
		return voiceKeyBinding{key: keyOption, downFlags: flagOption}, nil
	default:
		return voiceKeyBinding{}, fmt.Errorf("unsupported macOS voice_key %q", name)
	}
}

func matchProfile(path string, profiles []core.AppProfile) string {
	normalized := strings.ToLower(filepath.ToSlash(path))
	processName := strings.ToLower(filepath.Base(normalized))
	for _, profile := range profiles {
		if !matchesAny(processName, profile.ProcessNames, func(value, candidate string) bool {
			candidate = strings.ToLower(strings.TrimSuffix(candidate, ".exe"))
			return value == candidate
		}) {
			continue
		}
		if !matchesAny(normalized, profile.PathContains, func(value, candidate string) bool {
			candidate = strings.ToLower(strings.ReplaceAll(candidate, `\`, "/"))
			return strings.Contains(value, candidate)
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
