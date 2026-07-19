//go:build windows

package tray

import (
	"context"
	"testing"
	"unsafe"
)

func TestWindowsStructuresMatchWindowsLayout(t *testing.T) {
	wantNotify := uintptr(956)
	wantMessage := uintptr(32)
	wantClass := uintptr(48)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		wantNotify = 976
		wantMessage = 48
		wantClass = 80
	}
	if got := unsafe.Sizeof(notifyIconData{}); got != wantNotify {
		t.Errorf("notifyIconData size = %d, want %d", got, wantNotify)
	}
	if got := unsafe.Sizeof(message{}); got != wantMessage {
		t.Errorf("message size = %d, want %d", got, wantMessage)
	}
	if got := unsafe.Sizeof(windowClassEx{}); got != wantClass {
		t.Errorf("windowClassEx size = %d, want %d", got, wantClass)
	}
	wantIdentifier := uintptr(28)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		wantIdentifier = 40
	}
	if got := unsafe.Sizeof(notifyIconIdentifier{}); got != wantIdentifier {
		t.Errorf("notifyIconIdentifier size = %d, want %d", got, wantIdentifier)
	}
	if got := unsafe.Sizeof(trackPopupMenuParams{}); got != 20 {
		t.Errorf("trackPopupMenuParams size = %d, want 20", got)
	}
	if got := unsafe.Sizeof(monitorInfo{}); got != 40 {
		t.Errorf("monitorInfo size = %d, want 40", got)
	}

	notify := notifyIconData{}
	offsets := map[string]struct {
		got    uintptr
		want32 uintptr
		want64 uintptr
	}{
		"Window":      {unsafe.Offsetof(notify.Window), 4, 8},
		"ID":          {unsafe.Offsetof(notify.ID), 8, 16},
		"Icon":        {unsafe.Offsetof(notify.Icon), 20, 32},
		"Tip":         {unsafe.Offsetof(notify.Tip), 24, 40},
		"Info":        {unsafe.Offsetof(notify.Info), 288, 304},
		"Version":     {unsafe.Offsetof(notify.Version), 800, 816},
		"GUID":        {unsafe.Offsetof(notify.GUID), 936, 952},
		"BalloonIcon": {unsafe.Offsetof(notify.BalloonIcon), 952, 968},
	}
	for name, offset := range offsets {
		want := offset.want32
		if unsafe.Sizeof(uintptr(0)) == 8 {
			want = offset.want64
		}
		if offset.got != want {
			t.Errorf("notifyIconData.%s offset = %d, want %d", name, offset.got, want)
		}
	}
}

func TestPopupAnchorAvoidsTaskbarAndScreenEdges(t *testing.T) {
	tests := []struct {
		name       string
		icon       rect
		work       rect
		wantAnchor point
		wantRight  bool
		wantBottom bool
	}{
		{
			name:       "bottom taskbar near right",
			icon:       rect{Left: 1800, Top: 1040, Right: 1820, Bottom: 1060},
			work:       rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040},
			wantAnchor: point{X: 1820, Y: 1040},
			wantRight:  true,
			wantBottom: true,
		},
		{
			name:       "top taskbar near left",
			icon:       rect{Left: 100, Top: 0, Right: 120, Bottom: 40},
			work:       rect{Left: 0, Top: 40, Right: 1920, Bottom: 1080},
			wantAnchor: point{X: 100, Y: 40},
		},
		{
			name:       "left taskbar",
			icon:       rect{Left: 0, Top: 500, Right: 40, Bottom: 520},
			work:       rect{Left: 40, Top: 0, Right: 1920, Bottom: 1080},
			wantAnchor: point{X: 40, Y: 520},
		},
		{
			name:       "right taskbar",
			icon:       rect{Left: 1880, Top: 500, Right: 1920, Bottom: 520},
			work:       rect{Left: 0, Top: 0, Right: 1880, Bottom: 1080},
			wantAnchor: point{X: 1880, Y: 520},
			wantRight:  true,
		},
		{
			name:       "overflow flyout near bottom right",
			icon:       rect{Left: 1800, Top: 900, Right: 1820, Bottom: 920},
			work:       rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040},
			wantAnchor: point{X: 1820, Y: 900},
			wantRight:  true,
			wantBottom: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			anchor, alignment := popupAnchorForWorkArea(test.icon, test.work)
			if anchor != test.wantAnchor {
				t.Fatalf("anchor = %+v, want %+v", anchor, test.wantAnchor)
			}
			if got := alignment&tpmRightAlign != 0; got != test.wantRight {
				t.Errorf("right aligned = %t, want %t", got, test.wantRight)
			}
			if got := alignment&tpmBottomAlign != 0; got != test.wantBottom {
				t.Errorf("bottom aligned = %t, want %t", got, test.wantBottom)
			}
		})
	}
}

func TestExitCommandCancelsWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tray := nativeTray{cancel: cancel}

	tray.handleCommand(menuExitCouchPilot)

	select {
	case <-ctx.Done():
	default:
		t.Fatal("exit command did not cancel the worker context")
	}
}

func TestCopyUTF16AlwaysTerminates(t *testing.T) {
	destination := make([]uint16, 4)
	copyUTF16(destination, "abcdef")
	if destination[len(destination)-1] != 0 {
		t.Fatalf("truncated UTF-16 string is not terminated: %v", destination)
	}
}
