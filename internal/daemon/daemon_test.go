package daemon

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestReservePIDDoesNotPublishUntilReady(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.pid")
	claim, err := ReservePID(path)
	if err != nil {
		t.Fatal(err)
	}
	defer claim.Release()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("PID file was published before readiness: %v", err)
	}
	if err := claim.MarkReady(); err != nil {
		t.Fatal(err)
	}
	if pid, running := Status(path); !running || pid != os.Getpid() {
		t.Fatalf("published pid=%d running=%t", pid, running)
	}
}

func TestStatusRejectsLivePIDWithoutRuntimeLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.pid")
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if pid, running := Status(path); running || pid != 0 {
		t.Fatalf("unowned pid reported as CouchPilot: pid=%d running=%t", pid, running)
	}
}

func TestRuntimeCleanupRequiresOwningTheKernelLock(t *testing.T) {
	base := t.TempDir()
	paths := RuntimePaths(filepath.Join(base, "config.json"))
	claim, err := ReservePID(paths.PIDFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.StopFile, []byte("stop\n"), 0o644); err != nil {
		claim.Release()
		t.Fatal(err)
	}
	if cleaned, err := cleanupRuntimeFilesIfUnlocked(paths, 0); err != nil || cleaned {
		claim.Release()
		t.Fatalf("cleanup while owned: cleaned=%t err=%v", cleaned, err)
	}
	if _, err := os.Stat(paths.StopFile); err != nil {
		claim.Release()
		t.Fatalf("stop file removed while runtime lock was held: %v", err)
	}
	claim.Release()
	if cleaned, err := cleanupRuntimeFilesIfUnlocked(paths, 0); err != nil || !cleaned {
		t.Fatalf("cleanup after release: cleaned=%t err=%v", cleaned, err)
	}
	if _, err := os.Stat(paths.StopFile); !os.IsNotExist(err) {
		t.Fatalf("stop file remains after cleanup: %v", err)
	}
}

func TestRuntimeCleanupTreatsMissingDirectoryAsAlreadyClean(t *testing.T) {
	base := filepath.Join(t.TempDir(), "missing")
	paths := RuntimePaths(filepath.Join(base, "config.json"))
	if cleaned, err := cleanupRuntimeFilesIfUnlocked(paths, 0); err != nil || !cleaned {
		t.Fatalf("cleaned=%t err=%v", cleaned, err)
	}
}

func TestStopRequestPathIsGenerationScoped(t *testing.T) {
	base := filepath.Join(t.TempDir(), "couchpilot.stop")
	if first, second := StopRequestPath(base, 41), StopRequestPath(base, 42); first == second {
		t.Fatalf("stop request paths collided: %q", first)
	}
}

func TestClaimPIDAtomicallyExcludesCanonicalAlias(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "app.pid")
	firstRelease, err := ClaimPID(path)
	if err != nil {
		t.Fatal(err)
	}
	defer firstRelease()

	alias := filepath.Join(directory, ".", "app.pid")
	if release, err := ClaimPID(alias); err == nil {
		release()
		t.Fatal("second ClaimPID unexpectedly acquired the same canonical process lock")
	}

	firstRelease()
	secondRelease, err := ClaimPID(alias)
	if err != nil {
		t.Fatalf("ClaimPID after release: %v", err)
	}
	secondRelease()
}

func TestClaimPIDConcurrentSingleWinner(t *testing.T) {
	const contenders = 32
	path := filepath.Join(t.TempDir(), "app.pid")
	start := make(chan struct{})
	type result struct {
		release func()
		err     error
	}
	results := make(chan result, contenders)
	var wait sync.WaitGroup
	wait.Add(contenders)
	for index := 0; index < contenders; index++ {
		go func() {
			defer wait.Done()
			<-start
			release, err := ClaimPID(path)
			results <- result{release: release, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	var winner func()
	for result := range results {
		if result.err != nil {
			continue
		}
		if winner != nil {
			result.release()
			winner()
			t.Fatal("multiple concurrent ClaimPID calls acquired the same process lock")
		}
		winner = result.release
	}
	if winner == nil {
		t.Fatal("no concurrent ClaimPID call acquired the process lock")
	}
	winner()
}

func TestClaimPIDReplacesStalePIDAfterAcquiringKernelLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.pid")
	if err := os.WriteFile(path, []byte("999999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := ClaimPID(path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("stale pid was not replaced: %q", data)
	}
}

func TestClaimPIDKernelLockIsReleasedAfterOwnerCrash(t *testing.T) {
	const helperEnvironment = "COUCHPILOT_PID_LOCK_HELPER"
	if os.Getenv(helperEnvironment) == "1" {
		path := os.Getenv("COUCHPILOT_PID_LOCK_PATH")
		ready := os.Getenv("COUCHPILOT_PID_LOCK_READY")
		release, err := ClaimPID(path)
		if err != nil {
			t.Fatal(err)
		}
		// Keep both the release closure and a live timer reachable. A bare
		// select{} lets the Go runtime treat this helper as deadlocked and exit,
		// which would release the kernel mutex before the parent competes for it.
		defer release()
		if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Hour)
		}
	}

	path := filepath.Join(t.TempDir(), "app.pid")
	ready := filepath.Join(t.TempDir(), "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestClaimPIDKernelLockIsReleasedAfterOwnerCrash$")
	var childOutput bytes.Buffer
	command.Stdout = &childOutput
	command.Stderr = &childOutput
	command.Env = append(os.Environ(),
		helperEnvironment+"=1",
		"COUCHPILOT_PID_LOCK_PATH="+path,
		"COUCHPILOT_PID_LOCK_READY="+ready,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	childExited := false
	t.Cleanup(func() {
		if !childExited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = command.Process.Kill()
			_ = command.Wait()
			childExited = true
			t.Fatalf("pid lock helper did not become ready: %s", childOutput.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if release, err := ClaimPID(path); err == nil {
		release()
		t.Fatal("parent acquired PID lock while helper process held it")
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	childExited = true

	release, err := ClaimPID(path)
	if err != nil {
		t.Fatalf("ClaimPID after helper crash: %v", err)
	}
	release()
}

func TestRuntimePathsKeepTraceBesideConfig(t *testing.T) {
	base := filepath.Join(t.TempDir(), "portable")
	paths := RuntimePaths(filepath.Join(base, "config.json"))
	if paths.TraceDirectory != filepath.Join(base, "trace") {
		t.Fatalf("trace directory = %q", paths.TraceDirectory)
	}
}
