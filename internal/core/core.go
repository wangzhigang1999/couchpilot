package core

import "time"

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
	_ // reserved XInput bit
	_ // reserved XInput bit
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

// AppProfile describes how a foreground application maps to a binding profile.
// Match fields are case-insensitive. Values within a field are ORed; populated
// fields are ANDed, so process_names plus path_contains can disambiguate apps
// that share an executable name.
type AppProfile struct {
	Name         string   `json:"name"`
	ProcessNames []string `json:"process_names,omitempty"`
	PathContains []string `json:"path_contains,omitempty"`
}

type Action string

const (
	ClickLeft           Action = "click_left"
	ClickRight          Action = "click_right"
	MouseLeftDown       Action = "mouse_left_down"
	MouseLeftUp         Action = "mouse_left_up"
	MouseRightDown      Action = "mouse_right_down"
	MouseRightUp        Action = "mouse_right_up"
	NavigateBack        Action = "navigate_back"
	Escape              Action = "escape"
	ArrowUp             Action = "arrow_up"
	ArrowDown           Action = "arrow_down"
	ArrowLeft           Action = "arrow_left"
	ArrowRight          Action = "arrow_right"
	Backspace           Action = "backspace"
	Enter               Action = "enter"
	TabPrevious         Action = "tab_previous"
	TabNext             Action = "tab_next"
	TabNew              Action = "tab_new"
	FocusLocation       Action = "focus_location"
	Find                Action = "find"
	NewDocument         Action = "new_document"
	PageUp              Action = "page_up"
	PageDown            Action = "page_down"
	CommandPalette      Action = "command_palette"
	QuickOpen           Action = "quick_open"
	MediaPreviousTrack  Action = "media_previous_track"
	MediaNextTrack      Action = "media_next_track"
	MediaPlayPause      Action = "media_play_pause"
	VolumeMute          Action = "volume_mute"
	Voice               Action = "voice"
	VoiceTap            Action = "voice_tap"
	VoiceDown           Action = "voice_down"
	VoiceUp             Action = "voice_up"
	WindowPrevious      Action = "window_previous"
	WindowNext          Action = "window_next"
	WindowCyclePrevious Action = "window_cycle_previous"
	WindowCycleNext     Action = "window_cycle_next"
	WindowCycleCommit   Action = "window_cycle_commit"
	CodexBack           Action = "codex_back"
	CodexPreviousTask   Action = "codex_previous_task"
	CodexNextTask       Action = "codex_next_task"
	CodexCommandMenu    Action = "codex_command_menu"
	CodexTerminal       Action = "codex_terminal"
	ChromePreviousTab   Action = "chrome_previous_tab"
	ChromeNextTab       Action = "chrome_next_tab"
	ChromeAddressBar    Action = "chrome_address_bar"
	ChromeNewTab        Action = "chrome_new_tab"
)

var KnownActions = []Action{
	ClickLeft, ClickRight, NavigateBack, Escape,
	ArrowUp, ArrowDown, ArrowLeft, ArrowRight, Backspace, Enter,
	TabPrevious, TabNext, TabNew, FocusLocation, Find, NewDocument,
	PageUp, PageDown, CommandPalette, QuickOpen,
	MediaPreviousTrack, MediaNextTrack, MediaPlayPause, VolumeMute,
	Voice, WindowPrevious, WindowNext,
	CodexBack, CodexPreviousTask, CodexNextTask, CodexCommandMenu, CodexTerminal,
	ChromePreviousTab, ChromeNextTab, ChromeAddressBar, ChromeNewTab,
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
	// ForegroundContext returns the matched CouchPilot profile and the
	// foreground executable's base name. It never returns the full process path
	// or a window title.
	ForegroundContext() (profile, processName string)
}

type Clock interface {
	Now() time.Time
	Sleep(time.Duration)
}

type RealClock struct{}

func (RealClock) Now() time.Time        { return time.Now() }
func (RealClock) Sleep(d time.Duration) { time.Sleep(d) }
