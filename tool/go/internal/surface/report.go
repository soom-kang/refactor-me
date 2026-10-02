package surface

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ReportSchemaVersion identifies reports produced by the global CLI.
const ReportSchemaVersion = 3

func executeReport(repo string, args Args, stdout, stderr io.Writer) int {
	for _, dir := range []string{filepath.Join(repo, ".refactor"), filepath.Join(repo, ".refactor", "runs")} {
		if err := checkDirectory(dir); err != nil {
			fmt.Fprintln(stderr, "refactor-me:", err)
			return ExitAborted
		}
	}
	lastPath := filepath.Join(repo, ".refactor", "last-run.json")
	if err := checkRegularFile(lastPath); err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	lastData, err := os.ReadFile(lastPath)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(stderr, "no run recorded yet")
		return ExitAborted
	}
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	var last map[string]any
	if err = json.Unmarshal(lastData, &last); err != nil {
		fmt.Fprintln(stderr, "refactor-me: last-run.json:", err)
		return ExitAborted
	}
	if last["schemaVersion"] != float64(LastRunSchemaVersion) {
		fmt.Fprintln(stderr, "refactor-me: last-run.json: unsupported schemaVersion")
		return ExitAborted
	}
	runDir, _ := last["runDir"].(string)
	if runDir == "" {
		fmt.Fprintln(stderr, "refactor-me: last-run.json has no runDir")
		return ExitAborted
	}
	canonicalRun, err := filepath.EvalSymlinks(runDir)
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	runsDir, err := filepath.EvalSymlinks(filepath.Join(repo, ".refactor", "runs"))
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	rel, err := filepath.Rel(runsDir, canonicalRun)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		fmt.Fprintln(stderr, "refactor-me: last-run.json points outside this repository's runs directory")
		return ExitAborted
	}
	reportPath := filepath.Join(canonicalRun, "report.json")
	if err := checkRegularFile(reportPath); err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	if _, err := parseReport(data); err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	if args.JSON {
		if _, err := stdout.Write(data); err != nil {
			fmt.Fprintln(stderr, "refactor-me: write report:", err)
			return ExitAborted
		}
		return ExitOK
	}
	text, err := RenderReport(data, args.Language)
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	fmt.Fprint(stdout, text)
	return ExitOK
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func arr(v any) []any { a, _ := v.([]any); return a }
func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}
func val(m map[string]any, key string) string { return str(m[key]) }
func label(lang, en, ko string) string {
	if lang == "ko" {
		return ko
	}
	return en
}
func short(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// RenderReport renders a supported report without modifying the saved evidence.
func RenderReport(data []byte, language string) (string, error) {
	if language != "en" && language != "ko" {
		return "", fmt.Errorf("unsupported language: %s; use en or ko", language)
	}
	report, err := parseReport(data)
	if err != nil {
		return "", err
	}
	status := val(report, "status")
	statusNames := map[string][2]string{"DONE": {"Completed", "완료"}, "NO_CHANGES": {"No changes", "변경 없음"}, "DONE_PARTIAL": {"Partially completed", "부분 완료"}, "ABORTED": {"Aborted", "중단"}, "HALTED_UNSAFE": {"Safety halt", "안전 정지"}}
	statusLabel := status
	if pair, ok := statusNames[status]; ok {
		statusLabel = pair[0]
		if language == "ko" {
			statusLabel = pair[1]
		}
	}
	commits := arr(report["commits"])
	skipped := arr(report["skipped"])
	counters := obj(report["counters"])
	validation := obj(report["validation"])
	branch := val(report, "branch")
	if branch == "<nil>" {
		branch = ""
	}
	base := val(report, "baseCommit")
	lines := []string{"# refactor-me " + val(report, "runId") + ": " + statusLabel, "",
		"- " + label(language, "Duration", "소요") + ": " + val(report, "durationMinutes") + label(language, " min", "분"),
		"- " + label(language, "Tool version", "도구 버전") + ": refactor-me " + fallback(val(report, "toolVersion"), label(language, "Not recorded", "기록 없음")),
		"- " + label(language, "Repository", "저장소") + ": `" + val(report, "repoRoot") + "`",
		"- " + label(language, "Target directories", "대상 폴더") + ": " + reportTargets(arr(report["targets"]), language),
		"- " + label(language, "Base commit", "기준 커밋") + ": `" + short(base) + "` (" + fallback(val(report, "baseBranch"), "detached") + ")"}
	if branch == "" {
		lines = append(lines, label(language, "- Result branch: None (no committed changes)", "- 결과 브랜치: 없음 (커밋된 변경 없음)"))
	} else {
		lines = append(lines, "- "+label(language, "Result branch", "결과 브랜치")+": `"+branch+"`")
	}
	lines = append(lines, "- "+label(language, "Stop reason", "종료 사유")+": "+translateReason(val(report, "reason"), language), "",
		label(language, "## Committed changes", "## 커밋된 변경"), "")
	if none, filtered := numeric(counters["auditsNoProposals"]), numeric(counters["auditsAllFiltered"]); none+filtered > 0 {
		line := fmt.Sprintf("- Audits with no eligible candidate: %d proposed nothing, %d had every proposal filtered by policy", none, filtered)
		if language == "ko" {
			line = fmt.Sprintf("- 후보를 얻지 못한 audit: %d회는 제안 없음, %d회는 제안이 정책 필터에서 모두 제외됨", none, filtered)
		}
		lines = append(lines[:len(lines)-2], append([]string{line, ""}, lines[len(lines)-2:]...)...)
	}
	if len(commits) == 0 {
		lines = append(lines, label(language, "None.", "없음."))
	}
	for _, entry := range commits {
		c := obj(entry)
		lines = append(lines, "- `"+short(val(c, "oid"))+"` **"+val(c, "category")+"** "+val(c, "subject"))
		var paths []string
		for _, p := range arr(c["paths"]) {
			paths = append(paths, "`"+str(p)+"`")
		}
		lines = append(lines, "  - "+label(language, "Paths", "경로")+": "+strings.Join(paths, ", "))
	}
	lines = append(lines, "")
	lines = append(lines, renderCodeComparison(obj(report["codeComparison"]), language)...)
	lines = append(lines, "", label(language, "## Skipped candidates", "## 제외한 후보"), "")
	if len(skipped) == 0 {
		lines = append(lines, label(language, "None.", "없음."))
	} else {
		counts := map[string]int{}
		grouped := map[string][]map[string]any{}
		for _, entry := range skipped {
			key := val(obj(entry), "reason")
			counts[key]++
			grouped[key] = append(grouped[key], obj(entry))
		}
		var keys []string
		for key := range counts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("- **%s** (`%s`) × %d", translateReason(key, language), key, counts[key]))
			for _, item := range grouped[key] {
				detail := item["detail"]
				if nested := obj(detail); len(nested) > 0 {
					detail = nested["detail"]
				}
				if detail != nil && str(detail) != "" {
					text := str(detail)
					if len(text) > 200 {
						text = text[:200]
					}
					lines = append(lines, "  - "+text)
				}
			}
		}
	}
	lines = append(lines, "", label(language, "## Validation", "## 검증"), "",
		label(language, "Baseline", "기준선")+": "+fallback(val(validation, "describe"), label(language, "Not recorded", "기록 없음")), "",
		label(language, "For commands that failed at baseline, validation compares error signatures. Matching failures do not count as passing checks.", "기준선에서 실패한 명령은 오류 signature를 비교합니다. 같은 오류로 실패한 검사를 통과로 집계하지 않습니다."), "",
		label(language, "Commands run:", "실행한 명령:"), "")
	for _, cmd := range arr(validation["ran"]) {
		lines = append(lines, "- `"+str(cmd)+"`")
	}
	if len(arr(validation["notRun"])) > 0 {
		lines = append(lines, "", label(language, "### Checks not run", "### 실행하지 않은 검사"), "")
		for _, cmd := range arr(validation["notRun"]) {
			lines = append(lines, "- `"+str(cmd)+"`")
		}
	}
	lines = append(lines, "", label(language, "## Providers", "## 프로바이더"), "")
	providers := obj(report["providers"])
	var names []string
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := obj(providers[name])
		line := "- **" + name + "**: " + val(p, "status") + " · " + val(p, "calls") + label(language, " calls", "회 호출")
		if numeric(p["quotaHits"]) > 0 {
			line += fmt.Sprintf(" · %s %d", label(language, "quota hits", "쿼터 제한"), numeric(p["quotaHits"]))
		}
		if val(p, "lastError") != "" {
			line += " · " + label(language, "last error", "마지막 오류") + ": " + truncate(val(p, "lastError"), 120)
		}
		lines = append(lines, line)
	}
	lines = append(lines, renderProviderSettings(obj(report["providerSettings"]), language)...)
	lines = append(lines, "")
	lines = append(lines, renderUsage(report, language)...)
	lines = append(lines, label(language, "## Next steps", "## 다음 단계"), "")
	if branch != "" {
		lines = append(lines, "```bash", "git log --oneline "+base+".."+branch, "git diff --stat "+base+" "+branch, "```", "", label(language, "Review the local branch before merging. refactor-me does not merge, push, or deploy.", "로컬 브랜치를 검토한 뒤 병합하세요. refactor-me는 merge, push, 배포를 수행하지 않습니다."))
	} else {
		lines = append(lines, label(language, "No changes were committed; there is no result branch to review.", "커밋된 변경이 없어 검토할 결과 브랜치가 없습니다."))
	}
	if status == "HALTED_UNSAFE" {
		lines = append(lines, "", label(language, "> **Safety halt.** An invariant failed and the loop stopped writing.", "> **안전 정지.** 불변식이 깨져 쓰기를 중단했습니다."), "> "+label(language, "Worktree retained as evidence", "증거로 보존한 worktree")+": `"+val(report, "worktree")+"`")
	}
	_ = counters
	return strings.Join(append(lines, ""), "\n"), nil
}

