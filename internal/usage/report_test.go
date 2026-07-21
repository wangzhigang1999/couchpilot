package usage

import (
	"bytes"
	"errors"
	htmlstd "html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestReadSummaryMergesCompleteWALWithoutModifyingIt(t *testing.T) {
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	state := reportFixtureState()
	writeAggregateSnapshot(t, directory, state)
	batch := walBatch{
		SchemaVersion: schemaVersion,
		BatchID:       31,
		Dropped:       2,
		Deltas: []walDelta{{
			ForegroundApp: "chrome.exe",
			ActiveProfile: "chrome", BindingProfile: "default", Control: "rb", Gesture: "lt+rb",
			Action: "window_next", Resolution: ResolutionBound, Day: "2026-07-21",
			Attempts: 1, Failures: 1,
		}},
	}
	wal := append(mustJSON(t, batch), '\n')
	wal = append(wal, []byte(`{"schema_version":1,"batch_id":32,"deltas":[`)...)
	if err := os.WriteFile(WALPath(directory), wal, 0o600); err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), wal...)

	summary, err := ReadSummary(directory)
	if err != nil {
		t.Fatal(err)
	}
	if summary.UpdatedDay != "2026-07-21" || summary.SnapshotThrough != 30 || summary.AppliedThrough != 31 {
		t.Fatalf("date/batch = %q/%d", summary.UpdatedDay, summary.AppliedThrough)
	}
	if summary.FirstObservedDay != "2026-07-20" || summary.LastObservedDay != "2026-07-21" {
		t.Fatalf("observation window = %q–%q", summary.FirstObservedDay, summary.LastObservedDay)
	}
	if summary.TotalAttempts != 12 || summary.TotalSuccesses != 8 || summary.TotalFailures != 2 || summary.WithoutOutcome != 2 {
		t.Fatalf("totals = %#v", summary)
	}
	if summary.Dropped != 5 {
		t.Fatalf("dropped = %d, want 5", summary.Dropped)
	}
	if got := findControl(summary, "rb"); got.Attempts != 4 || got.Successes != 2 || got.Failures != 2 {
		t.Fatalf("rb = %#v", got)
	}
	if got := findControl(summary, "back+start"); got.Control != "" {
		t.Fatalf("composite control leaked into physical controls: %#v", got)
	}
	if got := findControl(summary, "back"); got.Attempts != 1 {
		t.Fatalf("Back physical edge was not counted exactly once: %#v", got)
	}
	if got := findControl(summary, "start"); got.Attempts != 1 {
		t.Fatalf("Start physical edge was not counted exactly once: %#v", got)
	}
	if strings.Join(summary.UnusedControls, ",") != "b" {
		t.Fatalf("unused controls = %#v, want only b", summary.UnusedControls)
	}
	if len(summary.ComboGestures) != 2 {
		t.Fatalf("combo gestures = %#v", summary.ComboGestures)
	}
	if len(summary.Mappings) != 5 {
		t.Fatalf("mappings = %#v", summary.Mappings)
	}
	if got := summary.Mappings[0]; got.ForegroundApp != "chrome.exe" || got.ActiveProfile != "chrome" || got.BindingProfile != "default" || got.Control != "a" {
		t.Fatalf("first mapping does not follow stable field order: %#v", got)
	}
	if got := summary.Mappings[1]; got.ActiveProfile != "chrome" || got.BindingProfile != "default" || got.Control != "rb" || got.Attempts != 4 {
		t.Fatalf("fallback mapping was not preserved and merged: %#v", got)
	}
	if combo := findCombo(summary, "lt+rb"); combo.Attempts != 4 || combo.Failures != 2 {
		t.Fatalf("lt+rb = %#v", combo)
	}
	if combo := findCombo(summary, "back+start"); combo.Attempts != 1 || combo.Resolution != ResolutionSystem {
		t.Fatalf("back+start = %#v", combo)
	}
	if profile := findProfile(summary, "chrome"); profile.Attempts != 9 {
		t.Fatalf("chrome profile = %#v", profile)
	}
	if len(summary.Apps) != 1 || summary.Apps[0].App != "chrome.exe" || summary.Apps[0].Attempts != 12 {
		t.Fatalf("apps = %#v", summary.Apps)
	}
	if len(summary.UnusedBindings) != 0 {
		t.Fatalf("generic profile activity must not imply a binding opportunity: %#v", summary.UnusedBindings)
	}
	after, err := os.ReadFile(WALPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("ReadSummary modified the WAL")
	}
}

