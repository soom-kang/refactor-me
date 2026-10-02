package controller

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
)

// Recovery fixtures use the public CLI, real disposable worktrees and fake
// provider executables. They never invoke an installed provider.
func runProgressRecoveryFixture(t *testing.T, f *progressRunFixture, args []string) (string, map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := surface.Execute(args, f.repo, &stdout, &stderr, surface.Callbacks{Run: Run, Doctor: Doctor})
	if code != surface.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, &stderr, &stdout)
	}
	var report, pointer map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not one report JSON: %v: %s", err, &stdout)
	}
	data, err := os.ReadFile(filepath.Join(f.repo, ".refactor", "last-run.json"))
	if err != nil || json.Unmarshal(data, &pointer) != nil {
		t.Fatal("missing valid run pointer", err)
	}
	runDir := str(pointer["runDir"])
	saved, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil || !bytes.Equal(saved, stdout.Bytes()) {
		t.Fatalf("JSON stdout differs from saved report: %v", err)
	}
	if strings.Contains(stderr.String(), "RAW_") || strings.Contains(stderr.String(), "/usr/bin/true") {
		t.Fatalf("terminal exposed provider/tool content: %s", &stderr)
	}
	if report["status"] != "NO_CHANGES" || report["branch"] != nil || obj(report["counters"])["commits"] != float64(0) || obj(report["counters"])["cycles"] != float64(1) {
		t.Fatalf("unexpected recovery result: %#v", report)
	}
	if gitTest(t, f.repo, "rev-parse", "HEAD") != f.base || gitTest(t, f.repo, "status", "--porcelain") != "" {
		t.Fatal("recovery changed the source checkout")
	}
	if got := gitTest(t, str(report["worktree"]), "status", "--porcelain"); got != "" {
		t.Fatalf("read-only recovery left worktree changes: %s", got)
	}
	return stderr.String(), report, runDir
}

func requireProgressRecoveryRecord(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("recovery record changed at %s: got=%q want=%q err=%v", path, data, want, err)
	}
}

func TestProgressRunProcessRetryPreservesRawRecords(t *testing.T) {
	for _, language := range []string{"", "ko"} {
		t.Run(map[bool]string{true: "ko", false: "default-en"}[language == "ko"], func(t *testing.T) {
			f := newProgressRunFixture(t)
			f.responses["audit"] = f.responses["audit-empty"]
			f.prepare(t)
			counter := filepath.Join(f.fixtures, "retry-count")
			failure := "RAW_RETRY_FAILURE_DETAIL_FIXTURE\n"
			script := `#!/bin/sh
if [ "$1" = '--version' ]; then printf '%s\n' 'fixture-provider 1.0'; exit 0; fi
cat >/dev/null
out=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = '--output-last-message' ]; then shift; out="$1"; fi
  shift
done
[ -n "$out" ] || exit 22
cat <<'PROVIDER_JSONL'
` + progressRawEvents + `PROVIDER_JSONL
if [ ! -f ` + progressShellQuote(counter) + ` ]; then
  printf '1\n' > ` + progressShellQuote(counter) + `
  printf '%s\n' 'RAW_RETRY_FAILURE_DETAIL_FIXTURE' >&2
  exit 1
fi
printf '2\n' > ` + progressShellQuote(counter) + `
cp ` + progressShellQuote(filepath.Join(f.fixtures, "audit.json")) + ` "$out"
`
			if err := os.WriteFile(f.bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			args := []string{"run", "--provider", "codex", "--fallback", "none", "--model", "fixture-model", "--no-live-probe", "--json"}
			if language != "" {
				args = append(args, "--lang", language)
			}
			output, report, runDir := runProgressRecoveryFixture(t, f, args)
			if language == "ko" {
				requireProgressOrder(t, output, "이 단계는 codex가 진행합니다.", "codex 호출에 실패했습니다. 5s 후 재시도합니다. 재시도 1/2회입니다. [PROCESS]", "이 단계는 codex가 진행합니다.", "후보 0개", "반복 작업을 종료")
			} else {
				requireProgressOrder(t, output, "codex is working on this stage.", "codex call failed; retrying in 5s (retry 1 of 2). [PROCESS]", "codex is working on this stage.", "found 0 candidates", "refactoring loop stopped")
			}
			requireProgressRecoveryRecord(t, counter, "2\n")
			provider := obj(obj(report["providers"])["codex"])
			usage := obj(provider["usage"])
			if provider["calls"] != float64(2) || usage["processes"] != float64(2) || usage["failedCalls"] != float64(1) || !strings.Contains(str(provider["lastError"]), strings.TrimSpace(failure)) {
				t.Fatalf("retry evidence/accounting lost: %#v", provider)
			}
			// The existing transcript schema reuses the primary path on retry.
			// Both attempts emit the same raw tool events; successful retry output
			// and the first attempt's stderr must remain available there.
			base := filepath.Join(runDir, "audits", "01", "provider", "audit-codex-primary")
			requireProgressRecoveryRecord(t, base+".stdout.jsonl", progressRawEvents)
			requireProgressRecoveryRecord(t, base+".stderr.log", failure)
		})
	}
}

func TestProgressRunQuotaFallbackPreservesRawRecords(t *testing.T) {
	for _, language := range []string{"", "ko"} {
		t.Run(map[bool]string{true: "ko", false: "default-en"}[language == "ko"], func(t *testing.T) {
			f := newProgressRunFixture(t)
			f.responses["audit"] = f.responses["audit-empty"]
			claude := filepath.Join(t.TempDir(), "fake-claude")
			f.config["agents"] = map[string]any{"primary": "claude", "fallback": "codex", "claude": map[string]any{"bin": claude}, "codex": map[string]any{"bin": f.bin}}
			f.prepare(t)
			quotaRaw := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"RAW_QUOTA_PATH_FIXTURE"}},{"type":"text","text":"RAW_QUOTA_PROSE_FIXTURE"}]}}
{"type":"result","is_error":true,"result":"usage limit reached RAW_QUOTA_DETAIL_FIXTURE"}
`
			claudeScript := "#!/bin/sh\nif [ \"$1\" = '--version' ]; then printf '%s\\n' 'fixture-provider 1.0'; exit 0; fi\ncat >/dev/null\ncat <<'PROVIDER_JSONL'\n" + quotaRaw + "PROVIDER_JSONL\n"
			if err := os.WriteFile(claude, []byte(claudeScript), 0700); err != nil {
				t.Fatal(err)
			}
			handoff := "RAW_HANDOFF_NOTES_FIXTURE\n"
			codexScript := `#!/bin/sh
