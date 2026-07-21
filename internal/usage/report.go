package usage

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	reportFileName          = "usage-v1-report.html"
	reportReadTries         = 24
	reportRetryInitialDelay = 2 * time.Millisecond
	reportRetryMaximumDelay = 25 * time.Millisecond

	minimumChordCandidateExposure = 5
	minimumChordActiveDays        = 2
	minimumChordNearMissPercent   = 30
	minimumChordLatePercent       = 30
	minimumTransitionExposure     = 10
	minimumPatternOccurrences     = 3
	minimumPatternPercent         = 30
	minimumPatternActiveDays      = 2
	minimumBindingOpportunities   = 3
)

// ErrNoUsageData indicates that CouchPilot has not created a usage snapshot in
// the requested directory yet.
var ErrNoUsageData = errors.New("no local usage data")

// CountSummary is the common aggregate used by report sections.
type CountSummary struct {
	Attempts  uint64
	Successes uint64
	Failures  uint64
}

// ControlSummary contains lifetime usage for one physical controller input.
type ControlSummary struct {
	Control string
	CountSummary
}

// ComboGestureSummary contains lifetime usage for a chord or contextual
// gesture. Provenance is retained so default-profile fallbacks remain visible.
type ComboGestureSummary struct {
	StrategyID     string
	ForegroundApp  string
	ActiveProfile  string
	BindingProfile string
	Gesture        string
	Action         string
	Resolution     Resolution
	CountSummary
}

// MappingSummary contains one complete aggregate entry. Keeping both active
// and binding profiles makes profile fallback behavior explicit.
type MappingSummary struct {
	Kind           EventKind
	StrategyID     string
	ForegroundApp  string
	ActiveProfile  string
	BindingProfile string
	Control        string
	Gesture        string
	Action         string
	Resolution     Resolution
	CountSummary
}

// TraceEvidenceSummary is a decision-oriented aggregate. It contains only
// coarse buckets and counters; no individual timestamp or raw input timeline
// is exposed to readers.
type TraceEvidenceSummary struct {
	Kind                EventKind
	StrategyID          string
	ForegroundApp       string
	ActiveProfile       string
	Control             string
	Gesture             string
	PhysicalGesture     string
	GestureKind         GestureKind
	Action              string
	RelatedGesture      string
	RelatedAction       string
	Resolution          Resolution
	CandidateResolution Resolution
	IntervalBucket      string
	DurationBucket      string
	CountBucket         string
	Reason              string
	Flags               string
	CountSummary
}

// ChordCandidateSummary keeps every probe for one candidate in a single
// decision denominator. Counters may overlap: for example one fallback can
// also be unbound and priority-blocked.
type ChordCandidateSummary struct {
	StrategyID               string
	ForegroundApp            string
	ActiveProfile            string
	Gesture                  string
	Total                    uint64
	Selected                 uint64
	NearMiss                 uint64
	Fallback                 uint64
	LateModifier             uint64
	PriorityBlocked          uint64
	Unbound                  uint64
	Disabled                 uint64
	PointerContext           uint64
	StickContext             uint64
	Ambiguous                uint64
	EligibleTotal            uint64
	EligibleSelected         uint64
	EligibleNearMiss         uint64
	EligibleLateModifier     uint64
	ActiveDays               int
	NearMissDays             int
	LateModifierDays         int
	EligibleActiveDays       int
	EligibleNearMissDays     int
	EligibleLateModifierDays int
	AppKnown                 bool
	Sufficient               bool
	Qualified                bool
}

// PhysicalOverlapSummary exposes simultaneous digital-button input attempts.
// It deliberately says attempts, not frames: exact timestamps/frame IDs are
// not persisted, so two rising buttons in one frame cannot be deduplicated.
type PhysicalOverlapSummary struct {
	StrategyID      string
	ForegroundApp   string
	ActiveProfile   string
	PhysicalGesture string
	InputAttempts   uint64
	ActiveDays      int
}

// TransitionPatternSummary compares a suspected correction or quick repeat
// with every transition originating from the same action and context.
type TransitionPatternSummary struct {
	StrategyID    string
	ForegroundApp string
	ActiveProfile string
	FromAction    string
	ToAction      string
	FromGesture   string
	ToGesture     string
	Suspected     uint64
	Total         uint64
	ActiveDays    int
	ExposureDays  int
	AppKnown      bool
	Qualified     bool
}

// StrategySummary separates evidence collected before and after remapping.
type StrategySummary struct {
	ID       string
	Attempts uint64
}

// ProfileSummary contains lifetime usage while one app profile was active.
type ProfileSummary struct {
	Profile string
	CountSummary
}

// AppSummary contains lifetime usage while one executable was foreground.
// An empty App represents historical records or a foreground lookup failure.
type AppSummary struct {
	App string
	CountSummary
}

// Summary is the stable, human-report-oriented view of snapshot plus WAL data.
type Summary struct {
	UpdatedDay         string
	FirstObservedDay   string
	LastObservedDay    string
	SnapshotThrough    uint64
	AppliedThrough     uint64
	TotalAttempts      uint64
	TotalSuccesses     uint64
	TotalFailures      uint64
	WithoutOutcome     uint64
	Dropped            uint64
	InvalidDropped     uint64
	Coalesced          uint64
	SequenceBreaks     uint64
	CurrentStrategyID  string
	Controls           []ControlSummary
	Mappings           []MappingSummary
	ComboGestures      []ComboGestureSummary
	Apps               []AppSummary
	Profiles           []ProfileSummary
	UnusedControls     []string
	UnusedBindings     []BindingDefinition
	Strategies         []StrategySummary
	ChordCandidates    []ChordCandidateSummary
	PhysicalOverlaps   []PhysicalOverlapSummary
	NearMisses         []TraceEvidenceSummary
	Transitions        []TraceEvidenceSummary
	Corrections        []TraceEvidenceSummary
	CorrectionPatterns []TransitionPatternSummary
	RepeatPatterns     []TransitionPatternSummary
	Holds              []TraceEvidenceSummary
	ComposeSessions    []TraceEvidenceSummary
	WindowSessions     []TraceEvidenceSummary
	ContextSessions    []TraceEvidenceSummary
	RepeatEpisodes     []TraceEvidenceSummary
	Recommendations    []string
}

// ReportPath returns the fixed local HTML report path for dir.
func ReportPath(dir string) string {
	return filepath.Join(dir, reportFileName)
}

// ReadSummary takes a read-only, point-in-time view of the snapshot and every
// complete WAL line. It never truncates or otherwise modifies the WAL. If a
// concurrent compaction swaps generations during the read, it retries.
func ReadSummary(directory string) (Summary, error) {
	if directory == "" {
		return Summary{}, fmt.Errorf("read usage summary: %w: empty directory", ErrNoUsageData)
	}
	if _, err := os.Stat(directory); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Summary{}, fmt.Errorf("read usage summary: %w in %s", ErrNoUsageData, directory)
		}
		return Summary{}, fmt.Errorf("read usage summary directory: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < reportReadTries; attempt++ {
		state, before, err := readSnapshotForReport(directory)
		if err != nil {
			lastErr = err
			reportRetry(attempt)
			continue
		}
		snapshotThrough := state.AppliedThrough
		mergeErr := mergeWALForReport(directory, &state)
		_, after, afterErr := readSnapshotForReport(directory)
		if afterErr != nil {
			lastErr = afterErr
			reportRetry(attempt)
			continue
		}
		if before != after {
			lastErr = errors.New("usage snapshot changed during report read")
			reportRetry(attempt)
			continue
		}
		if mergeErr != nil {
			lastErr = mergeErr
			reportRetry(attempt)
			continue
		}
		return summarize(state, snapshotThrough), nil
	}
	if lastErr == nil {
		lastErr = errors.New("usage files did not stabilize")
	}
	return Summary{}, fmt.Errorf("read usage summary after %d attempts: %w", reportReadTries, lastErr)
}

