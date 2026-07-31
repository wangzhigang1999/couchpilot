package core

import (
	"errors"
	"time"
)

var ErrHapticsUnsupported = errors.New("controller haptics are unsupported")

type DeviceID string

type Button uint16

const (
	DPadUp Button = 1 << iota
	DPadDown
	DPadLeft
	DPadRight
	Start
	Back
	LeftThumb
	RightThumb
	LeftShoulder
	RightShoulder
	A
	B
	X
	Y
)

type State struct {
	PacketNumber uint32
	Buttons      Button
	LeftTrigger  float64
	RightTrigger float64
	LeftX        float64
	LeftY        float64
	RightX       float64
	RightY       float64
}

type Gamepad interface {
	Devices() ([]DeviceID, error)
	Read(DeviceID, float64) (State, bool, error)
	Rumble(DeviceID, uint16, uint16) error
}

// GamepadDiagnostics is an optional raw-input view used when calibrating a
// platform driver. Implementations should not alter controller state.
type GamepadDiagnostics interface {
	Diagnostic(DeviceID) (string, error)
}

// GamepadCapabilities describes optional device features without forcing the
// portable engine to probe them by deliberately failing an operation.
type GamepadCapabilities interface {
	HapticsSupported(DeviceID) bool
}

// AppProfile describes how a foreground application maps to a binding profile.
// Match fields are case-insensitive. Values within a field are ORed; populated
// fields are ANDed, so process_names plus path_contains can disambiguate apps
// that share an executable name.
type AppProfile struct {
	Name         string   `json:"name"`
	ProcessNames []string `json:"process_names,omitempty"`
	PathContains []string `json:"path_contains,omitempty"`
}

// AppIdentity is the transient, platform-neutral identity of the foreground
// application. ExecutablePath is used only for in-memory profile matching;
// callers should persist ProcessName instead of the full path.
type AppIdentity struct {
	ProcessName    string
	ExecutablePath string
}

type Action string

const (
	ClickLeft          Action = "click_left"
	ClickRight         Action = "click_right"
	NavigateBack       Action = "navigate_back"
	Escape             Action = "escape"
	ArrowUp            Action = "arrow_up"
	ArrowDown          Action = "arrow_down"
	ArrowLeft          Action = "arrow_left"
	ArrowRight         Action = "arrow_right"
	Backspace          Action = "backspace"
	Enter              Action = "enter"
	TabPrevious        Action = "tab_previous"
	TabNext            Action = "tab_next"
	TabNew             Action = "tab_new"
	FocusLocation      Action = "focus_location"
	Find               Action = "find"
	NewDocument        Action = "new_document"
	PageUp             Action = "page_up"
	PageDown           Action = "page_down"
	CommandPalette     Action = "command_palette"
	QuickOpen          Action = "quick_open"
	MediaPreviousTrack Action = "media_previous_track"
	MediaNextTrack     Action = "media_next_track"
	MediaPlayPause     Action = "media_play_pause"
	VolumeMute         Action = "volume_mute"
	Voice              Action = "voice"
	WindowPrevious     Action = "window_previous"
	WindowNext         Action = "window_next"
	CodexBack          Action = "codex_back"
	CodexPreviousTask  Action = "codex_previous_task"
	CodexNextTask      Action = "codex_next_task"
	CodexCommandMenu   Action = "codex_command_menu"
	CodexTerminal      Action = "codex_terminal"
)

// Operation is an engine-only lifecycle command. Unlike Action, it is not a
// user-configurable binding and is never accepted in config.json.
type Operation string

const (
	MouseLeftDown       Operation = "mouse_left_down"
	MouseLeftUp         Operation = "mouse_left_up"
	MouseRightDown      Operation = "mouse_right_down"
	MouseRightUp        Operation = "mouse_right_up"
	VoiceTap            Operation = "voice_tap"
	VoiceDown           Operation = "voice_down"
	VoiceUp             Operation = "voice_up"
	WindowCyclePrevious Operation = "window_cycle_previous"
	WindowCycleNext     Operation = "window_cycle_next"
	WindowCycleCommit   Operation = "window_cycle_commit"
)

var KnownActions = []Action{
	ClickLeft, ClickRight, NavigateBack, Escape,
	ArrowUp, ArrowDown, ArrowLeft, ArrowRight, Backspace, Enter,
	TabPrevious, TabNext, TabNew, FocusLocation, Find, NewDocument,
	PageUp, PageDown, CommandPalette, QuickOpen,
	MediaPreviousTrack, MediaNextTrack, MediaPlayPause, VolumeMute,
	Voice, WindowPrevious, WindowNext,
	CodexBack, CodexPreviousTask, CodexNextTask, CodexCommandMenu, CodexTerminal,
}

func IsKnownAction(action Action) bool {
	for _, known := range KnownActions {
		if action == known {
			return true
		}
	}
	return false
}

type Desktop interface {
	MovePointer(dx, dy int) error
	Scroll(amount int) error
	Perform(Action) error
	PerformOperation(Operation) error
	// ForegroundContext returns the matched CouchPilot profile and the
	// foreground executable's base name. It never returns the full process path
	// or a window title.
	ForegroundContext() (profile, processName string)
}

// Readiness is implemented by adapters that require an operating-system
// permission before they can safely report actions as dispatched.
type Readiness interface {
	Ready() error
}

// SmoothScroller is an optional pixel-precision scroll path for platforms
// that support continuous gestures. Scroll remains the portable wheel API.
type SmoothScrollPhase uint8

const (
	SmoothScrollBegan   SmoothScrollPhase = 1
	SmoothScrollChanged SmoothScrollPhase = 2
	SmoothScrollEnded   SmoothScrollPhase = 4
)

type SmoothScroller interface {
	ScrollSmooth(amount float64, phase SmoothScrollPhase) error
}

type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

type RealClock struct{}

func (RealClock) Now() time.Time        { return time.Now() }
func (RealClock) Sleep(d time.Duration) { time.Sleep(d) }