func TestFormatTextIsChineseAndExplainsOverlappingViews(t *testing.T) {
	summary, err := summaryFromFixture(t)
	if err != nil {
		t.Fatal(err)
	}
	text := FormatText(summary)
	for _, want := range []string{
		"CouchPilot 本地按键使用报告",
		"观察期：2026-07-20–2026-07-21",
		"快照批次 30；已合并近期日志至 31",
		"输入尝试：12 次；派发成功 8；派发失败 2",
		"前台 App（进程名）",
		"chrome.exe：12 次",
		"各视图是同一批事件的不同统计维度",
		"不能用事件总数计算动作成功率",
		"诊断事件不计入输入尝试",
		"当前键位策略尚未观察到的物理控制",
		"右肩键 RB（rb）",
		"暂无可下结论的项目",
		"不会联网发送",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("text report does not contain %q:\n%s", want, text)
		}
	}
}

func TestHTMLReportIsLocalUTF8AndEscapesConfiguredValues(t *testing.T) {
	directory := t.TempDir()
	summary := Summary{
		UpdatedDay:       "2026-07-21",
		FirstObservedDay: "2026-07-20",
		LastObservedDay:  "2026-07-21",
		SnapshotThrough:  8,
		AppliedThrough:   9,
		Dropped:          2,
		TotalAttempts:    3,
		TotalSuccesses:   2,
		TotalFailures:    1,
		Controls: []ControlSummary{{
			Control: "a<script>alert(1)</script>", CountSummary: CountSummary{Attempts: 3, Successes: 2, Failures: 1},
		}},
		ComboGestures: []ComboGestureSummary{{
			ForegroundApp: "<script>combo.exe</script>",
			ActiveProfile: "<img src=x onerror=alert(1)>", BindingProfile: "default",
			Gesture: "lt+<script>x</script>", Action: "window_next", Resolution: ResolutionBound,
			CountSummary: CountSummary{Attempts: 1, Successes: 1},
		}},
		Mappings: []MappingSummary{{
			ForegroundApp: "notes<&>.exe",
			ActiveProfile: "notes<&>", BindingProfile: "default<script>source</script>",
			Control: "a<&>", Gesture: "a<script>gesture</script>", Action: "click_<left>", Resolution: ResolutionBound,
			CountSummary: CountSummary{Attempts: 2, Successes: 2},
		}},
		Apps:           []AppSummary{{App: "<script>app.exe</script>", CountSummary: CountSummary{Attempts: 3}}},
		Profiles:       []ProfileSummary{{Profile: "<b>private</b>", CountSummary: CountSummary{Attempts: 3}}},
		UnusedControls: []string{"x<&"},
		UnusedBindings: []BindingDefinition{{
			Profile: "<script>profile</script>", Gesture: "voice+<img>", Action: "enter", Resolution: ResolutionBound,
		}},
	}
	if err := writeHTMLReport(directory, summary); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(ReportPath(directory)) != reportFileName {
		t.Fatalf("report path = %s", ReportPath(directory))
	}
	data, err := os.ReadFile(ReportPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(data) {
		t.Fatal("HTML report is not valid UTF-8")
	}
	html := string(data)
	for _, forbidden := range []string{"http://", "https://", "<script", "<img", "url("} {
		if strings.Contains(strings.ToLower(html), forbidden) {
			t.Fatalf("HTML contains forbidden content %q", forbidden)
		}
	}
	for _, escaped := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "&lt;script&gt;app.exe&lt;/script&gt;", "notes&lt;&amp;&gt;.exe", "&lt;img src=x onerror=alert(1)&gt;", "&lt;b&gt;private&lt;/b&gt;", "notes&lt;&amp;&gt;", "default&lt;script&gt;source&lt;/script&gt;", "click_&lt;left&gt;"} {
		if !strings.Contains(html, escaped) {
			t.Fatalf("HTML does not contain escaped value %q", escaped)
		}
	}
	for _, want := range []string{"CouchPilot 按键使用报告", "实体按键与摇杆", "组合键与状态手势", "前台 App（进程名）", "仅观察", "映射执行明细", "当前场景", "映射来源", "解析结果", "已映射", "隐私说明", "各视图是同一批事件的不同统计维度"} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML does not contain %q", want)
		}
	}
	assertPrivateFile(t, ReportPath(directory))
}

func TestFriendlyResolutionNames(t *testing.T) {
	tests := map[Resolution]string{
		ResolutionBound:    "已映射",
		ResolutionUnbound:  "未映射",
		ResolutionDisabled: "已禁用",
		ResolutionObserved: "仅观察",
		ResolutionSystem:   "系统动作",
	}
	for resolution, want := range tests {
		if got := friendlyResolutionName(resolution); got != want {
			t.Fatalf("friendlyResolutionName(%q) = %q, want %q", resolution, got, want)
		}
	}
}

