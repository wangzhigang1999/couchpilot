package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestClaimPIDLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.pid")
	release, err := ClaimPID(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("unexpected pid: %q", data)
	}
	release()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("pid file should be removed")
	}
}
