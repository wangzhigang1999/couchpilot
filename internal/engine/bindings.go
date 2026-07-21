package engine

import (
	"sort"

	"github.com/wangzhigang1999/couchpilot/internal/core"
	"github.com/wangzhigang1999/couchpilot/internal/usage"
)

type Resolver struct {
	bindings map[string]map[string]string
}

// ResolvedBinding describes both what the active profile requested and which
// profile actually supplied the binding. Keeping those labels distinct makes
// default-profile fallbacks visible without changing resolution behavior.
type ResolvedBinding struct {
	ActiveProfile  string
	BindingProfile string
	Gesture        string
	Action         core.Action
	Resolution     usage.Resolution
}

func NewResolver(overrides map[string]map[string]string) *Resolver {
	bindings := defaultBindings()
	for profile, profileBindings := range overrides {
		if bindings[profile] == nil {
			bindings[profile] = map[string]string{}
		}
		for gesture, action := range profileBindings {
			bindings[profile][gesture] = action
		}
	}
	return &Resolver{bindings: bindings}
}

func (r *Resolver) Resolve(profile, gesture string) (core.Action, bool) {
	resolved := r.ResolveDetailed(profile, gesture)
	return resolved.Action, resolved.Resolution == usage.ResolutionBound
}

func (r *Resolver) ResolveDetailed(profile, gesture string) ResolvedBinding {
	if profile == "" {
		profile = "default"
	}
	resolved := ResolvedBinding{
		ActiveProfile: profile,
		Gesture:       gesture,
		Resolution:    usage.ResolutionUnbound,
	}
	if action, found := r.lookup(profile, gesture); found {
		resolved.BindingProfile = profile
		resolved.Action = action
		if action == "" {
			resolved.Resolution = usage.ResolutionDisabled
		} else {
			resolved.Resolution = usage.ResolutionBound
		}
		return resolved
	}
	if profile != "default" {
		if action, found := r.lookup("default", gesture); found {
			resolved.BindingProfile = "default"
			resolved.Action = action
			if action == "" {
				resolved.Resolution = usage.ResolutionDisabled
			} else {
				resolved.Resolution = usage.ResolutionBound
			}
		}
	}
	return resolved
}

// BindingInventory returns the effective binding table in a deterministic
// order. Disabled definitions remain in the inventory so reports can
// distinguish an explicit opt-out from a gesture that was never configured.
func (r *Resolver) BindingInventory() []usage.BindingDefinition {
	definitions := make([]usage.BindingDefinition, 0)
	for profile, bindings := range r.bindings {
		for gesture, action := range bindings {
			resolution := usage.ResolutionBound
			if action == "" {
				resolution = usage.ResolutionDisabled
			}
			definitions = append(definitions, usage.BindingDefinition{
				Profile:    profile,
				Gesture:    gesture,
				Action:     action,
				Resolution: resolution,
			})
		}
	}
	sort.Slice(definitions, func(left, right int) bool {
		if definitions[left].Profile != definitions[right].Profile {
			return definitions[left].Profile < definitions[right].Profile
		}
		if definitions[left].Gesture != definitions[right].Gesture {
			return definitions[left].Gesture < definitions[right].Gesture
		}
		return definitions[left].Action < definitions[right].Action
	})
	return definitions
}

func (r *Resolver) lookup(profile, gesture string) (core.Action, bool) {
	bindings, found := r.bindings[profile]
	if !found {
		return "", false
	}
	action, found := bindings[gesture]
	return core.Action(action), found
}

func defaultBindings() map[string]map[string]string {
	return map[string]map[string]string{
		"default": {
			"a":          string(core.ClickLeft),
			"b":          string(core.NavigateBack),
			"x":          string(core.ClickRight),
			"y":          string(core.Voice),
			"dpad_up":    string(core.ArrowUp),
			"dpad_down":  string(core.ArrowDown),
			"dpad_left":  string(core.ArrowLeft),
			"dpad_right": string(core.ArrowRight),
			"lt+lb":      string(core.WindowPrevious),
			"lt+rb":      string(core.WindowNext),
		},
		"codex": {
			"b":       string(core.CodexBack),
			"lb":      string(core.CodexPreviousTask),
			"rb":      string(core.CodexNextTask),
			"l3":      string(core.CodexCommandMenu),
			"r3":      string(core.CodexTerminal),
			"rt+a":    string(core.Enter),
			"voice+a": string(core.Enter),
			"voice+b": string(core.Backspace),
		},
		"chrome": {
			"lb": string(core.TabPrevious),
			"rb": string(core.TabNext),
			"l3": string(core.FocusLocation),
			"r3": string(core.TabNew),
		},
		"raycast": {
			"a":  string(core.Enter),
			"b":  string(core.Escape),
			"lb": string(core.ArrowUp),
			"rb": string(core.ArrowDown),
		},
		"typeless": {
			"b": string(core.Escape),
		},
		"notes": {
			"lb": string(core.TabPrevious),
			"rb": string(core.TabNext),
			"l3": string(core.Find),
			"r3": string(core.NewDocument),
		},
		"vscode": {
			"b":  string(core.Escape),
			"lb": string(core.TabPrevious),
			"rb": string(core.TabNext),
			"l3": string(core.CommandPalette),
			"r3": string(core.QuickOpen),
		},
		"jetbrains": {
			"b":  string(core.Escape),
			"l3": string(core.Find),
		},
		"chat": {
			"b":       string(core.Escape),
			"l3":      string(core.Find),
			"rt+a":    string(core.Enter),
			"voice+a": string(core.Enter),
			"voice+b": string(core.Backspace),
		},
		"assistant": {
			"b":       string(core.Escape),
			"l3":      string(core.Find),
			"rt+a":    string(core.Enter),
			"voice+a": string(core.Enter),
			"voice+b": string(core.Backspace),
		},
		"media": {
			"lb": string(core.MediaPreviousTrack),
			"rb": string(core.MediaNextTrack),
			"l3": string(core.VolumeMute),
			"r3": string(core.MediaPlayPause),
		},
		"document": {
			"lb": string(core.PageUp),
			"rb": string(core.PageDown),
			"l3": string(core.Find),
		},
		"terminal": {
			"b":  string(core.Escape),
			"lb": string(core.TabPrevious),
			"rb": string(core.TabNext),
			"l3": string(core.CommandPalette),
			"r3": string(core.TabNew),
		},
	}
}
