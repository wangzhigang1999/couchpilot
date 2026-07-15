package winplatform

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	winapi "golang.org/x/sys/windows"
)

const errorDeviceNotConnected = 1167

type xinputGamepad struct {
	Buttons      uint16
	LeftTrigger  uint8
	RightTrigger uint8
	ThumbLX      int16
	ThumbLY      int16
	ThumbRX      int16
	ThumbRY      int16
}

type xinputState struct {
	PacketNumber uint32
	Gamepad      xinputGamepad
}

type xinputVibration struct {
	LeftMotor  uint16
	RightMotor uint16
}

type Gamepad struct {
	getState *winapi.LazyProc
	setState *winapi.LazyProc
}

func NewGamepad() (*Gamepad, error) {
	var failures []string
	for _, name := range []string{"xinput1_4.dll", "xinput9_1_0.dll", "xinput1_3.dll"} {
		dll := winapi.NewLazySystemDLL(name)
		if err := dll.Load(); err != nil {
			failures = append(failures, name+": "+err.Error())
			continue
		}
		getState := dll.NewProc("XInputGetState")
		setState := dll.NewProc("XInputSetState")
		if err := getState.Find(); err != nil {
			failures = append(failures, name+": "+err.Error())
			continue
		}
		if err := setState.Find(); err != nil {
			failures = append(failures, name+": "+err.Error())
			continue
		}
		return &Gamepad{getState: getState, setState: setState}, nil
	}
	return nil, fmt.Errorf("no XInput DLL available: %s", strings.Join(failures, "; "))
}

func (g *Gamepad) Devices() ([]core.DeviceID, error) {
	devices := make([]core.DeviceID, 0, 4)
	for index := 0; index < 4; index++ {
		_, connected, err := g.readIndex(index, 0.18)
		if err != nil {
			return nil, err
		}
		if connected {
			devices = append(devices, core.DeviceID(fmt.Sprintf("xinput:%d", index)))
		}
	}
	return devices, nil
}

func (g *Gamepad) Read(device core.DeviceID, deadzone float64) (core.State, bool, error) {
	index, err := deviceIndex(device)
	if err != nil {
		return core.State{}, false, err
	}
	return g.readIndex(index, deadzone)
}

func (g *Gamepad) readIndex(index int, deadzone float64) (core.State, bool, error) {
	var raw xinputState
	result, _, _ := g.getState.Call(uintptr(index), uintptr(unsafe.Pointer(&raw)))
	runtime.KeepAlive(&raw)
	if result == errorDeviceNotConnected {
		return core.State{}, false, nil
	}
	if result != 0 {
		return core.State{}, false, fmt.Errorf("XInputGetState(%d): error %d", index, result)
	}
	leftX, leftY := normalizeStick(raw.Gamepad.ThumbLX, raw.Gamepad.ThumbLY, deadzone)
	rightX, rightY := normalizeStick(raw.Gamepad.ThumbRX, raw.Gamepad.ThumbRY, deadzone)
	return core.State{
		PacketNumber: raw.PacketNumber,
		Buttons:      core.Button(raw.Gamepad.Buttons),
		LeftTrigger:  float64(raw.Gamepad.LeftTrigger) / 255,
		RightTrigger: float64(raw.Gamepad.RightTrigger) / 255,
		LeftX:        leftX,
		LeftY:        leftY,
		RightX:       rightX,
		RightY:       rightY,
	}, true, nil
}

func (g *Gamepad) Rumble(device core.DeviceID, left, right uint16) error {
	index, err := deviceIndex(device)
	if err != nil {
		return err
	}
	vibration := xinputVibration{LeftMotor: left, RightMotor: right}
	result, _, _ := g.setState.Call(uintptr(index), uintptr(unsafe.Pointer(&vibration)))
	runtime.KeepAlive(&vibration)
	if result == errorDeviceNotConnected {
		return nil
	}
	if result != 0 {
		return fmt.Errorf("XInputSetState(%d): error %d", index, result)
	}
	return nil
}

func deviceIndex(device core.DeviceID) (int, error) {
	prefix, value, found := strings.Cut(string(device), ":")
	if !found || prefix != "xinput" {
		return 0, fmt.Errorf("unsupported Windows gamepad id %q", device)
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 0 || index > 3 {
		return 0, fmt.Errorf("invalid XInput index in %q", device)
	}
	return index, nil
}

func normalizeStick(x, y int16, deadzone float64) (float64, float64) {
	fx := math.Max(-1, float64(x)/32767)
	fy := math.Max(-1, float64(y)/32767)
	magnitude := math.Min(1, math.Hypot(fx, fy))
	if magnitude <= deadzone {
		return 0, 0
	}
	scaled := (magnitude - deadzone) / (1 - deadzone)
	return fx / magnitude * scaled, fy / magnitude * scaled
}