// FormatText renders a compact Chinese report suitable for a terminal.
func FormatText(summary Summary) string {
	var output strings.Builder
	output.WriteString("CouchPilot 本地按键使用报告\n")
	fmt.Fprintf(&output, "统计日期：%s\n", displayValue(summary.UpdatedDay))
	fmt.Fprintf(&output, "观察期：%s–%s；快照批次 %d；已合并近期日志至 %d\n",
		displayValue(summary.FirstObservedDay), displayValue(summary.LastObservedDay), summary.SnapshotThrough, summary.AppliedThrough)
	fmt.Fprintf(&output, "输入尝试：%d 次；派发成功 %d；派发失败 %d；未派发 %d；队列丢弃 %d\n",
		summary.TotalAttempts, summary.TotalSuccesses, summary.TotalFailures, summary.WithoutOutcome, summary.Dropped)
	fmt.Fprintf(&output, "数据健康：无效 %d；折叠 %d；序列断点 %d\n",
		summary.InvalidDropped, summary.Coalesced, summary.SequenceBreaks)
	output.WriteString("派发成功只表示系统接收了动作，不表示用户意图已经正确完成；诊断事件不计入输入尝试。\n")
	output.WriteString("各视图是同一批事件的不同统计维度，彼此可能重叠，不能相加，也不能用事件总数计算动作成功率。\n")
	if summary.Dropped > 0 {
		fmt.Fprintf(&output, "注意：有 %d 条事件因记录队列繁忙而未写入，下面的“未观察到”清单可能不可靠。\n", summary.Dropped)
	} else {
		output.WriteString("数据完整：没有检测到因记录队列繁忙而丢弃的事件。\n")
	}

	output.WriteString("\n物理控制\n")
	if len(summary.Controls) == 0 {
		output.WriteString("  暂无记录\n")
	} else {
		for _, item := range summary.Controls {
			fmt.Fprintf(&output, "  %s（%s）：%d 次（派发成功 %d，派发失败 %d）\n",
				friendlyControlName(item.Control), displayValue(item.Control), item.Attempts, item.Successes, item.Failures)
		}
	}

	output.WriteString("\n组合手势\n")
	if len(summary.ComboGestures) == 0 {
		output.WriteString("  暂无记录\n")
	} else {
		for _, item := range summary.ComboGestures {
			fmt.Fprintf(&output, "  %s / %s → %s：%d 次（App %s，场景 %s，策略 %s，派发成功 %d，派发失败 %d）\n",
				displayValue(item.BindingProfile), displayValue(item.Gesture), displayValue(item.Action), item.Attempts,
				displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile), displayValue(item.StrategyID), item.Successes, item.Failures)
		}
	}

	output.WriteString("\n前台 App（进程名）\n")
	if len(summary.Apps) == 0 {
		output.WriteString("  暂无记录\n")
	} else {
		for _, item := range summary.Apps {
			fmt.Fprintf(&output, "  %s：%d 次（派发成功 %d，派发失败 %d）\n",
				displayAppName(item.App), item.Attempts, item.Successes, item.Failures)
		}
	}

	output.WriteString("\n使用场景\n")
	if len(summary.Profiles) == 0 {
		output.WriteString("  暂无记录\n")
	} else {
		for _, item := range summary.Profiles {
			fmt.Fprintf(&output, "  %s：%d 次（派发成功 %d，派发失败 %d）\n",
				displayValue(item.Profile), item.Attempts, item.Successes, item.Failures)
		}
	}

	output.WriteString("\n当前键位策略尚未观察到的物理控制\n")
	if len(summary.UnusedControls) == 0 {
		output.WriteString("  无\n")
	} else {
		fmt.Fprintf(&output, "  %s\n", strings.Join(friendlyControlValues(summary.UnusedControls), "、"))
	}

	output.WriteString("\n有足够使用机会但尚未观察到的已启用组合绑定\n")
	if len(summary.UnusedBindings) == 0 {
		output.WriteString("  暂无可下结论的项目\n")
	} else {
		for _, item := range summary.UnusedBindings {
			fmt.Fprintf(&output, "  %s / %s → %s\n",
				displayValue(item.Profile), displayValue(item.Gesture), displayValue(item.Action))
		}
	}

	output.WriteString("\n映射执行明细（按策略版本区分）\n")
	if len(summary.Mappings) == 0 {
		output.WriteString("  暂无记录\n")
	} else {
		for _, item := range summary.Mappings {
			fmt.Fprintf(&output, "  %s / %s → %s：%d 次（控制 %s，App %s，当前场景 %s，映射来源 %s，策略 %s，解析 %s，派发成功 %d，派发失败 %d）\n",
				displayValue(item.Control), displayValue(item.Gesture), displayValue(item.Action), item.Attempts,
				displayValue(item.Control), displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile),
				displayValue(item.BindingProfile), displayValue(item.StrategyID), friendlyResolutionName(item.Resolution),
				item.Successes, item.Failures)
		}
	}

	output.WriteString("\n键位策略证据\n")
	if len(summary.Strategies) == 0 {
		output.WriteString("  策略版本：历史数据 / 无法识别\n")
	} else {
		for _, item := range summary.Strategies {
			fmt.Fprintf(&output, "  策略 %s：%d 次输入尝试\n", displayValue(item.ID), item.Attempts)
		}
	}
	output.WriteString("  建议门槛：组合候选需有效曝光 ≥5、有效活跃日 ≥2、信号占比 ≥30% 且信号跨 ≥2 日；修正/快速重复需同源转移 ≥10、模式 ≥3、占比 ≥30% 且跨 ≥2 日。所有结果只作为线索，不会自动改键。\n")
	for _, item := range summary.ChordCandidates {
		status := "样本不足，仅展示，不形成建议"
		if !item.AppKnown {
			status = "App 未知或已合并多个 App，仅展示，不形成建议"
		} else if item.Sufficient {
			status = "曝光已达门槛，当前比率未触发建议"
		}
		if item.Qualified {
			status = "达到建议门槛"
		}
		fmt.Fprintf(&output, "  组合候选 %s（App %s，场景 %s，策略 %s）：总曝光 %d（歧义 %d：pointer %d、stick %d；有效分母 %d）；原始选中 %d、有效选中 %d；原始近失误 %d；有效近失误 %d/%d（%d%%，%d 天）；回退 %d；原始慢按 %d；有效慢按 %d/%d（%d%%，%d 天）；优先级阻挡 %d；未绑定 %d；禁用 %d；总活跃 %d 天、有效活跃 %d 天（%s）\n",
			displayValue(item.Gesture), displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile),
			displayValue(item.StrategyID), item.Total, item.Ambiguous, item.PointerContext, item.StickContext,
			item.EligibleTotal, item.Selected, item.EligibleSelected, item.NearMiss, item.EligibleNearMiss, item.EligibleTotal,
			evidencePercent(item.EligibleNearMiss, item.EligibleTotal), item.EligibleNearMissDays,
			item.Fallback, item.LateModifier, item.EligibleLateModifier, item.EligibleTotal,
			evidencePercent(item.EligibleLateModifier, item.EligibleTotal), item.EligibleLateModifierDays,
			item.PriorityBlocked, item.Unbound, item.Disabled, item.ActiveDays, item.EligibleActiveDays, status)
	}
	if len(summary.PhysicalOverlaps) > 0 {
		output.WriteString("  同帧数字键重叠只按 input_attempt 条数展示；一个实际帧可能产生多条，无法按帧去重，因此只作为线索，不直接推荐。\n")
		for _, item := range summary.PhysicalOverlaps {
			fmt.Fprintf(&output, "  数字键重叠 %s（App %s，场景 %s，策略 %s）：%d 条 input_attempt，活跃 %d 天\n",
				displayValue(item.PhysicalGesture), displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile),
				displayValue(item.StrategyID), item.InputAttempts, item.ActiveDays)
		}
	}
	for _, item := range summary.CorrectionPatterns {
		status := "样本不足，仅展示，不形成建议"
		if !item.AppKnown {
			status = "App 未知或已合并多个 App，仅展示，不形成建议"
		} else if item.Qualified {
			status = "达到建议门槛"
		}
		fmt.Fprintf(&output, "  疑似修正 %s → %s（手势 %s → %s；App %s，场景 %s，策略 %s）：%d/%d（%d%%）；证据活跃 %d 天，总曝光覆盖 %d 天（%s）\n",
			displayValue(item.FromAction), displayValue(item.ToAction),
			displayValue(item.FromGesture), displayValue(item.ToGesture), displayAppName(item.ForegroundApp),
			displayValue(item.ActiveProfile), displayValue(item.StrategyID), item.Suspected, item.Total,
			evidencePercent(item.Suspected, item.Total), item.ActiveDays, item.ExposureDays, status)
	}
	for _, item := range summary.RepeatPatterns {
		status := "样本不足，仅展示，不形成建议"
		if !item.AppKnown {
			status = "App 未知或已合并多个 App，仅展示，不形成建议"
		} else if item.Qualified {
			status = "达到建议门槛"
		}
		fmt.Fprintf(&output, "  快速重复 %s（手势 %s → %s；App %s，场景 %s，策略 %s）：%d/%d（%d%%）；证据活跃 %d 天，总曝光覆盖 %d 天（%s）\n",
			displayValue(item.FromAction), displayValue(item.FromGesture), displayValue(item.ToGesture), displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile),
			displayValue(item.StrategyID), item.Suspected, item.Total,
			evidencePercent(item.Suspected, item.Total), item.ActiveDays, item.ExposureDays, status)
	}
	for _, item := range summary.Holds {
		fmt.Fprintf(&output, "  Hold %s：%d 次（App %s，场景 %s，策略 %s；时长 %s，计数档 %s，原因 %s；结果 成功 %d / 失败 %d / 无结果 %d）\n",
			displayValue(firstNonEmpty(item.PhysicalGesture, item.Gesture, item.Control)), item.Attempts,
			displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile), displayValue(item.StrategyID),
			displayValue(item.DurationBucket), displayValue(item.CountBucket), displayValue(item.Reason),
			item.Successes, item.Failures, withoutOutcomeCount(item.Attempts, item.Successes, item.Failures))
	}
	for _, item := range summary.ComposeSessions {
		fmt.Fprintf(&output, "  Compose %s → %s：%d 次（App %s，场景 %s，策略 %s；时长 %s，计数档 %s，原因 %s；结果 成功 %d / 失败 %d / 无结果 %d）\n",
			displayValue(item.Gesture), displayValue(item.Action), item.Attempts,
			displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile), displayValue(item.StrategyID),
			displayValue(item.DurationBucket), displayValue(item.CountBucket), displayValue(item.Reason),
			item.Successes, item.Failures, withoutOutcomeCount(item.Attempts, item.Successes, item.Failures))
	}
	for _, item := range summary.WindowSessions {
		fmt.Fprintf(&output, "  窗口切换 %s → %s：%d 次（App %s，场景 %s，策略 %s；时长 %s，计数档 %s，原因 %s；结果 成功 %d / 失败 %d / 无结果 %d）\n",
			displayValue(item.Gesture), displayValue(item.Action), item.Attempts,
			displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile), displayValue(item.StrategyID),
			displayValue(item.DurationBucket), displayValue(item.CountBucket), displayValue(item.Reason),
			item.Successes, item.Failures, withoutOutcomeCount(item.Attempts, item.Successes, item.Failures))
	}
	for _, item := range summary.RepeatEpisodes {
		fmt.Fprintf(&output, "  连续操作 %s / %s：%d 个 episode（App %s，场景 %s，策略 %s；重复档 %s，时长 %s，原因 %s；结果 成功 %d / 失败 %d / 无结果 %d）\n",
			displayValue(firstNonEmpty(item.PhysicalGesture, item.Gesture, item.Control)), displayValue(item.Action), item.Attempts,
			displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile), displayValue(item.StrategyID),
			displayValue(item.CountBucket), displayValue(item.DurationBucket), displayValue(item.Reason),
			item.Successes, item.Failures, withoutOutcomeCount(item.Attempts, item.Successes, item.Failures))
	}
	contextCount := uint64(0)
	for _, item := range summary.ContextSessions {
		contextCount = saturatingAdd(contextCount, item.Attempts)
	}
	fmt.Fprintf(&output, "  App 手柄使用机会：%d 个交互会话\n", contextCount)
	for _, item := range summary.ContextSessions {
		fmt.Fprintf(&output, "  交互会话（App %s，场景 %s，策略 %s）：%d 次（原因 %s）\n",
			displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile), displayValue(item.StrategyID),
			item.Attempts, displayValue(item.Reason))
	}

	output.WriteString("\n证据型建议（不会自动改键）\n")
	if len(summary.Recommendations) == 0 {
		output.WriteString("  当前样本尚不足以形成具体建议。\n")
	} else {
		for _, recommendation := range summary.Recommendations {
			fmt.Fprintf(&output, "  - %s\n", recommendation)
		}
	}
	output.WriteString("\n隐私：报告只汇总前台可执行文件名、CouchPilot 手柄控制名称、映射场景和计数；不包含输入文字、窗口标题、完整进程路径、指针位置或精确操作时间，也不会联网发送。\n")
	return output.String()
}

type snapshotToken struct {
	Source string
	Digest [sha256.Size]byte
}

