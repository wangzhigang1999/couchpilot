package desktop

import (
	"fmt"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/mapping"
)

type Key string

const (
	KeyEscape       Key = "escape"
	KeyArrowUp      Key = "arrow_up"
	KeyArrowDown    Key = "arrow_down"
	KeyArrowLeft    Key = "arrow_left"
	KeyArrowRight   Key = "arrow_right"
	KeyBackspace    Key = "backspace"
	KeyEnter        Key = "enter"
	KeyTab          Key = "tab"
	KeyPageUp       Key = "page_up"
	KeyPageDown     Key = "page_down"
	KeyLeftBracket  Key = "left_bracket"
	KeyRightBracket Key = "right_bracket"
	KeyGrave        Key = "grave"
	KeyF            Key = "f"
	KeyK            Key = "k"
	KeyL            Key = "l"
	KeyN            Key = "n"
	KeyP            Key = "p"
	KeyT            Key = "t"
)

type Modifier string

const (
	ModifierPrimary Modifier = "primary"
	ModifierControl Modifier = "control"
	ModifierShift   Modifier = "shift"
)

type MouseButton string

const (
	MouseLeft  MouseButton = "left"
	MouseRight MouseButton = "right"
)

type ButtonState string

const (
	ButtonDown ButtonState = "down"
	ButtonUp   ButtonState = "up"
)

type Direction string

const (
	Previous Direction = "previous"
	Next     Direction = "next"
)

type VoiceEvent string

const (
	VoiceTap  VoiceEvent = "tap"
	VoiceDown VoiceEvent = "down"
	VoiceUp   VoiceEvent = "up"
)

type MediaKey string

const (
	MediaPrevious  MediaKey = "previous"
	MediaNext      MediaKey = "next"
	MediaPlayPause MediaKey = "play_pause"
	MediaMute      MediaKey = "mute"
)

// Driver is the narrow platform port. It contains OS primitives, not
// CouchPilot's user-facing actions or binding policy.
type Driver interface {
	MovePointer(dx, dy int) error
	Scroll(amount int) error
	TapKey(Key) error
	TapChord([]Modifier, Key) error
	Mouse(MouseButton, ButtonState) error
	NavigateBack() error
	SwitchWindow(Direction) error
	CycleWindow(Direction) error
	CommitWindowSwitch() error
	Voice(VoiceEvent) error
	Media(MediaKey) error
	ForegroundApplication() core.AppIdentity
}

type SmoothDriver interface {
	ScrollSmooth(amount float64, phase core.SmoothScrollPhase) error
}

type SmoothDesktopDriver interface {
	Driver
	SmoothDriver
}

type readiness interface {
	Ready() error
}

// Executor translates stable semantic actions into portable operation recipes.
// Adding a normal shortcut should require editing this file, not every OS.
type Executor struct {
	driver  Driver
	matcher *mapping.ProfileMatcher
}

var _ core.Desktop = (*Executor)(nil)
var _ core.Readiness = (*Executor)(nil)

func NewExecutor(driver Driver, profiles []core.AppProfile) *Executor {
	return &Executor{
		driver:  driver,
		matcher: mapping.NewProfileMatcher(profiles),
	}
}

func (e *Executor) MovePointer(dx, dy int) error {
	return e.driver.MovePointer(dx, dy)
}

func (e *Executor) Scroll(amount int) error {
	return e.driver.Scroll(amount)
}

func (e *Executor) Ready() error {
	if driver, ok := e.driver.(readiness); ok {
		return driver.Ready()
	}
	return nil
}

func (e *Executor) ForegroundContext() (string, string) {
	identity := e.driver.ForegroundApplication()
	return e.matcher.Match(identity), identity.ProcessName
}