func TestDisplayAppNameExplainsHistoricalRecords(t *testing.T) {
	if got := displayAppName(""); got != "历史数据 / 无法识别" {
		t.Fatalf("displayAppName(empty) = %q", got)
	}
	if got := displayAppName(" ChatGPT.exe "); got != "ChatGPT.exe" {
		t.Fatalf("displayAppName(process) = %q", got)
	}
}

func TestRecorderRefreshesHTMLOnOpenFlushAndClose(t *testing.T) {
	directory := t.TempDir()
	recorder, err := Open(Options{
		Directory: directory,
		Inventory: []BindingDefinition{{
			Profile: "default", Gesture: "lt+rb", Action: "window_next", Resolution: ResolutionBound,
		}},
		Controls:      []string{"rb", "lt", "back+start"},
		FlushInterval: time.Hour,
		MaxBatchSize:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := os.ReadFile(ReportPath(directory))
	if err != nil {
		t.Fatal("Open did not create HTML report:", err)
	}
	if !strings.Contains(htmlstd.UnescapeString(string(opened)), "暂无可下结论的项目") {
		t.Fatal("opening report should not call an unexposed binding unused")
	}

	recorder.Record(Observation{
		At: time.Now(), ForegroundApp: "msedge.exe", ActiveProfile: "chrome-live", BindingProfile: "default",
		Control: "rb", Gesture: "lt+rb", Action: "window_next",
		Resolution: ResolutionBound, Outcome: OutcomeSuccess,
	})
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, readErr := os.ReadFile(ReportPath(directory))
		if readErr == nil && strings.Contains(string(data), "chrome-live") && strings.Contains(string(data), "msedge.exe") {
			break
		}
		if time.Now().After(deadline) {
			_ = recorder.Close()
			t.Fatalf("successful flush did not refresh HTML: %v", readErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadSummary(directory); err != nil || got.TotalAttempts != 1 {
		t.Fatalf("summary after close = %#v, err=%v", got, err)
	}
}

func TestReportFailureOnlyCallsOnErrorAndDoesNotAffectWAL(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(ReportPath(directory), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ReportPath(directory), "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	reported := make(chan error, 8)
	recorder, err := Open(Options{
		Directory: directory, FlushInterval: time.Hour, MaxBatchSize: 1,
		OnError: func(err error) { reported <- err },
	})
	if err != nil {
		t.Fatal("report failure affected Open:", err)
	}
	select {
	case reportErr := <-reported:
		if !strings.Contains(reportErr.Error(), "refresh local usage report") {
			t.Fatalf("unexpected report error: %v", reportErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Open report failure was not sent to OnError")
	}

	recorder.Record(Observation{Kind: EventInputAttempt, Control: "a", Gesture: "a", Resolution: ResolutionObserved, Outcome: OutcomeNone})
	deadline := time.Now().Add(2 * time.Second)
	for {
		info, statErr := os.Stat(WALPath(directory))
		if statErr == nil && info.Size() > 0 {
			break
		}
		if time.Now().After(deadline) {
			_ = recorder.Close()
			t.Fatalf("WAL was not written after report error: %v", statErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal("report failure affected Close:", err)
	}
	summary, err := ReadSummary(directory)
	if err != nil || summary.TotalAttempts != 1 {
		t.Fatalf("durable usage after report errors = %#v, err=%v", summary, err)
	}
}

func TestReadSummaryDuringConcurrentCompaction(t *testing.T) {
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	state := emptyAggregate()
	state.Controls = []string{"a"}
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, testLocation)
	if err := compact(directory, &state, now); err != nil {
		t.Fatal(err)
	}

	writerStarted := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		close(writerStarted)
		for batchID := uint64(1); batchID <= 48; batchID++ {
			batch := walBatch{
				SchemaVersion: schemaVersion,
				BatchID:       batchID,
				Deltas: []walDelta{{
					Kind: EventInputAttempt, Control: "a", Gesture: "a", Resolution: ResolutionObserved,
					Day: "2026-07-21", Attempts: 1,
				}},
			}
			if err := appendWALBatch(directory, batch); err != nil {
				writerDone <- err
				return
			}
			applyWALBatch(&state, batch)
			if err := compact(directory, &state, now); err != nil {
				writerDone <- err
				return
			}
		}
		writerDone <- nil
	}()
	<-writerStarted

	for {
		select {
		case writerErr := <-writerDone:
			if writerErr != nil {
				t.Fatal(writerErr)
			}
			final, err := ReadSummary(directory)
			if err != nil {
				t.Fatal(err)
			}
			if final.AppliedThrough != 48 || final.TotalAttempts != 48 {
				t.Fatalf("final summary = %#v", final)
			}
			return
		default:
			if _, err := ReadSummary(directory); err != nil {
				t.Fatalf("read during concurrent compaction: %v", err)
			}
		}
	}
}

func TestReadSummaryMissingDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "not-created")
	_, err := ReadSummary(directory)
	if err == nil || !errors.Is(err, ErrNoUsageData) || !strings.Contains(err.Error(), "no local usage data") {
		t.Fatalf("ReadSummary error = %v", err)
	}
}

func reportFixtureState() aggregateState {
	state := aggregateState{
		UpdatedDay:     "2026-07-20",
		AppliedThrough: 30,
		Dropped:        3,
		Controls:       []string{"a", "b", "rb", "lt", "back", "start", "back+start"},
		Inventory: []BindingDefinition{
			{Profile: "default", Gesture: "lt+rb", Action: "window_next", Resolution: ResolutionBound},
			{Profile: "default", Gesture: "voice+a", Action: "enter", Resolution: ResolutionBound},
			{Profile: "default", Gesture: "rt+a", Action: "enter", Resolution: ResolutionDisabled},
			{Profile: "default", Gesture: "a", Action: "click_left", Resolution: ResolutionBound},
		},
		Entries: make(map[entryKey]*aggregateEntry),
	}
	addReportFixtureEntry(&state, entryKey{
		ActiveProfile: "chrome", Control: "lt", Gesture: "lt", Resolution: ResolutionObserved,
	}, counters{Attempts: 2})
	addReportFixtureEntry(&state, entryKey{
		ActiveProfile: "chrome", BindingProfile: "default", Control: "a", Gesture: "a",
		Action: "click_left", Resolution: ResolutionBound,
	}, counters{Attempts: 5, Successes: 5})
	addReportFixtureEntry(&state, entryKey{
		ActiveProfile: "chrome", BindingProfile: "default", Control: "rb", Gesture: "lt+rb",
		Action: "window_next", Resolution: ResolutionBound,
	}, counters{Attempts: 3, Successes: 2, Failures: 1})
	addReportFixtureEntry(&state, entryKey{
		ActiveProfile: "default", Control: "back", Gesture: "back", Resolution: ResolutionUnbound,
	}, counters{Attempts: 1})
	addReportFixtureEntry(&state, entryKey{
		ActiveProfile: "default", Control: "start", Gesture: "start", Resolution: ResolutionUnbound,
	}, counters{Attempts: 1})
	addReportFixtureEntry(&state, entryKey{
		ActiveProfile: "default", Control: "back+start", Gesture: "back+start",
		Action: "emergency_exit", Resolution: ResolutionSystem,
	}, counters{Attempts: 1, Successes: 1})
	return state
}

func addReportFixtureEntry(state *aggregateState, key entryKey, value counters) {
	if key.ForegroundApp == "" {
		key.ForegroundApp = "chrome.exe"
	}
	state.Entries[key] = &aggregateEntry{
		Key: key, Attempts: value.Attempts, Successes: value.Successes, Failures: value.Failures,
		FirstUsedDay: "2026-07-20", LastUsedDay: "2026-07-20",
		Daily: map[string]counters{"2026-07-20": value},
	}
}

func summaryFromFixture(t *testing.T) (Summary, error) {
	t.Helper()
	directory := t.TempDir()
	if err := prepareDirectory(directory); err != nil {
		return Summary{}, err
	}
	state := reportFixtureState()
	writeAggregateSnapshot(t, directory, state)
	batch := walBatch{
		SchemaVersion: schemaVersion, BatchID: 31, Dropped: 2,
		Deltas: []walDelta{{
			ForegroundApp: "chrome.exe",
			ActiveProfile: "chrome", BindingProfile: "default", Control: "rb", Gesture: "lt+rb",
			Action: "window_next", Resolution: ResolutionBound, Day: "2026-07-21", Attempts: 1, Failures: 1,
		}},
	}
	if err := os.WriteFile(WALPath(directory), append(mustJSON(t, batch), '\n'), 0o600); err != nil {
		return Summary{}, err
	}
	return ReadSummary(directory)
}

func findControl(summary Summary, control string) ControlSummary {
	for _, item := range summary.Controls {
		if item.Control == control {
			return item
		}
	}
	return ControlSummary{}
}

func findCombo(summary Summary, gesture string) ComboGestureSummary {
	for _, item := range summary.ComboGestures {
		if item.Gesture == gesture {
			return item
		}
	}
	return ComboGestureSummary{}
}

func findProfile(summary Summary, profile string) ProfileSummary {
	for _, item := range summary.Profiles {
		if item.Profile == profile {
			return item
		}
	}
	return ProfileSummary{}
}