func readSnapshotForReport(directory string) (aggregateState, snapshotToken, error) {
	type candidate struct {
		path   string
		source string
	}
	candidates := []candidate{
		{path: SnapshotPath(directory), source: "primary"},
		{path: snapshotBackupPath(directory), source: "backup"},
	}
	var problems []string
	missing := 0
	for _, item := range candidates {
		data, err := readReportFile(item.path)
		if errors.Is(err, os.ErrNotExist) {
			missing++
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", item.source, err))
			continue
		}
		state, err := decodeSnapshot(data, item.path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", item.source, err))
			continue
		}
		return state, snapshotToken{Source: item.source, Digest: sha256.Sum256(data)}, nil
	}
	if missing == len(candidates) {
		return aggregateState{}, snapshotToken{}, fmt.Errorf("read usage summary: %w in %s", ErrNoUsageData, directory)
	}
	return aggregateState{}, snapshotToken{}, fmt.Errorf("read usage snapshot: %s", strings.Join(problems, "; "))
}

func mergeWALForReport(directory string, state *aggregateState) error {
	data, err := os.ReadFile(WALPath(directory))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read usage WAL for report: %w", err)
	}
	completeLength := 0
	if index := bytes.LastIndexByte(data, '\n'); index >= 0 {
		completeLength = index + 1
	}
	for _, line := range bytes.Split(data[:completeLength], []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var batch walBatch
		if err := json.Unmarshal(line, &batch); err != nil {
			return fmt.Errorf("decode usage WAL for report: %w", err)
		}
		if err := validateWALBatch(batch); err != nil {
			return err
		}
		if batch.BatchID <= state.AppliedThrough {
			continue
		}
		if batch.BatchID != state.AppliedThrough+1 {
			return fmt.Errorf("decode usage WAL for report: expected batch %d, got %d", state.AppliedThrough+1, batch.BatchID)
		}
		applyWALBatch(state, batch)
	}
	return nil
}

type chordCandidateKey struct {
	StrategyID    string
	ForegroundApp string
	ActiveProfile string
	Gesture       string
}

type chordCandidateAccumulator struct {
	value                    ChordCandidateSummary
	days                     map[string]struct{}
	nearMissDays             map[string]struct{}
	lateModifierDays         map[string]struct{}
	eligibleDays             map[string]struct{}
	eligibleNearMissDays     map[string]struct{}
	eligibleLateModifierDays map[string]struct{}
}

type physicalOverlapKey struct {
	StrategyID      string
	ForegroundApp   string
	ActiveProfile   string
	PhysicalGesture string
}

type physicalOverlapAccumulator struct {
	attempts uint64
	days     map[string]struct{}
}

type transitionOriginKey struct {
	StrategyID    string
	ForegroundApp string
	ActiveProfile string
	FromAction    string
	FromGesture   string
}

type transitionOriginAccumulator struct {
	total uint64
	days  map[string]struct{}
}

type transitionPatternKey struct {
	transitionOriginKey
	ToAction  string
	ToGesture string
}

type transitionPatternAccumulator struct {
	suspected uint64
	days      map[string]struct{}
}

