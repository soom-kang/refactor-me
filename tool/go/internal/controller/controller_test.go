package controller

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestRunNoCandidatesPreservesSource(t *testing.T) {
	globalSkillsFixture(t)
	repo := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "init")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, skill := range requiredSkills {
		path := filepath.Join(os.Getenv("HOME"), ".agents", "skills", skill, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: "+skill+"\ndescription: test skill\n---\nTest instructions.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "fixture")
	head := gitTest(t, repo, "rev-parse", "HEAD")
	index := gitTest(t, repo, "ls-files", "-s")
	bin := filepath.Join(t.TempDir(), "fake-codex")
	script := "#!/bin/sh\nout=''\nwhile [ $# -gt 0 ]; do\n  if [ \"$1\" = '--output-last-message' ]; then shift; out=$1; fi\n  shift\ndone\n[ -n \"$out\" ] || exit 22\nprintf '%s\\n' '{\"schema_version\":\"1\",\"scan_scope\":\"entire repository\",\"rejected_count\":0,\"notes\":null,\"candidates\":[]}' > \"$out\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	refactor := filepath.Join(repo, ".refactor")
	if err := os.MkdirAll(refactor, 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{"schema_version":2,"agents":{"primary":"codex","fallback":"none","codex":{"bin":"` + bin + `"}},"policy":{"empty_audits_to_stop":1},"workspace":{"worktree_parent":"` + t.TempDir() + `","keep_worktree":false}}`
	if err := os.WriteFile(filepath.Join(refactor, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	commands := `{"locked":true,"commands":[{"id":".:T1:ok","area":".","tier":"T1","family":"custom","name":"ok","argv":["/usr/bin/true"],"cwd":".","timeoutMs":10000,"source":"operator-defined"}]}`
	if err := os.WriteFile(filepath.Join(refactor, "commands.json"), []byte(commands), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := surface.Execute([]string{"run", "--provider", "codex", "--fallback", "none", "--no-live-probe", "--json"}, repo, &stdout, &stderr, surface.Callbacks{Run: Run, Doctor: Doctor, Clean: Clean})
	if code != surface.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, &stderr, &stdout)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"status": "NO_CHANGES"`)) {
		t.Fatalf("unexpected report: %s", &stdout)
	}
	if got := gitTest(t, repo, "rev-parse", "HEAD"); got != head {
		t.Fatalf("source HEAD changed: %s", got)
	}
	if got := gitTest(t, repo, "ls-files", "-s"); got != index {
		t.Fatal("source index changed")
	}
	if got := gitTest(t, repo, "status", "--porcelain"); got != "" {
		t.Fatalf("source worktree changed: %s", got)
	}
	stdout.Reset()
	stderr.Reset()
	if code := surface.Execute([]string{"report", "--json"}, repo, &stdout, &stderr, surface.Callbacks{}); code != surface.ExitOK {
		t.Fatalf("report exit=%d: %s", code, &stderr)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"status": "NO_CHANGES"`)) {
		t.Fatalf("report unreadable: %s", &stdout)
	}
}

func TestComparisonOfPublishedCommit(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	file := filepath.Join(repo, "source.txt")
	if err := os.WriteFile(file, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", "source.txt")
	gitTest(t, repo, "commit", "-m", "base")
	base := gitTest(t, repo, "rev-parse", "HEAD")
	if err := os.WriteFile(file, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", "source.txt")
	gitTest(t, repo, "commit", "-m", "result")
	result := gitTest(t, repo, "rev-parse", "HEAD")
	r := &runner{runDir: t.TempDir(), state: &runState{RepoRoot: repo, BaseOID: base, PublishedOID: result}}
	comparison := r.collectComparison()
	if comparison["status"] != "AVAILABLE" {
		t.Fatalf("comparison: %#v", comparison)
	}
	if !strings.Contains(comparison["preview"].(string), "+after") {
		t.Fatalf("missing diff preview: %#v", comparison)
	}
	if _, err := os.Stat(filepath.Join(r.runDir, "changes.patch")); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "mv", "source.txt", "renamed.txt")
	if err := os.WriteFile(filepath.Join(repo, "binary.dat"), []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", "binary.dat")
	gitTest(t, repo, "commit", "-m", "rename and binary")
	r.state.BaseOID = result
	r.state.PublishedOID = gitTest(t, repo, "rev-parse", "HEAD")
	comparison = r.collectComparison()
	if comparison["status"] != "AVAILABLE" {
		t.Fatalf("renamed comparison: %#v", comparison)
	}
	totals := comparison["totals"].(map[string]int)
	if totals["files"] != 2 || totals["binary"] != 1 {
		t.Fatalf("comparison totals: %#v", totals)
	}
}

func TestRunPublishesReviewedDeletion(t *testing.T) {
	globalSkillsFixture(t)
	repo := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "init")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	for name, body := range map[string]string{"src/legacy-parser.mjs": "export function parseLegacy() { return 1; }\n", "src/index.mjs": "export const value = 1;\n"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, skill := range requiredSkills {
		path := filepath.Join(os.Getenv("HOME"), ".agents", "skills", skill, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: "+skill+"\ndescription: test skill\n---\nTest instructions.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "fixture")
	base := gitTest(t, repo, "rev-parse", "HEAD")
	workflow, err := os.ReadFile(filepath.Join("..", "..", "..", "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures := t.TempDir()
	for _, name := range []string{"audit-success", "deepcheck-success", "preflight-success", "execute-success", "review-success"} {
		pattern := regexp.MustCompile(`(?s)<!-- example: ` + name + ` schema: \w+ -->\n` + "```json" + `\n(.*?)\n` + "```")
		match := pattern.FindSubmatch(workflow)
		if len(match) != 2 {
			t.Fatalf("missing workflow fixture %s", name)
		}
		if err := os.WriteFile(filepath.Join(fixtures, name+".json"), match[1], 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(fixtures, "audit-empty.json"), []byte(`{"schema_version":"1","scan_scope":"entire repository","rejected_count":0,"notes":null,"candidates":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "fake-codex")
	script := "#!/bin/sh\nout=''\nwhile [ $# -gt 0 ]; do\n  if [ \"$1\" = '--output-last-message' ]; then shift; out=$1; fi\n  shift\ndone\n[ -n \"$out\" ] || exit 22\ncase \"$out\" in\n  */audit-last.json) case \"$out\" in */audits/02/*) cp '" + filepath.Join(fixtures, "audit-empty.json") + "' \"$out\" ;; *) cp '" + filepath.Join(fixtures, "audit-success.json") + "' \"$out\" ;; esac ;;\n  */deep_check-last.json) cp '" + filepath.Join(fixtures, "deepcheck-success.json") + "' \"$out\" ;;\n  */preflight-last.json) cp '" + filepath.Join(fixtures, "preflight-success.json") + "' \"$out\" ;;\n  */execute-last.json) cp '" + filepath.Join(fixtures, "execute-success.json") + "' \"$out\" ;;\n  */review-last.json) cp '" + filepath.Join(fixtures, "review-success.json") + "' \"$out\" ;;\n  *) exit 23 ;;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	refactor := filepath.Join(repo, ".refactor")
	if err := os.MkdirAll(refactor, 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{"schema_version":2,"agents":{"primary":"codex","fallback":"none","codex":{"bin":"` + bin + `"}},"policy":{"empty_audits_to_stop":1},"workspace":{"worktree_parent":"` + t.TempDir() + `","keep_worktree":true}}`
	if err := os.WriteFile(filepath.Join(refactor, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	commands := `{"locked":true,"commands":[{"id":".:T1:ok","area":".","tier":"T1","family":"custom","name":"ok","argv":["/usr/bin/true"],"cwd":".","timeoutMs":10000,"source":"operator-defined"}]}`
	if err := os.WriteFile(filepath.Join(refactor, "commands.json"), []byte(commands), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := surface.Execute([]string{"run", "--provider", "codex", "--fallback", "none", "--no-live-probe", "--json"}, repo, &stdout, &stderr, surface.Callbacks{Run: Run, Doctor: Doctor, Clean: Clean})
	if code != surface.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, &stderr, &stdout)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"status": "DONE"`)) {
		t.Fatalf("report: %s", &stdout)
	}
	if got := gitTest(t, repo, "rev-parse", "HEAD"); got != base {
		t.Fatalf("source HEAD moved: %s", got)
	}
	if got := gitTest(t, repo, "status", "--porcelain"); got != "" {
		t.Fatalf("source dirty: %s", got)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"patchFile": "changes.patch"`)) {
		t.Fatalf("missing published patch: %s", &stdout)
	}
}

func globalSkillsFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	for _, name := range requiredSkills {
		path := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: "+name+"\ndescription: test skill\n---\nTest instructions.\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}
