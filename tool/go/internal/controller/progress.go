package controller

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

// All run writers, including doctor cleanup, share this sink.
type progressWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *progressWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(data)
}

type progressLogger struct {
	writer        io.Writer
	language      string
	now           func() time.Time
	ticker        func() (<-chan time.Time, func())
	stopHeartbeat func()
}

func newProgress(writer io.Writer, language string) *progressLogger {
	if writer == nil {
		writer = io.Discard
	}
	return &progressLogger{writer: &progressWriter{w: writer}, language: language, now: time.Now,
		ticker: func() (<-chan time.Time, func()) {
			t := time.NewTicker(30 * time.Second)
			return t.C, t.Stop
		}}
}

func (p *progressLogger) text(en, ko string) string {
	if p.language == "ko" {
		return ko
	}
	return en
}

func (p *progressLogger) line(en, ko string, args ...any) {
	fmt.Fprintln(p.writer, fmt.Sprintf(p.text(en, ko), args...))
}

// Only the controller starts/stops stages. The goroutine uses an immutable label
// and timestamp, never a runState or candidate map.
func (p *progressLogger) begin(en, ko string, args ...any) {
	p.stop()
	message := fmt.Sprintf(p.text(en, ko), args...)
	fmt.Fprintln(p.writer, message)
	started := p.now()
	ticks, cancel := p.ticker()
	stop, done := make(chan struct{}), make(chan struct{})
	p.stopHeartbeat = func() {
		close(stop)
		cancel()
		<-done
	}
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case now, ok := <-ticks:
				if !ok {
					return
				}
				select {
				case <-stop:
					return
				default:
				}
				elapsed := max(time.Duration(0), now.Sub(started)).Round(time.Second)
				p.line("Still working: %s (%s elapsed)", "계속 진행 중입니다: %s (%s 경과)", message, elapsed)
			}
		}
	}()
}

func (p *progressLogger) stop() {
	if p.stopHeartbeat != nil {
		p.stopHeartbeat()
		p.stopHeartbeat = nil
	}
}

func (r *runner) progressLog() *progressLogger {
	if r.progress == nil {
		r.progress = newProgress(r.surface.Stderr, r.surface.Args.Language)
		r.surface.Stderr = r.progress.writer
	}
	return r.progress
}

func (r *runner) endProgress() { r.progressLog().stop() }

func (r *runner) begin(en, ko string, args ...any)  { r.progressLog().begin(en, ko, args...) }
func (r *runner) notice(en, ko string, args ...any) { r.progressLog().line(en, ko, args...) }

func (r *runner) startPhase(phase string) {
	label := r.candidateLabel
	if label == "" {
		label = r.progressLog().text("selected candidate", "선택한 항목")
	}
	switch phase {
	case "BASELINE":
		r.begin("Checking validation before changes.", "변경 전 검증 상태를 확인하고 있습니다.")
	case "AUDIT":
		r.begin("Looking for refactoring candidates (audit %d).", "리팩토링 후보를 찾고 있습니다. 조사 %d회차입니다.", r.state.Cycle)
	case "DEEP_CHECK":
		r.begin("Checking %s.", "%s 항목을 확인하고 있습니다.", label)
	case "CHARACTERIZE":
		r.begin("Adding tests for the current behavior of %s.", "%s의 현재 동작을 확인할 테스트를 작성하고 있습니다.", label)
	case "PREFLIGHT":
		r.begin("Checking whether %s is ready to implement.", "%s의 구현 준비 상태를 확인하고 있습니다.", label)
	case "EXECUTE":
		r.begin("Implementing %s.", "%s 항목을 수정하고 있습니다.", label)
	case "GATE":
		r.begin("Checking the scope and validation of %s.", "%s의 변경 범위와 검증 결과를 확인하고 있습니다.", label)
	case "REVIEW":
		r.begin("Independently reviewing %s.", "%s의 변경을 독립 검토하고 있습니다.", label)
	case "COMMIT":
		r.begin("Saving the reviewed change for %s.", "%s의 검토된 변경을 저장하고 있습니다.", label)
	}
}

func candidateDescription(candidate map[string]any, language string) string {
	categories := map[string][2]string{
		"DEAD_CODE":             {"unused code", "미사용 코드 제거"},
		"COMPATIBILITY_REMOVAL": {"obsolete compatibility code", "이전 호환 코드 제거"},
		"DEDUPLICATION":         {"duplicate logic", "중복 로직 정리"},
		"LARGE_COMPONENT_SPLIT": {"large component split", "큰 component 분리"},
	}
	category := categories[str(candidate["category"])][0]
	if language == "ko" {
		category = categories[str(candidate["category"])][1]
	}
	symbol := str(candidate["primary_symbol"])
	if symbol == "" {
		symbol = str(candidate["candidate_id"])
	}
	label := surface.DisplayText(symbol)
	if category != "" {
		label += " (" + category + ")"
	}
	if paths := stringsOf(candidate["related_files"]); len(paths) > 0 {
		path := filepath.Clean(paths[0])
		if !filepath.IsAbs(path) && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			label += " — " + surface.DisplayText(filepath.ToSlash(path))
		}
	}
	return surface.DisplayText(label)
}