func summarize(state aggregateState, snapshotThrough uint64) Summary {
	summary := Summary{
		UpdatedDay:      state.UpdatedDay,
		SnapshotThrough: snapshotThrough,
		AppliedThrough:  state.AppliedThrough,
		Dropped:         state.Dropped,
		InvalidDropped:  state.InvalidDropped,
		Coalesced:       state.Coalesced,
		SequenceBreaks:  state.SequenceBreaks,
	}
	controlCounts := make(map[string]CountSummary, len(state.Controls))
	for _, control := range state.Controls {
		if isCompositeControl(control) {
			continue
		}
		if _, exists := controlCounts[control]; !exists {
			controlCounts[control] = CountSummary{}
		}
	}
	comboCounts := make(map[entryKey]CountSummary)
	appCounts := make(map[string]CountSummary)
	profileCounts := make(map[string]CountSummary)
	strategyCounts := make(map[string]uint64)
	chordCandidateCounts := make(map[chordCandidateKey]*chordCandidateAccumulator)
	physicalOverlapCounts := make(map[physicalOverlapKey]*physicalOverlapAccumulator)
	transitionOrigins := make(map[transitionOriginKey]*transitionOriginAccumulator)
	correctionPatterns := make(map[transitionPatternKey]*transitionPatternAccumulator)
	repeatPatterns := make(map[transitionPatternKey]*transitionPatternAccumulator)
	for _, strategy := range state.Strategies {
		strategyCounts[strategy.ID] = 0
	}
	usedBindings := make(map[bindingUsageKey]struct{})
	currentUsedControls := make(map[string]struct{})
	triggerBindingExposure := make(map[bindingUsageKey]uint64)
	composeProfileExposure := make(map[string]uint64)
	var totalComposeExposure uint64
	currentStrategy := ""
	if len(state.Strategies) > 0 {
		currentStrategy = state.Strategies[len(state.Strategies)-1].ID
	}
	summary.CurrentStrategyID = currentStrategy

	for _, entry := range state.Entries {
		value := CountSummary{Attempts: entry.Attempts, Successes: entry.Successes, Failures: entry.Failures}
		if entry.LastUsedDay > summary.UpdatedDay {
			summary.UpdatedDay = entry.LastUsedDay
		}
		if entry.FirstUsedDay != "" && (summary.FirstObservedDay == "" || entry.FirstUsedDay < summary.FirstObservedDay) {
			summary.FirstObservedDay = entry.FirstUsedDay
		}
		if entry.LastUsedDay > summary.LastObservedDay {
			summary.LastObservedDay = entry.LastUsedDay
		}

		trace := traceSummaryFromEntry(entry, value)
		kind := entry.Key.Kind
		if isLegacyPhysicalActivation(entry.Key) {
			kind = EventPhysicalActivation
		}
		switch kind {
		case EventLegacy, EventInputAttempt:
			summary.Mappings = append(summary.Mappings, MappingSummary{
				Kind:           entry.Key.Kind,
				StrategyID:     entry.Key.StrategyID,
				ForegroundApp:  entry.Key.ForegroundApp,
				ActiveProfile:  entry.Key.ActiveProfile,
				BindingProfile: entry.Key.BindingProfile,
				Control:        entry.Key.Control,
				Gesture:        entry.Key.Gesture,
				Action:         entry.Key.Action,
				Resolution:     entry.Key.Resolution,
				CountSummary:   value,
			})
			summary.TotalAttempts = saturatingAdd(summary.TotalAttempts, entry.Attempts)
			summary.TotalSuccesses = saturatingAdd(summary.TotalSuccesses, entry.Successes)
			summary.TotalFailures = saturatingAdd(summary.TotalFailures, entry.Failures)
			if entry.Key.StrategyID != "" {
				strategyCounts[entry.Key.StrategyID] = saturatingAdd(strategyCounts[entry.Key.StrategyID], entry.Attempts)
			}
			if entry.Key.Control != "" && !isCompositeControl(entry.Key.Control) {
				controlCounts[entry.Key.Control] = addCountSummary(controlCounts[entry.Key.Control], value)
				if currentStrategy == "" || entry.Key.StrategyID == currentStrategy {
					currentUsedControls[entry.Key.Control] = struct{}{}
				}
			}
			if isComboEvidence(entry.Key) {
				comboCounts[entry.Key] = addCountSummary(comboCounts[entry.Key], value)
			}
			appCounts[entry.Key.ForegroundApp] = addCountSummary(appCounts[entry.Key.ForegroundApp], value)
			if entry.Key.ActiveProfile != "" {
				profileCounts[entry.Key.ActiveProfile] = addCountSummary(profileCounts[entry.Key.ActiveProfile], value)
			}
			if entry.Key.Resolution == ResolutionBound && entry.Attempts > 0 &&
				(currentStrategy == "" || entry.Key.StrategyID == currentStrategy) {
				usedBindings[bindingUsageKey{
					Profile: entry.Key.BindingProfile,
					Gesture: entry.Key.Gesture,
					Action:  entry.Key.Action,
				}] = struct{}{}
			}
			if entry.Key.Kind == EventInputAttempt &&
				hasTraceFlag(entry.Key.Flags, "simultaneous_buttons") &&
				strings.Contains(entry.Key.PhysicalGesture, "+") {
				key := physicalOverlapKey{
					StrategyID: entry.Key.StrategyID, ForegroundApp: entry.Key.ForegroundApp,
					ActiveProfile: entry.Key.ActiveProfile, PhysicalGesture: entry.Key.PhysicalGesture,
				}
				overlap := physicalOverlapCounts[key]
				if overlap == nil {
					overlap = &physicalOverlapAccumulator{days: make(map[string]struct{})}
					physicalOverlapCounts[key] = overlap
				}
				overlap.attempts = saturatingAdd(overlap.attempts, entry.Attempts)
				addAggregateDays(overlap.days, entry)
			}
		case EventPhysicalActivation:
			if entry.Key.Control != "" && !isCompositeControl(entry.Key.Control) {
				controlCounts[entry.Key.Control] = addCountSummary(controlCounts[entry.Key.Control], value)
				if currentStrategy == "" || entry.Key.StrategyID == currentStrategy {
					currentUsedControls[entry.Key.Control] = struct{}{}
				}
			}
		case EventChordProbe:
			nearMiss := isNearMiss(entry.Key) || hasTraceFlag(entry.Key.Flags, "priority_blocked")
			lateModifier := hasTraceFlag(entry.Key.Flags, "late_modifier")
			pointerContext := hasTraceFlag(entry.Key.Flags, "pointer_context")
			stickContext := hasTraceFlag(entry.Key.Flags, "left_stick_active") || hasTraceFlag(entry.Key.Flags, "right_stick_active")
			ambiguous := pointerContext || stickContext
			if nearMiss {
				summary.NearMisses = append(summary.NearMisses, trace)
			}
			if currentStrategy == "" || entry.Key.StrategyID == currentStrategy {
				if entry.Key.BindingProfile != "" && entry.Key.Gesture != "" {
					exposureKey := bindingUsageKey{Profile: entry.Key.BindingProfile, Gesture: entry.Key.Gesture}
					triggerBindingExposure[exposureKey] = saturatingAdd(triggerBindingExposure[exposureKey], entry.Attempts)
				}
				key := chordCandidateKey{
					StrategyID: entry.Key.StrategyID, ForegroundApp: entry.Key.ForegroundApp,
					ActiveProfile: entry.Key.ActiveProfile,
					Gesture:       firstNonEmpty(entry.Key.Gesture, entry.Key.PhysicalGesture, entry.Key.Control),
				}
				candidate := chordCandidateCounts[key]
				if candidate == nil {
					candidate = &chordCandidateAccumulator{
						value: ChordCandidateSummary{
							StrategyID: key.StrategyID, ForegroundApp: key.ForegroundApp,
							ActiveProfile: key.ActiveProfile, Gesture: key.Gesture,
						},
						days:                     make(map[string]struct{}),
						nearMissDays:             make(map[string]struct{}),
						lateModifierDays:         make(map[string]struct{}),
						eligibleDays:             make(map[string]struct{}),
						eligibleNearMissDays:     make(map[string]struct{}),
						eligibleLateModifierDays: make(map[string]struct{}),
					}
					chordCandidateCounts[key] = candidate
				}
				candidate.value.Total = saturatingAdd(candidate.value.Total, entry.Attempts)
				if hasTraceFlag(entry.Key.Flags, "selected") {
					candidate.value.Selected = saturatingAdd(candidate.value.Selected, entry.Attempts)
				}
				if nearMiss {
					candidate.value.NearMiss = saturatingAdd(candidate.value.NearMiss, entry.Attempts)
					addAggregateDays(candidate.nearMissDays, entry)
				}
				if hasTraceFlag(entry.Key.Flags, "fallback") {
					candidate.value.Fallback = saturatingAdd(candidate.value.Fallback, entry.Attempts)
				}
				if lateModifier {
					candidate.value.LateModifier = saturatingAdd(candidate.value.LateModifier, entry.Attempts)
					addAggregateDays(candidate.lateModifierDays, entry)
				}
				if hasTraceFlag(entry.Key.Flags, "priority_blocked") {
					candidate.value.PriorityBlocked = saturatingAdd(candidate.value.PriorityBlocked, entry.Attempts)
				}
				if entry.Key.CandidateResolution == ResolutionUnbound {
					candidate.value.Unbound = saturatingAdd(candidate.value.Unbound, entry.Attempts)
				}
				if entry.Key.CandidateResolution == ResolutionDisabled {
					candidate.value.Disabled = saturatingAdd(candidate.value.Disabled, entry.Attempts)
				}
				if pointerContext {
					candidate.value.PointerContext = saturatingAdd(candidate.value.PointerContext, entry.Attempts)
				}
				if stickContext {
					candidate.value.StickContext = saturatingAdd(candidate.value.StickContext, entry.Attempts)
				}
				if ambiguous {
					candidate.value.Ambiguous = saturatingAdd(candidate.value.Ambiguous, entry.Attempts)
				} else {
					candidate.value.EligibleTotal = saturatingAdd(candidate.value.EligibleTotal, entry.Attempts)
					addAggregateDays(candidate.eligibleDays, entry)
					if hasTraceFlag(entry.Key.Flags, "selected") {
						candidate.value.EligibleSelected = saturatingAdd(candidate.value.EligibleSelected, entry.Attempts)
					}
					if nearMiss {
						candidate.value.EligibleNearMiss = saturatingAdd(candidate.value.EligibleNearMiss, entry.Attempts)
						addAggregateDays(candidate.eligibleNearMissDays, entry)
					}
					if lateModifier {
						candidate.value.EligibleLateModifier = saturatingAdd(candidate.value.EligibleLateModifier, entry.Attempts)
						addAggregateDays(candidate.eligibleLateModifierDays, entry)
					}
				}
				addAggregateDays(candidate.days, entry)
			}
		case EventTransition:
			summary.Transitions = append(summary.Transitions, trace)
			short := isShortCorrectionBucket(entry.Key.IntervalBucket)
			correction := isCorrectionPair(entry.Key.RelatedAction, entry.Key.Action) && short
			if correction {
				summary.Corrections = append(summary.Corrections, trace)
			}
			if (currentStrategy == "" || entry.Key.StrategyID == currentStrategy) && entry.Key.RelatedAction != "" {
				originKey := transitionOriginKey{
					StrategyID: entry.Key.StrategyID, ForegroundApp: entry.Key.ForegroundApp,
					ActiveProfile: entry.Key.ActiveProfile, FromAction: entry.Key.RelatedAction,
					FromGesture: entry.Key.RelatedGesture,
				}
				origin := transitionOrigins[originKey]
				if origin == nil {
					origin = &transitionOriginAccumulator{days: make(map[string]struct{})}
					transitionOrigins[originKey] = origin
				}
				origin.total = saturatingAdd(origin.total, entry.Attempts)
				addAggregateDays(origin.days, entry)
				patternKey := transitionPatternKey{
					transitionOriginKey: originKey, ToAction: entry.Key.Action, ToGesture: entry.Key.Gesture,
				}
				if correction {
					addTransitionPattern(correctionPatterns, patternKey, entry)
				}
				if short && entry.Key.Action != "" && entry.Key.Action == entry.Key.RelatedAction {
					addTransitionPattern(repeatPatterns, patternKey, entry)
				}
			}
		case EventHoldEpisode:
			summary.Holds = append(summary.Holds, trace)
		case EventComposeSession:
			summary.ComposeSessions = append(summary.ComposeSessions, trace)
			if currentStrategy == "" || entry.Key.StrategyID == currentStrategy {
				composeProfileExposure[entry.Key.ActiveProfile] = saturatingAdd(composeProfileExposure[entry.Key.ActiveProfile], entry.Attempts)
				totalComposeExposure = saturatingAdd(totalComposeExposure, entry.Attempts)
			}
		case EventWindowSession:
			summary.WindowSessions = append(summary.WindowSessions, trace)
		case EventContextSession:
			summary.ContextSessions = append(summary.ContextSessions, trace)
		case EventRepeatEpisode:
			summary.RepeatEpisodes = append(summary.RepeatEpisodes, trace)
		}
	}
	for _, candidate := range chordCandidateCounts {
		value := candidate.value
		value.ActiveDays = len(candidate.days)
		value.NearMissDays = len(candidate.nearMissDays)
		value.LateModifierDays = len(candidate.lateModifierDays)
		value.EligibleActiveDays = len(candidate.eligibleDays)
		value.EligibleNearMissDays = len(candidate.eligibleNearMissDays)
		value.EligibleLateModifierDays = len(candidate.eligibleLateModifierDays)
		value.AppKnown = decisionAppKnown(value.ForegroundApp)
		value.Sufficient = value.EligibleTotal >= minimumChordCandidateExposure && value.EligibleActiveDays >= minimumChordActiveDays
		value.Qualified = value.AppKnown && value.Sufficient &&
			((meetsPercent(value.EligibleNearMiss, value.EligibleTotal, minimumChordNearMissPercent) && value.EligibleNearMissDays >= minimumChordActiveDays) ||
				(meetsPercent(value.EligibleLateModifier, value.EligibleTotal, minimumChordLatePercent) && value.EligibleLateModifierDays >= minimumChordActiveDays))
		summary.ChordCandidates = append(summary.ChordCandidates, value)
	}
	for key, overlap := range physicalOverlapCounts {
		summary.PhysicalOverlaps = append(summary.PhysicalOverlaps, PhysicalOverlapSummary{
			StrategyID: key.StrategyID, ForegroundApp: key.ForegroundApp,
			ActiveProfile: key.ActiveProfile, PhysicalGesture: key.PhysicalGesture,
			InputAttempts: overlap.attempts, ActiveDays: len(overlap.days),
		})
	}
	for key, pattern := range correctionPatterns {
		origin := transitionOrigins[key.transitionOriginKey]
		value := TransitionPatternSummary{
			StrategyID: key.StrategyID, ForegroundApp: key.ForegroundApp,
			ActiveProfile: key.ActiveProfile, FromAction: key.FromAction, ToAction: key.ToAction,
			FromGesture: key.FromGesture, ToGesture: key.ToGesture,
			Suspected: pattern.suspected, Total: origin.total,
			ActiveDays: len(pattern.days), ExposureDays: len(origin.days),
		}
		value.AppKnown = decisionAppKnown(value.ForegroundApp)
		value.Qualified = transitionPatternQualified(value)
		summary.CorrectionPatterns = append(summary.CorrectionPatterns, value)
	}
	for key, pattern := range repeatPatterns {
		origin := transitionOrigins[key.transitionOriginKey]
		value := TransitionPatternSummary{
			StrategyID: key.StrategyID, ForegroundApp: key.ForegroundApp,
			ActiveProfile: key.ActiveProfile, FromAction: key.FromAction, ToAction: key.ToAction,
			FromGesture: key.FromGesture, ToGesture: key.ToGesture,
			Suspected: pattern.suspected, Total: origin.total,
			ActiveDays: len(pattern.days), ExposureDays: len(origin.days),
		}
		value.AppKnown = decisionAppKnown(value.ForegroundApp)
		value.Qualified = transitionPatternQualified(value)
		summary.RepeatPatterns = append(summary.RepeatPatterns, value)
	}
	accounted := saturatingAdd(summary.TotalSuccesses, summary.TotalFailures)
	if summary.TotalAttempts > accounted {
		summary.WithoutOutcome = summary.TotalAttempts - accounted
	}

	for control, value := range controlCounts {
		summary.Controls = append(summary.Controls, ControlSummary{Control: control, CountSummary: value})
		if _, used := currentUsedControls[control]; !used {
			summary.UnusedControls = append(summary.UnusedControls, control)
		}
	}
	for key, value := range comboCounts {
		summary.ComboGestures = append(summary.ComboGestures, ComboGestureSummary{
			StrategyID:     key.StrategyID,
			ForegroundApp:  key.ForegroundApp,
			ActiveProfile:  key.ActiveProfile,
			BindingProfile: key.BindingProfile,
			Gesture:        key.Gesture,
			Action:         key.Action,
			Resolution:     key.Resolution,
			CountSummary:   value,
		})
	}
	for app, value := range appCounts {
		summary.Apps = append(summary.Apps, AppSummary{App: app, CountSummary: value})
	}
	for profile, value := range profileCounts {
		summary.Profiles = append(summary.Profiles, ProfileSummary{Profile: profile, CountSummary: value})
	}
	for strategy, attempts := range strategyCounts {
		summary.Strategies = append(summary.Strategies, StrategySummary{ID: strategy, Attempts: attempts})
	}
	unusedSeen := make(map[bindingUsageKey]struct{})
	for _, definition := range state.Inventory {
		if definition.Resolution != ResolutionBound || !strings.Contains(definition.Gesture, "+") {
			continue
		}
		key := bindingUsageKey{Profile: definition.Profile, Gesture: definition.Gesture, Action: definition.Action}
		opportunities := uint64(0)
		switch {
		case strings.HasPrefix(definition.Gesture, "voice+"):
			opportunities = composeProfileExposure[definition.Profile]
			if definition.Profile == "default" {
				opportunities = totalComposeExposure
			}
		case strings.HasPrefix(definition.Gesture, "lt+") || strings.HasPrefix(definition.Gesture, "rt+"):
			opportunities = triggerBindingExposure[bindingUsageKey{Profile: definition.Profile, Gesture: definition.Gesture}]
		default:
			// Stateful/system gestures need their own exposure signal before a
			// "had an opportunity" conclusion is safe.
			continue
		}
		if opportunities < minimumBindingOpportunities {
			continue
		}
		if _, used := usedBindings[key]; used {
			continue
		}
		if _, duplicate := unusedSeen[key]; duplicate {
			continue
		}
		unusedSeen[key] = struct{}{}
		summary.UnusedBindings = append(summary.UnusedBindings, definition)
	}

	sort.Slice(summary.Controls, func(left, right int) bool {
		if summary.Controls[left].Attempts != summary.Controls[right].Attempts {
			return summary.Controls[left].Attempts > summary.Controls[right].Attempts
		}
		return summary.Controls[left].Control < summary.Controls[right].Control
	})
	sort.Slice(summary.Mappings, func(left, right int) bool {
		leftValue, rightValue := summary.Mappings[left], summary.Mappings[right]
		leftFields := [...]string{string(leftValue.Kind), leftValue.StrategyID, leftValue.ForegroundApp, leftValue.ActiveProfile, leftValue.BindingProfile, leftValue.Control, leftValue.Gesture, leftValue.Action, string(leftValue.Resolution)}
		rightFields := [...]string{string(rightValue.Kind), rightValue.StrategyID, rightValue.ForegroundApp, rightValue.ActiveProfile, rightValue.BindingProfile, rightValue.Control, rightValue.Gesture, rightValue.Action, string(rightValue.Resolution)}
		for index := range leftFields {
			if leftFields[index] != rightFields[index] {
				return leftFields[index] < rightFields[index]
			}
		}
		return false
	})
	sort.Slice(summary.ComboGestures, func(left, right int) bool {
		if summary.ComboGestures[left].Attempts != summary.ComboGestures[right].Attempts {
			return summary.ComboGestures[left].Attempts > summary.ComboGestures[right].Attempts
		}
		leftKey := summary.ComboGestures[left].StrategyID + "\x00" + summary.ComboGestures[left].BindingProfile + "\x00" + summary.ComboGestures[left].Gesture + "\x00" + summary.ComboGestures[left].ActiveProfile + "\x00" + summary.ComboGestures[left].ForegroundApp
		rightKey := summary.ComboGestures[right].StrategyID + "\x00" + summary.ComboGestures[right].BindingProfile + "\x00" + summary.ComboGestures[right].Gesture + "\x00" + summary.ComboGestures[right].ActiveProfile + "\x00" + summary.ComboGestures[right].ForegroundApp
		return leftKey < rightKey
	})
	sort.Slice(summary.Apps, func(left, right int) bool {
		if summary.Apps[left].Attempts != summary.Apps[right].Attempts {
			return summary.Apps[left].Attempts > summary.Apps[right].Attempts
		}
		return summary.Apps[left].App < summary.Apps[right].App
	})
	sort.Slice(summary.Profiles, func(left, right int) bool {
		if summary.Profiles[left].Attempts != summary.Profiles[right].Attempts {
			return summary.Profiles[left].Attempts > summary.Profiles[right].Attempts
		}
		return summary.Profiles[left].Profile < summary.Profiles[right].Profile
	})
	sort.Strings(summary.UnusedControls)
	sort.Slice(summary.UnusedBindings, func(left, right int) bool {
		leftKey := summary.UnusedBindings[left].Profile + "\x00" + summary.UnusedBindings[left].Gesture + "\x00" + summary.UnusedBindings[left].Action
		rightKey := summary.UnusedBindings[right].Profile + "\x00" + summary.UnusedBindings[right].Gesture + "\x00" + summary.UnusedBindings[right].Action
		return leftKey < rightKey
	})
	sort.Slice(summary.Strategies, func(left, right int) bool {
		if summary.Strategies[left].Attempts != summary.Strategies[right].Attempts {
			return summary.Strategies[left].Attempts > summary.Strategies[right].Attempts
		}
		return summary.Strategies[left].ID < summary.Strategies[right].ID
	})
	sort.Slice(summary.ChordCandidates, func(left, right int) bool {
		leftValue, rightValue := summary.ChordCandidates[left], summary.ChordCandidates[right]
		if leftValue.Total != rightValue.Total {
			return leftValue.Total > rightValue.Total
		}
		leftKey := strings.Join([]string{leftValue.StrategyID, leftValue.ForegroundApp, leftValue.ActiveProfile, leftValue.Gesture}, "\x00")
		rightKey := strings.Join([]string{rightValue.StrategyID, rightValue.ForegroundApp, rightValue.ActiveProfile, rightValue.Gesture}, "\x00")
		return leftKey < rightKey
	})
	sort.Slice(summary.PhysicalOverlaps, func(left, right int) bool {
		leftValue, rightValue := summary.PhysicalOverlaps[left], summary.PhysicalOverlaps[right]
		if leftValue.InputAttempts != rightValue.InputAttempts {
			return leftValue.InputAttempts > rightValue.InputAttempts
		}
		leftKey := strings.Join([]string{leftValue.StrategyID, leftValue.ForegroundApp, leftValue.ActiveProfile, leftValue.PhysicalGesture}, "\x00")
		rightKey := strings.Join([]string{rightValue.StrategyID, rightValue.ForegroundApp, rightValue.ActiveProfile, rightValue.PhysicalGesture}, "\x00")
		return leftKey < rightKey
	})
	sortTransitionPatterns(summary.CorrectionPatterns)
	sortTransitionPatterns(summary.RepeatPatterns)
	sortTraceEvidence(summary.NearMisses)
	sortTraceEvidence(summary.Transitions)
	sortTraceEvidence(summary.Corrections)
	sortTraceEvidence(summary.Holds)
	sortTraceEvidence(summary.ComposeSessions)
	sortTraceEvidence(summary.WindowSessions)
	sortTraceEvidence(summary.ContextSessions)
	sortTraceEvidence(summary.RepeatEpisodes)
	summary.Recommendations = buildRecommendations(summary)
	return summary
}

