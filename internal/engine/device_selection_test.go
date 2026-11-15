package engine

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/wangzhigang1999/couchpilot/internal/core"
)

func TestRunHonorsControllerSelection(t *testing.T) {
	for _, test := range []struct {
		name  string
		id    string
		index int
		want  []core.Action
	}{
		{"explicit ID overrides index", "test:1", 0, []core.Action{core.Action(core.MouseRightDown), core.Action(core.MouseRightUp)}},
		{"missing ID does not fall back to index", "missing", 0, nil},
		{"missing ID does not fall back to automatic", "missing", -1, nil},
		{"index selects second controller", "", 1, []core.Action{core.Action(core.MouseRightDown), core.Action(core.MouseRightUp)}},
		{"missing index does not fall back", "", 2, nil},
		{"active controller keeps priority", "", -1, []core.Action{core.Action(core.MouseLeftDown), core.Action(core.MouseLeftUp)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := defaultOptions()
			settings.DeviceID = test.id
			settings.ControllerIndex = test.index
			gamepad := &multiGamepad{
				devices: []core.DeviceID{"test:0", "test:1"},
				states: map[core.DeviceID]core.State{
					"test:0": {Buttons: core.A},
					"test:1": {Buttons: core.X},
				},
			}
			desktop := &fakeDesktop{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			controller := New(settings, gamepad, desktop, false, nil)
			controller.clock = &cancelOnSleepClock{now: time.Unix(1, 0), cancel: cancel}
			if err := controller.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(desktop.actions, test.want) {
				t.Fatalf("actions=%v, want %v", desktop.actions, test.want)
			}
		})
	}
}
