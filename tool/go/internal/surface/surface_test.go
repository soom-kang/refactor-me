package surface

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return repo
}

func TestParseAndVersionOutsideRepo(t *testing.T) {
	for _, argv := range [][]string{{"version", "--json"}, {"--version", "--json"}, {"run", "--version", "--json"}} {
		var out, errors bytes.Buffer
		if code := Execute(argv, t.TempDir(), &out, &errors, Callbacks{}); code != ExitOK {
			t.Fatalf("%v: exit %d: %s", argv, code, errors.String())
		}
		var version map[string]any
		if err := json.Unmarshal(out.Bytes(), &version); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"name", "version", "platform", "source", "go", "arch", "commit"} {
			if version[key] == nil {
				t.Errorf("missing %s", key)
			}
		}
		if _, hasNode := version["node"]; hasNode {
			t.Fatal("Node runtime field remains")
		}
	}
	for _, argv := range [][]string{{"--lang"}, {"--lang", "fr"}, {"doctor", "--lang", "ko"}, {"version", "--lang", "en"}} {
		var out, errors bytes.Buffer
		if code := Execute(argv, t.TempDir(), &out, &errors, Callbacks{}); code != ExitAborted {
			t.Fatalf("%v: exit %d", argv, code)
		}
		if out.Len() != 0 {
			t.Fatalf("unexpected output: %s", out.String())
		}
	}
}

func TestLoadConfigMergesSettings(t *testing.T) {
	repo := testRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".refactor"), 0755); err != nil {
		t.Fatal(err)
	}
	data := `{"schema_version":2,"agents":{"claude":{"effort":"low"},"codex":{"effort_by_phase":{"audit":"low"}}},"policy":{"max_commits":3},"future":{"keep":true}}`
	if err := os.WriteFile(filepath.Join(repo, ".refactor", "config.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if ConfigString(cfg, "agents", "claude", "effort") != "low" {
		t.Fatal("lost effort override")
	}
	if ConfigString(cfg, "agents", "codex", "bin") != "codex" {
		t.Fatal("lost default bin")
	}
	if obj(cfg["policy"])["unknown_risk"] != "set_aside" {
		t.Fatal("lost conservative policy")
	}
	if obj(cfg["policy"])["max_commits"] != float64(3) {
		t.Fatal("lost user policy")
	}
	if obj(cfg["future"])["keep"] != true {
		t.Fatal("lost unknown key")
	}
}

func TestReportJSONRawAndMarkdown(t *testing.T) {
	repo := testRepo(t)
	runDir := filepath.Join(repo, ".refactor", "runs", "old")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	report := `{"schemaVersion":3,"runId":"old","status":"NO_CHANGES","reason":"no eligible candidates remain","durationMinutes":4,"repoRoot":"/old","baseCommit":"abcdef0123","baseBranch":"main","branch":null,"commits":[],"skipped":[],"providers":{},"validation":{"describe":"GREEN 1","ran":["go test ./..."],"notRun":[]}}`
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".refactor", "last-run.json"), []byte(`{"schemaVersion":1,"runDir":"`+runDir+`"}`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "ko"} {
		var out, errors bytes.Buffer
		if code := Execute([]string{"report", "--lang", lang}, repo, &out, &errors, Callbacks{}); code != ExitOK {
			t.Fatalf("%s: %s", lang, errors.String())
		}
		want := "## Committed changes"
		if lang == "ko" {
			want = "## 커밋된 변경"
		}
		if !strings.Contains(out.String(), want) || strings.Contains(out.String(), "<nil>") {
			t.Fatalf("%s: %s", lang, out.String())
		}
		out.Reset()
		errors.Reset()
		if code := Execute([]string{"report", "--json", "--lang", lang}, repo, &out, &errors, Callbacks{}); code != ExitOK || out.String() != report {
			t.Fatalf("raw report drift: %d %q", code, out.String())
		}
	}
	if err := writeRunFailure(repo, errors.New("doctor failed")); err != nil {
		t.Fatal(err)
	}
	var failedOut, failedErr bytes.Buffer
	if code := executeReport(repo, Args{JSON: true}, &failedOut, &failedErr); code != ExitAborted || failedOut.Len() != 0 || !strings.Contains(failedErr.String(), "doctor failed") {
		t.Fatalf("stale report returned after failure: %d %s %s", code, &failedOut, &failedErr)
	}
	if err := writeLastRun(repo, RunResult{RunID: "old", RunDir: runDir, Status: "NO_CHANGES"}); err != nil {
		t.Fatal(err)
	}
	failedErr.Reset()
	if code := executeReport(repo, Args{JSON: true}, &failedOut, &failedErr); code != ExitOK || failedOut.String() != report {
		t.Fatalf("successful run did not clear failure: %d %s", code, &failedErr)
	}
	readBack, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil || string(readBack) != report {
		t.Fatal("report changed")
	}
}