func isLegacyPhysicalActivation(key entryKey) bool {
	return key.Kind == EventLegacy && key.Resolution == ResolutionObserved && key.Action == ""
}

func traceSummaryFromEntry(entry *aggregateEntry, value CountSummary) TraceEvidenceSummary {
	return TraceEvidenceSummary{
		Kind: entry.Key.Kind, StrategyID: entry.Key.StrategyID,
		ForegroundApp: entry.Key.ForegroundApp, ActiveProfile: entry.Key.ActiveProfile,
		Control: entry.Key.Control, Gesture: entry.Key.Gesture,
		PhysicalGesture: entry.Key.PhysicalGesture, GestureKind: entry.Key.GestureKind,
		Action: entry.Key.Action, RelatedGesture: entry.Key.RelatedGesture,
		RelatedAction: entry.Key.RelatedAction, Resolution: entry.Key.Resolution,
		CandidateResolution: entry.Key.CandidateResolution,
		IntervalBucket:      entry.Key.IntervalBucket, DurationBucket: entry.Key.DurationBucket,
		CountBucket: entry.Key.CountBucket, Reason: entry.Key.Reason, Flags: entry.Key.Flags,
		CountSummary: value,
	}
}

func isComboEvidence(key entryKey) bool {
	return key.GestureKind == GestureTriggerChord || key.GestureKind == GestureModeSequence ||
		key.GestureKind == GestureSystemHold || isComboGesture(key.Control, key.Gesture)
}

func isNearMiss(key entryKey) bool {
	if key.CandidateResolution == ResolutionUnbound || key.CandidateResolution == ResolutionDisabled ||
		key.Resolution == ResolutionUnbound || key.Resolution == ResolutionDisabled {
		return true
	}
	if key.PhysicalGesture != "" && key.Gesture != "" && key.PhysicalGesture != key.Gesture {
		return true
	}
	value := strings.ToLower(key.Reason + " " + key.Flags)
	return strings.Contains(value, "miss") || strings.Contains(value, "fallback") ||
		strings.Contains(value, "abandon") || strings.Contains(value, "cancel")
}

func isCorrectionPair(previous, current string) bool {
	pairs := map[string]string{
		"arrow_up": "arrow_down", "arrow_down": "arrow_up",
		"arrow_left": "arrow_right", "arrow_right": "arrow_left",
		"tab_previous": "tab_next", "tab_next": "tab_previous",
		"page_up": "page_down", "page_down": "page_up",
		"window_previous": "window_next", "window_next": "window_previous",
		"codex_previous_task": "codex_next_task", "codex_next_task": "codex_previous_task",
	}
	return pairs[previous] == current
}

func isShortCorrectionBucket(value string) bool {
	return value == "under_250ms" || value == "250_750ms"
}

func hasTraceFlag(flags, wanted string) bool {
	for _, flag := range strings.Split(flags, ",") {
		if strings.TrimSpace(flag) == wanted {
			return true
		}
	}
	return false
}

func addAggregateDays(destination map[string]struct{}, entry *aggregateEntry) {
	for day, value := range entry.Daily {
		if value.Attempts > 0 {
			destination[day] = struct{}{}
		}
	}
}

func addTransitionPattern(
	destination map[transitionPatternKey]*transitionPatternAccumulator,
	key transitionPatternKey,
	entry *aggregateEntry,
) {
	pattern := destination[key]
	if pattern == nil {
		pattern = &transitionPatternAccumulator{days: make(map[string]struct{})}
		destination[key] = pattern
	}
	pattern.suspected = saturatingAdd(pattern.suspected, entry.Attempts)
	addAggregateDays(pattern.days, entry)
}

func evidencePercent(numerator, denominator uint64) uint64 {
	if denominator == 0 {
		return 0
	}
	if numerator >= denominator {
		return 100
	}
	return uint64(float64(numerator)*100/float64(denominator) + 0.5)
}

func meetsPercent(numerator, denominator, minimum uint64) bool {
	if denominator == 0 || minimum > 100 {
		return false
	}
	if numerator >= denominator {
		return true
	}
	threshold := (denominator / 100) * minimum
	threshold = saturatingAdd(threshold, ((denominator%100)*minimum+99)/100)
	return numerator >= threshold
}

func transitionPatternQualified(value TransitionPatternSummary) bool {
	return value.AppKnown && value.Total >= minimumTransitionExposure &&
		value.Suspected >= minimumPatternOccurrences &&
		meetsPercent(value.Suspected, value.Total, minimumPatternPercent) &&
		value.ActiveDays >= minimumPatternActiveDays
}

