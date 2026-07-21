package engine

import "github.com/wangzhigang1999/couchpilot/internal/core"

type BindingResolution string

const (
	BindingBound    BindingResolution = "bound"
	BindingDisabled BindingResolution = "disabled"
	BindingUnbound  BindingResolution = "unbound"
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
	Resolution     BindingResolution
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
	return resolved.Action, resolved.Resolution == BindingBound
}

func (r *Resolver) ResolveDetailed(profile, gesture string) ResolvedBinding {
	if profile == "" {
		profile = "default"
	}
	resolved := ResolvedBinding{
		ActiveProfile: profile,
		Gesture:       gesture,
		Resolution:    BindingUnbound,
	}
	if action, found := r.lookup(profile, gesture); found {
		resolved.BindingProfile = profile
		resolved.Action = action
		if action == "" {
			resolved.Resolution = BindingDisabled
		} else {
			resolved.Resolution = BindingBound
		}
		return resolved
	}
	if profile != "default" {
		if action, found := r.lookup("default", gesture); found {
			resolved.BindingProfile = "default"
			resolved.Action = action
			if action == "" {
				resolved.Resolution = BindingDisabled
			} else {
				resolved.Resolution = BindingBound
			}
		}
	}
	return resolved
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
	}
}
