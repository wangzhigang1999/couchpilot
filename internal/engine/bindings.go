package engine

import "github.com/wangzhigang1999/couchpilot/internal/core"

type Resolver struct {
	bindings map[string]map[string]string
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
	if profile == "" {
		profile = "default"
	}
	if action, found := r.lookup(profile, gesture); found {
		return action, action != ""
	}
	if profile != "default" {
		if action, found := r.lookup("default", gesture); found {
			return action, action != ""
		}
	}
	return "", false
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
			"b":  string(core.CodexBack),
			"lb": string(core.CodexPreviousTask),
			"rb": string(core.CodexNextTask),
			"l3": string(core.CodexCommandMenu),
			"r3": string(core.CodexTerminal),
		},
		"chrome": {
			"lb": string(core.ChromePreviousTab),
			"rb": string(core.ChromeNextTab),
			"l3": string(core.ChromeAddressBar),
			"r3": string(core.ChromeNewTab),
		},
	}
}