func decisionAppKnown(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.EqualFold(value, otherAppBucket)
}

func sortTransitionPatterns(values []TransitionPatternSummary) {
	sort.Slice(values, func(left, right int) bool {
		leftValue, rightValue := values[left], values[right]
		if leftValue.Qualified != rightValue.Qualified {
			return leftValue.Qualified
		}
		leftPercent := evidencePercent(leftValue.Suspected, leftValue.Total)
		rightPercent := evidencePercent(rightValue.Suspected, rightValue.Total)
		if leftPercent != rightPercent {
			return leftPercent > rightPercent
		}
		if leftValue.Suspected != rightValue.Suspected {
			return leftValue.Suspected > rightValue.Suspected
		}
		leftKey := strings.Join([]string{leftValue.StrategyID, leftValue.ForegroundApp, leftValue.ActiveProfile, leftValue.FromAction, leftValue.FromGesture, leftValue.ToAction, leftValue.ToGesture}, "\x00")
		rightKey := strings.Join([]string{rightValue.StrategyID, rightValue.ForegroundApp, rightValue.ActiveProfile, rightValue.FromAction, rightValue.FromGesture, rightValue.ToAction, rightValue.ToGesture}, "\x00")
		return leftKey < rightKey
	})
}

func sortTraceEvidence(values []TraceEvidenceSummary) {
	sort.Slice(values, func(left, right int) bool {
		if values[left].Attempts != values[right].Attempts {
			return values[left].Attempts > values[right].Attempts
		}
		leftKey := strings.Join([]string{values[left].StrategyID, values[left].ForegroundApp,
			values[left].ActiveProfile, values[left].RelatedAction, values[left].Action,
			values[left].PhysicalGesture, values[left].Gesture, values[left].IntervalBucket,
			values[left].DurationBucket, values[left].Reason}, "\x00")
		rightKey := strings.Join([]string{values[right].StrategyID, values[right].ForegroundApp,
			values[right].ActiveProfile, values[right].RelatedAction, values[right].Action,
			values[right].PhysicalGesture, values[right].Gesture, values[right].IntervalBucket,
			values[right].DurationBucket, values[right].Reason}, "\x00")
		return leftKey < rightKey
	})
}

func buildRecommendations(summary Summary) []string {
	var result []string
	for _, item := range summary.ChordCandidates {
		if !item.Qualified {
			continue
		}
		var signals []string
		if meetsPercent(item.EligibleNearMiss, item.EligibleTotal, minimumChordNearMissPercent) && item.EligibleNearMissDays >= minimumChordActiveDays {
			signals = append(signals, fmt.Sprintf("有效近失误 %d/%d（%d%%，%d 个活跃日）", item.EligibleNearMiss, item.EligibleTotal, evidencePercent(item.EligibleNearMiss, item.EligibleTotal), item.EligibleNearMissDays))
		}
		if meetsPercent(item.EligibleLateModifier, item.EligibleTotal, minimumChordLatePercent) && item.EligibleLateModifierDays >= minimumChordActiveDays {
			signals = append(signals, fmt.Sprintf("有效慢按修饰键 %d/%d（%d%%，%d 个活跃日）", item.EligibleLateModifier, item.EligibleTotal, evidencePercent(item.EligibleLateModifier, item.EligibleTotal), item.EligibleLateModifierDays))
		}
		result = append(result, fmt.Sprintf("优先检查组合 %s（App %s，场景 %s，策略 %s）：%s；已从决策分母排除 %d 次指针/摇杆歧义探测。",
			displayValue(item.Gesture), displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile),
			displayValue(item.StrategyID), strings.Join(signals, "；"), item.Ambiguous))
		break
	}
	for _, item := range summary.CorrectionPatterns {
		if !item.Qualified {
			continue
		}
		result = append(result, fmt.Sprintf("检查 %s → %s（手势 %s → %s；App %s，场景 %s，策略 %s）：短间隔反向动作 %d/%d（%d%%），分布在 %d 个活跃日。",
			displayValue(item.FromAction), displayValue(item.ToAction),
			displayValue(item.FromGesture), displayValue(item.ToGesture), displayAppName(item.ForegroundApp),
			displayValue(item.ActiveProfile), displayValue(item.StrategyID), item.Suspected, item.Total,
			evidencePercent(item.Suspected, item.Total), item.ActiveDays))
		break
	}
	for _, item := range summary.RepeatPatterns {
		if !item.Qualified {
			continue
		}
		result = append(result, fmt.Sprintf("动作 %s（手势 %s → %s；App %s，场景 %s，策略 %s）快速重复 %d/%d（%d%%），分布在 %d 个活跃日；可评估更适合连续操作的键位。",
			displayValue(item.FromAction), displayValue(item.FromGesture), displayValue(item.ToGesture), displayAppName(item.ForegroundApp), displayValue(item.ActiveProfile),
			displayValue(item.StrategyID), item.Suspected, item.Total,
			evidencePercent(item.Suspected, item.Total), item.ActiveDays))
		break
	}
	contextSessions := uint64(0)
	for _, item := range summary.ContextSessions {
		if summary.CurrentStrategyID == "" || item.StrategyID == summary.CurrentStrategyID {
			contextSessions = saturatingAdd(contextSessions, item.Attempts)
		}
	}
	versionedInputs := uint64(0)
	for _, item := range summary.Strategies {
		if summary.CurrentStrategyID == "" || item.ID == summary.CurrentStrategyID {
			versionedInputs = saturatingAdd(versionedInputs, item.Attempts)
		}
	}
	if len(summary.UnusedControls) > 0 && (contextSessions >= 5 || versionedInputs >= 20) {
		result = append(result, fmt.Sprintf("当前窗口有 %d 个实体控制尚未观察到，可作为高频动作的候选空闲键位。", len(summary.UnusedControls)))
	}
	if summary.Dropped > 0 || summary.InvalidDropped > 0 || summary.Coalesced > 0 {
		result = append(result, "数据存在丢弃、无效或折叠记录；在自动改键前应先扩大完整样本。")
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type bindingUsageKey struct {
	Profile string
	Gesture string
	Action  string
}

func addCountSummary(value, increment CountSummary) CountSummary {
	value.Attempts = saturatingAdd(value.Attempts, increment.Attempts)
	value.Successes = saturatingAdd(value.Successes, increment.Successes)
	value.Failures = saturatingAdd(value.Failures, increment.Failures)
	return value
}

func withoutOutcomeCount(attempts, successes, failures uint64) uint64 {
	accounted := saturatingAdd(successes, failures)
	if attempts > accounted {
		return attempts - accounted
	}
	return 0
}

func isCompositeControl(control string) bool {
	return strings.Contains(control, "+")
}

func isComboGesture(control, gesture string) bool {
	return gesture != "" && (gesture != control || strings.Contains(gesture, "+"))
}

func displayValue(value string) string {
	value = strings.Map(func(character rune) rune {
		switch character {
		case '\r', '\n', '\t':
			return ' '
		default:
			return character
		}
	}, value)
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	return value
}

func sanitizedValues(values []string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = displayValue(value)
	}
	return result
}

func friendlyControlValues(values []string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = friendlyControlName(value) + "（" + displayValue(value) + "）"
	}
	return result
}

func displayAppName(value string) string {
	if strings.TrimSpace(value) == "" {
		return "历史数据 / 无法识别"
	}
	return displayValue(value)
}

func friendlyControlName(control string) string {
	labels := map[string]string{
		"a":           "A 键",
		"b":           "B 键",
		"x":           "X 键",
		"y":           "Y 键",
		"back":        "返回键 Back",
		"start":       "菜单键 Start",
		"dpad_up":     "十字键上",
		"dpad_down":   "十字键下",
		"dpad_left":   "十字键左",
		"dpad_right":  "十字键右",
		"lb":          "左肩键 LB",
		"rb":          "右肩键 RB",
		"l3":          "左摇杆按下 L3",
		"r3":          "右摇杆按下 R3",
		"lt":          "左扳机 LT",
		"rt":          "右扳机 RT",
		"left_stick":  "左摇杆",
		"right_stick": "右摇杆",
	}
	if label, found := labels[control]; found {
		return label
	}
	return displayValue(control)
}

func friendlyResolutionName(resolution Resolution) string {
	switch resolution {
	case ResolutionBound:
		return "已映射"
	case ResolutionUnbound:
		return "未映射"
	case ResolutionDisabled:
		return "已禁用"
	case ResolutionObserved:
		return "仅观察"
	case ResolutionSystem:
		return "系统动作"
	default:
		return displayValue(string(resolution))
	}
}

func reportRetry(attempt int) {
	if attempt+1 >= reportReadTries {
		return
	}
	delay := reportRetryInitialDelay
	for step := 0; step < attempt && delay < reportRetryMaximumDelay; step++ {
		delay *= 2
		if delay > reportRetryMaximumDelay {
			delay = reportRetryMaximumDelay
		}
	}
	time.Sleep(delay)
}

func refreshReportBestEffort(directory string, onError func(error)) {
	summary, err := ReadSummary(directory)
	if err == nil {
		err = writeHTMLReport(directory, summary)
	}
	if err != nil {
		safeReport(onError, fmt.Errorf("refresh local usage report: %w", err))
	}
}

func writeHTMLReport(directory string, summary Summary) error {
	templateValue, err := template.New("usage-report").Funcs(template.FuncMap{
		"display":        displayValue,
		"appName":        displayAppName,
		"controlName":    friendlyControlName,
		"resolutionName": friendlyResolutionName,
		"percent":        evidencePercent,
		"first":          firstNonEmpty,
		"withoutOutcome": withoutOutcomeCount,
	}).Parse(reportHTMLTemplate)
	if err != nil {
		return fmt.Errorf("prepare usage HTML report: %w", err)
	}
	var output bytes.Buffer
	if err := templateValue.Execute(&output, summary); err != nil {
		return fmt.Errorf("render usage HTML report: %w", err)
	}
	if !utf8.Valid(output.Bytes()) {
		return errors.New("render usage HTML report: output is not valid UTF-8")
	}
	return replaceReport(ReportPath(directory), output.Bytes())
}