func fallback(value, other string) string {
	if value == "" || value == "<nil>" {
		return other
	}
	return value
}

func numeric(value any) int {
	switch n := value.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
func decimal(value any) float64 {
	switch n := value.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}
func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func reportTargets(targets []any, language string) string {
	if len(targets) == 0 {
		return label(language, "Entire repository", "저장소 전체")
	}
	var parts []string
	for _, target := range targets {
		parts = append(parts, "`"+str(target)+"`")
	}
	return strings.Join(parts, ", ")
}

func translateReason(reason, language string) string {
	if language != "ko" {
		if translated, ok := reasonEN[reason]; ok {
			return translated
		}
		return reason
	}
	known := map[string]string{"no eligible candidates remain": "실행할 수 있는 후보가 남아 있지 않습니다", "every proposed candidate was filtered by policy": "감사가 제안한 후보를 정책 필터가 모두 제외했습니다", "the audit proposed no candidates": "감사가 후보를 제안하지 않았습니다", "doctor found a blocking problem": "doctor가 실행을 막는 문제를 발견했습니다", "no deterministic validation command could be discovered": "반복 실행할 검증 명령을 찾지 못했습니다"}
	if text, ok := known[reason]; ok {
		return text
	}
	if translated, ok := reasonKO[reason]; ok {
		return translated
	}
	for _, pair := range [][2]string{{"stopped on cycle budget (", "사이클 한도 "}, {"stopped on commit budget (", "커밋 한도 "}} {
		if strings.HasPrefix(reason, pair[0]) && strings.HasSuffix(reason, ")") {
			number := strings.TrimSuffix(strings.TrimPrefix(reason, pair[0]), ")")
			suffix := "회에 도달했습니다"
			if pair[0] == "stopped on commit budget (" {
				suffix = "개에 도달했습니다"
			}
			return pair[1] + number + suffix
		}
	}
	if reason == "stopped on all providers exhausted" {
		return "사용 가능한 프로바이더가 없습니다"
	}
	if strings.HasPrefix(reason, "stopped on wall clock (") && strings.HasSuffix(reason, "m)") {
		minutes := strings.TrimSuffix(strings.TrimPrefix(reason, "stopped on wall clock ("), "m)")
		return "경과 시간 한도 " + minutes + "분에 도달했습니다"
	}
	if strings.HasPrefix(reason, "stopped on ") && strings.HasSuffix(reason, " consecutive failures") {
		count := strings.TrimSuffix(strings.TrimPrefix(reason, "stopped on "), " consecutive failures")
		return "후보가 " + count + "회 연속 실패했습니다"
	}
	if strings.HasPrefix(reason, "no validation command could be executed: ") {
		return "검증 명령을 실행하지 못했습니다: " + strings.TrimPrefix(reason, "no validation command could be executed: ")
	}
	if code, detail, found := strings.Cut(reason, ": "); found {
		if translated, ok := reasonKO[code]; ok {
			return translated + " (" + code + "): " + detail
		}
	}
	return reason
}

var reasonEN = map[string]string{"RISK_EXCLUDED": "Risk outside policy", "MODEL_REJECTED": "Rejected during audit", "ATTEMPTS_EXHAUSTED": "Attempt limit reached", "RISK_UNKNOWN": "Insufficient evidence to assess risk", "OUT_OF_TARGET": "Outside target directories", "TARGET_SCOPE_EMPTY": "Changes only outside target directories", "TOO_LARGE": "Scope limit exceeded", "NOT_READY": "Insufficient evidence", "PREFLIGHT_BLOCKED": "Blocked at preflight", "NO_OP": "No changes", "FORBIDDEN_PATH": "Forbidden path changed", "FORBIDDEN_BINARY": "Binary changed", "OUT_OF_SCOPE": "Outside allowlist", "NET_SIZE_RULE": "Size rule violated", "TEST_WEAKENED": "Test weakened", "THRASH_REVERT": "Earlier work reversed", "REGRESSION": "Validation regression", "VALIDATION_TIMEOUT": "Validation timeout", "REVIEW_REJECT": "Rejected by independent review", "EXECUTE_REJECTED": "Implementation declined", "EXECUTE_FAILED": "Implementation failed", "DEEP_CHECK_FAILED": "Deep check failed", "REVIEW_FAILED": "Review failed", "CHARACTERIZATION_REJECTED": "Characterization scope violated", "CHARACTERIZATION_RED": "Characterization test failed"}
var reasonKO = map[string]string{"RISK_EXCLUDED": "허용 위험도 밖", "MODEL_REJECTED": "감사 단계에서 거절", "ATTEMPTS_EXHAUSTED": "시도 횟수 소진", "RISK_UNKNOWN": "위험도 판정 근거 부족", "OUT_OF_TARGET": "대상 폴더 밖", "TARGET_SCOPE_EMPTY": "대상 폴더 밖만 수정", "TOO_LARGE": "범위 초과", "NOT_READY": "증거 부족", "PREFLIGHT_BLOCKED": "preflight 차단", "NO_OP": "변경 없음", "FORBIDDEN_PATH": "금지 경로 수정", "FORBIDDEN_BINARY": "바이너리 변경", "OUT_OF_SCOPE": "allowlist 밖 수정", "NET_SIZE_RULE": "증감 규칙 위반", "TEST_WEAKENED": "테스트 약화", "THRASH_REVERT": "이전 작업 되돌림", "REGRESSION": "검증 회귀", "VALIDATION_TIMEOUT": "검증 타임아웃", "REVIEW_REJECT": "독립 리뷰 거절", "EXECUTE_REJECTED": "구현 자체 중단", "EXECUTE_FAILED": "구현 실패", "DEEP_CHECK_FAILED": "심층 검증 실패", "REVIEW_FAILED": "리뷰 실패", "CHARACTERIZATION_REJECTED": "characterization 범위 위반", "CHARACTERIZATION_RED": "characterization 테스트 실패"}

func renderCodeComparison(comparison map[string]any, language string) []string {
	lines := []string{label(language, "## Code comparison", "## 코드 비교"), ""}
	if len(comparison) == 0 {
		return append(lines, label(language, "No code comparison was saved for this run.", "이 실행에는 코드 비교가 저장되지 않았습니다."))
	}
	lines = append(lines, label(language, "Base commit", "시작 커밋")+": `"+fallback(val(comparison, "baseCommit"), "-")+"`", "",
		label(language, "Published commit", "최종 반영 커밋")+": `"+fallback(val(comparison, "resultCommit"), "-")+"`", "")
	switch val(comparison, "status") {
	case "UNAVAILABLE":
		return append(lines, label(language, "Code comparison is unavailable. The run result is unchanged.", "코드 비교를 수집하지 못했습니다. 실행 결과에는 영향을 주지 않습니다."), "", fencedEvidence(val(comparison, "error"), ""))
	case "NO_CHANGES":
		lines = append(lines, label(language, "No committed code changes.", "커밋된 코드 변경이 없습니다."))
		return comparisonPatchLink(lines, comparison, language)
	}
	totals := obj(comparison["totals"])
	if language == "ko" {
		lines = append(lines, fmt.Sprintf("파일 %s개 · 텍스트 +%s줄 / -%s줄 · 바이너리 %s개", val(totals, "files"), val(totals, "insertions"), val(totals, "deletions"), val(totals, "binary")))
	} else {
		lines = append(lines, fmt.Sprintf("%s files; +%s / -%s text lines; %s binary files.", val(totals, "files"), val(totals, "insertions"), val(totals, "deletions"), val(totals, "binary")))
	}
	lines = append(lines, "", label(language, "| File | Status | Added | Deleted | Mode |", "| 파일 | 상태 | 추가 | 삭제 | 파일 모드 |"), "|---|---|---:|---:|---|")
	statusEN := map[byte]string{'A': "Added", 'D': "Deleted", 'M': "Modified", 'R': "Renamed", 'C': "Copied", 'T': "Type changed"}
	statusKO := map[byte]string{'A': "추가", 'D': "삭제", 'M': "수정", 'R': "이름 변경", 'C': "복사", 'T': "유형 변경"}
	for _, raw := range arr(comparison["files"]) {
		file := obj(raw)
		name := tableCode(val(file, "path"))
		if val(file, "oldPath") != "" {
			name = tableCode(val(file, "oldPath")) + " → " + name
		}
		status := val(file, "status")
		statusText := status
		if len(status) > 0 {
			if language == "ko" {
				statusText = fallback(statusKO[status[0]], status)
			} else {
				statusText = fallback(statusEN[status[0]], status)
			}
		}
		if file["binary"] == true {
			statusText += label(language, " (binary)", " (바이너리)")
		}
		lines = append(lines, fmt.Sprintf("| %s | %s | %s | %s | %s → %s |", name, statusText, displayNumber(file["insertions"]), displayNumber(file["deletions"]), val(file, "oldMode"), val(file, "newMode")))
	}
	lines = append(lines, "", label(language, "The comparison includes published characterization commits. Rejected or rolled-back edits are excluded.", "최종 반영된 characterization 커밋을 포함합니다. 거절되거나 롤백된 수정은 제외합니다."), "", label(language, "### Diff preview", "### Diff 미리보기"), "", fencedEvidence(val(comparison, "preview"), "diff"))
	if comparison["truncated"] == true {
		lines = append(lines, "", label(language, "Preview omitted after 200 lines or 32 KiB, at a line boundary. See the full text patch below.", "200줄 또는 32 KiB 한도에서 줄 단위로 미리보기를 생략했습니다. 아래 전체 텍스트 patch를 확인하세요."))
	}
	return comparisonPatchLink(lines, comparison, language)
}

func displayNumber(value any) string {
	if value == nil {
		return "—"
	}
	return str(value)
}
func tableCode(value string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "`", "&#96;", "|", "&#124;", "\n", "&#92;n", "\r", "&#92;r", "\t", "&#92;t")
	return "<code>" + r.Replace(value) + "</code>"
}
func fencedEvidence(value, language string) string {
	width := 3
	count := 0
	for _, char := range value {
		if char == '`' {
			count++
			if count+1 > width {
				width = count + 1
			}
		} else {
			count = 0
		}
	}
	fence := strings.Repeat("`", width)
	end := "\n"
	if strings.HasSuffix(value, "\n") {
		end = ""
	}
	return fence + language + "\n" + value + end + fence
}
func comparisonPatchLink(lines []string, comparison map[string]any, language string) []string {
	patch := val(comparison, "patchFile")
	if patch == "" {
		return lines
	}
	patch = strings.ReplaceAll(strings.ReplaceAll(patch, "(", "%28"), ")", "%29")
	return append(lines, "", "["+label(language, "Full text patch", "전체 텍스트 patch")+"]("+patch+")", "", label(language, "Binary contents are omitted; binary and file mode changes appear as metadata.", "바이너리 본문은 생략하며 바이너리·파일 모드 변경은 메타데이터로 표시합니다."))
}

