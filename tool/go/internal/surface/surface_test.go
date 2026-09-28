package surface

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
		for _, key := range []string{"name", "version", "platform", "source", "go", "arch"} {
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

func TestLoadConfigMergesOldSettings(t *testing.T) {
	repo := testRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".refactor"), 0755); err != nil {
		t.Fatal(err)
	}
	data := `{"agents":{"claude":{"effort":"low"},"codex":{"effort_by_phase":{"audit":"low"}}},"policy":{"max_commits":3},"future":{"keep":true}}`
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

func TestReportJSONRawAndHistoricalMarkdown(t *testing.T) {
	repo := testRepo(t)
	runDir := filepath.Join(repo, ".refactor", "runs", "old")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	report := `{"runId":"old","status":"NO_CHANGES","reason":"no eligible candidates remain","durationMinutes":4,"repoRoot":"/old","baseCommit":"abcdef0123","baseBranch":"main","branch":null,"commits":[],"skipped":[],"providers":{},"validation":{"describe":"GREEN 1","ran":["node --test"],"notRun":[]}}`
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".refactor", "last-run.json"), []byte(`{"runDir":"`+runDir+`"}`), 0644); err != nil {
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
	var out, stderr bytes.Buffer
	if code := Execute([]string{"report", "--json"}, repo, &out, &stderr, Callbacks{}); code != ExitAborted {
		t.Fatalf("corrupt report exit=%d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("corrupt report produced stdout: %s", out.String())
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "{broken" {
		t.Fatal("corrupt historical record was changed")
	}
}

func TestInstallPreservesDataAndRejectsUnknownFiles(t *testing.T) {
	repo := testRepo(t)
	binDir := filepath.Join(repo, ".refactor", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(binDir, "refactor-me")
	if err := os.WriteFile(unknown, []byte("user command"), 0755); err != nil {
		t.Fatal(err)
	}
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, source); err == nil || !strings.Contains(err.Error(), "unrecognized") {
		t.Fatalf("unexpected error: %v", err)
	}
	if data, _ := os.ReadFile(unknown); string(data) != "user command" {
		t.Fatal("unknown command changed")
	}
	nodeShim := "#!/bin/sh\n# refactor-me dev — installed 2026-09-28 from /old/tool\nexec node \"$(dirname \"$0\")/../lib/bin/refactor-me.mjs\" \"$@\"\n"
	if err := os.WriteFile(unknown, []byte(nodeShim), 0755); err != nil {
		t.Fatal(err)
	}
	legacyLib := filepath.Join(repo, ".refactor", "lib")
	if err := os.MkdirAll(filepath.Join(legacyLib, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	modifiedProvider := filepath.Join(legacyLib, "src", "provider.mjs")
	if err := os.WriteFile(modifiedProvider, []byte("user modified provider"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyLib, "src", "custom.mjs"), []byte("user file"), 0644); err != nil {
		t.Fatal(err)
	}
	knownRuntime := filepath.Join(legacyLib, "bin", "refactor-me.mjs")
	if err := os.MkdirAll(filepath.Dir(knownRuntime), 0755); err != nil {
		t.Fatal(err)
	}
	knownBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "bin", "refactor-me.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(knownBytes)); got != knownNodeFiles["bin/refactor-me.mjs"] {
		t.Fatal("test fixture no longer matches the released Node file")
	}
	if err := os.WriteFile(knownRuntime, knownBytes, 0644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(repo, ".refactor", "config.json")
	if err := os.WriteFile(configPath, []byte(`{"custom":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	runFile := filepath.Join(repo, ".refactor", "runs", "old", "report.json")
	if err := os.MkdirAll(filepath.Dir(runFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runFile, []byte("evidence"), 0644); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(binDir, "custom-command")
	if err := os.WriteFile(custom, []byte("user"), 0644); err != nil {
		t.Fatal(err)
	}
	retained, err := InstallWithReport(repo, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(knownRuntime); !os.IsNotExist(err) {
		t.Fatal("verified Node runtime file remains")
	}
	if data, err := os.ReadFile(modifiedProvider); err != nil || string(data) != "user modified provider" {
		t.Fatal("modified Node runtime file changed")
	}
	canonicalRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(retained, filepath.Join(canonicalRepo, ".refactor", "lib", "src", "provider.mjs")) ||
		!slices.Contains(retained, filepath.Join(canonicalRepo, ".refactor", "lib", "src", "custom.mjs")) {
		t.Fatalf("retained paths not reported: %v", retained)
	}
	if data, _ := os.ReadFile(filepath.Join(legacyLib, "src", "custom.mjs")); string(data) != "user file" {
		t.Fatal("unknown library child changed")
	}
	if err := Install(repo, source); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	for path, want := range map[string]string{configPath: `{"custom":true}`, runFile: "evidence", custom: "user"} {
		data, e := os.ReadFile(path)
		if e != nil || string(data) != want {
			t.Fatalf("%s changed: %v", path, e)
		}
	}
	if err := Uninstall(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unknown); !os.IsNotExist(err) {
		t.Fatal("owned binary remains")
	}
	if data, _ := os.ReadFile(runFile); string(data) != "evidence" {
		t.Fatal("run evidence changed")
	}
}

func TestInstallCommandReportsRetainedLegacyFile(t *testing.T) {
	repo := testRepo(t)
	bin := filepath.Join(repo, ".refactor", "bin")
	legacy := filepath.Join(repo, ".refactor", "lib", "src", "provider.mjs")
	for _, dir := range []string{bin, filepath.Dir(legacy)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	shim := "#!/bin/sh\n# refactor-me dev — installed 2026-09-28 from /old/tool\nexec node \"$(dirname \"$0\")/../lib/bin/refactor-me.mjs\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "refactor-me"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("user modified provider"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"install", repo}, repo, &stdout, &stderr, Callbacks{}); code != ExitOK {
		t.Fatalf("install exited %d: %s", code, stderr.String())
	}
	canonicalRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	canonicalLegacy := filepath.Join(canonicalRepo, ".refactor", "lib", "src", "provider.mjs")
	if !strings.Contains(stderr.String(), "retained unverified legacy file: "+canonicalLegacy) {
		t.Fatalf("missing retained file warning: %s", stderr.String())
	}
	if data, err := os.ReadFile(legacy); err != nil || string(data) != "user modified provider" {
		t.Fatal("legacy file changed")
	}
}

func TestRichReportRendersSavedEvidenceInBothLanguages(t *testing.T) {
	report := map[string]any{
		"runId": "rich", "status": "DONE_PARTIAL", "reason": "stopped on cycle budget (3)", "durationMinutes": 9, "providerMinutes": 4,
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

func TestInstallRefusesSymlinkedRuntimeDirectory(t *testing.T) {
	repo := testRepo(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repo, ".refactor")); err != nil {
		t.Fatal(err)
	}
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, source); err == nil || !strings.Contains(err.Error(), "not a regular directory") {
		t.Fatalf("unexpected error: %v", err)
	}
	if names, err := os.ReadDir(outside); err != nil || len(names) != 0 {
		t.Fatal("installer wrote through symlink")
	}
}

func TestInstallDoesNotDeleteThroughLegacyLibrarySymlink(t *testing.T) {
	repo := testRepo(t)
	outside := t.TempDir()
	bin := filepath.Join(repo, ".refactor", "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	shim := "#!/bin/sh\n# refactor-me dev — installed 2026-09-28 from /old/tool\nexec node \"$(dirname \"$0\")/../lib/bin/refactor-me.mjs\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "refactor-me"), []byte(shim), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".refactor", "lib")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "provider.mjs"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, source); err == nil {
		t.Fatal("accepted symlinked Node runtime")
	}
	if data, _ := os.ReadFile(filepath.Join(outside, "provider.mjs")); string(data) != "keep" {
		t.Fatal("wrote through symlink")
	}
}