func (r *runner) OnProviderEvent(event engine.ProviderEvent) {
	switch event.Kind {
	case "schema_repair":
		r.notice("%s returned an invalid response format; trying one read-only repair.", "%s 응답 형식이 맞지 않아 읽기 전용으로 한 번 보정합니다.", surface.DisplayText(event.Provider))
	case "permission_denied":
		r.notice("Blocked %d write attempt(s) during a read-only check by %s.", "읽기 전용 검사 중 쓰기 시도 %d회를 차단했습니다. provider: %s", event.Count, surface.DisplayText(event.Provider))
	}
}

func (r *runner) observeCommand(event workspace.CommandEvent) {
	name, area := surface.DisplayText(event.Command.Name), surface.DisplayText(event.Command.Area)
	if event.Kind == "started" {
		r.notice("Running %s check. Area: %s", "%s 검사를 실행하고 있습니다. 대상: %s", name, area)
		return
	}
	if event.Kind != "completed" {
		return
	}
	result := event.Result
	switch {
	case result.TimedOut || result.Status == workspace.StatusTimeout:
		r.notice("%s check timed out. Area: %s", "%s 검사가 제한 시간을 초과했습니다. 대상: %s", name, area)
	case result.SpawnError != "" || result.Status == workspace.StatusUnrunnable:
		r.notice("Could not start %s check. Area: %s", "%s 검사를 시작하지 못했습니다. 대상: %s", name, area)
	case result.Status == workspace.StatusGreen:
		r.notice("%s check passed. Area: %s", "%s 검사가 통과했습니다. 대상: %s", name, area)
	case !event.Baseline && result.OK && result.Delta != nil:
		r.notice("%s still fails as at baseline; no new errors. Area: %s", "%s 검사는 변경 전과 같이 실패했습니다. 새 오류는 없습니다. 대상: %s", name, area)
	case result.Status == workspace.StatusOpaque:
		r.notice("%s failed without a usable error signature. Area: %s", "%s 검사가 실패했지만 비교할 오류 정보를 얻지 못했습니다. 대상: %s", name, area)
	case event.Baseline:
		r.notice("%s fails before changes; recording errors for comparison. Area: %s", "%s 검사는 변경 전부터 실패합니다. 비교할 오류를 기록합니다. 대상: %s", name, area)
	default:
		r.notice("%s check failed validation. Area: %s [%s]", "%s 검사에서 검증에 실패했습니다. 대상: %s [%s]", name, area, surface.DisplayText(result.FailureKind))
	}
}

func (r *runner) diagnostic(names ...string) {
	for _, name := range names {
		path := filepath.Join(r.runDir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			if surface.DisplayText(path) != path && r.surface.Repo != "" {
				if rel, err := filepath.Rel(r.surface.Repo, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					r.notice("Details (relative to the selected repository): %s", "상세 기록 (선택한 저장소 기준 상대 경로): %s", surface.DisplayText(rel))
					return
				}
			}
			r.notice("Details: %s", "상세 기록: %s", surface.DisplayText(path))
			return
		}
	}
	r.notice("A diagnostic record was not saved for this result.", "이 결과의 진단 기록을 저장하지 못했습니다.")
}

func (r *runner) outcome(code string) {
	r.endProgress()
	r.notice("Skipped %s: %s. [%s]", "%s 항목을 제외했습니다. %s [%s]", r.candidateLabel,
		surface.ReasonLabel(code, r.surface.Args.Language), surface.DisplayText(code))
	r.diagnostic("state.json")
}

func (r *runner) saveSkipped(code string) error {
	if err := r.save(); err != nil {
		return err
	}
	r.outcome(code)
	return nil
}

func (r *runner) runSummary(status string, branch any) string {
	p := r.progressLog()
	branchText := p.text("None (no committed changes)", "없음 (커밋된 변경 없음)")
	if branch != nil && str(branch) != "" {
		branchText = surface.DisplayText(str(branch))
	}
	return fmt.Sprintf("refactor-me %s · %s\n  %s: %s\n  %s: %d\n  %s: %d\n  %s: %s\n  %s: %s\n  %s: %s",
		surface.DisplayText(surface.Version), status,
		p.text("Stop reason", "종료 사유"), surface.StopReason(r.state.Terminal.Reason, r.surface.Args.Language),
		p.text("Audits", "후보 조사"), r.state.Counters.Cycles,
		p.text("Refactor commits", "리팩토링 커밋"), r.state.Counters.Commits,
		p.text("Local branch", "로컬 branch"), branchText,
		p.text("Report", "보고서"), surface.DisplayText(filepath.Join(r.runDir, "report.md")),
		p.text("Worktree", "worktree"), surface.DisplayText(r.wt))
}