func renderUsage(report map[string]any, language string) []string {
	usage := obj(report["usage"])
	total := obj(usage["totals"])
	if numeric(total["processes"]) == 0 {
		return nil
	}
	lines := []string{label(language, "## Cost and time", "## 비용과 시간"), ""}
	if language == "ko" {
		lines = append(lines, fmt.Sprintf("- 총 소요: %s분 (프로바이더 %s분, 나머지는 검증과 Git 작업 포함)", val(report, "durationMinutes"), val(report, "providerMinutes")), fmt.Sprintf("- 프로바이더: 호출 %d회, CLI 프로세스 %d개, 실패한 호출 %d회", numeric(total["calls"]), numeric(total["processes"]), numeric(total["failedCalls"])), fmt.Sprintf("- 토큰: 입력 %s, 출력 %s", commas(numeric(total["inputTokens"])), commas(numeric(total["outputTokens"]))))
	} else {
		lines = append(lines, fmt.Sprintf("- Duration: %s min (%s min in providers; the remainder includes validation and Git)", val(report, "durationMinutes"), val(report, "providerMinutes")), fmt.Sprintf("- Providers: %d calls, %d CLI processes, %d failed calls", numeric(total["calls"]), numeric(total["processes"]), numeric(total["failedCalls"])), fmt.Sprintf("- Tokens: %s input, %s output", commas(numeric(total["inputTokens"])), commas(numeric(total["outputTokens"]))))
	}
	cost := costText(total)
	if cost == "" {
		lines = append(lines, label(language, "- Cost: **Not reported**. No monetary total is available for these calls.", "- 비용: **미보고**. 이 호출의 비용 합계를 확인할 수 없습니다."))
	} else if numeric(total["costMissing"]) > 0 {
		if language == "ko" {
			lines = append(lines, fmt.Sprintf("- 비용: **%s 이상**. 프로세스 %d개가 비용을 보고하지 않았습니다.", cost, numeric(total["costMissing"])))
		} else {
			lines = append(lines, fmt.Sprintf("- Cost: **%s or more**. %d process(es) did not report cost.", cost, numeric(total["costMissing"])))
		}
	} else {
		lines = append(lines, "- "+label(language, "Cost", "비용")+": **"+cost+"**")
	}
	lines = append(lines, "", label(language, "| Phase | Default effort | Calls | Duration | Input | Output | Cost |", "| 단계 | 기본 추론 수준 | 호출 | 소요 | 입력 | 출력 | 비용 |"), "|---|---|---|---|---|---|---|")
	byPhase := obj(usage["byPhase"])
	phaseOrder := []string{"doctor", "audit", "deep_check", "characterization", "preflight", "execute", "review", "handoff"}
	seen := map[string]bool{}
	var phases []string
	for _, phase := range phaseOrder {
		if _, ok := byPhase[phase]; ok {
			phases = append(phases, phase)
			seen[phase] = true
		}
	}
	var extra []string
	for phase := range byPhase {
		if !seen[phase] {
			extra = append(extra, phase)
		}
	}
	sort.Strings(extra)
	phases = append(phases, extra...)
	efforts := map[string]string{"doctor": "low", "audit": "high", "deep_check": "high", "characterization": "medium", "preflight": "medium", "execute": "high", "review": "high", "handoff": "low"}
	for _, phase := range phases {
		u := obj(byPhase[phase])
		effort := fallback(efforts[phase], "-")
		if phase == "audit" && numeric(u["calls"]) > 1 {
			effort = "high→medium"
		}
		cost := costText(u)
		if cost == "" {
			cost = label(language, "Not reported", "미보고")
		} else if numeric(u["costMissing"]) > 0 {
			cost += "+"
		}
		lines = append(lines, fmt.Sprintf("| %s | %s | %d | %s | %s | %s | %s |", phase, effort, numeric(u["calls"]), formatMS(numeric(u["ms"])), formatTokens(numeric(u["inputTokens"])), formatTokens(numeric(u["outputTokens"])), cost))
	}
	return append(lines, "", label(language, "Default effort shows the phase policy, not observed provider effort. CLI effort overrides every phase; without it, existing configuration and operational overrides apply. A plus sign marks a partial cost.", "기본 추론 수준은 단계별 정책이며 실제 provider의 추론 수준을 관측한 값이 아닙니다. CLI effort는 모든 단계에 우선 적용됩니다. 미지정 시 기존 설정과 운영 단계의 별도 값이 적용됩니다. 비용의 +는 일부 비용만 집계했음을 뜻합니다."), "")
}