func TestReportRejectsCorruptPointerWithoutRewritingIt(t *testing.T) {
	repo := testRepo(t)
	dir := filepath.Join(repo, ".refactor")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "last-run.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeRunFailure(repo, errors.New("preparation failed")); err == nil {
		t.Fatal("failed attempt overwrote a corrupt previous pointer")
	}
	var out, stderr bytes.Buffer
	if code := Execute([]string{"report", "--json"}, repo, &out, &stderr, Callbacks{}); code != ExitAborted {
		t.Fatalf("corrupt report exit=%d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("corrupt report produced stdout: %s", out.String())
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "{broken" {
		t.Fatal("corrupt record was changed")
	}
}

func TestRichReportRendersSavedEvidenceInBothLanguages(t *testing.T) {
	report := map[string]any{
		"schemaVersion": 3, "runId": "rich", "status": "DONE_PARTIAL", "reason": "stopped on cycle budget (3)", "durationMinutes": 9, "providerMinutes": 4,
		"toolVersion": "dev", "repoRoot": "/repo", "targets": []string{"app"}, "baseCommit": "1234567890abcdef", "baseBranch": "main", "branch": "refactor/auto-test", "worktree": "/worktree",
		"counters":       map[string]any{"commits": 1, "auditsNoProposals": 1, "auditsAllFiltered": 2},
		"commits":        []any{map[string]any{"oid": "abcdef0123456789", "category": "REFACTOR", "subject": "extract helper", "paths": []string{"app/a.go"}}},
		"skipped":        []any{map[string]any{"reason": "RISK_UNKNOWN", "detail": "no signal"}},
		"providers":      map[string]any{"codex": map[string]any{"status": "READY", "calls": 2, "quotaHits": 1}},
		"validation":     map[string]any{"describe": "GREEN 1", "ran": []string{"go test ./..."}, "notRun": []string{"slow integration"}},
		"codeComparison": map[string]any{"status": "CHANGED", "baseCommit": "1234567890abcdef", "resultCommit": "abcdef0123456789", "totals": map[string]any{"files": 1, "insertions": 3, "deletions": 1, "binary": 0}, "files": []any{map[string]any{"path": "app/a.go", "status": "M", "insertions": 3, "deletions": 1, "oldMode": "100644", "newMode": "100644"}}, "preview": "+new\n-old\n"},
		"usage":          map[string]any{"totals": map[string]any{"processes": 2, "calls": 2, "failedCalls": 0, "inputTokens": 1200, "outputTokens": 300, "costUsd": 0.42}, "byPhase": map[string]any{"audit": map[string]any{"calls": 2, "ms": 4500, "inputTokens": 1200, "outputTokens": 300, "costUsd": 0.42}}},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		lang string
		want []string
	}{
		{"en", []string{"## Committed changes", "## Code comparison", "## Skipped candidates", "## Validation", "## Providers", "## Cost and time", "## Next steps", "cycle budget", "RISK_UNKNOWN", "$0.42"}},
		{"ko", []string{"## 커밋된 변경", "## 코드 비교", "## 제외한 후보", "## 검증", "## 프로바이더", "## 비용과 시간", "## 다음 단계", "사이클 한도", "위험도 판정 근거 부족", "$0.42"}},
	} {
		text, err := RenderReport(data, tc.lang)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range tc.want {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", tc.lang, want)
			}
		}
		if strings.Contains(text, "<nil>") {
			t.Errorf("%s leaked nil", tc.lang)
		}
	}
}