func (e *Executor) Perform(action core.Action) error {
	switch action {
	case core.ClickLeft:
		return e.click(MouseLeft)
	case core.ClickRight:
		return e.click(MouseRight)
	case core.NavigateBack:
		return e.driver.NavigateBack()
	case core.Escape:
		return e.driver.TapKey(KeyEscape)
	case core.ArrowUp:
		return e.driver.TapKey(KeyArrowUp)
	case core.ArrowDown:
		return e.driver.TapKey(KeyArrowDown)
	case core.ArrowLeft:
		return e.driver.TapKey(KeyArrowLeft)
	case core.ArrowRight:
		return e.driver.TapKey(KeyArrowRight)
	case core.Backspace:
		return e.driver.TapKey(KeyBackspace)
	case core.Enter:
		return e.driver.TapKey(KeyEnter)
	case core.TabPrevious:
		return e.driver.TapChord([]Modifier{ModifierControl, ModifierShift}, KeyTab)
	case core.TabNext:
		return e.driver.TapChord([]Modifier{ModifierControl}, KeyTab)
	case core.TabNew:
		return e.driver.TapChord([]Modifier{ModifierPrimary}, KeyT)
	case core.FocusLocation:
		return e.driver.TapChord([]Modifier{ModifierPrimary}, KeyL)
	case core.Find:
		return e.driver.TapChord([]Modifier{ModifierPrimary}, KeyF)
	case core.NewDocument:
		return e.driver.TapChord([]Modifier{ModifierPrimary}, KeyN)
	case core.PageUp:
		return e.driver.TapKey(KeyPageUp)
	case core.PageDown:
		return e.driver.TapKey(KeyPageDown)
	case core.CommandPalette:
		return e.driver.TapChord([]Modifier{ModifierPrimary, ModifierShift}, KeyP)
	case core.QuickOpen:
		return e.driver.TapChord([]Modifier{ModifierPrimary}, KeyP)
	case core.MediaPreviousTrack:
		return e.driver.Media(MediaPrevious)
	case core.MediaNextTrack:
		return e.driver.Media(MediaNext)
	case core.MediaPlayPause:
		return e.driver.Media(MediaPlayPause)
	case core.VolumeMute:
		return e.driver.Media(MediaMute)
	case core.WindowPrevious:
		return e.driver.SwitchWindow(Previous)
	case core.WindowNext:
		return e.driver.SwitchWindow(Next)
	case core.CodexBack:
		return e.driver.TapChord([]Modifier{ModifierPrimary}, KeyLeftBracket)
	case core.CodexPreviousTask:
		return e.driver.TapChord([]Modifier{ModifierPrimary, ModifierShift}, KeyLeftBracket)
	case core.CodexNextTask:
		return e.driver.TapChord([]Modifier{ModifierPrimary, ModifierShift}, KeyRightBracket)
	case core.CodexCommandMenu:
		return e.driver.TapChord([]Modifier{ModifierPrimary}, KeyK)
	case core.CodexTerminal:
		return e.driver.TapChord([]Modifier{ModifierPrimary}, KeyGrave)
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
}

func (e *Executor) PerformOperation(operation core.Operation) error {
	switch operation {
	case core.MouseLeftDown:
		return e.driver.Mouse(MouseLeft, ButtonDown)
	case core.MouseLeftUp:
		return e.driver.Mouse(MouseLeft, ButtonUp)
	case core.MouseRightDown:
		return e.driver.Mouse(MouseRight, ButtonDown)
	case core.MouseRightUp:
		return e.driver.Mouse(MouseRight, ButtonUp)
	case core.VoiceTap:
		return e.driver.Voice(VoiceTap)
	case core.VoiceDown:
		return e.driver.Voice(VoiceDown)
	case core.VoiceUp:
		return e.driver.Voice(VoiceUp)
	case core.WindowCyclePrevious:
		return e.driver.CycleWindow(Previous)
	case core.WindowCycleNext:
		return e.driver.CycleWindow(Next)
	case core.WindowCycleCommit:
		return e.driver.CommitWindowSwitch()
	default:
		return fmt.Errorf("unsupported operation %q", operation)
	}
}

func (e *Executor) click(button MouseButton) error {
	if err := e.driver.Mouse(button, ButtonDown); err != nil {
		return err
	}
	return e.driver.Mouse(button, ButtonUp)
}

type SmoothExecutor struct {
	*Executor
	driver SmoothDriver
}

var _ core.SmoothScroller = (*SmoothExecutor)(nil)

func NewSmoothExecutor(driver SmoothDesktopDriver, profiles []core.AppProfile) *SmoothExecutor {
	return &SmoothExecutor{
		Executor: NewExecutor(driver, profiles),
		driver:   driver,
	}
}

func (e *SmoothExecutor) ScrollSmooth(amount float64, phase core.SmoothScrollPhase) error {
	return e.driver.ScrollSmooth(amount, phase)
}