func replaceReport(path string, data []byte) error {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return fmt.Errorf("replace usage HTML report: %s is a directory", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect usage HTML report: %w", err)
	}
	temporary := path + ".tmp"
	backup := path + ".replace-backup"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create usage HTML report temporary file: %w", err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return fmt.Errorf("protect usage HTML report temporary file: %w", err)
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("write usage HTML report temporary file: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("close usage HTML report temporary file: %w", closeErr)
	}

	hadCurrent := false
	if _, err := os.Stat(path); err == nil {
		hadCurrent = true
		_ = removeForReplace(backup)
		if err := renameForReplace(path, backup); err != nil {
			_ = os.Remove(temporary)
			return fmt.Errorf("stage old usage HTML report: %w", err)
		}
	}
	if err := renameForReplace(temporary, path); err != nil {
		if hadCurrent {
			_ = renameForReplace(backup, path)
		}
		_ = os.Remove(temporary)
		return fmt.Errorf("replace usage HTML report: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect usage HTML report: %w", err)
	}
	if hadCurrent {
		_ = removeForReplace(backup)
	}
	return nil
}

const reportHTMLTemplate = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src 'none'; connect-src 'none'; font-src 'none'; script-src 'none'">
  <title>CouchPilot 本地按键使用报告</title>
  <style>
    :root { color-scheme: light; --paper:#f7efe4; --card:#fffaf2; --ink:#3f3028; --muted:#7d6c60; --line:#e6d5c4; --accent:#b85f3f; --accent-soft:#f1d8c8; --good:#62785e; --bad:#a14e43; }
    * { box-sizing: border-box; }
    body { margin:0; background:linear-gradient(145deg,#f4e8d9,#fbf6ef 52%,#efe1d2); color:var(--ink); font:15px/1.55 system-ui,-apple-system,"Segoe UI","Microsoft YaHei",sans-serif; }
    main { width:min(1120px,calc(100% - 32px)); margin:32px auto 56px; }
    header { padding:28px 30px; border:1px solid var(--line); border-radius:22px; background:rgba(255,250,242,.94); box-shadow:0 18px 50px rgba(92,60,39,.09); }
    h1 { margin:0; font-size:clamp(26px,4vw,42px); letter-spacing:-.03em; }
    h2 { margin:0 0 16px; font-size:20px; }
    p { margin:8px 0 0; color:var(--muted); }
    .eyebrow { color:var(--accent); font-weight:750; letter-spacing:.08em; text-transform:uppercase; }
    .grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(150px,1fr)); gap:14px; margin:18px 0; }
    .metric,.panel { border:1px solid var(--line); background:var(--card); border-radius:18px; box-shadow:0 10px 32px rgba(92,60,39,.06); }
    .metric { padding:18px; }
    .metric strong { display:block; font-size:28px; line-height:1.15; }
    .metric span { color:var(--muted); }
    .panel { padding:22px; margin-top:16px; overflow:hidden; }
	.table-scroll { overflow-x:auto; }
	.mapping-table { min-width:1040px; }
    .two { display:grid; grid-template-columns:1fr 1fr; gap:16px; }
    table { width:100%; border-collapse:collapse; }
    th,td { padding:10px 9px; border-bottom:1px solid var(--line); text-align:left; vertical-align:top; }
    th { color:var(--muted); font-size:12px; letter-spacing:.05em; }
    td.num,th.num { text-align:right; font-variant-numeric:tabular-nums; }
    tr:last-child td { border-bottom:0; }
    code,.chip { display:inline-block; border:1px solid var(--line); background:#f8eadc; border-radius:9px; padding:2px 7px; color:#6f402f; font:13px/1.45 ui-monospace,"Cascadia Code",monospace; }
    .status { display:inline-block; border:1px solid #d8cfbd; background:#f2eee3; border-radius:999px; padding:2px 8px; color:#5c594c; font-size:12px; white-space:nowrap; }
    .chips { display:flex; flex-wrap:wrap; gap:8px; }
    .empty { color:var(--muted); }
    .notice { margin:14px 0 0; padding:12px 14px; border:1px solid #e3c579; border-radius:12px; background:#fff3c9; color:#6f5316; font-weight:650; }
    .complete { margin:14px 0 0; padding:12px 14px; border:1px solid #cbd8c5; border-radius:12px; background:#edf4e9; color:#4f684a; }
    .privacy { border-color:#d9c9b9; background:#f4eadf; }
    .privacy strong { color:var(--good); }
    @media (max-width:820px) { .grid { grid-template-columns:1fr 1fr; } .two { grid-template-columns:1fr; } .panel { overflow-x:auto; } }
    @media (max-width:480px) { main { width:min(100% - 20px,1120px); margin-top:10px; } header,.panel { padding:18px; border-radius:15px; } .grid { gap:9px; } .metric { padding:14px; } }
  </style>
</head>
<body>
<main>
  <header>
    <div class="eyebrow">本地 · 隐私 · 只读</div>
    <h1>CouchPilot 按键使用报告</h1>
    <p>统计日期 {{display .UpdatedDay}} · 观察期 {{display .FirstObservedDay}}–{{display .LastObservedDay}} · 快照批次 {{.SnapshotThrough}} · 已合并近期日志至 {{.AppliedThrough}} · 仅保存在这台电脑上</p>
  </header>

  <section class="grid" aria-label="总览">
    <div class="metric"><strong>{{.TotalAttempts}}</strong><span>输入尝试</span></div>
    <div class="metric"><strong>{{.TotalSuccesses}}</strong><span>派发成功</span></div>
    <div class="metric"><strong>{{.TotalFailures}}</strong><span>派发失败</span></div>
    <div class="metric"><strong>{{.WithoutOutcome}}</strong><span>未派发 / 仅观察</span></div>
    <div class="metric"><strong>{{.Dropped}}</strong><span>因繁忙未记录</span></div>
  </section>
  <p>派发成功只表示系统接收了动作，不表示用户意图已经正确完成；诊断事件不计入输入尝试。各视图是同一批事件的不同统计维度，彼此可能重叠，不能相加。无效 {{.InvalidDropped}} · 折叠 {{.Coalesced}} · 序列断点 {{.SequenceBreaks}}。</p>
  {{if gt .Dropped 0}}<div class="notice">注意：有 {{.Dropped}} 条事件因记录队列繁忙而未写入，因此“当前键位策略尚未观察到”的清单可能不可靠。</div>{{else}}<div class="complete">数据完整：没有检测到因记录队列繁忙而丢弃的事件。</div>{{end}}

  <section class="panel">
    <h2>实体按键与摇杆</h2>
    {{if .Controls}}
    <table><thead><tr><th>控制</th><th class="num">次数</th><th class="num">派发成功</th><th class="num">派发失败</th></tr></thead><tbody>
    {{range .Controls}}<tr><td>{{controlName .Control}}<br><code>{{.Control}}</code></td><td class="num">{{.Attempts}}</td><td class="num">{{.Successes}}</td><td class="num">{{.Failures}}</td></tr>{{end}}
    </tbody></table>
    {{else}}<p class="empty">暂无控制记录。</p>{{end}}
  </section>

  <section class="panel">
    <h2>组合键与状态手势</h2>
    {{if .ComboGestures}}
    <table><thead><tr><th>手势</th><th>前台 App</th><th>当前场景</th><th>映射来源</th><th>策略</th><th>动作</th><th class="num">次数</th><th class="num">派发成功</th><th class="num">派发失败</th></tr></thead><tbody>
    {{range .ComboGestures}}<tr><td><code>{{.Gesture}}</code></td><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td>{{display .BindingProfile}}</td><td><code>{{display .StrategyID}}</code></td><td>{{display .Action}}</td><td class="num">{{.Attempts}}</td><td class="num">{{.Successes}}</td><td class="num">{{.Failures}}</td></tr>{{end}}
    </tbody></table>
    {{else}}<p class="empty">本观察窗口尚未观察到组合手势。</p>{{end}}
  </section>

  <div class="two">
    <section class="panel">
      <h2>前台 App（进程名）</h2>
      {{if .Apps}}<table><thead><tr><th>App</th><th class="num">次数</th><th class="num">派发成功</th><th class="num">派发失败</th></tr></thead><tbody>
      {{range .Apps}}<tr><td>{{appName .App}}</td><td class="num">{{.Attempts}}</td><td class="num">{{.Successes}}</td><td class="num">{{.Failures}}</td></tr>{{end}}
      </tbody></table>{{else}}<p class="empty">暂无 App 记录。</p>{{end}}
    </section>
    <section class="panel">
      <h2>使用场景</h2>
      {{if .Profiles}}<table><thead><tr><th>场景</th><th class="num">次数</th><th class="num">派发成功</th><th class="num">派发失败</th></tr></thead><tbody>
      {{range .Profiles}}<tr><td>{{.Profile}}</td><td class="num">{{.Attempts}}</td><td class="num">{{.Successes}}</td><td class="num">{{.Failures}}</td></tr>{{end}}
      </tbody></table>{{else}}<p class="empty">暂无场景记录。</p>{{end}}
    </section>
  </div>

  <section class="panel">
    <h2>当前键位策略尚未观察到的物理控制</h2>
    {{if .UnusedControls}}<div class="chips">{{range .UnusedControls}}<span class="chip">{{controlName .}}（{{.}}）</span>{{end}}</div>{{else}}<p class="empty">当前清单中的控制均已观察到。</p>{{end}}
  </section>

  <section class="panel">
    <h2>有足够使用机会但尚未观察到的已启用组合绑定</h2>
    {{if .UnusedBindings}}
    <table><thead><tr><th>映射场景</th><th>组合手势</th><th>动作</th></tr></thead><tbody>
    {{range .UnusedBindings}}<tr><td>{{.Profile}}</td><td><code>{{.Gesture}}</code></td><td>{{.Action}}</td></tr>{{end}}
    </tbody></table>
    {{else}}<p class="empty">暂无可下结论的项目。</p>{{end}}
  </section>

  <section class="panel">
    <h2>映射执行明细</h2>
    {{if .Mappings}}
    <div class="table-scroll"><table class="mapping-table"><thead><tr><th>前台 App</th><th>当前场景</th><th>映射来源</th><th>策略</th><th>控制</th><th>手势</th><th>动作</th><th>解析结果</th><th class="num">次数</th><th class="num">派发成功</th><th class="num">派发失败</th></tr></thead><tbody>
    {{range .Mappings}}<tr><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td>{{display .BindingProfile}}</td><td><code>{{display .StrategyID}}</code></td><td><code>{{display .Control}}</code></td><td><code>{{display .Gesture}}</code></td><td>{{display .Action}}</td><td><span class="status">{{resolutionName .Resolution}}</span></td><td class="num">{{.Attempts}}</td><td class="num">{{.Successes}}</td><td class="num">{{.Failures}}</td></tr>{{end}}
    </tbody></table></div>
    {{else}}<p class="empty">暂无映射执行记录。</p>{{end}}
  </section>

  <section class="panel">
    <h2>键位策略证据</h2>
    {{if .Recommendations}}<div class="chips">{{range .Recommendations}}<span class="chip">{{.}}</span>{{end}}</div>{{else}}<p class="empty">当前样本尚不足以形成具体建议。</p>{{end}}
    <p>策略版本 {{range .Strategies}}<code>{{display .ID}}</code>（{{.Attempts}} 次） {{else}}历史数据 / 无法识别{{end}}</p>
    <p>以下比率只在当前策略内、同一 App 与场景中比较。组合候选门槛：有效曝光 ≥5、有效活跃日 ≥2、信号占比 ≥30% 且信号跨 ≥2 日；修正/快速重复门槛：同源转移 ≥10、模式 ≥3、占比 ≥30% 且跨 ≥2 日。未达到门槛的项目只展示线索，不形成建议，也不会自动改键。</p>
    {{if .ChordCandidates}}
    <div class="table-scroll"><table class="mapping-table"><thead><tr><th>组合候选</th><th>App</th><th>场景</th><th>策略</th><th class="num">原始总曝光</th><th class="num">歧义（pointer / stick）</th><th class="num">有效分母</th><th class="num">原始 / 有效选中</th><th class="num">有效近失误 / 分母</th><th class="num">原始近失误</th><th class="num">回退</th><th class="num">有效慢按 / 分母</th><th class="num">原始慢按</th><th class="num">优先级阻挡</th><th class="num">未绑定</th><th class="num">禁用</th><th class="num">总 / 有效活跃日</th><th>结论</th></tr></thead><tbody>
    {{range .ChordCandidates}}<tr><td><code>{{display .Gesture}}</code></td><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td><code>{{display .StrategyID}}</code></td><td class="num">{{.Total}}</td><td class="num">{{.Ambiguous}}（{{.PointerContext}} / {{.StickContext}}）</td><td class="num">{{.EligibleTotal}}</td><td class="num">{{.Selected}} / {{.EligibleSelected}}</td><td class="num">{{.EligibleNearMiss}} / {{.EligibleTotal}}（{{percent .EligibleNearMiss .EligibleTotal}}%，{{.EligibleNearMissDays}} 天）</td><td class="num">{{.NearMiss}}</td><td class="num">{{.Fallback}}</td><td class="num">{{.EligibleLateModifier}} / {{.EligibleTotal}}（{{percent .EligibleLateModifier .EligibleTotal}}%，{{.EligibleLateModifierDays}} 天）</td><td class="num">{{.LateModifier}}</td><td class="num">{{.PriorityBlocked}}</td><td class="num">{{.Unbound}}</td><td class="num">{{.Disabled}}</td><td class="num">{{.ActiveDays}} / {{.EligibleActiveDays}}</td><td>{{if not .AppKnown}}App 未知或已合并多个 App，仅展示，不形成建议{{else if .Qualified}}达到建议门槛{{else if .Sufficient}}有效曝光已达门槛，当前比率或跨日稳定性未触发建议{{else}}有效样本不足，仅展示，不形成建议{{end}}</td></tr>{{end}}
    </tbody></table></div>
    {{else}}<p class="empty">暂无组合候选探测记录。</p>{{end}}

    {{if .PhysicalOverlaps}}
    <h2>同帧数字键重叠线索</h2>
    <p>这里只能统计 input_attempt 条数。一个实际帧可能因多个 rising button 产生多条，聚合数据无法按帧去重，所以只作为线索，不直接形成改键建议。</p>
    <div class="table-scroll"><table><thead><tr><th>实体重叠</th><th>App</th><th>场景</th><th>策略</th><th class="num">input_attempt 条数</th><th class="num">活跃日</th></tr></thead><tbody>
    {{range .PhysicalOverlaps}}<tr><td><code>{{display .PhysicalGesture}}</code></td><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td><code>{{display .StrategyID}}</code></td><td class="num">{{.InputAttempts}}</td><td class="num">{{.ActiveDays}}</td></tr>{{end}}
    </tbody></table></div>
    {{end}}

    {{if .CorrectionPatterns}}
    <h2>疑似修正比率</h2>
    <div class="table-scroll"><table><thead><tr><th>动作</th><th>App</th><th>场景</th><th>策略</th><th class="num">疑似 / from 总转移</th><th class="num">证据活跃日</th><th class="num">曝光日</th><th>结论</th></tr></thead><tbody>
    {{range .CorrectionPatterns}}<tr><td><code>{{display .FromAction}}</code> → <code>{{display .ToAction}}</code><br><code>{{display .FromGesture}}</code> → <code>{{display .ToGesture}}</code></td><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td><code>{{display .StrategyID}}</code></td><td class="num">{{.Suspected}} / {{.Total}}（{{percent .Suspected .Total}}%）</td><td class="num">{{.ActiveDays}}</td><td class="num">{{.ExposureDays}}</td><td>{{if not .AppKnown}}App 未知或已合并多个 App，仅展示，不形成建议{{else if .Qualified}}达到建议门槛{{else}}样本或稳定性不足，仅展示，不形成建议{{end}}</td></tr>{{end}}
    </tbody></table></div>
    {{end}}

    {{if .RepeatPatterns}}
    <h2>快速重复比率</h2>
    <div class="table-scroll"><table><thead><tr><th>动作</th><th>App</th><th>场景</th><th>策略</th><th class="num">快速重复 / from 总转移</th><th class="num">证据活跃日</th><th class="num">曝光日</th><th>结论</th></tr></thead><tbody>
    {{range .RepeatPatterns}}<tr><td><code>{{display .FromAction}}</code><br><code>{{display .FromGesture}}</code> → <code>{{display .ToGesture}}</code></td><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td><code>{{display .StrategyID}}</code></td><td class="num">{{.Suspected}} / {{.Total}}（{{percent .Suspected .Total}}%）</td><td class="num">{{.ActiveDays}}</td><td class="num">{{.ExposureDays}}</td><td>{{if not .AppKnown}}App 未知或已合并多个 App，仅展示，不形成建议{{else if .Qualified}}达到建议门槛{{else}}样本或稳定性不足，仅展示，不形成建议{{end}}</td></tr>{{end}}
    </tbody></table></div>
    {{end}}

    {{if or .Holds .ComposeSessions .WindowSessions .RepeatEpisodes}}
    <h2>过程证据</h2>
    <div class="table-scroll"><table class="mapping-table"><thead><tr><th>类型</th><th>手势 / 动作</th><th>App</th><th>场景</th><th>策略</th><th class="num">次数</th><th>时长档</th><th>计数档</th><th>原因</th><th class="num">结果：成功 / 失败 / 无结果</th></tr></thead><tbody>
    {{range .Holds}}<tr><td>Hold</td><td><code>{{display (first .PhysicalGesture .Gesture .Control)}}</code> / <code>{{display (first .Action .RelatedAction)}}</code></td><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td><code>{{display .StrategyID}}</code></td><td class="num">{{.Attempts}}</td><td>{{display .DurationBucket}}</td><td>{{display .CountBucket}}</td><td>{{display .Reason}}</td><td class="num">{{.Successes}} / {{.Failures}} / {{withoutOutcome .Attempts .Successes .Failures}}</td></tr>{{end}}
    {{range .ComposeSessions}}<tr><td>Compose</td><td><code>{{display .Gesture}}</code> / <code>{{display .Action}}</code></td><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td><code>{{display .StrategyID}}</code></td><td class="num">{{.Attempts}}</td><td>{{display .DurationBucket}}</td><td>{{display .CountBucket}}</td><td>{{display .Reason}}</td><td class="num">{{.Successes}} / {{.Failures}} / {{withoutOutcome .Attempts .Successes .Failures}}</td></tr>{{end}}
    {{range .WindowSessions}}<tr><td>Window</td><td><code>{{display .Gesture}}</code> / <code>{{display .Action}}</code></td><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td><code>{{display .StrategyID}}</code></td><td class="num">{{.Attempts}}</td><td>{{display .DurationBucket}}</td><td>{{display .CountBucket}}</td><td>{{display .Reason}}</td><td class="num">{{.Successes}} / {{.Failures}} / {{withoutOutcome .Attempts .Successes .Failures}}</td></tr>{{end}}
    {{range .RepeatEpisodes}}<tr><td>Repeat</td><td><code>{{display (first .PhysicalGesture .Gesture .Control)}}</code> / <code>{{display .Action}}</code></td><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td><code>{{display .StrategyID}}</code></td><td class="num">{{.Attempts}}</td><td>{{display .DurationBucket}}</td><td>{{display .CountBucket}}</td><td>{{display .Reason}}</td><td class="num">{{.Successes}} / {{.Failures}} / {{withoutOutcome .Attempts .Successes .Failures}}</td></tr>{{end}}
    </tbody></table></div>
    {{end}}

    {{if .ContextSessions}}
    <h2>App 交互会话</h2>
    <div class="table-scroll"><table><thead><tr><th>App</th><th>场景</th><th>策略</th><th class="num">会话次数</th><th>原因</th></tr></thead><tbody>
    {{range .ContextSessions}}<tr><td>{{appName .ForegroundApp}}</td><td>{{display .ActiveProfile}}</td><td><code>{{display .StrategyID}}</code></td><td class="num">{{.Attempts}}</td><td>{{display .Reason}}</td></tr>{{end}}
    </tbody></table></div>
    {{end}}
    <p>组合候选 {{len .ChordCandidates}} 类 · 数字键重叠 {{len .PhysicalOverlaps}} 类 · 动作转移 {{len .Transitions}} 类 · 疑似修正 {{len .CorrectionPatterns}} 类 · 快速重复 {{len .RepeatPatterns}} 类 · Hold {{len .Holds}} 类 · 连续操作 episode {{len .RepeatEpisodes}} 类 · Compose {{len .ComposeSessions}} 类 · 窗口会话 {{len .WindowSessions}} 类 · App 交互会话 {{len .ContextSessions}} 类。</p>
  </section>

  <section class="panel privacy">
    <h2>隐私说明</h2>
    <p><strong>这是一份完全本地的聚合报告。</strong>它只包含前台可执行文件名（例如 <code>ChatGPT.exe</code>）、CouchPilot 手柄控制名称、映射场景和累计计数；不记录输入文字、窗口标题、完整进程路径、指针位置或精确操作时间。报告没有外链、脚本或网络请求，也不会自动上传。</p>
    <p>其中 {{.WithoutOutcome}} 次输入尝试没有派发结果；组合、Hold、Compose、窗口切换和会话证据是独立聚合视图，不会重复加入输入尝试总数。</p>
  </section>
</main>
</body>
</html>
`
