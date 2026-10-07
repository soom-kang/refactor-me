package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

// These fixtures exercise the public CLI and actual disposable Git worktrees.
// The fake providers emit recognizable tool/prose markers without running tools.
type progressRunFixture struct {
	repo, base, bin, fixtures string
	responses                 map[string]map[string]any
	config                    map[string]any
	commands                  []workspace.Command
}

func newProgressRunFixture(t *testing.T) *progressRunFixture {
	t.Helper()
	globalSkillsFixture(t)
	f := &progressRunFixture{repo: filepath.Join(t.TempDir(), "project"), fixtures: t.TempDir(), responses: map[string]map[string]any{}}
	if err := os.MkdirAll(filepath.Join(f.repo, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, f.repo, "init", "-q")
	gitTest(t, f.repo, "config", "user.name", "Test")
	gitTest(t, f.repo, "config", "user.email", "test@example.invalid")
	for name, body := range map[string]string{
		"src/legacy-parser.mjs": "export function parseLegacy() { return 1; }\n",
		"src/index.mjs":         "export const value = 1;\n",
	} {
		if err := os.WriteFile(filepath.Join(f.repo, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	gitTest(t, f.repo, "add", ".")
	gitTest(t, f.repo, "commit", "-qm", "fixture")
	f.base = gitTest(t, f.repo, "rev-parse", "HEAD")
	workflow, err := os.ReadFile(filepath.Join("..", "..", "..", "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	for phase, example := range map[string]string{"audit": "audit-success", "deep_check": "deepcheck-success", "preflight": "preflight-success", "execute": "execute-success", "review": "review-success"} {
		match := regexp.MustCompile("(?s)<!-- example: " + example + " schema: \\w+ -->\\n```json\\n(.*?)\\n```").FindSubmatch(workflow)
		if len(match) != 2 {
			t.Fatalf("missing workflow fixture %s", example)
		}
		var response map[string]any
		if err := json.Unmarshal(match[1], &response); err != nil {
			t.Fatal(err)
		}
		response["notes"] = "RAW_MODEL_NOTES_FIXTURE"
		f.responses[phase] = response
	}
	f.responses["audit-empty"] = map[string]any{"schema_version": "1", "scan_scope": "entire repository", "rejected_count": 0, "notes": nil, "candidates": []any{}}
	f.bin = filepath.Join(t.TempDir(), "fake-codex")
	f.config = map[string]any{
		"schema_version": 2,
		"agents":         map[string]any{"primary": "codex", "fallback": "none", "codex": map[string]any{"bin": f.bin}},
		"policy":         map[string]any{"empty_audits_to_stop": 1},
		"workspace":      map[string]any{"worktree_parent": t.TempDir(), "keep_worktree": true},
	}
	f.commands = []workspace.Command{{ID: ".:T1:ok", Area: ".", Tier: "T1", Family: "custom", Name: "test", Argv: []string{"/usr/bin/true"}, Cwd: ".", TimeoutMS: 10000, Source: "operator-defined"}}
	return f
}

func progressFixtureJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func progressShellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "'\"'\"'") + "'"
}

const progressRawEvents = `{"type":"item.completed","item":{"type":"command_execution","command":"RAW_COMMAND_FIXTURE","aggregated_output":"RAW_TOOL_OUTPUT_FIXTURE"}}
{"type":"item.completed","item":{"type":"agent_message","text":"RAW_MODEL_PROSE_FIXTURE"}}
`

func (f *progressRunFixture) prepare(t *testing.T) {
	t.Helper()
	for phase, response := range f.responses {
		progressFixtureJSON(t, filepath.Join(f.fixtures, phase+".json"), response)
	}
	progressFixtureJSON(t, filepath.Join(f.repo, ".refactor", "config.json"), f.config)
	progressFixtureJSON(t, filepath.Join(f.repo, ".refactor", "commands.json"), map[string]any{"locked": true, "commands": f.commands})
	script := `#!/bin/sh
if [ "$1" = '--version' ]; then printf '%s\n' 'fixture-provider 1.0'; exit 0; fi
cat >/dev/null
out=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = '--output-last-message' ]; then shift; out="$1"; fi
  shift
done
[ -n "$out" ] || exit 22
case "$out" in
  */audits/01/*) phase=audit ;;
  */audits/02/*) phase=audit-empty; if [ -f ` + progressShellQuote(filepath.Join(f.fixtures, "audit-second.json")) + ` ]; then phase=audit-second; fi ;;
  */audit-last.json) phase=audit-empty ;;
  */deep_check-last.json) phase=deep_check ;;
  */preflight-last.json) phase=preflight ;;
  */execute-last.json) phase=execute ;;
  */review-last.json) phase=review ;;
  *) exit 23 ;;
esac
case "$out" in
  */cycles/02-*) if [ -f ` + progressShellQuote(f.fixtures) + `/"$phase-second.json" ]; then phase="$phase-second"; fi ;;
esac
cp ` + progressShellQuote(f.fixtures) + `/"$phase.json" "$out"
cat <<'PROVIDER_JSONL'
` + progressRawEvents + `PROVIDER_JSONL
`
	if err := os.WriteFile(f.bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
}

func (f *progressRunFixture) run(t *testing.T, language string) (string, map[string]any, string) {
	t.Helper()
	args := []string{"run", "--provider", "codex", "--fallback", "none", "--model", "fixture-model", "--no-live-probe", "--json"}
	if language != "" {
		args = append(args, "--lang", language)
	}
	var stdout, stderr bytes.Buffer
	code := surface.Execute(args, f.repo, &stdout, &stderr, surface.Callbacks{Run: Run, Doctor: Doctor, Clean: Clean})
	if code != surface.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, &stderr, &stdout)
	}
	var report, pointer map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not one report JSON: %v: %s", err, &stdout)
	}
	pointerData, err := os.ReadFile(filepath.Join(f.repo, ".refactor", "last-run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(pointerData, &pointer); err != nil {
		t.Fatal(err)
	}
	runDir := str(pointer["runDir"])
	saved, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil || !bytes.Equal(saved, stdout.Bytes()) {
		t.Fatalf("JSON stdout differs from saved report: %v", err)
	}
	for _, marker := range []string{"RAW_COMMAND_FIXTURE", "RAW_TOOL_OUTPUT_FIXTURE", "RAW_MODEL_PROSE_FIXTURE", "RAW_MODEL_NOTES_FIXTURE", "RAW_FAILURE_DETAIL_FIXTURE", "/usr/bin/true"} {
		if strings.Contains(stderr.String(), marker) {
			t.Fatalf("terminal leaked provider/tool prose %q: %s", marker, &stderr)
		}
	}
	transcript := filepath.Join(runDir, "audits", "01", "provider", "audit-codex-primary.stdout.jsonl")
	raw, err := os.ReadFile(transcript)
	if err != nil || string(raw) != progressRawEvents {
		t.Fatalf("original transcript was not retained: %q: %v", raw, err)
	}
	if got := gitTest(t, f.repo, "rev-parse", "HEAD"); got != f.base {
		t.Fatalf("source HEAD changed: %s", got)
	}
	if got := gitTest(t, f.repo, "status", "--porcelain"); got != "" {
		t.Fatalf("source checkout changed: %s", got)
	}
	return stderr.String(), report, runDir
}

func requireProgressOrder(t *testing.T, output string, fragments ...string) {
	t.Helper()
	position := 0
	for _, fragment := range fragments {
		next := strings.Index(output[position:], fragment)
		if next < 0 {
			t.Fatalf("missing or out-of-order progress %q after byte %d:\n%s", fragment, position, output)
		}
		position += next + len(fragment)
	}
}

func TestProgressRunLanguagesAuditCountsAndConfirmedSuccess(t *testing.T) {
	for _, language := range []string{"", "ko"} {
		t.Run(map[bool]string{true: "ko", false: "default-en"}[language == "ko"], func(t *testing.T) {
			f := newProgressRunFixture(t)
			candidates := f.responses["audit"]["candidates"].([]any)
			clone := func(id, symbol, risk string) map[string]any {
				data, _ := json.Marshal(candidates[0])
				var candidate map[string]any
				if err := json.Unmarshal(data, &candidate); err != nil {
					t.Fatal(err)
				}
				candidate["candidate_id"], candidate["primary_symbol"], candidate["risk_level"] = id, symbol, risk
				return candidate
			}
			f.responses["audit"]["candidates"] = []any{candidates[0], clone("policy-rejected", "policyRejected", "L3_CRITICAL"), clone("outside-cap", "unreviewedOutsideCap", "L0_LOW")}
			obj(f.config["policy"])["max_audit_candidates"] = 2
			f.prepare(t)
			output, report, runDir := f.run(t, language)
			if report["status"] != "DONE" || obj(report["counters"])["commits"] != float64(1) {
				t.Fatalf("unexpected result: %#v", report)
			}
			if language == "ko" {
				requireProgressOrder(t, output, "필수 Skill을 확인", "worktree를 준비", "변경 전 검증", "후보 3개", "1개를 진행", "후보 2개를 검토", "parseLegacy (미사용 코드 제거)", "구현 준비 상태", "항목을 수정", "변경 범위와 검증", "test 검사가 통과", "독립 검토", "로컬 결과 branch에 저장", "후보 0개", "반복 작업을 종료")
			} else {
				requireProgressOrder(t, output, "required Skills", "isolated worktree", "validation before changes", "found 3 candidates; 1 are eligible", "Reviewed 2 candidates", "Checking parseLegacy (unused code)", "ready to implement", "Implementing parseLegacy", "scope and validation", "test check passed", "Independently reviewing", "on the local result branch. Commit:", "found 0 candidates", "refactoring loop stopped")
			}
			if strings.Contains(output, "unreviewedOutsideCap") || !strings.Contains(output, "[RISK_EXCLUDED]") {
				t.Fatalf("candidate cap/policy messages are inaccurate:\n%s", output)
			}
			branch := str(report["branch"])
			commits := mapsOf(report["commits"])
			if branch == "" || len(commits) != 1 || gitTest(t, f.repo, "rev-parse", "refs/heads/"+branch) != str(commits[0]["oid"]) {
				t.Fatalf("completion did not correspond to saved local branch: %s %#v", branch, commits)
			}
			state, err := os.ReadFile(filepath.Join(runDir, "state.json"))
			if err != nil || !bytes.Contains(state, []byte(str(commits[0]["oid"]))) {
				t.Fatalf("completion did not correspond to saved state: %v", err)
			}
		})
	}
}

func TestProgressRunSkippedOutcomesHaveDiagnosticsAndNoPublishedCommit(t *testing.T) {
	for _, tc := range []struct{ name, code string }{
		{"preflight", "PREFLIGHT_BLOCKED"},
		{"risk", "RISK_UNKNOWN"},
		{"declined", "EXECUTE_REJECTED"},
		{"no-op", "NO_OP"},
		{"regression", "REGRESSION"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProgressRunFixture(t)
			switch tc.name {
			case "preflight":
				f.responses["preflight"]["verdict"] = "BLOCKED"
				f.responses["preflight"]["falsification_result"] = "INCONCLUSIVE"
				f.responses["preflight"]["notes"] = "RAW_FAILURE_DETAIL_FIXTURE"
			case "risk":
				f.responses["deep_check"]["risk_level"] = "UNKNOWN"
			case "declined":
				f.responses["execute"]["verdict"] = "FAIL"
				f.responses["execute"]["rationale"] = "RAW_FAILURE_DETAIL_FIXTURE"
				f.responses["execute"]["notes"] = "RAW_MODEL_NOTES_FIXTURE"
			case "no-op":
				f.responses["execute"]["deleted_files"] = []any{}
			case "regression":
				f.commands[0].Argv = []string{"/bin/sh", "-c", "if [ ! -f src/legacy-parser.mjs ]; then echo src/index.mjs:1; exit 1; fi"}
			}
			f.prepare(t)
			output, report, runDir := f.run(t, "ko")
			if tc.name == "declined" && obj(mapsOf(report["skipped"])[0]["detail"])["detail"] != "RAW_FAILURE_DETAIL_FIXTURE" {
				t.Fatal("execution rejection lost the implementer's rationale")
			}
			if report["status"] != "NO_CHANGES" || report["branch"] != nil || obj(report["counters"])["commits"] != float64(0) {
				t.Fatalf("unexpected skipped result: %#v", report)
			}
			if strings.Contains(output, "로컬 결과 branch에 저장했습니다") || !strings.Contains(output, "["+tc.code+"]") || !strings.Contains(output, "parseLegacy") {
				t.Fatalf("skip outcome missing or premature success:\n%s", output)
			}
			statePath := filepath.Join(runDir, "state.json")
			stateInfo, err := os.Stat(statePath)
			if err != nil {
				t.Fatal(err)
			}
			diagnosticFound := false
			for _, line := range strings.Split(output, "\n") {
				line = progressBody(line)
				var printedPath string
				if strings.HasPrefix(line, "상세 기록: ") {
					printedPath = strings.TrimPrefix(line, "상세 기록: ")
				} else if strings.HasPrefix(line, "상세 기록 (선택한 저장소 기준 상대 경로): ") {
					printedPath = filepath.Join(f.repo, strings.TrimPrefix(line, "상세 기록 (선택한 저장소 기준 상대 경로): "))
				}
				if info, err := os.Stat(printedPath); err == nil && info.Mode().IsRegular() && os.SameFile(info, stateInfo) {
					diagnosticFound = true
				}
			}
			if !diagnosticFound {
				t.Fatalf("missing actual diagnostic path:\n%s", output)
			}
			stateData, err := os.ReadFile(statePath)
			if err != nil || !bytes.Contains(stateData, []byte(tc.code)) {
				t.Fatalf("diagnostic lacks recorded decision: %v", err)
			}
			if branch := gitTest(t, f.repo, "for-each-ref", "--format=%(refname)", "refs/heads/refactor/"); branch != "" {
				t.Fatalf("skipped outcome published a result branch: %s", branch)
			}
			wt := str(report["worktree"])
			if got := gitTest(t, wt, "rev-parse", "HEAD"); got != f.base {
				t.Fatalf("rejected change moved worktree HEAD: %s", got)
			}
			if got := gitTest(t, wt, "status", "--porcelain"); got != "" {
				t.Fatalf("rejected change was not rolled back: %s", got)
			}
			if _, err := os.Stat(filepath.Join(wt, "src", "legacy-parser.mjs")); err != nil {
				t.Fatalf("rejected deletion was not restored: %v", err)
			}
			if tc.name == "regression" {
				requireProgressOrder(t, output, "test 검사가 통과", "검사에서 검증에 실패", "변경을 되돌렸습니다", "[REGRESSION]", "상세 기록")
			}
		})
	}
}

func TestProgressRunExistingBaselineFailureIsNotReportedAsPassed(t *testing.T) {
	f := newProgressRunFixture(t)
	f.commands = append(f.commands, workspace.Command{ID: ".:T1:known", Area: ".", Tier: "T1", Family: "custom", Name: "known-failure", Argv: []string{"/bin/sh", "-c", "echo src/index.mjs:1; exit 1"}, Cwd: ".", TimeoutMS: 10000, Source: "operator-defined"})
	f.prepare(t)
	output, report, _ := f.run(t, "")
	if report["status"] != "DONE" || strings.Contains(output, "known-failure check passed") {
		t.Fatalf("existing failure misrepresented:\n%s", output)
	}
	requireProgressOrder(t, output, "known-failure fails before changes", "known-failure still fails as at baseline; no new errors", "on the local result branch. Commit:")
}

func TestProgressRunReauditUsesCurrentCandidateLabel(t *testing.T) {
	f := newProgressRunFixture(t)
	if err := os.WriteFile(filepath.Join(f.repo, "src", "other-parser.mjs"), []byte("export function parseOther() { return 2; }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, f.repo, "add", "src/other-parser.mjs")
	gitTest(t, f.repo, "commit", "-qm", "second candidate")
	f.base = gitTest(t, f.repo, "rev-parse", "HEAD")
	for _, phase := range []string{"audit", "deep_check", "preflight", "execute", "review"} {
		data, err := json.Marshal(f.responses[phase])
		if err != nil {
			t.Fatal(err)
		}
		text := strings.NewReplacer("parseLegacy", "parseOther", "dead-legacy-parser", "dead-other-parser", "legacy-parser.mjs", "other-parser.mjs").Replace(string(data))
		var second map[string]any
		if err := json.Unmarshal([]byte(text), &second); err != nil {
			t.Fatal(err)
		}
		f.responses[phase+"-second"] = second
	}
	f.prepare(t)
	output, report, _ := f.run(t, "")
	if report["status"] != "DONE" || obj(report["counters"])["commits"] != float64(2) {
		t.Fatalf("did not process both actual candidates: %#v", report)
	}
	requireProgressOrder(t, output, "Checking parseLegacy (unused code)", "Saved parseLegacy (unused code)", "audit 2", "Checking parseOther (unused code)", "Implementing parseOther (unused code)", "Saved parseOther (unused code)", "audit 3", "found 0 candidates")
	auditTwo := strings.Index(output, "audit 2")
	if strings.Contains(output[auditTwo:], "parseLegacy") {
		t.Fatalf("re-audit reused a previous packet's label:\n%s", output)
	}
}

func TestProgressRunSchemaRepairReportsOnlyControlledMetadata(t *testing.T) {
	f := newProgressRunFixture(t)
	f.prepare(t)
	script, err := os.ReadFile(f.bin)
	if err != nil {
		t.Fatal(err)
	}
	hook := `if [ "$phase" = 'audit' ] && [ ! -f "$0.repaired" ]; then
  printf '%s\n' '{}' > "$out"
  touch "$0.repaired"
fi
`
	script = []byte(strings.Replace(string(script), "cat <<'PROVIDER_JSONL'", hook+"cat <<'PROVIDER_JSONL'", 1))
	if err := os.WriteFile(f.bin, script, 0700); err != nil {
		t.Fatal(err)
	}
	output, report, runDir := f.run(t, "")
	if report["status"] != "DONE" || strings.Count(output, "trying one read-only repair") != 1 {
		t.Fatalf("repair progress did not match the controlled one-time repair:\n%s", output)
	}
	auditUsage := obj(obj(obj(report["usage"])["byPhase"])["audit"])
	if auditUsage["processes"] != float64(3) || auditUsage["calls"] != float64(2) {
		t.Fatalf("repair lost execution accounting: %#v", auditUsage)
	}
	for _, attempt := range []string{"primary", "repair"} {
		path := filepath.Join(runDir, "audits", "01", "provider", "audit-codex-"+attempt+".stdout.jsonl")
		data, err := os.ReadFile(path)
		if err != nil || string(data) != progressRawEvents {
			t.Fatalf("missing original %s transcript: %v", attempt, err)
		}
	}
}

func TestProgressRunClaudeReadOnlyDenialSuppressesRawProse(t *testing.T) {
	f := newProgressRunFixture(t)
	f.prepare(t)
	answer, err := json.Marshal(f.responses["audit-empty"])
	if err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"RAW_PATH_FIXTURE"}},{"type":"text","text":"RAW_MODEL_PROSE_FIXTURE"}]}}
{"type":"result","is_error":false,"result":"RAW_RESULT_FIXTURE","structured_output":%s,"permission_denials":[{"detail":"RAW_DENIAL_FIXTURE"}]}
`, answer)
	script := "#!/bin/sh\nif [ \"$1\" = '--version' ]; then printf '%s\\n' 'fixture-provider 1.0'; exit 0; fi\ncat >/dev/null\ncat <<'PROVIDER_JSONL'\n" + raw + "PROVIDER_JSONL\n"
	if err := os.WriteFile(f.bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	f.config["agents"] = map[string]any{"primary": "claude", "fallback": "none", "claude": map[string]any{"bin": f.bin}}
	progressFixtureJSON(t, filepath.Join(f.repo, ".refactor", "config.json"), f.config)
	var stdout, stderr bytes.Buffer
	code := surface.Execute([]string{"run", "--provider", "claude", "--fallback", "none", "--model", "fixture-model", "--no-live-probe", "--json", "--lang", "ko"}, f.repo, &stdout, &stderr, surface.Callbacks{Run: Run, Doctor: Doctor})
	if code != surface.ExitOK || !bytes.Contains(stdout.Bytes(), []byte(`"status": "NO_CHANGES"`)) {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	if !strings.Contains(stderr.String(), "쓰기 시도 1회를 차단") || strings.Contains(stderr.String(), "RAW_") {
		t.Fatalf("denial/raw prose output contract failed:\n%s", &stderr)
	}
	var pointer map[string]any
	data, err := os.ReadFile(filepath.Join(f.repo, ".refactor", "last-run.json"))
	if err != nil || json.Unmarshal(data, &pointer) != nil {
		t.Fatal("missing run pointer", err)
	}
	runDir := str(pointer["runDir"])
	data, err = os.ReadFile(filepath.Join(runDir, "audits", "01", "provider", "audit-claude-primary.stdout.jsonl"))
	if err != nil || string(data) != raw {
		t.Fatalf("raw Claude transcript was not preserved: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil || !bytes.Equal(data, stdout.Bytes()) {
		t.Fatalf("Claude stdout differed from saved report: %v", err)
	}
	if gitTest(t, f.repo, "rev-parse", "HEAD") != f.base || gitTest(t, f.repo, "status", "--porcelain") != "" {
		t.Fatal("read-only audit changed source checkout")
	}
}