if [ "$1" = '--version' ]; then printf '%s\n' 'fixture-provider 1.0'; exit 0; fi
cat >/dev/null
out=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = '--output-last-message' ]; then shift; out="$1"; fi
  shift
done
case "$out" in
  */handoff-last.json) printf '%s\n' 'RAW_HANDOFF_NOTES_FIXTURE' > "$out" ;;
  */audit-last.json) cp ` + progressShellQuote(filepath.Join(f.fixtures, "audit.json")) + ` "$out" ;;
  *) exit 23 ;;
esac
cat <<'PROVIDER_JSONL'
` + progressRawEvents + `PROVIDER_JSONL
`
			if err := os.WriteFile(f.bin, []byte(codexScript), 0700); err != nil {
				t.Fatal(err)
			}
			args := []string{"run", "--provider", "claude", "--fallback", "codex", "--model", "fixture-model", "--fallback-model", "fixture-fallback-model", "--no-live-probe", "--json"}
			if language != "" {
				args = append(args, "--lang", language)
			}
			output, report, runDir := runProgressRecoveryFixture(t, f, args)
			if language == "ko" {
				requireProgressOrder(t, output, "이 단계는 claude가 진행합니다.", "claude가 이번 호출을 진행할 수 없습니다. [QUOTA]", "codex에 전달할 작업 기록을 준비", "조사 1회차", "provider를 claude에서 codex로 전환", "이 단계는 codex가 진행합니다.", "후보 0개", "반복 작업을 종료")
			} else {
				requireProgressOrder(t, output, "claude is working on this stage.", "claude is unavailable for this call. [QUOTA]", "Preparing context for codex after claude became unavailable.", "audit 1", "Switching provider from claude to codex.", "codex is working on this stage.", "found 0 candidates", "refactoring loop stopped")
			}
			providers := obj(report["providers"])
			leading, fallback := obj(providers["claude"]), obj(providers["codex"])
			if leading["status"] != "COOLDOWN" || leading["quotaHits"] != float64(1) || leading["calls"] != float64(1) || fallback["calls"] != float64(1) || obj(fallback["usage"])["processes"] != float64(2) {
				t.Fatalf("fallback evidence/accounting lost: %#v", providers)
			}
			if !strings.Contains(str(leading["lastError"]), "RAW_QUOTA_DETAIL_FIXTURE") {
				t.Fatalf("original quota detail was not saved: %#v", leading)
			}
			phaseUsage := obj(obj(report["usage"])["byPhase"])
			if obj(phaseUsage["audit"])["processes"] != float64(2) || obj(phaseUsage["handoff"])["processes"] != float64(1) {
				t.Fatalf("handoff or audit usage was lost: %#v", phaseUsage)
			}
			providerDir := filepath.Join(runDir, "audits", "01", "provider")
			requireProgressRecoveryRecord(t, filepath.Join(providerDir, "audit-claude-primary.stdout.jsonl"), quotaRaw)
			requireProgressRecoveryRecord(t, filepath.Join(providerDir, "audit-codex-primary.stdout.jsonl"), progressRawEvents)
			requireProgressRecoveryRecord(t, filepath.Join(runDir, "provider", "handoff-codex-primary.stdout.jsonl"), progressRawEvents)
			requireProgressRecoveryRecord(t, filepath.Join(runDir, "provider", "handoff-last.json"), handoff)
			data, err := os.ReadFile(filepath.Join(runDir, "handoff.md"))
			if err != nil || !strings.Contains(string(data), handoff) {
				t.Fatalf("original handoff notes were not saved: %q: %v", data, err)
			}
		})
	}
}