func renderProviderSettings(settings map[string]any, language string) []string {
	if len(settings) == 0 {
		return nil
	}
	lines := []string{"", label(language, "### Requested provider settings", "### 요청한 provider 설정"), "",
		label(language, "| Provider | Requested model | CLI effort |", "| Provider | 요청한 모델 | CLI effort |"), "|---|---|---|"}
	var names []string
	for name := range settings {
		names = append(names, name)
	}
	sort.Strings(names)
	escape := strings.NewReplacer("\\", "\\\\", "|", "\\|", "\r", " ", "\n", " ")
	for _, name := range names {
		value := obj(settings[name])
		model := fallback(val(value, "requestedModel"), label(language, "Not specified", "미지정"))
		effort := fallback(val(value, "cliEffort"), label(language, "Not overridden", "CLI override 없음"))
		lines = append(lines, fmt.Sprintf("| %s | %s | %s |", escape.Replace(name), escape.Replace(model), escape.Replace(effort)))
	}
	return append(lines, "", label(language, "These are requested settings; they do not verify which model the provider executed.", "요청한 설정을 기록한 정보이며 provider가 실제로 실행한 모델을 확인한 증거는 아닙니다."))
}

func costText(u map[string]any) string {
	if u["costUsd"] == nil {
		return ""
	}
	return fmt.Sprintf("$%.2f", decimal(u["costUsd"]))
}
func formatMS(ms int) string {
	sec := (ms + 500) / 1000
	if sec < 60 {
		return strconv.Itoa(sec) + "s"
	}
	return fmt.Sprintf("%dm%02ds", sec/60, sec%60)
}
func formatTokens(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	if n < 100000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.0fk", float64(n)/1000)
}
func commas(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func parseReport(data []byte) (map[string]any, error) {
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("report.json: %w", err)
	}
	if report["schemaVersion"] != float64(ReportSchemaVersion) {
		return nil, fmt.Errorf("report.json: unsupported schemaVersion %v; expected %d", report["schemaVersion"], ReportSchemaVersion)
	}
	return report, nil
}
